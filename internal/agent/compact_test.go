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
	if len(tr.Output) > 1_300 {
		t.Fatalf("expected output truncated near 1KB, got %d bytes", len(tr.Output))
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

// reuse helpers from loop_test.go (same package)
func asstMsg(text string) ai.Message {
	return ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}}
}
