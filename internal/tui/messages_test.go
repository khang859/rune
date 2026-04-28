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

type errString string

func (e errString) Error() string { return string(e) }
