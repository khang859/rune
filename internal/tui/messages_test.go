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
	out := m.Render(DefaultStylesWithIconMode("nerd"), false, false, time.Time{})
	if !strings.Contains(out, "you>") {
		t.Fatalf("user label missing:\n%s", out)
	}
	if !strings.Contains(out, "hi there") {
		t.Fatalf("user text missing:\n%s", out)
	}
}

func TestMessages_AssistantUsesRuneLabel(t *testing.T) {
	m := NewMessages(80)
	m.OnAssistantDelta("hello")
	out := m.Render(DefaultStylesWithIconMode("nerd"), false, false, time.Time{})
	if !strings.Contains(out, "󰬯 rune") || !strings.Contains(out, "hello") {
		t.Fatalf("assistant label missing:\n%s", out)
	}
}

func TestMessages_StreamingAssistantText(t *testing.T) {
	m := NewMessages(80)
	m.OnAssistantDelta("hel")
	m.OnAssistantDelta("lo")
	m.OnTurnDone("stop")
	if !strings.Contains(m.Render(DefaultStyles(), false, false, time.Time{}), "hello") {
		t.Fatal("streamed text not rendered")
	}
}

func TestMessages_ToolCallAndResult(t *testing.T) {
	m := NewMessages(80)
	call := ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"/x"}`)}
	m.OnToolStarted(call)
	m.OnToolFinished(agent.ToolFinished{Call: call, Result: tools.Result{Output: "file content"}})
	out := m.Render(DefaultStyles(), false, true, time.Time{})
	if !strings.Contains(out, "read") {
		t.Fatalf("tool name missing:\n%s", out)
	}
	if !strings.Contains(out, "file content") {
		t.Fatalf("tool output missing:\n%s", out)
	}
}

func TestMessages_ToolResultCollapsedByDefault(t *testing.T) {
	m := NewMessages(80)
	call := ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"/x"}`)}
	m.OnToolStarted(call)
	m.OnToolFinished(agent.ToolFinished{Call: call, Result: tools.Result{Output: "line1\nline2\nline3"}})
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, time.Time{})
	if !strings.Contains(out, "▸ ⚒ read (3 lines)") {
		t.Fatalf("expected collapsed header with line count, got:\n%s", out)
	}
	if strings.Contains(out, "line1") || strings.Contains(out, "line2") || strings.Contains(out, "line3") {
		t.Fatalf("body should be hidden when collapsed, got:\n%s", out)
	}
}

func TestMessages_ToolResultExpandedShowsBody(t *testing.T) {
	m := NewMessages(80)
	call := ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"/x"}`)}
	m.OnToolStarted(call)
	m.OnToolFinished(agent.ToolFinished{Call: call, Result: tools.Result{Output: "line1\nline2"}})
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, true, time.Time{})
	if !strings.Contains(out, "▾ ⚒ read") {
		t.Fatalf("expected expanded header with ▾, got:\n%s", out)
	}
	if !strings.Contains(out, "line1") || !strings.Contains(out, "line2") {
		t.Fatalf("body should be visible when expanded, got:\n%s", out)
	}
}

func TestMessages_EditToolRendersAsDiff(t *testing.T) {
	m := NewMessages(80)
	args := []byte(`{"path":"foo.go","old_string":"old1\nold2","new_string":"new1\nnew2"}`)
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "edit", Args: args})
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, time.Time{})
	if !strings.Contains(out, "edit foo.go") {
		t.Fatalf("expected diff header with path, got:\n%s", out)
	}
	for _, want := range []string{"- old1", "- old2", "+ new1", "+ new2"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in diff output, got:\n%s", want, out)
		}
	}
	// Raw JSON keys should not leak into the rendered diff.
	if strings.Contains(out, "old_string") || strings.Contains(out, "new_string") {
		t.Fatalf("raw JSON leaked into diff output:\n%s", out)
	}
}

func TestMessages_EditToolMalformedArgsFallsBack(t *testing.T) {
	m := NewMessages(80)
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "edit", Args: []byte(`not json`)})
	out := m.Render(DefaultStyles(), false, false, time.Time{})
	if !strings.Contains(out, "edit(not json)") {
		t.Fatalf("expected fallback to generic format, got:\n%s", out)
	}
}

func TestMessages_BashToolRendersCommand(t *testing.T) {
	m := NewMessages(80)
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "bash", Args: []byte(`{"command":"ls -la\necho hi"}`)})
	out := m.Render(DefaultStyles(), false, false, time.Time{})
	for _, want := range []string{"bash", "ls -la", "echo hi"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in bash output, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, `\n`) || strings.Contains(out, "command") {
		t.Fatalf("escaped JSON leaked into bash output:\n%s", out)
	}
}

func TestMessages_WriteToolRendersSummary(t *testing.T) {
	m := NewMessages(80)
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "write", Args: []byte(`{"path":"/tmp/foo.txt","content":"a\nb\nc"}`)})
	out := m.Render(DefaultStyles(), false, false, time.Time{})
	if !strings.Contains(out, "write /tmp/foo.txt (3 lines, 5 bytes)") {
		t.Fatalf("expected write summary, got:\n%s", out)
	}
}

func TestMessages_ReadToolRendersPath(t *testing.T) {
	m := NewMessages(80)
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"/x"}`)})
	out := m.Render(DefaultStyles(), false, false, time.Time{})
	if !strings.Contains(out, "read /x") {
		t.Fatalf("expected 'read /x', got:\n%s", out)
	}
	if strings.Contains(out, "{") {
		t.Fatalf("raw JSON leaked into read output:\n%s", out)
	}
}

func TestMessages_ReadToolWithOffsetAndLimit(t *testing.T) {
	m := NewMessages(80)
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"/x","offset":10,"limit":5}`)})
	out := m.Render(DefaultStyles(), false, false, time.Time{})
	if !strings.Contains(out, "read /x (lines 10-14)") {
		t.Fatalf("expected line range, got:\n%s", out)
	}
}

func TestMessages_ReadToolWithReadAll(t *testing.T) {
	m := NewMessages(80)
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"/x","read_all":true}`)})
	out := m.Render(DefaultStyles(), false, false, time.Time{})
	if !strings.Contains(out, "read /x (all)") {
		t.Fatalf("expected (all) suffix, got:\n%s", out)
	}
}

func TestMessages_SubagentToolCallRendersAsFamiliar(t *testing.T) {
	m := NewMessages(120)
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "spawn_subagent", Args: []byte(`{"name":"repo-plan","prompt":"secret prompt text","dependencies":["subagent_1"],"background":false,"timeout_secs":30}`)})
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, time.Time{})
	for _, want := range []string{"familiar", "summon a familiar for repo-plan", "after 1 omen", "awaiting return", "30s ward"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in familiar call, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret prompt text") || strings.Contains(out, "prompt") || strings.Contains(out, "{") {
		t.Fatalf("raw spawn_subagent JSON leaked into output:\n%s", out)
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
	out := m.Render(DefaultStyles(), false, false, time.Time{})
	if !strings.Contains(out, "hello") {
		t.Fatalf("assistant deltas split by thinking: want 'hello' in output, got:\n%s", out)
	}
}

func TestMessages_TurnError(t *testing.T) {
	m := NewMessages(80)
	m.OnTurnError(errString("bad thing"))
	if !strings.Contains(m.Render(DefaultStyles(), false, false, time.Time{}), "bad thing") {
		t.Fatal("error not rendered")
	}
}

func TestMessages_AppendSummaryRendersHeader(t *testing.T) {
	m := NewMessages(80)
	m.AppendSummary("the gist", 5)
	out := m.Render(DefaultStyles(), false, false, time.Time{})
	if !strings.Contains(out, "compacted memory") {
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
	out := m.Render(DefaultStyles(), false, false, time.Time{})
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
	rendered := m.Render(DefaultStyles(), false, false, time.Time{})
	if !strings.Contains(rendered, "hello") {
		t.Fatalf("assistant deltas were fragmented by OnInfo: %q", rendered)
	}
}

func TestMessages_RenderWrapsToWidth(t *testing.T) {
	m := NewMessages(20)
	m.OnAssistantDelta(strings.Repeat("word ", 30))
	m.OnTurnDone("stop")
	out := m.Render(DefaultStyles(), false, false, time.Time{})
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
	out := m.Render(DefaultStyles(), false, true, time.Time{})
	for _, line := range strings.Split(out, "\n") {
		if w := visibleWidth(line); w > 10 {
			t.Fatalf("line exceeds width 10 (got %d): %q", w, line)
		}
	}
}

// visibleWidth strips ANSI escapes and returns rune count.
func visibleWidth(s string) int {
	n := 0
	skip := false
	for _, r := range s {
		if skip {
			if r == 'm' || r == 0x07 {
				skip = false
			}
			continue
		}
		if r == 0x1b {
			skip = true
			continue
		}
		n++
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
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, now)
	if !strings.Contains(out, "▸ ✦ thinking… (3s)") {
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
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, end.Add(time.Hour))
	if !strings.Contains(out, "▸ ✦ thought for 5s") {
		t.Fatalf("expected finalized-collapsed header, got:\n%s", out)
	}
}

func TestMessages_ThinkingExpandedShowsBody(t *testing.T) {
	m := NewMessages(80)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.OnThinkingDeltaAt("hmm", start)
	out := m.Render(DefaultStylesWithIconMode("unicode"), true, false, start.Add(2*time.Second))
	if !strings.Contains(out, "▾ ✦ thinking… (2s)") {
		t.Fatalf("expected expanded header with ▾, got:\n%s", out)
	}
	if !strings.Contains(out, "hmm") {
		t.Fatalf("body should be visible when expanded, got:\n%s", out)
	}
}

func TestMessages_AssistantDeltaFinalizesPriorThinking(t *testing.T) {
	m := NewMessages(80)
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m.OnThinkingDeltaAt("planning", start)
	if !m.HasInProgressThinking() {
		t.Fatal("thinking should be in progress before assistant delta")
	}
	m.OnAssistantDelta("answer")
	if m.HasInProgressThinking() {
		t.Fatal("assistant delta should have finalized the thinking block")
	}
}

func TestMessages_ToolStartedFinalizesPriorThinking(t *testing.T) {
	m := NewMessages(80)
	m.OnThinkingDeltaAt("planning", time.Now())
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{}`)})
	if m.HasInProgressThinking() {
		t.Fatal("tool started should have finalized the thinking block")
	}
}

func TestMessages_TurnDoneFinalizesPriorThinking(t *testing.T) {
	m := NewMessages(80)
	m.OnThinkingDeltaAt("planning", time.Now())
	m.OnTurnDone("stop")
	if m.HasInProgressThinking() {
		t.Fatal("turn done should have finalized the thinking block")
	}
}

func TestMessages_TurnErrorFinalizesPriorThinking(t *testing.T) {
	m := NewMessages(80)
	m.OnThinkingDeltaAt("planning", time.Now())
	m.OnTurnError(errString("boom"))
	if m.HasInProgressThinking() {
		t.Fatal("turn error should have finalized the thinking block")
	}
}

func TestMessages_TurnDoneStopDoesNotProduceThinkingBlock(t *testing.T) {
	m := NewMessages(80)
	m.OnTurnDone("stop")
	out := m.Render(DefaultStyles(), false, false, time.Time{})
	// stop is the normal case — no info/thinking line should be rendered.
	if out != "" {
		t.Fatalf("turn done with stop should produce no rendered block, got:\n%s", out)
	}
}

func TestMessages_TurnDoneAbnormalIsInfo(t *testing.T) {
	m := NewMessages(80)
	m.OnTurnDone("max_tokens")
	out := m.Render(DefaultStyles(), false, false, time.Time{})
	if !strings.Contains(out, "(turn ended: max_tokens)") {
		t.Fatalf("expected info notice, got:\n%s", out)
	}
}
