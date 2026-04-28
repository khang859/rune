package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/tools"
)

func TestMessages_AppendUser(t *testing.T) {
	m := NewMessages(80)
	m.AppendUser("hi there")
	if !strings.Contains(m.Render(DefaultStyles(), false, time.Time{}), "hi there") {
		t.Fatal("user text missing")
	}
}

func TestMessages_StreamingAssistantText(t *testing.T) {
	m := NewMessages(80)
	m.OnAssistantDelta("hel")
	m.OnAssistantDelta("lo")
	m.OnTurnDone("stop")
	if !strings.Contains(m.Render(DefaultStyles(), false, time.Time{}), "hello") {
		t.Fatal("streamed text not rendered")
	}
}

func TestMessages_ToolCallAndResult(t *testing.T) {
	m := NewMessages(80)
	call := ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"/x"}`)}
	m.OnToolStarted(call)
	m.OnToolFinished(agent.ToolFinished{Call: call, Result: tools.Result{Output: "file content"}})
	out := m.Render(DefaultStyles(), false, time.Time{})
	if !strings.Contains(out, "read") {
		t.Fatalf("tool name missing:\n%s", out)
	}
	if !strings.Contains(out, "file content") {
		t.Fatalf("tool output missing:\n%s", out)
	}
}

func TestMessages_AssistantDeltaSurvivesIntervening(t *testing.T) {
	m := NewMessages(80)
	m.OnAssistantDelta("hel")
	// Append enough thinking to force a slice realloc on most growth schedules.
	for range 8 {
		m.OnThinkingDelta("x")
	}
	m.OnAssistantDelta("lo")
	out := m.Render(DefaultStyles(), false, time.Time{})
	if !strings.Contains(out, "hello") {
		t.Fatalf("assistant deltas split by thinking: want 'hello' in output, got:\n%s", out)
	}
}

func TestMessages_TurnError(t *testing.T) {
	m := NewMessages(80)
	m.OnTurnError(errString("bad thing"))
	if !strings.Contains(m.Render(DefaultStyles(), false, time.Time{}), "bad thing") {
		t.Fatal("error not rendered")
	}
}

func TestMessages_AppendSummaryRendersHeader(t *testing.T) {
	m := NewMessages(80)
	m.AppendSummary("the gist", 5)
	out := m.Render(DefaultStyles(), false, time.Time{})
	if !strings.Contains(out, "compacted summary") {
		t.Fatalf("missing header: %q", out)
	}
	if !strings.Contains(out, "5 messages") {
		t.Fatalf("missing count: %q", out)
	}
	if !strings.Contains(out, "the gist") {
		t.Fatalf("missing body: %q", out)
	}
}

func TestMessages_AppendSummaryEndsAssistantStream(t *testing.T) {
	m := NewMessages(80)
	m.OnAssistantDelta("partial")
	m.AppendSummary("S", 1)
	m.OnAssistantDelta("after")
	out := m.Render(DefaultStyles(), false, time.Time{})
	// "after" should be its own assistant block, not concatenated onto "partial".
	if strings.Contains(out, "partialafter") {
		t.Fatalf("AppendSummary did not break the assistant stream: %q", out)
	}
}

func TestMessages_OnInfoDoesNotEndAssistantStream(t *testing.T) {
	m := NewMessages(80)
	m.OnAssistantDelta("hel")
	m.OnInfo("queued (1 in queue)")
	m.OnAssistantDelta("lo")
	rendered := m.Render(DefaultStyles(), false, time.Time{})
	if !strings.Contains(rendered, "hello") {
		t.Fatalf("assistant deltas were fragmented by OnInfo: %q", rendered)
	}
}

func TestMessages_RenderWrapsToWidth(t *testing.T) {
	m := NewMessages(20)
	m.OnAssistantDelta(strings.Repeat("word ", 30))
	m.OnTurnDone("stop")
	out := m.Render(DefaultStyles(), false, time.Time{})
	for _, line := range strings.Split(out, "\n") {
		if w := visibleWidth(line); w > 20 {
			t.Fatalf("line exceeds width 20 (got %d): %q", w, line)
		}
	}
}

func TestMessages_RenderHardBreaksLongTokens(t *testing.T) {
	m := NewMessages(10)
	m.OnToolFinished(agent.ToolFinished{
		Call:   ai.ToolCall{Name: "read"},
		Result: tools.Result{Output: strings.Repeat("a", 50)},
	})
	out := m.Render(DefaultStyles(), false, time.Time{})
	for _, line := range strings.Split(out, "\n") {
		if w := visibleWidth(line); w > 10 {
			t.Fatalf("line exceeds width 10 (got %d): %q", w, line)
		}
	}
}

// visibleWidth strips ANSI escapes and returns rune count.
func visibleWidth(s string) int {
	var n, i int
	for i < len(s) {
		if s[i] == 0x1b {
			// skip CSI / OSC sequence
			for i < len(s) && s[i] != 'm' && s[i] != 0x07 {
				i++
			}
			i++
			continue
		}
		n++
		i++
	}
	return n
}

type errString string

func (e errString) Error() string { return string(e) }

func TestMessages_ThinkingHeaderStreamingCollapsed(t *testing.T) {
	m := NewMessages(80)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.OnThinkingDeltaAt("hmm", start)
	now := start.Add(3 * time.Second)
	out := m.Render(DefaultStyles(), false, now)
	if !strings.Contains(out, "▸ thinking… (3s)") {
		t.Fatalf("expected streaming-collapsed header, got:\n%s", out)
	}
	if strings.Contains(out, "hmm") {
		t.Fatalf("body should be hidden when collapsed, got:\n%s", out)
	}
}

func TestMessages_ThinkingHeaderFinalizedCollapsed(t *testing.T) {
	m := NewMessages(80)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.OnThinkingDeltaAt("hmm", start)
	end := start.Add(5 * time.Second)
	m.FinalizeStreamingThinking(end)
	out := m.Render(DefaultStyles(), false, end.Add(time.Hour))
	if !strings.Contains(out, "▸ thought for 5s") {
		t.Fatalf("expected finalized-collapsed header, got:\n%s", out)
	}
}

func TestMessages_ThinkingExpandedShowsBody(t *testing.T) {
	m := NewMessages(80)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.OnThinkingDeltaAt("hmm", start)
	out := m.Render(DefaultStyles(), true, start.Add(2*time.Second))
	if !strings.Contains(out, "▾ thinking… (2s)") {
		t.Fatalf("expected expanded header with ▾, got:\n%s", out)
	}
	if !strings.Contains(out, "hmm") {
		t.Fatalf("body should be visible when expanded, got:\n%s", out)
	}
}
