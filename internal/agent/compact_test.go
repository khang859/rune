// internal/agent/compact_test.go
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

func TestCompact_UsesProviderForSummary(t *testing.T) {
	s := session.New("gpt-5")
	s.Append(userMsg("u1"))
	s.Append(asstMsg("a1"))
	s.Append(userMsg("u2"))

	f := faux.New().Reply("here is a summary").Done()
	a := New(f, tools.NewRegistry(), s, "")
	if err := a.Compact(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	path := s.PathToActive()
	if len(path) < 2 {
		t.Fatalf("path len = %d", len(path))
	}
	if !strings.Contains(path[0].Content[0].(ai.TextBlock).Text, "summary") {
		t.Fatalf("first msg not summary: %#v", path[0])
	}
}

func TestCompact_OverflowDuringSummaryFailsWithoutMutatingHistory(t *testing.T) {
	s := session.New("gpt-5")
	s.Append(userMsg("u1"))
	s.Append(asstMsg("a1"))
	s.Append(userMsg("u2"))
	before := s.PathToActive()

	f := faux.New().Reply("partial summary").DoneOverflow()
	a := New(f, tools.NewRegistry(), s, "")
	err := a.Compact(context.Background(), "")
	if err == nil {
		t.Fatal("expected compact to fail when summarizer overflows")
	}
	if !strings.Contains(err.Error(), "context overflow") {
		t.Fatalf("error = %q, want it to mention context overflow", err)
	}

	after := s.PathToActive()
	if len(after) != len(before) {
		t.Fatalf("history mutated on overflow: len before=%d after=%d", len(before), len(after))
	}
	for i := range before {
		if before[i].Content[0].(ai.TextBlock).Text != after[i].Content[0].(ai.TextBlock).Text {
			t.Fatalf("message %d changed: %#v -> %#v", i, before[i], after[i])
		}
	}
}

func TestShrinkHistoryForCompact_TruncatesOversizedToolResults(t *testing.T) {
	big := strings.Repeat("X", 50_000)
	history := []ai.Message{
		userMsg("hi"),
		{
			Role: ai.RoleAssistant,
			Content: []ai.ContentBlock{
				ai.TextBlock{Text: "running"},
				ai.ToolUseBlock{ID: "t1", Name: "bash", Args: nil},
			},
		},
		{
			Role:       ai.RoleToolResult,
			ToolCallID: "t1",
			Content: []ai.ContentBlock{
				ai.ToolResultBlock{ToolCallID: "t1", Output: big},
			},
		},
		userMsg("ok continue"),
	}
	out := shrinkHistoryForCompact(history, 1_000)
	tr := out[2].Content[0].(ai.ToolResultBlock)
	if len(tr.Output) > 1_000 {
		t.Fatalf("expected output within 1KB budget, got %d bytes", len(tr.Output))
	}
	if !strings.Contains(tr.Output, "[... truncated") {
		t.Fatalf("expected truncation marker, got: %q", tr.Output)
	}
	if !strings.HasPrefix(tr.Output, "X") || !strings.HasSuffix(tr.Output, "X") {
		t.Fatal("head/tail should be preserved as Xs")
	}
	// Original history must not be mutated.
	origTR := history[2].Content[0].(ai.ToolResultBlock)
	if len(origTR.Output) != 50_000 {
		t.Fatalf("original history mutated, len=%d", len(origTR.Output))
	}
	// Other messages pass through unchanged.
	if out[0].Content[0].(ai.TextBlock).Text != "hi" {
		t.Fatal("user message text changed")
	}
	if len(out[1].Content) != 2 {
		t.Fatal("assistant turn content reshaped")
	}
}

func TestShrinkHistoryForCompact_LeavesSmallOutputsAlone(t *testing.T) {
	history := []ai.Message{
		{
			Role:       ai.RoleToolResult,
			ToolCallID: "t1",
			Content: []ai.ContentBlock{
				ai.ToolResultBlock{ToolCallID: "t1", Output: "small"},
			},
		},
	}
	out := shrinkHistoryForCompact(history, 1_000)
	tr := out[0].Content[0].(ai.ToolResultBlock)
	if tr.Output != "small" {
		t.Fatalf("small output altered: %q", tr.Output)
	}
}

// TestTruncateMiddle_NeverExceedsBudget is the regression test for issue #31:
// the returned string, marker included, must never exceed maxBytes — even for
// budgets too small to fit the marker (which hard-caps instead).
func TestTruncateMiddle_NeverExceedsBudget(t *testing.T) {
	s := strings.Repeat("X", 50_000)
	for _, maxBytes := range []int{1, 5, 50, 99, 100, 1_000, 8_000, 49_999} {
		got := truncateMiddle(s, maxBytes)
		if len(got) > maxBytes {
			t.Fatalf("maxBytes=%d: output len=%d exceeds budget", maxBytes, len(got))
		}
	}
}

// TestTruncateMiddle_PreservesHeadTailWithinBudget verifies that for a normal
// budget the marker is present and head/tail content is preserved.
func TestTruncateMiddle_PreservesHeadTailWithinBudget(t *testing.T) {
	s := strings.Repeat("A", 25_000) + strings.Repeat("B", 25_000)
	got := truncateMiddle(s, 1_000)
	if len(got) > 1_000 {
		t.Fatalf("output len=%d exceeds budget 1000", len(got))
	}
	if !strings.Contains(got, "[... truncated") {
		t.Fatalf("missing truncation marker: %q", got)
	}
	if !strings.HasPrefix(got, "A") {
		t.Fatalf("head content not preserved: %q", got[:20])
	}
	if !strings.HasSuffix(got, "B") {
		t.Fatalf("tail content not preserved: %q", got[len(got)-20:])
	}
}

// reuse helpers from loop_test.go (same package)
func asstMsg(text string) ai.Message {
	return ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}}
}
