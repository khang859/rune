# Subagent Improvements Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make rune's subagent feature both more likely to be used and less noisy when it runs, by (1) teaching the model when to delegate, (2) keeping the parent turn alive until in-flight subagents finish so the model can stop after spawning instead of polling or duplicating work, and (3) collapsing TUI lifecycle events into a single per-task block with trimmed labels.

**Architecture:** Three independent changes in three packages. (a) Supervisor gets `HasInFlight` + `AnyCompletion` methods; the agent loop blocks on completion before emitting `TurnDone` if any subagent is still pending/blocked/running. (b) System prompt and `spawn_subagent` tool description gain positive delegation guidance and the explicit "end the turn — system auto-resumes you" instruction. (c) TUI `block` struct gets a `taskID` field; `OnSubagentEvent` updates the existing block in place instead of appending a new one each transition; lifecycle labels are trimmed of flavor text.

**Tech Stack:** Go 1.x, existing rune internals (`internal/agent`, `internal/tools`, `internal/tui`), `internal/ai/faux` test provider.

**Spec:** `docs/superpowers/specs/2026-05-23-subagent-improvements-design.md`

---

## File Map

| File | Change |
| --- | --- |
| `internal/agent/subagents.go` | Add `HasInFlight()` and `AnyCompletion() <-chan struct{}`. Wire completion signal into `finish()`. |
| `internal/agent/subagents_test.go` | New unit tests for the two methods. |
| `internal/agent/loop.go` | In `runTurn`, before `TurnDone`, wait on `AnyCompletion` if `HasInFlight`. |
| `internal/agent/loop_test.go` | New test asserting the loop blocks while a subagent is in-flight and resumes when it completes. |
| `internal/agent/system.go` | Replace the defensive subagent line in `BasePrompt` with positive delegation guidance. |
| `internal/agent/system_test.go` | Update or add assertion on `BasePrompt` content. |
| `internal/tools/subagents.go` | Expand `SpawnSubagent.Spec().Description` with per-type "when to use" hints and the "end your turn" instruction. |
| `internal/tools/subagents_test.go` | Add assertion that the spec description mentions auto-resume and per-type guidance. |
| `internal/tui/messages.go` | Add `taskID` field to `block`. Rewrite `OnSubagentEvent` to update in place. Trim flavor text in `renderSubagentEventText`. |
| `internal/tui/messages_test.go` | New tests for in-place update behavior and trimmed labels. |

---

## Task 1: Add `HasInFlight` and `AnyCompletion` to the supervisor

**Files:**
- Modify: `internal/agent/subagents.go`
- Modify: `internal/agent/subagents_test.go`

The supervisor already tracks every task in `s.tasks`. `HasInFlight` returns true if any task is in `SubagentBlocked`, `SubagentPending`, or `SubagentRunning`. `AnyCompletion` returns a channel that closes the next time `finish()` is called. The channel is re-created after each close so callers get a fresh one for each wait.

- [ ] **Step 1: Write failing tests**

Append to `internal/agent/subagents_test.go`:

```go
func TestSubagentSupervisor_HasInFlight(t *testing.T) {
	a := New(faux.New(), tools.NewRegistry(), session.New("gpt-test"), "")
	sup := a.Subagents()
	if sup.HasInFlight() {
		t.Fatal("expected HasInFlight=false on empty supervisor")
	}

	// Manually insert a running task to avoid races with a real spawn.
	sup.mu.Lock()
	sup.tasks["t1"] = &SubagentTask{ID: "t1", Status: SubagentRunning}
	sup.order = append(sup.order, "t1")
	sup.mu.Unlock()
	if !sup.HasInFlight() {
		t.Fatal("expected HasInFlight=true while task is running")
	}

	sup.mu.Lock()
	sup.tasks["t1"].Status = SubagentCompleted
	sup.mu.Unlock()
	if sup.HasInFlight() {
		t.Fatal("expected HasInFlight=false after task completes")
	}
}

func TestSubagentSupervisor_AnyCompletion_FiresOnFinish(t *testing.T) {
	a := New(faux.New(), tools.NewRegistry(), session.New("gpt-test"), "")
	sup := a.Subagents()

	sup.mu.Lock()
	sup.tasks["t1"] = &SubagentTask{ID: "t1", Status: SubagentRunning}
	sup.order = append(sup.order, "t1")
	sup.mu.Unlock()

	ch := sup.AnyCompletion()
	select {
	case <-ch:
		t.Fatal("channel should not be closed before completion")
	default:
	}

	go sup.finish("t1", SubagentCompleted, "done", "")

	select {
	case <-ch:
		// ok
	case <-time.After(time.Second):
		t.Fatal("AnyCompletion did not fire within 1s")
	}
}

func TestSubagentSupervisor_AnyCompletion_FreshChannelPerWait(t *testing.T) {
	a := New(faux.New(), tools.NewRegistry(), session.New("gpt-test"), "")
	sup := a.Subagents()

	sup.mu.Lock()
	sup.tasks["t1"] = &SubagentTask{ID: "t1", Status: SubagentRunning}
	sup.tasks["t2"] = &SubagentTask{ID: "t2", Status: SubagentRunning}
	sup.order = append(sup.order, "t1", "t2")
	sup.mu.Unlock()

	first := sup.AnyCompletion()
	sup.finish("t1", SubagentCompleted, "first", "")
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("first channel did not fire")
	}

	second := sup.AnyCompletion()
	if second == first {
		t.Fatal("expected a fresh channel after the first one fired")
	}
	select {
	case <-second:
		t.Fatal("second channel should not fire until next completion")
	default:
	}

	sup.finish("t2", SubagentCompleted, "second", "")
	select {
	case <-second:
	case <-time.After(time.Second):
		t.Fatal("second channel did not fire after second completion")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/agent/ -run 'TestSubagentSupervisor_HasInFlight|TestSubagentSupervisor_AnyCompletion' -v`

Expected: compilation failure — `HasInFlight` and `AnyCompletion` undefined.

- [ ] **Step 3: Add the methods and the completion-channel field**

In `internal/agent/subagents.go`, add a new field to `SubagentSupervisor` and a constructor initializer.

Edit the `SubagentSupervisor` struct (currently around line 85):

```go
type SubagentSupervisor struct {
	parent *Agent
	cfg    SubagentConfig
	sem    chan struct{}

	mu             sync.Mutex
	tasks          map[string]*SubagentTask
	order          []string
	cancels        map[string]context.CancelFunc
	subscribers    map[chan SubagentEvent]struct{}
	completionCh   chan struct{} // closed when any task transitions to a terminal state
}
```

In `NewSubagentSupervisor` (around line 99), initialize the new field after the existing map initializations:

```go
s := &SubagentSupervisor{
	parent:       parent,
	cfg:          cfg,
	sem:          make(chan struct{}, cfg.MaxConcurrent),
	tasks:        map[string]*SubagentTask{},
	cancels:      map[string]context.CancelFunc{},
	subscribers:  map[chan SubagentEvent]struct{}{},
	completionCh: make(chan struct{}),
}
```

Add the two new methods anywhere in the file (suggest immediately after `Get`, around line 366):

```go
// HasInFlight reports whether any tracked subagent is pending, blocked, or
// running. Used by the agent loop to decide whether to wait for completion
// instead of ending the turn.
func (s *SubagentSupervisor) HasInFlight() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range s.order {
		t := s.tasks[id]
		if t == nil {
			continue
		}
		switch t.Status {
		case SubagentBlocked, SubagentPending, SubagentRunning:
			return true
		}
	}
	return false
}

// AnyCompletion returns a channel that closes the next time any tracked
// subagent transitions to a terminal state. The channel is re-armed after
// each fire; callers must call AnyCompletion again before each wait.
func (s *SubagentSupervisor) AnyCompletion() <-chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.completionCh
}
```

In `finish` (currently around line 492), after `s.publishLocked(t)` and before unlocking, close the current completion channel and create a fresh one. The full block becomes:

```go
func (s *SubagentSupervisor) finish(id string, status SubagentStatus, summary, errMsg string) {
	var toStart []string
	s.mu.Lock()
	t := s.tasks[id]
	if t == nil {
		s.mu.Unlock()
		return
	}
	if t.Status == SubagentCancelled && status != SubagentCancelled {
		s.mu.Unlock()
		return
	}
	now := time.Now()
	t.Status = status
	t.CompletedAt = &now
	t.Summary = summary
	t.Error = errMsg
	s.publishLocked(t)
	close(s.completionCh)
	s.completionCh = make(chan struct{})
	toStart = s.resolveBlockedLocked()
	s.persistLocked()
	s.mu.Unlock()
	s.startReadyTasks(context.Background(), toStart)
}
```

Apply the same close-and-replace pattern in `Cancel` (around line 383) just before unlocking, so cancellations also wake waiters:

```go
func (s *SubagentSupervisor) Cancel(id string) error {
	var toStart []string
	s.mu.Lock()
	cancel := s.cancels[id]
	t := s.tasks[id]
	if t == nil {
		s.mu.Unlock()
		return fmt.Errorf("unknown subagent task %q", id)
	}
	if t.Status == SubagentPending || t.Status == SubagentBlocked {
		now := time.Now()
		t.Status = SubagentCancelled
		t.CompletedAt = &now
		s.publishLocked(t)
		close(s.completionCh)
		s.completionCh = make(chan struct{})
		toStart = s.resolveBlockedLocked()
		s.persistLocked()
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.startReadyTasks(context.Background(), toStart)
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/agent/ -run 'TestSubagentSupervisor_HasInFlight|TestSubagentSupervisor_AnyCompletion' -v`

Expected: PASS (all three tests).

- [ ] **Step 5: Run the full agent package tests to confirm no regression**

Run: `go test ./internal/agent/ -v`

Expected: all existing tests still pass.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/subagents.go internal/agent/subagents_test.go
git commit -m "Add HasInFlight and AnyCompletion to subagent supervisor"
```

---

## Task 2: Make `runTurn` wait for in-flight subagents

**Files:**
- Modify: `internal/agent/loop.go:188`
- Modify: `internal/agent/loop_test.go`

When the model produces no tool calls and no invalid calls, instead of unconditionally emitting `TurnDone`, first check `HasInFlight`. If true, wait on `AnyCompletion` (or `ctx.Done()`), then `continue` the loop. The existing `injectCompletedSubagentSummaries` at the top of the loop will deliver the new summary as a user-role message.

- [ ] **Step 1: Write the failing test**

Append to `internal/agent/loop_test.go`:

```go
func TestRun_WaitsForInFlightSubagentBeforeTurnDone(t *testing.T) {
	// Parent: emits text and ends; the loop should NOT end while a subagent is running.
	// After the subagent finishes, the loop resumes and emits a second text and ends.
	p := faux.New().
		Reply("starting work").Done().
		Reply("got the summary: subagent result").Done()
	a := New(p, tools.NewRegistry(), session.New("gpt-test"), "system")

	sup := a.Subagents()
	// Inject a running task by hand to avoid racing with the model's tool calls.
	sup.mu.Lock()
	sup.tasks["t1"] = &SubagentTask{
		ID:     "t1",
		Name:   "inspect",
		Status: SubagentRunning,
	}
	sup.order = append(sup.order, "t1")
	sup.mu.Unlock()

	events := a.Run(context.Background(), userMsg("go"))

	// Simulate the subagent finishing 50ms after Run starts.
	go func() {
		time.Sleep(50 * time.Millisecond)
		sup.finish("t1", SubagentCompleted, "subagent result", "")
	}()

	var texts []string
	var sawDone bool
	for ev := range events {
		switch v := ev.(type) {
		case AssistantText:
			texts = append(texts, v.Delta)
		case TurnDone:
			sawDone = true
		}
	}

	if !sawDone {
		t.Fatal("turn never finished")
	}
	joined := strings.Join(texts, "")
	if !strings.Contains(joined, "starting work") {
		t.Fatalf("expected first turn text in output, got: %q", joined)
	}
	if !strings.Contains(joined, "subagent result") {
		t.Fatalf("expected second turn (after auto-resume) text in output, got: %q", joined)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestRun_WaitsForInFlightSubagentBeforeTurnDone -v`

Expected: FAIL — turn ends immediately after "starting work" without ever entering the second scripted turn. The "subagent result" string is never produced.

- [ ] **Step 3: Modify `runTurn` to wait when subagents are in flight**

In `internal/agent/loop.go`, replace the early `TurnDone` block at line 188 (`if len(calls) == 0 && len(invalidCallNames) == 0 {`) with:

```go
if len(calls) == 0 && len(invalidCallNames) == 0 {
	if a.subagents != nil && a.subagents.HasInFlight() {
		ch := a.subagents.AnyCompletion()
		select {
		case <-ch:
			continue
		case <-ctx.Done():
			out <- TurnAborted{}
			return
		}
	}
	out <- TurnDone{Reason: doneRsn}
	return
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/agent/ -run TestRun_WaitsForInFlightSubagentBeforeTurnDone -v`

Expected: PASS — both "starting work" and "subagent result" appear, and `TurnDone` fires.

- [ ] **Step 5: Run the full agent package tests to catch regressions**

Run: `go test ./internal/agent/ -v`

Expected: all tests pass. If any existing test was implicitly relying on `TurnDone` firing while a subagent is still running, it will fail and needs an update (the most likely candidates are background-subagent tests in `subagents_test.go`; if they hang, audit them for missing `finish()` or short-lived tasks).

- [ ] **Step 6: Commit**

```bash
git add internal/agent/loop.go internal/agent/loop_test.go
git commit -m "Keep agent turn alive while subagents are in flight"
```

---

## Task 3: Rewrite the system-prompt subagent guidance

**Files:**
- Modify: `internal/agent/system.go:37`
- Modify: `internal/agent/system_test.go`

Replace the defensive single line with a positive delegation policy tied to the new auto-wait behavior.

- [ ] **Step 1: Write the failing assertion**

Append to `internal/agent/system_test.go`:

```go
func TestBasePrompt_TeachesAutoResumeAndDelegation(t *testing.T) {
	got := BasePrompt()
	mustContain := []string{
		"prefer spawning a background subagent",
		"end your turn",
		"resume you automatically",
		"Do not call `get_subagent_result` to poll",
	}
	for _, frag := range mustContain {
		if !strings.Contains(got, frag) {
			t.Errorf("BasePrompt missing required guidance: %q", frag)
		}
	}
}
```

(If the file does not already import `strings`, add it.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestBasePrompt_TeachesAutoResumeAndDelegation -v`

Expected: FAIL — none of the new fragments are present.

- [ ] **Step 3: Replace the prompt line**

In `internal/agent/system.go`, replace the current line 37:

```go
"- When you start a subagent, let it run; do not call get_subagent_result immediately after starting it. The subagent will post its result back when it is done. Do not immediately duplicate delegated work yourself unless the user asks, the subagent fails, or you need a small amount of extra context to synthesize its findings.",
```

with:

```go
"- When a task is self-contained and would take many tool calls to investigate (broad code search, design across multiple files, end-to-end review), prefer spawning a background subagent over doing the work yourself. After spawning, end your turn — the system will resume you automatically when the subagent finishes and inject its summary. Do not call `get_subagent_result` to poll. Do not duplicate the subagent's work in the meantime.",
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/agent/ -run TestBasePrompt_TeachesAutoResumeAndDelegation -v`

Expected: PASS.

- [ ] **Step 5: Run the full agent package tests**

Run: `go test ./internal/agent/ -v`

Expected: all tests pass. Watch for any `system_test.go` test that string-matched the old line; update it if needed.

- [ ] **Step 6: Commit**

```bash
git add internal/agent/system.go internal/agent/system_test.go
git commit -m "Teach the base prompt to delegate to subagents and yield"
```

---

## Task 4: Expand the `spawn_subagent` tool description

**Files:**
- Modify: `internal/tools/subagents.go:97` (the `SpawnSubagent.Spec()` method)
- Modify: `internal/tools/subagents_test.go`

The tool description is the per-call reminder the model sees every time `spawn_subagent` is presented. Expand it from a single generic sentence to per-type "when to use" hints plus the auto-resume instruction.

- [ ] **Step 1: Write the failing assertion**

Append to `internal/tools/subagents_test.go`:

```go
func TestSpawnSubagent_SpecDescribesAutoResumeAndTypes(t *testing.T) {
	spec := SpawnSubagent{}.Spec()
	mustContain := []string{
		"end your turn",
		"code-explorer",
		"code-architect",
		"code-reviewer",
		"general",
	}
	for _, frag := range mustContain {
		if !strings.Contains(spec.Description, frag) {
			t.Errorf("spawn_subagent description missing %q", frag)
		}
	}
}
```

(If the file does not already import `strings`, add it.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tools/ -run TestSpawnSubagent_SpecDescribesAutoResumeAndTypes -v`

Expected: FAIL — current description lacks the new fragments.

- [ ] **Step 3: Rewrite the description**

In `internal/tools/subagents.go`, in the `SpawnSubagent.Spec()` method, replace the existing `Description` field with:

```go
Description: "Start a specialized subagent with isolated context. After spawning, end your turn — the system resumes you and injects the summary when the subagent finishes; do not poll with get_subagent_result. When to use each type: code-explorer: investigating an unfamiliar feature area (>3 file reads or broad grep). code-architect: designing a non-trivial feature touching multiple files. code-reviewer: verifying a substantial change before reporting it complete. exploration: broader read-only discovery. validator: sanity-checking a plan before implementation. general: catch-all for self-contained delegated work. Custom subagents from ~/.rune/agents and ./.rune/agents are also available.",
```

(Keep `Name` and `Schema` unchanged.)

- [ ] **Step 4: Run the test to verify it passes**

Run: `go test ./internal/tools/ -run TestSpawnSubagent_SpecDescribesAutoResumeAndTypes -v`

Expected: PASS.

- [ ] **Step 5: Run the full tools package tests**

Run: `go test ./internal/tools/ -v`

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/tools/subagents.go internal/tools/subagents_test.go
git commit -m "Expand spawn_subagent description with per-type guidance"
```

---

## Task 5: Coalesce TUI subagent lifecycle events into one block per task

**Files:**
- Modify: `internal/tui/messages.go` (struct, `OnSubagentEvent`)
- Modify: `internal/tui/messages_test.go`

Add a `taskID` field to the `block` struct. Rewrite `OnSubagentEvent` to find the existing block for the same task ID and update it in place; only append a new block on first sighting.

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/messages_test.go`:

```go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/tui/ -run TestMessages_OnSubagentEvent -v`

Expected: `TestMessages_OnSubagentEvent_CoalescesByTaskID` FAILS — three separate blocks rendered, "Nyx" appears three times. The second test should already pass since today's code appends a block per event.

- [ ] **Step 3: Add `taskID` to the `block` struct**

In `internal/tui/messages.go`, update the `block` struct around line 35:

```go
type block struct {
	kind      blockKind
	text      string
	meta      string
	count     int
	startedAt time.Time
	endedAt   time.Time
	taskID    string
}
```

- [ ] **Step 4: Rewrite `OnSubagentEvent` to coalesce**

Replace the current `OnSubagentEvent` (around line 148):

```go
func (m *Messages) OnSubagentEvent(ev agent.SubagentEvent) {
	m.FinalizeStreamingThinking(time.Now())
	m.streamingAsstIdx = -1
	text := renderSubagentEventText(ev)
	taskID := ev.Task.ID
	if taskID != "" {
		for i := range m.blocks {
			if m.blocks[i].kind == bkSubagent && m.blocks[i].taskID == taskID {
				m.blocks[i].meta = string(ev.Status)
				m.blocks[i].text = text
				return
			}
		}
	}
	m.blocks = append(m.blocks, block{
		kind:   bkSubagent,
		meta:   string(ev.Status),
		text:   text,
		taskID: taskID,
	})
}
```

- [ ] **Step 5: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestMessages_OnSubagentEvent -v`

Expected: both tests PASS.

- [ ] **Step 6: Run the full TUI package tests**

Run: `go test ./internal/tui/ -v`

Expected: all tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/messages.go internal/tui/messages_test.go
git commit -m "Coalesce subagent lifecycle into one TUI block per task"
```

---

## Task 6: Trim flavor text in `renderSubagentEventText`

**Files:**
- Modify: `internal/tui/messages.go` (`renderSubagentEventText`)
- Modify: `internal/tui/messages_test.go`

Replace the fantasy phrasing with short, status-focused labels. Keep `familiarLabel` for per-task identity (`Nyx, familiar of inspect`); only the surrounding verbs change.

- [ ] **Step 1: Write the failing tests**

Append to `internal/tui/messages_test.go`:

```go
func TestRenderSubagentEventText_TrimmedLabels(t *testing.T) {
	mk := func(status agent.SubagentStatus, summary, errMsg string) agent.SubagentEvent {
		return agent.SubagentEvent{
			Task: tools.SubagentTask{
				ID: "t1", Name: "inspect", FamiliarName: "Nyx", Summary: summary, Error: errMsg,
			},
			Status: status,
		}
	}

	cases := []struct {
		name   string
		ev     agent.SubagentEvent
		expect string
	}{
		{"pending", mk(agent.SubagentPending, "", ""), "summoning Nyx"},
		{"running", mk(agent.SubagentRunning, "", ""), "Nyx working"},
		{"blocked", mk(agent.SubagentBlocked, "", ""), "waiting on dependencies"},
		{"completed", mk(agent.SubagentCompleted, "line a\nline b", ""), "Nyx returned (2 lines)"},
		{"completed-empty", mk(agent.SubagentCompleted, "", ""), "Nyx returned"},
		{"failed", mk(agent.SubagentFailed, "", "boom"), "Nyx failed: boom"},
		{"cancelled", mk(agent.SubagentCancelled, "", ""), "Nyx dismissed"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderSubagentEventText(c.ev)
			if !strings.Contains(got, c.expect) {
				t.Fatalf("expected %q to contain %q", got, c.expect)
			}
			if strings.Contains(got, "scrying") || strings.Contains(got, "summoning circle") || strings.Contains(got, "lost the thread") || strings.Contains(got, "veil") {
				t.Fatalf("flavor text not trimmed in %q", got)
			}
		})
	}
}
```

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/tui/ -run TestRenderSubagentEventText_TrimmedLabels -v`

Expected: FAIL on most cases — current output contains "scrying", "summoning circle", "lost the thread", etc.

- [ ] **Step 3: Rewrite `renderSubagentEventText`**

In `internal/tui/messages.go`, replace the existing function (around line 221) with:

```go
func renderSubagentEventText(ev agent.SubagentEvent) string {
	t := ev.Task
	label := familiarLabel(t.FamiliarName, t.Name)
	switch ev.Status {
	case agent.SubagentBlocked:
		return fmt.Sprintf("◌ %s waiting on dependencies", label)
	case agent.SubagentPending:
		return fmt.Sprintf("◌ summoning %s", label)
	case agent.SubagentRunning:
		return fmt.Sprintf("◐ %s working…", label)
	case agent.SubagentCompleted:
		if strings.TrimSpace(t.Summary) == "" {
			return fmt.Sprintf("✓ %s returned", label)
		}
		lines := strings.Count(strings.TrimSpace(t.Summary), "\n") + 1
		return fmt.Sprintf("✓ %s returned (%d lines)", label, lines)
	case agent.SubagentFailed:
		if strings.TrimSpace(t.Error) == "" {
			return fmt.Sprintf("✗ %s failed", label)
		}
		return fmt.Sprintf("✗ %s failed: %s", label, strings.TrimSpace(t.Error))
	case agent.SubagentCancelled:
		return fmt.Sprintf("⊘ %s dismissed", label)
	default:
		return fmt.Sprintf("%s %s", label, ev.Status)
	}
}
```

The task-ID suffix (`(subagent_3)` etc.) that the old version appended is dropped — task IDs aren't user-facing identifiers; the familiar name serves that purpose, and the in-place coalescing means the block updates instead of accumulating, so disambiguation isn't needed.

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test ./internal/tui/ -run TestRenderSubagentEventText_TrimmedLabels -v`

Expected: all subtests PASS.

- [ ] **Step 5: Run the full TUI package tests**

Run: `go test ./internal/tui/ -v`

Expected: all tests pass. If any earlier test asserted on the old flavor strings, update those to the new wording.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/messages.go internal/tui/messages_test.go
git commit -m "Trim flavor text from subagent lifecycle labels"
```

---

## Task 7: Whole-tree verification

**Files:** none (verification only)

- [ ] **Step 1: Run the full project test suite**

Run: `go test ./...`

Expected: all packages pass.

- [ ] **Step 2: Run `go vet` and `gofmt`**

Run: `make all` (which runs vet + fmt + test + build per `Makefile`).

Expected: no vet warnings, no formatting changes, all tests pass, binary builds.

- [ ] **Step 3: Manual smoke test**

1. Build: `make build`
2. Start: `./rune` (interactive)
3. Ask: *"Use a subagent to explore how the subagent feature itself works in this codebase."*
4. Verify:
   - The TUI shows a single in-place updating line for the spawned subagent (no `pending → running → completed` block stack).
   - The labels use short forms (e.g. `summoning Nyx`, `Nyx working…`, `Nyx returned (N lines)`) — no "scrying" or "summoning circle".
   - After the model issues `spawn_subagent`, the parent does not call `get_subagent_result` in a tight loop and does not start doing the same exploration itself. It waits.
   - When the subagent finishes, the parent automatically resumes and produces a final answer in the same turn (no user prompt required to trigger continuation).
5. Note any UX surprises in the PR description for future tuning.

- [ ] **Step 4: Done**

No final commit unless smoke test surfaced fixes.

---

## Self-Review Notes

- Spec coverage: each numbered section of the design (`Auto-wait`, `Prompt nudges`, `TUI lifecycle coalescing`) maps to Tasks 1+2, 3+4, and 5+6 respectively. Task 7 covers manual verification.
- No placeholders, no "TBD"/"TODO" markers in any task.
- Type consistency: `HasInFlight() bool` and `AnyCompletion() <-chan struct{}` are introduced in Task 1 and used in Task 2 with matching signatures. The `taskID` field in `block` is added in Task 5 and used in the same task.
- Out-of-scope items from the spec (`get_subagent_result` deprecation, Approach B yield-and-resume, per-type quiet/verbose TUI mode) are not implemented — intentional, matches non-goals.
