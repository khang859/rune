# Thinking-Step Display Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Surface OpenAI reasoning summaries in the rune TUI as a collapsed-by-default `▸ thinking…` block toggled globally with `Ctrl+T`.

**Architecture:** Three change points along the existing pipeline. (1) `Agent` gains an in-memory effort field, threaded into `req.Reasoning.Effort` so the Codex Responses API actually emits reasoning summary events. (2) The Codex SSE handler parses `response.reasoning_summary_text.delta` and `response.reasoning_summary_part.added` and emits `ai.Thinking` events. (3) The TUI thinking block is rewritten to track start/end timestamps and render as a collapsible header with a global `showThinking` flag.

**Tech Stack:** Go 1.22+, Bubble Tea (`github.com/charmbracelet/bubbletea`), Lipgloss, OpenAI Responses API (SSE).

**Spec:** `docs/superpowers/specs/2026-04-28-thinking-display-design.md`

---

## File map

| File | Role |
|---|---|
| `internal/ai/codex/sse.go` | Add cases for two reasoning SSE event names. |
| `internal/ai/codex/testdata/stream_reasoning.sse` | New SSE fixture used by tests. |
| `internal/ai/codex/sse_test.go` | Add coverage for reasoning summary delta + part separator. |
| `internal/agent/agent.go` | Add `effort string`, default `"medium"`, `SetReasoningEffort`. |
| `internal/agent/loop.go` | Populate `req.Reasoning.Effort` from `a.effort`. |
| `internal/agent/loop_test.go` | New test using inline capturing provider asserts the request carries effort. |
| `internal/tui/messages.go` | Thinking block tracks timestamps; `Render` takes `showThinking`/`now`; finalize-on-other-event semantics; `(turn ended)` notice moves to `bkInfo`. |
| `internal/tui/messages_test.go` | Update existing call sites to new `Render` signature; add tests for collapsed/expanded/streaming/finalized headers, finalize-on-assistant-delta, `OnTurnDone` no longer producing thinking blocks. |
| `internal/tui/styles.go` | Add `ThinkingHeader` style. |
| `internal/tui/root.go` | `showThinking bool`; intercept `Ctrl+T`; tea.Tick driver while any in-progress thinking block exists; pass flags into `Render`; thread settings effort into agent on modal close. |
| `internal/tui/modal/hotkeys.go` | Add `Ctrl+T   toggle thinking` line. |

No new packages, no schema changes, no persistence changes.

---

## Task 1: Codex SSE — parse `response.reasoning_summary_text.delta`

**Files:**
- Create: `internal/ai/codex/testdata/stream_reasoning.sse`
- Modify: `internal/ai/codex/sse.go`
- Modify: `internal/ai/codex/sse_test.go`

- [ ] **Step 1: Create the SSE fixture for the failing test**

Create `internal/ai/codex/testdata/stream_reasoning.sse` with:

```
event: response.created
data: {"type":"response.created","response":{"id":"r1","status":"in_progress"}}

event: response.reasoning_summary_text.delta
data: {"type":"response.reasoning_summary_text.delta","delta":"Considering"}

event: response.reasoning_summary_text.delta
data: {"type":"response.reasoning_summary_text.delta","delta":" the problem"}

event: response.completed
data: {"type":"response.completed","response":{"id":"r1","status":"completed","usage":{"input_tokens":5,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}}
```

- [ ] **Step 2: Write the failing test**

Append to `internal/ai/codex/sse_test.go`:

```go
func TestParseSSE_ReasoningSummary(t *testing.T) {
	b, _ := os.ReadFile("testdata/stream_reasoning.sse")
	out := make(chan ai.Event, 32)
	if err := parseSSE(context.Background(), strings.NewReader(string(b)), out); err != nil {
		t.Fatal(err)
	}
	close(out)
	evs := collect(t, out)

	var thinking strings.Builder
	for _, e := range evs {
		if t, ok := e.(ai.Thinking); ok {
			thinking.WriteString(t.Text)
		}
	}
	if thinking.String() != "Considering the problem" {
		t.Fatalf("thinking text = %q", thinking.String())
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/ai/codex/ -run TestParseSSE_ReasoningSummary -v`
Expected: FAIL with `thinking text = ""` (no `ai.Thinking` events emitted yet).

- [ ] **Step 4: Implement the parser case**

In `internal/ai/codex/sse.go::dispatchEvent`, add a case under the existing `switch name` block (between `response.output_item.added` and `response.completed`):

```go
case "response.reasoning_summary_text.delta":
	var d textDelta
	if err := json.Unmarshal([]byte(data), &d); err != nil {
		return nil
	}
	return send(ctx, out, ai.Thinking{Text: d.Delta})
```

The existing `textDelta` struct has the field `Delta string \`json:"delta"\`` which matches the wire format — no new type needed.

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/ai/codex/ -run TestParseSSE_ReasoningSummary -v`
Expected: PASS.

- [ ] **Step 6: Run the full codex test suite to confirm no regressions**

Run: `go test ./internal/ai/codex/ -v`
Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ai/codex/sse.go internal/ai/codex/sse_test.go internal/ai/codex/testdata/stream_reasoning.sse
git commit -m "feat(codex): parse reasoning_summary_text.delta as ai.Thinking"
```

---

## Task 2: Codex SSE — separator on `response.reasoning_summary_part.added`

**Files:**
- Modify: `internal/ai/codex/testdata/stream_reasoning.sse`
- Modify: `internal/ai/codex/sse.go`
- Modify: `internal/ai/codex/sse_test.go`

Multi-part summaries arrive as separate `summary_text` parts. The Codex API signals a new part with `response.reasoning_summary_part.added` between the delta runs. We emit `ai.Thinking{Text: "\n\n"}` when a part starts AFTER the first delta has already arrived, so the in-memory thinking block reads as separate paragraphs.

- [ ] **Step 1: Extend the SSE fixture with a second summary part**

Replace the fixture body of `internal/ai/codex/testdata/stream_reasoning.sse` with:

```
event: response.created
data: {"type":"response.created","response":{"id":"r1","status":"in_progress"}}

event: response.reasoning_summary_part.added
data: {"type":"response.reasoning_summary_part.added","item_id":"r1","summary_index":0,"part":{"type":"summary_text","text":""}}

event: response.reasoning_summary_text.delta
data: {"type":"response.reasoning_summary_text.delta","delta":"Considering"}

event: response.reasoning_summary_text.delta
data: {"type":"response.reasoning_summary_text.delta","delta":" the problem"}

event: response.reasoning_summary_part.added
data: {"type":"response.reasoning_summary_part.added","item_id":"r1","summary_index":1,"part":{"type":"summary_text","text":""}}

event: response.reasoning_summary_text.delta
data: {"type":"response.reasoning_summary_text.delta","delta":"then deciding"}

event: response.completed
data: {"type":"response.completed","response":{"id":"r1","status":"completed","usage":{"input_tokens":5,"output_tokens":1,"input_tokens_details":{"cached_tokens":0}}}}
```

- [ ] **Step 2: Update the test to assert the separator**

Replace the body of `TestParseSSE_ReasoningSummary` in `internal/ai/codex/sse_test.go` with:

```go
func TestParseSSE_ReasoningSummary(t *testing.T) {
	b, _ := os.ReadFile("testdata/stream_reasoning.sse")
	out := make(chan ai.Event, 32)
	if err := parseSSE(context.Background(), strings.NewReader(string(b)), out); err != nil {
		t.Fatal(err)
	}
	close(out)
	evs := collect(t, out)

	var thinking strings.Builder
	for _, e := range evs {
		if t, ok := e.(ai.Thinking); ok {
			thinking.WriteString(t.Text)
		}
	}
	if thinking.String() != "Considering the problem\n\nthen deciding" {
		t.Fatalf("thinking text = %q", thinking.String())
	}
}
```

- [ ] **Step 3: Run the test to verify it fails**

Run: `go test ./internal/ai/codex/ -run TestParseSSE_ReasoningSummary -v`
Expected: FAIL — assembled string lacks the `\n\n` separator (it currently reads `"Considering the problemthen deciding"`).

- [ ] **Step 4: Implement the part-added case**

The first `part.added` arrives before any deltas, so we must NOT emit a separator on that one. Easiest correct logic: emit a separator only after we've already seen at least one delta. Track that with a parser-scoped flag.

Modify `parseSSE` in `internal/ai/codex/sse.go` to thread a small state through `dispatchEvent`. Replace the function bodies as follows:

```go
func parseSSE(ctx context.Context, r io.Reader, out chan<- ai.Event) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var (
		eventName       string
		dataBuf         strings.Builder
		seenSummaryText bool
	)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Text()
		if line == "" {
			if dataBuf.Len() > 0 || eventName != "" {
				if err := dispatchEvent(ctx, eventName, dataBuf.String(), out, &seenSummaryText); err != nil {
					return err
				}
			}
			eventName = ""
			dataBuf.Reset()
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(strings.TrimPrefix(line, "data:"))
			continue
		}
	}
	if dataBuf.Len() > 0 || eventName != "" {
		if err := dispatchEvent(ctx, eventName, dataBuf.String(), out, &seenSummaryText); err != nil {
			return err
		}
	}
	return scanner.Err()
}
```

Update the `dispatchEvent` signature and add the new case:

```go
func dispatchEvent(ctx context.Context, name, data string, out chan<- ai.Event, seenSummaryText *bool) error {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil
	}
	switch name {
	case "response.output_text.delta":
		var d textDelta
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			return nil
		}
		return send(ctx, out, ai.TextDelta{Text: d.Delta})

	case "response.reasoning_summary_text.delta":
		var d textDelta
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			return nil
		}
		*seenSummaryText = true
		return send(ctx, out, ai.Thinking{Text: d.Delta})

	case "response.reasoning_summary_part.added":
		if *seenSummaryText {
			return send(ctx, out, ai.Thinking{Text: "\n\n"})
		}
		return nil

	case "response.output_item.added":
		// ... unchanged
```

(Leave the remaining cases in `dispatchEvent` untouched.)

- [ ] **Step 5: Run the test to verify it passes**

Run: `go test ./internal/ai/codex/ -run TestParseSSE_ReasoningSummary -v`
Expected: PASS — assembled string is `"Considering the problem\n\nthen deciding"`.

- [ ] **Step 6: Run the full codex test suite**

Run: `go test ./internal/ai/codex/ -v`
Expected: all tests PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/ai/codex/sse.go internal/ai/codex/sse_test.go internal/ai/codex/testdata/stream_reasoning.sse
git commit -m "feat(codex): emit \\n\\n separator between reasoning summary parts"
```

---

## Task 3: Agent — `effort` field, `SetReasoningEffort`, default `"medium"`

**Files:**
- Modify: `internal/agent/agent.go`
- Modify: `internal/agent/loop_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/agent/loop_test.go`:

```go
func TestAgent_DefaultEffortIsMedium(t *testing.T) {
	a := New(faux.New(), tools.NewRegistry(), session.New("gpt-5"), "")
	if got := a.ReasoningEffort(); got != "medium" {
		t.Fatalf("default effort = %q, want %q", got, "medium")
	}
}

func TestAgent_SetReasoningEffort(t *testing.T) {
	a := New(faux.New(), tools.NewRegistry(), session.New("gpt-5"), "")
	a.SetReasoningEffort("high")
	if got := a.ReasoningEffort(); got != "high" {
		t.Fatalf("after set, effort = %q, want %q", got, "high")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/agent/ -run "TestAgent_DefaultEffortIsMedium|TestAgent_SetReasoningEffort" -v`
Expected: BUILD FAILURE — `a.ReasoningEffort` and `a.SetReasoningEffort` are undefined.

- [ ] **Step 3: Implement the field, getter, setter, and default**

Replace the body of `internal/agent/agent.go` with:

```go
package agent

import (
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

type Agent struct {
	provider ai.Provider
	tools    *tools.Registry
	session  *session.Session
	system   string
	effort   string
}

func New(p ai.Provider, t *tools.Registry, s *session.Session, systemPrompt string) *Agent {
	return &Agent{
		provider: p,
		tools:    t,
		session:  s,
		system:   systemPrompt,
		effort:   "medium",
	}
}

func (a *Agent) Provider() ai.Provider  { return a.provider }
func (a *Agent) Tools() *tools.Registry { return a.tools }
func (a *Agent) System() string         { return a.system }

func (a *Agent) ReasoningEffort() string         { return a.effort }
func (a *Agent) SetReasoningEffort(effort string) { a.effort = effort }
```

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `go test ./internal/agent/ -run "TestAgent_DefaultEffortIsMedium|TestAgent_SetReasoningEffort" -v`
Expected: PASS.

- [ ] **Step 5: Run the full agent test suite**

Run: `go test ./internal/agent/ -v`
Expected: all tests PASS (existing tests are unaffected).

- [ ] **Step 6: Commit**

```bash
git add internal/agent/agent.go internal/agent/loop_test.go
git commit -m "feat(agent): default reasoning effort to medium, add setter"
```

---

## Task 4: Agent — thread effort into the per-turn request

**Files:**
- Modify: `internal/agent/loop.go`
- Modify: `internal/agent/loop_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/agent/loop_test.go`:

```go
type captureProvider struct {
	gotReq ai.Request
}

func (c *captureProvider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	c.gotReq = req
	out := make(chan ai.Event, 2)
	out <- ai.Usage{Input: 1, Output: 1}
	out <- ai.Done{Reason: "stop"}
	close(out)
	return out, nil
}

func TestRun_ThreadsReasoningEffortIntoRequest(t *testing.T) {
	cp := &captureProvider{}
	a := New(cp, tools.NewRegistry(), session.New("gpt-5"), "")
	a.SetReasoningEffort("high")
	_ = collect(t, a.Run(context.Background(), userMsg("hi")))
	if cp.gotReq.Reasoning.Effort != "high" {
		t.Fatalf("req.Reasoning.Effort = %q, want %q", cp.gotReq.Reasoning.Effort, "high")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `go test ./internal/agent/ -run TestRun_ThreadsReasoningEffortIntoRequest -v`
Expected: FAIL — `req.Reasoning.Effort = ""`.

- [ ] **Step 3: Implement effort threading**

In `internal/agent/loop.go`, modify the `req` literal inside `runTurn`. Replace:

```go
		req := ai.Request{
			Model:    a.session.Model,
			System:   a.system,
			Messages: a.session.PathToActive(),
			Tools:    a.tools.Specs(),
		}
```

with:

```go
		req := ai.Request{
			Model:     a.session.Model,
			System:    a.system,
			Messages:  a.session.PathToActive(),
			Tools:     a.tools.Specs(),
			Reasoning: ai.ReasoningConfig{Effort: a.effort},
		}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/agent/ -run TestRun_ThreadsReasoningEffortIntoRequest -v`
Expected: PASS.

- [ ] **Step 5: Run the full agent test suite**

Run: `go test ./internal/agent/ -v`
Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/loop.go internal/agent/loop_test.go
git commit -m "feat(agent): thread reasoning effort into per-turn request"
```

---

## Task 5: TUI Messages — track timestamps; new `Render` signature; collapsed/expanded headers

**Files:**
- Modify: `internal/tui/messages.go`
- Modify: `internal/tui/messages_test.go`
- Modify: `internal/tui/root.go` (single call site for `Render`)

The thinking block now stores `startedAt` (set on first delta) and `endedAt` (zero while streaming, set on finalize — which Task 6 wires up). `Render` becomes `Render(s Styles, showThinking bool, now time.Time)`. Existing tests that call `m.Render(DefaultStyles())` are updated to `m.Render(DefaultStyles(), false, time.Time{})`.

- [ ] **Step 1: Update existing tests to pass new args (still asserting current behavior)**

In `internal/tui/messages_test.go`, replace every call of the form `m.Render(DefaultStyles())` with `m.Render(DefaultStyles(), false, time.Time{})`. Add `"time"` to the imports. The assertions stay unchanged — these existing tests should keep passing after the signature change.

- [ ] **Step 2: Write the failing tests for headers and toggle**

Append to `internal/tui/messages_test.go`:

```go
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
```

- [ ] **Step 3: Run the new tests to verify they fail**

Run: `go test ./internal/tui/ -run "TestMessages_ThinkingHeaderStreamingCollapsed|TestMessages_ThinkingHeaderFinalizedCollapsed|TestMessages_ThinkingExpandedShowsBody" -v`
Expected: BUILD FAILURE — `OnThinkingDeltaAt`, `FinalizeStreamingThinking`, and the new `Render` signature are undefined.

- [ ] **Step 4: Rewrite `internal/tui/messages.go` with the new model**

Replace the body of `internal/tui/messages.go` with:

```go
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
)

type Messages struct {
	width            int
	blocks           []block
	streamingAsstIdx int // -1 when no assistant block is currently streaming
}

type blockKind int

const (
	bkUser blockKind = iota
	bkAssistant
	bkThinking
	bkToolCall
	bkToolResult
	bkError
	bkInfo
)

type block struct {
	kind      blockKind
	text      string
	meta      string
	startedAt time.Time
	endedAt   time.Time
}

func NewMessages(width int) *Messages { return &Messages{width: width, streamingAsstIdx: -1} }

func (m *Messages) SetWidth(w int) { m.width = w }

func (m *Messages) AppendUser(text string) {
	m.blocks = append(m.blocks, block{kind: bkUser, text: text})
	m.streamingAsstIdx = -1
}

func (m *Messages) OnAssistantDelta(delta string) {
	if m.streamingAsstIdx == -1 {
		m.blocks = append(m.blocks, block{kind: bkAssistant})
		m.streamingAsstIdx = len(m.blocks) - 1
	}
	m.blocks[m.streamingAsstIdx].text += delta
}

// OnThinkingDelta appends to (or starts) the active thinking block, using time.Now()
// for the start timestamp. Tests should use OnThinkingDeltaAt for deterministic times.
func (m *Messages) OnThinkingDelta(delta string) {
	m.OnThinkingDeltaAt(delta, time.Now())
}

func (m *Messages) OnThinkingDeltaAt(delta string, now time.Time) {
	last := len(m.blocks)
	if last > 0 && m.blocks[last-1].kind == bkThinking && m.blocks[last-1].endedAt.IsZero() {
		m.blocks[last-1].text += delta
		return
	}
	m.blocks = append(m.blocks, block{kind: bkThinking, text: delta, startedAt: now})
}

// FinalizeStreamingThinking sets endedAt on the most recent in-progress thinking block, if any.
func (m *Messages) FinalizeStreamingThinking(now time.Time) {
	for i := len(m.blocks) - 1; i >= 0; i-- {
		if m.blocks[i].kind != bkThinking {
			continue
		}
		if m.blocks[i].endedAt.IsZero() {
			m.blocks[i].endedAt = now
		}
		return
	}
}

// HasInProgressThinking reports whether at least one thinking block has not yet been finalized.
// Used by RootModel to decide whether to keep its 1-second tick alive.
func (m *Messages) HasInProgressThinking() bool {
	for _, b := range m.blocks {
		if b.kind == bkThinking && b.endedAt.IsZero() {
			return true
		}
	}
	return false
}

func (m *Messages) OnToolStarted(call ai.ToolCall) {
	m.streamingAsstIdx = -1
	m.blocks = append(m.blocks, block{
		kind: bkToolCall,
		meta: call.Name,
		text: string(call.Args),
	})
}

func (m *Messages) OnToolFinished(f agent.ToolFinished) {
	kind := bkToolResult
	if f.Result.IsError {
		kind = bkError
	}
	m.blocks = append(m.blocks, block{
		kind: kind,
		meta: f.Call.Name,
		text: f.Result.Output,
	})
}

func (m *Messages) OnTurnDone(reason string) {
	m.streamingAsstIdx = -1
	if reason != "" && reason != "stop" {
		m.blocks = append(m.blocks, block{kind: bkInfo, text: fmt.Sprintf("(turn ended: %s)", reason)})
	}
}

func (m *Messages) OnTurnError(err error) {
	m.streamingAsstIdx = -1
	m.blocks = append(m.blocks, block{kind: bkError, text: err.Error()})
}

func (m *Messages) OnInfo(text string) {
	m.blocks = append(m.blocks, block{kind: bkInfo, text: text})
}

func (m *Messages) Render(s Styles, showThinking bool, now time.Time) string {
	var sb strings.Builder
	for i, b := range m.blocks {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		switch b.kind {
		case bkUser:
			sb.WriteString(s.User.Render("user> ") + b.text)
		case bkAssistant:
			sb.WriteString(s.Assistant.Render(b.text))
		case bkThinking:
			sb.WriteString(renderThinking(s, b, showThinking, now))
		case bkToolCall:
			sb.WriteString(s.ToolCall.Render(fmt.Sprintf("· %s(%s)", b.meta, b.text)))
		case bkToolResult:
			sb.WriteString(s.ToolResult.Render(fmt.Sprintf("← %s\n%s", b.meta, b.text)))
		case bkError:
			sb.WriteString(s.ToolError.Render("error: " + b.text))
		case bkInfo:
			sb.WriteString(s.Info.Render(b.text))
		}
	}
	return sb.String()
}

func renderThinking(s Styles, b block, showThinking bool, now time.Time) string {
	caret := "▸"
	if showThinking {
		caret = "▾"
	}
	var header string
	if b.endedAt.IsZero() {
		secs := int(now.Sub(b.startedAt).Seconds())
		if secs < 0 {
			secs = 0
		}
		header = fmt.Sprintf("%s thinking… (%ds)", caret, secs)
	} else {
		secs := int(b.endedAt.Sub(b.startedAt).Seconds())
		if secs < 0 {
			secs = 0
		}
		header = fmt.Sprintf("%s thought for %ds", caret, secs)
	}
	headerLine := s.ThinkingHeader.Render(header)
	if !showThinking {
		return headerLine
	}
	return headerLine + "\n" + s.Thinking.Render(b.text)
}
```

(Note: this introduces a reference to `s.ThinkingHeader`. Task 7 adds the style. Until then the file will not compile — that's expected because we're following a TDD/build-broken-then-fix flow within a single commit per task. But Task 5 must compile on its own, so add a temporary line in styles.go now and Task 7 will refine it.)

- [ ] **Step 5: Add a placeholder `ThinkingHeader` style so the package compiles**

Open `internal/tui/styles.go` and add to the `Styles` struct (above `Footer`):

```go
	ThinkingHeader lipgloss.Style
```

And inside `DefaultStyles()` (above `Footer`):

```go
		ThinkingHeader: lipgloss.NewStyle().Faint(true),
```

(Task 7 revisits this for final styling.)

- [ ] **Step 6: Update the single `Render` caller in root.go**

In `internal/tui/root.go::refreshViewport`, replace:

```go
	m.viewport.SetContent(m.msgs.Render(m.styles))
```

with:

```go
	m.viewport.SetContent(m.msgs.Render(m.styles, false, time.Now()))
```

(`"time"` is already imported in `root.go`.)

- [ ] **Step 7: Run the new tests to verify they pass**

Run: `go test ./internal/tui/ -run "TestMessages_ThinkingHeaderStreamingCollapsed|TestMessages_ThinkingHeaderFinalizedCollapsed|TestMessages_ThinkingExpandedShowsBody" -v`
Expected: PASS.

- [ ] **Step 8: Run the full TUI test suite to confirm no regressions**

Run: `go test ./internal/tui/... -v`
Expected: all tests PASS, including the existing `TestMessages_AssistantDeltaSurvivesIntervening` (which uses the public `OnThinkingDelta`, unaffected by the private timestamp logic).

- [ ] **Step 9: Build the whole tree to confirm root.go compiles with the new signature**

Run: `go build ./...`
Expected: success.

- [ ] **Step 10: Commit**

```bash
git add internal/tui/messages.go internal/tui/messages_test.go internal/tui/styles.go internal/tui/root.go
git commit -m "feat(tui): collapsible thinking block with header + global toggle scaffold"
```

---

## Task 6: TUI Messages — finalize-on-other-event semantics

**Files:**
- Modify: `internal/tui/messages.go`
- Modify: `internal/tui/messages_test.go`

When the assistant starts replying, a tool starts, the turn ends, or the turn errors, any in-progress thinking block must be finalized so its header switches from `thinking…` to `thought for Ns`.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/messages_test.go`:

```go
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
	out := m.Render(DefaultStyles(), false, time.Time{})
	// stop is the normal case — no info/thinking line should be rendered.
	if out != "" {
		t.Fatalf("turn done with stop should produce no rendered block, got:\n%s", out)
	}
}

func TestMessages_TurnDoneAbnormalIsInfo(t *testing.T) {
	m := NewMessages(80)
	m.OnTurnDone("max_tokens")
	out := m.Render(DefaultStyles(), false, time.Time{})
	if !strings.Contains(out, "(turn ended: max_tokens)") {
		t.Fatalf("expected info notice, got:\n%s", out)
	}
}
```

- [ ] **Step 2: Run the new tests to verify they fail**

Run: `go test ./internal/tui/ -run "TestMessages_AssistantDeltaFinalizesPriorThinking|TestMessages_ToolStartedFinalizesPriorThinking|TestMessages_TurnDoneFinalizesPriorThinking|TestMessages_TurnErrorFinalizesPriorThinking|TestMessages_TurnDoneStopDoesNotProduceThinkingBlock|TestMessages_TurnDoneAbnormalIsInfo" -v`
Expected: tests fail — finalization isn't called from the four event handlers, and `OnTurnDone("stop")` currently produces no block (this test should already pass), and `OnTurnDone("max_tokens")` already produces an info block (this should pass after Task 5's `bkInfo` move). The four finalization tests will fail because nothing calls `FinalizeStreamingThinking`.

- [ ] **Step 3: Wire finalization into the four event handlers**

In `internal/tui/messages.go`, modify the four methods to call `FinalizeStreamingThinking(time.Now())` at the top:

```go
func (m *Messages) OnAssistantDelta(delta string) {
	m.FinalizeStreamingThinking(time.Now())
	if m.streamingAsstIdx == -1 {
		m.blocks = append(m.blocks, block{kind: bkAssistant})
		m.streamingAsstIdx = len(m.blocks) - 1
	}
	m.blocks[m.streamingAsstIdx].text += delta
}

func (m *Messages) OnToolStarted(call ai.ToolCall) {
	m.FinalizeStreamingThinking(time.Now())
	m.streamingAsstIdx = -1
	m.blocks = append(m.blocks, block{
		kind: bkToolCall,
		meta: call.Name,
		text: string(call.Args),
	})
}

func (m *Messages) OnTurnDone(reason string) {
	m.FinalizeStreamingThinking(time.Now())
	m.streamingAsstIdx = -1
	if reason != "" && reason != "stop" {
		m.blocks = append(m.blocks, block{kind: bkInfo, text: fmt.Sprintf("(turn ended: %s)", reason)})
	}
}

func (m *Messages) OnTurnError(err error) {
	m.FinalizeStreamingThinking(time.Now())
	m.streamingAsstIdx = -1
	m.blocks = append(m.blocks, block{kind: bkError, text: err.Error()})
}
```

- [ ] **Step 4: Run the new tests to verify they pass**

Run: `go test ./internal/tui/ -run "TestMessages_" -v`
Expected: all `TestMessages_` tests PASS.

- [ ] **Step 5: Run the full TUI test suite**

Run: `go test ./internal/tui/... -v`
Expected: all PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/messages.go internal/tui/messages_test.go
git commit -m "feat(tui): finalize thinking block on assistant/tool/done/error events"
```

---

## Task 7: TUI Styles — refine `ThinkingHeader`

**Files:**
- Modify: `internal/tui/styles.go`

The placeholder added in Task 5 was a bare `Faint(true)`. Make it visually distinct from the body (which is `Faint(true).Italic(true)`): non-italic so the header reads as a header, but still subdued.

- [ ] **Step 1: Replace the `ThinkingHeader` style**

In `internal/tui/styles.go::DefaultStyles`, change the `ThinkingHeader` line to:

```go
		ThinkingHeader: lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("8")),
```

(The body keeps its existing `Faint(true).Italic(true)`. Color 8 is the existing "subdued" gray used for `ToolResult` and `Info`.)

- [ ] **Step 2: Run the TUI test suite**

Run: `go test ./internal/tui/... -v`
Expected: all PASS — the header tests assert on the literal text, not the ANSI escapes, so styling changes are invisible to assertions.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/styles.go
git commit -m "feat(tui): give thinking header its own subdued style"
```

---

## Task 8: TUI Root — `Ctrl+T` toggle, ticker, settings → effort

**Files:**
- Modify: `internal/tui/root.go`

This task has no automated tests (per spec — `internal/tui/root.go` ticker plumbing is verified by manual run). The work is mechanical.

- [ ] **Step 1: Add the `showThinking` and `pendingTickCmd` fields plus tick types**

Open `internal/tui/root.go`. Inside the `RootModel` struct, after `clipboardErr error`, add:

```go
	showThinking   bool
	pendingTickCmd tea.Cmd
```

Beneath `type compactDoneMsg struct{}` near the bottom of the file (keep tick types co-located with other tea.Msg types), add:

```go
type thinkingTickMsg struct{}

func thinkingTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return thinkingTickMsg{} })
}
```

- [ ] **Step 2: Pass `showThinking` and `time.Now()` into `Render`**

Replace the body of `refreshViewport` with:

```go
func (m *RootModel) refreshViewport() {
	atBottom := m.viewport.AtBottom()
	m.viewport.SetContent(m.msgs.Render(m.styles, m.showThinking, time.Now()))
	if atBottom {
		m.viewport.GotoBottom()
	}
}
```

(Task 5 already added the `time.Now()` form; this just substitutes `m.showThinking` for the literal `false`.)

- [ ] **Step 3: Intercept `Ctrl+T` and handle the tick message**

In `RootModel.Update`, find the existing `case AgentEventMsg:` and just before it, add a new case for the tick:

```go
	case thinkingTickMsg:
		m.refreshViewport()
		if m.msgs.HasInProgressThinking() {
			return m, thinkingTickCmd()
		}
		return m, nil
```

Then, locate the keybinding interception block that starts with `if k, ok := msg.(tea.KeyMsg); ok {` (around `root.go:166` before this change). Inside that block, after the `Ctrl+C` and `Esc` interceptors but before the `if !m.streaming` block, add:

```go
		if k.Type == tea.KeyCtrlT {
			m.showThinking = !m.showThinking
			m.refreshViewport()
			return m, nil
		}
```

- [ ] **Step 4: Schedule the ticker on the first thinking event of a turn**

In `RootModel.handleEvent`, replace the `case agent.ThinkingText:` line. Currently it reads:

```go
	case agent.ThinkingText:
		m.msgs.OnThinkingDelta(v.Delta)
```

Replace with:

```go
	case agent.ThinkingText:
		wasIdle := !m.msgs.HasInProgressThinking()
		m.msgs.OnThinkingDelta(v.Delta)
		if wasIdle {
			m.pendingTickCmd = thinkingTickCmd()
		}
```

The `pendingTickCmd` field added in Step 1 is the carrier — `handleEvent` populates it, the `AgentEventMsg` branch picks it up and batches it with the next-event command:

Replace:

```go
	case AgentEventMsg:
		if v.Ch != m.eventCh {
			// Stale event from a swapped-out session; drop it.
			return m, nil
		}
		m.handleEvent(v.Event)
		m.refreshViewport()
		return m, nextEventCmd(m.eventCh)
```

with:

```go
	case AgentEventMsg:
		if v.Ch != m.eventCh {
			// Stale event from a swapped-out session; drop it.
			return m, nil
		}
		m.handleEvent(v.Event)
		m.refreshViewport()
		cmds := []tea.Cmd{nextEventCmd(m.eventCh)}
		if m.pendingTickCmd != nil {
			cmds = append(cmds, m.pendingTickCmd)
			m.pendingTickCmd = nil
		}
		return m, tea.Batch(cmds...)
```

- [ ] **Step 5: Wire `/settings` modal application into the agent**

In `RootModel.applyModalResult`, find the `case *modal.SettingsModal:` branch:

```go
	case *modal.SettingsModal:
		if s, ok := payload.(modal.Settings); ok {
			m.settings = s
		}
```

Replace it with:

```go
	case *modal.SettingsModal:
		if s, ok := payload.(modal.Settings); ok {
			m.settings = s
			if s.Effort != "" {
				m.agent.SetReasoningEffort(s.Effort)
			}
		}
```

- [ ] **Step 6: Verify the package compiles**

Run: `go build ./...`
Expected: success.

- [ ] **Step 7: Run the full test suite to confirm nothing regressed**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 8: Manual smoke test**

Build and run:

```bash
go build -o /tmp/rune ./cmd/rune
/tmp/rune
```

Send a prompt that should provoke reasoning (e.g., `"think step by step about how 7 * 13 + 4 should be computed"`). Confirm in the TUI:

1. A `▸ thinking… (Ns)` line appears with `N` incrementing each second.
2. When the assistant text starts streaming, the header switches to `▸ thought for Ns`.
3. Pressing `Ctrl+T` toggles between `▸ thought for Ns` (collapsed) and `▾ thought for Ns` + body (expanded), for every thinking block in the transcript.
4. Open `/settings`, change effort to `low`, send another prompt. The next turn's reasoning should be visibly shorter.
5. Open `/settings`, change effort to `minimal`. The next turn typically produces no `▸ thinking…` block at all.

If any step fails, debug before committing.

- [ ] **Step 9: Commit**

```bash
git add internal/tui/root.go
git commit -m "feat(tui): Ctrl+T toggle, live timer, settings effort threading"
```

---

## Task 9: Hotkeys modal — document `Ctrl+T`

**Files:**
- Modify: `internal/tui/modal/hotkeys.go`

- [ ] **Step 1: Add the new hotkey line**

In `internal/tui/modal/hotkeys.go::View`, replace:

```go
	return `Hotkeys:
  Enter           submit
  Shift+Enter     newline
  Esc             cancel turn / close modal
  Ctrl+C ×2       quit
  Ctrl+L          /model
  Tab             path completion
  @               file picker
  /               command menu
  !cmd            run shell, send output
  !!cmd           run shell, do not send

(any key to close)`
```

with:

```go
	return `Hotkeys:
  Enter           submit
  Shift+Enter     newline
  Esc             cancel turn / close modal
  Ctrl+C ×2       quit
  Ctrl+L          /model
  Ctrl+T          toggle thinking
  Tab             path completion
  @               file picker
  /               command menu
  !cmd            run shell, send output
  !!cmd           run shell, do not send

(any key to close)`
```

- [ ] **Step 2: Verify the package compiles**

Run: `go build ./...`
Expected: success.

- [ ] **Step 3: Run the full test suite for one final pass**

Run: `go test ./...`
Expected: all PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/modal/hotkeys.go
git commit -m "docs(tui): list Ctrl+T thinking toggle in /hotkeys"
```

---

## Final verification

- [ ] **All tests pass:** `go test ./...`
- [ ] **No vet warnings:** `go vet ./...`
- [ ] **Format clean:** `gofmt -l . | wc -l` reports `0`
- [ ] **Build succeeds:** `go build ./...`
- [ ] **Spec coverage** — every section of `docs/superpowers/specs/2026-04-28-thinking-display-design.md` maps to a task above:
  - "Request side" → Tasks 3, 4, 8 (Step 5)
  - "Provider — parse the reasoning SSE events" → Tasks 1, 2
  - "TUI — collapsed-by-default block with global toggle" → Tasks 5, 6, 7, 8
  - "Edge cases: `OnTurnDone` collision" → Task 6 (Step 3 moves it to `bkInfo`)
  - "Edge cases: minimal effort" → no code path needed; absence of events naturally produces no block
  - "Edge cases: tick after stream end" → Task 8 (Step 3, the `HasInProgressThinking` check on each tick)
  - "Edge cases: session swap mid-stream" → no new code; existing `stopActiveTurn` plus the same `HasInProgressThinking` check handles it
  - "Testing: SSE" → Tasks 1, 2
  - "Testing: Messages" → Tasks 5, 6
  - "Testing: Agent loop" → Task 4
  - "/hotkeys" → Task 9
