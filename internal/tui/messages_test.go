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
	for _, want := range []string{"familiar", "open a summoning circle for repo-plan", "after 1 omen", "awaiting return", "30s ward"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in familiar call, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "secret prompt text") || strings.Contains(out, "prompt") || strings.Contains(out, "{") {
		t.Fatalf("raw spawn_subagent JSON leaked into output:\n%s", out)
	}
}

func TestMessages_GroupedSpawnCallLinesShowSubagentProgress(t *testing.T) {
	m := NewMessages(120)
	call1 := ai.ToolCall{ID: "t1", Name: "spawn_subagent", Args: []byte(`{"name":"task-a","prompt":"p","agent_type":"code-explorer"}`)}
	call2 := ai.ToolCall{ID: "t2", Name: "spawn_subagent", Args: []byte(`{"name":"task-b","prompt":"p","agent_type":"code-explorer"}`)}
	m.OnToolStarted(call1)
	m.OnToolFinished(agent.ToolFinished{Call: call1, Result: tools.Result{Output: "spawned"}})
	m.OnToolStarted(call2)
	m.OnToolFinished(agent.ToolFinished{Call: call2, Result: tools.Result{Output: "spawned"}})

	now := time.Now()
	started := now.Add(-75 * time.Second)
	m.OnSubagentEvent(agent.SubagentEvent{Status: agent.SubagentRunning, Task: tools.SubagentTask{
		ID: "subagent_1", Name: "task-a", AgentType: "code-explorer", Status: string(agent.SubagentRunning),
		CreatedAt: now, StartedAt: &started, InputTokens: 1_000, OutputTokens: 234,
	}})

	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, now)
	if !strings.Contains(out, "summon familiar (2)") {
		t.Fatalf("expected grouped summon header, got:\n%s", out)
	}
	if !strings.Contains(out, "task-a [code-explorer] · 1m15s · 1.2k tok") {
		t.Fatalf("expected live progress suffix on task-a line, got:\n%s", out)
	}
	if strings.Contains(out, "task-b [code-explorer] ·") {
		t.Fatalf("task-b has no subagent events; expected no suffix, got:\n%s", out)
	}
}

func TestMessages_SingleSpawnCallShowsSubagentProgress(t *testing.T) {
	m := NewMessages(120)
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "spawn_subagent", Args: []byte(`{"name":"repo-plan","prompt":"p"}`)})

	now := time.Now()
	started := now.Add(-30 * time.Second)
	m.OnSubagentEvent(agent.SubagentEvent{Status: agent.SubagentRunning, Task: tools.SubagentTask{
		ID: "subagent_1", Name: "repo-plan", Status: string(agent.SubagentRunning),
		CreatedAt: now, StartedAt: &started, InputTokens: 500, OutputTokens: 100,
	}})

	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, now)
	if !strings.Contains(out, "open a summoning circle for repo-plan · 30s · 600 tok") {
		t.Fatalf("expected progress suffix on summon line, got:\n%s", out)
	}
}

func TestMessages_GroupsConsecutiveReadCallsAcrossResults(t *testing.T) {
	m := NewMessages(120)
	call1 := ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"something.png"}`)}
	call2 := ai.ToolCall{ID: "t2", Name: "read", Args: []byte(`{"path":"file.go","offset":10,"limit":5}`)}
	m.OnToolStarted(call1)
	m.OnToolFinished(agent.ToolFinished{Call: call1, Result: tools.Result{Output: "image-ish"}})
	m.OnToolStarted(call2)
	m.OnToolFinished(agent.ToolFinished{Call: call2, Result: tools.Result{Output: "package main"}})
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, time.Time{})
	for _, want := range []string{"read (2)", "something.png", "file.go (lines 10-14)"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in grouped read output, got:\n%s", want, out)
		}
	}
	if strings.Count(out, "read (2)") != 1 || strings.Contains(out, "image-ish") || strings.Contains(out, "package main") {
		t.Fatalf("expected one collapsed grouped read without result bodies, got:\n%s", out)
	}
}

func TestMessages_GroupedReadExpandedShowsResults(t *testing.T) {
	m := NewMessages(120)
	call1 := ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"a.go"}`)}
	call2 := ai.ToolCall{ID: "t2", Name: "read", Args: []byte(`{"path":"b.go"}`)}
	m.OnToolStarted(call1)
	m.OnToolFinished(agent.ToolFinished{Call: call1, Result: tools.Result{Output: "a body"}})
	m.OnToolStarted(call2)
	m.OnToolFinished(agent.ToolFinished{Call: call2, Result: tools.Result{Output: "b body"}})
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, true, time.Time{})
	for _, want := range []string{"read (2)", "a.go", "a body", "b.go", "b body"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in expanded grouped read output, got:\n%s", want, out)
		}
	}
}

func TestMessages_DifferentToolsDoNotGroup(t *testing.T) {
	m := NewMessages(120)
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"a.go"}`)})
	m.OnToolStarted(ai.ToolCall{ID: "t2", Name: "write", Args: []byte(`{"path":"b.go","content":"x"}`)})
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, time.Time{})
	if strings.Contains(out, "read (2)") || strings.Contains(out, "write (2)") {
		t.Fatalf("different tools should not group, got:\n%s", out)
	}
	for _, want := range []string{"read a.go", "write b.go"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q, got:\n%s", want, out)
		}
	}
}

func TestMessages_AssistantTextBreaksToolGrouping(t *testing.T) {
	m := NewMessages(120)
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"a.go"}`)})
	m.OnAssistantDelta("between")
	m.OnToolStarted(ai.ToolCall{ID: "t2", Name: "read", Args: []byte(`{"path":"b.go"}`)})
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, time.Time{})
	if strings.Contains(out, "read (2)") {
		t.Fatalf("assistant text should break grouping, got:\n%s", out)
	}
	for _, want := range []string{"read a.go", "between", "read b.go"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q, got:\n%s", want, out)
		}
	}
}

func TestMessages_GroupsOtherCommonToolsWithSummaries(t *testing.T) {
	m := NewMessages(160)
	m.OnToolStarted(ai.ToolCall{ID: "t1", Name: "bash", Args: []byte(`{"command":"go test ./internal/tui"}`)})
	m.OnToolFinished(agent.ToolFinished{Call: ai.ToolCall{Name: "bash"}, Result: tools.Result{Output: "ok"}})
	m.OnToolStarted(ai.ToolCall{ID: "t2", Name: "bash", Args: []byte(`{"command":"go test ./..."}`)})
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, time.Time{})
	for _, want := range []string{"bash (2)", "go test ./internal/tui", "go test ./..."} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in grouped bash output, got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "command") || strings.Contains(out, "{") {
		t.Fatalf("raw bash JSON leaked into grouped output:\n%s", out)
	}
}

func TestMessages_GroupsConsecutiveEditCallsWithDiffStats(t *testing.T) {
	m := NewMessages(160)
	call1 := ai.ToolCall{ID: "t1", Name: "edit", Args: []byte(`{"path":"foo.go","old_string":"a\nb","new_string":"x\ny\nz"}`)}
	call2 := ai.ToolCall{ID: "t2", Name: "edit", Args: []byte(`{"path":"foo.go","old_string":"old1\nold2\nold3\nold4","new_string":""}`)}
	m.OnToolStarted(call1)
	m.OnToolFinished(agent.ToolFinished{Call: call1, Result: tools.Result{Output: "edited foo.go"}})
	m.OnToolStarted(call2)
	m.OnToolFinished(agent.ToolFinished{Call: call2, Result: tools.Result{Output: "edited foo.go"}})
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, time.Time{})
	for _, want := range []string{"edit (2)", "foo.go", "+3", "-2", "+0", "-4"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in grouped edit output, got:\n%s", want, out)
		}
	}
	// Raw JSON keys must not leak.
	if strings.Contains(out, "old_string") || strings.Contains(out, "new_string") || strings.Contains(out, `\n`) {
		t.Fatalf("raw JSON leaked into grouped edit output:\n%s", out)
	}
}

func TestMessages_GroupedEditPureInsertionAndDeletion(t *testing.T) {
	m := NewMessages(160)
	ins := ai.ToolCall{ID: "t1", Name: "edit", Args: []byte(`{"path":"foo.go","old_string":"","new_string":"line1\nline2"}`)}
	del := ai.ToolCall{ID: "t2", Name: "edit", Args: []byte(`{"path":"foo.go","old_string":"line1","new_string":""}`)}
	m.OnToolStarted(ins)
	m.OnToolStarted(del)
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, time.Time{})
	for _, want := range []string{"edit (2)", "+2", "-0", "+0", "-1"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected %q in grouped edit output, got:\n%s", want, out)
		}
	}
}

func TestMessages_GroupedEditMalformedArgsFallsBackToPath(t *testing.T) {
	m := NewMessages(160)
	good := ai.ToolCall{ID: "t1", Name: "edit", Args: []byte(`{"path":"foo.go","old_string":"a","new_string":"b"}`)}
	bad := ai.ToolCall{ID: "t2", Name: "edit", Args: []byte(`not json`)}
	m.OnToolStarted(good)
	m.OnToolStarted(bad)
	out := m.Render(DefaultStylesWithIconMode("unicode"), false, false, time.Time{})
	// Good row gets stats; bad row falls back to the generic compact-JSON summary.
	if !strings.Contains(out, "edit (2)") || !strings.Contains(out, "+1") || !strings.Contains(out, "-1") {
		t.Fatalf("expected grouped header + stats for the good row, got:\n%s", out)
	}
	if !strings.Contains(out, "not json") {
		t.Fatalf("expected malformed row to fall back to raw args, got:\n%s", out)
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

func TestMessages_OnSubagentEvent_CoalescesByTaskID(t *testing.T) {
	m := NewMessages(80)

	mkEvent := func(status agent.SubagentStatus) agent.SubagentEvent {
		return agent.SubagentEvent{
			Task:   tools.SubagentTask{ID: "task-1", Name: "inspect", FamiliarName: "Nyx"},
			Status: status,
		}
	}

	m.OnSubagentEvent(mkEvent(agent.SubagentPending))
	m.OnSubagentEvent(mkEvent(agent.SubagentRunning))
	m.OnSubagentEvent(mkEvent(agent.SubagentCompleted))

	out := m.Render(DefaultStyles(), false, false, time.Time{})
	if strings.Count(out, "Nyx") != 1 {
		t.Fatalf("expected one block for task-1, render was:\n%s", out)
	}
	if !strings.Contains(out, "returned") {
		t.Fatalf("expected final completed text, got:\n%s", out)
	}
}

func TestMessages_OnSubagentEvent_SeparateBlocksForDifferentTasks(t *testing.T) {
	m := NewMessages(80)
	m.OnSubagentEvent(agent.SubagentEvent{
		Task:   tools.SubagentTask{ID: "task-1", FamiliarName: "Nyx"},
		Status: agent.SubagentRunning,
	})
	m.OnSubagentEvent(agent.SubagentEvent{
		Task:   tools.SubagentTask{ID: "task-2", FamiliarName: "Puck"},
		Status: agent.SubagentRunning,
	})
	out := m.Render(DefaultStyles(), false, false, time.Time{})
	if !strings.Contains(out, "Nyx") || !strings.Contains(out, "Puck") {
		t.Fatalf("expected both familiars rendered, got:\n%s", out)
	}
}

func TestRenderSubagentEventText_TrimmedLabels(t *testing.T) {
	started := time.Date(2026, 6, 9, 7, 0, 0, 0, time.UTC)
	completed := started.Add(2*time.Minute + 4*time.Second)
	mk := func(status agent.SubagentStatus, summary, errMsg string) agent.SubagentEvent {
		task := tools.SubagentTask{
			ID: "t1", Name: "inspect", FamiliarName: "Nyx", Summary: summary, Error: errMsg,
		}
		if status == agent.SubagentRunning {
			task.StartedAt = &started
			task.InputTokens = 1_200
			task.OutputTokens = 34
		}
		if status == agent.SubagentCompleted {
			task.StartedAt = &started
			task.CompletedAt = &completed
			task.InputTokens = 12_000
			task.OutputTokens = 345
		}
		return agent.SubagentEvent{Task: task, Status: status}
	}
	// Variant with no task name so familiarLabel returns just "Nyx" — lets us
	// pin the literal "Nyx waiting on dependencies" substring for the blocked case.
	mkBare := func(status agent.SubagentStatus) agent.SubagentEvent {
		return agent.SubagentEvent{
			Task:   tools.SubagentTask{ID: "t1", FamiliarName: "Nyx"},
			Status: status,
		}
	}

	cases := []struct {
		name   string
		ev     agent.SubagentEvent
		expect string
	}{
		{"pending", mk(agent.SubagentPending, "", ""), "summoning Nyx"},
		{"running", mk(agent.SubagentRunning, "", ""), "working… · 2m04s · 1.2k tok"},
		{"blocked", mkBare(agent.SubagentBlocked), "Nyx waiting on dependencies"},
		{"completed", mk(agent.SubagentCompleted, "line a\nline b", ""), "returned (2 lines) · 2m04s · 12k tok"},
		{"completed-empty", mk(agent.SubagentCompleted, "", ""), "returned"},
		{"failed", mk(agent.SubagentFailed, "", "boom"), "failed: boom"},
		{"cancelled", mk(agent.SubagentCancelled, "", ""), "dismissed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderSubagentEventTextAt(c.ev, completed)
			if !strings.Contains(got, c.expect) {
				t.Fatalf("expected %q to contain %q", got, c.expect)
			}
			if strings.Contains(got, "scrying") || strings.Contains(got, "summoning circle") || strings.Contains(got, "lost the thread") || strings.Contains(got, "veil") {
				t.Fatalf("flavor text not trimmed in %q", got)
			}
		})
	}
}
