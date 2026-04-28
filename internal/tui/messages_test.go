package tui

import (
	"strings"
	"testing"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/tools"
)

func TestMessages_AppendUser(t *testing.T) {
	m := NewMessages(80)
	m.AppendUser("hi there")
	if !strings.Contains(m.Render(DefaultStyles()), "hi there") {
		t.Fatal("user text missing")
	}
}

func TestMessages_StreamingAssistantText(t *testing.T) {
	m := NewMessages(80)
	m.OnAssistantDelta("hel")
	m.OnAssistantDelta("lo")
	m.OnTurnDone("stop")
	if !strings.Contains(m.Render(DefaultStyles()), "hello") {
		t.Fatal("streamed text not rendered")
	}
}

func TestMessages_ToolCallAndResult(t *testing.T) {
	m := NewMessages(80)
	call := ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"/x"}`)}
	m.OnToolStarted(call)
	m.OnToolFinished(agent.ToolFinished{Call: call, Result: tools.Result{Output: "file content"}})
	out := m.Render(DefaultStyles())
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
	out := m.Render(DefaultStyles())
	if !strings.Contains(out, "hello") {
		t.Fatalf("assistant deltas split by thinking: want 'hello' in output, got:\n%s", out)
	}
}

func TestMessages_TurnError(t *testing.T) {
	m := NewMessages(80)
	m.OnTurnError(errString("bad thing"))
	if !strings.Contains(m.Render(DefaultStyles()), "bad thing") {
		t.Fatal("error not rendered")
	}
}

func TestMessages_AppendSummaryRendersHeader(t *testing.T) {
	m := NewMessages(80)
	m.AppendSummary("the gist", 5)
	out := m.Render(DefaultStyles())
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
	out := m.Render(DefaultStyles())
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
	rendered := m.Render(DefaultStyles())
	if !strings.Contains(rendered, "hello") {
		t.Fatalf("assistant deltas were fragmented by OnInfo: %q", rendered)
	}
}

type errString string

func (e errString) Error() string { return string(e) }
