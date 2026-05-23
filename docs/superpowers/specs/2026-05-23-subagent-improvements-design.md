# Subagent improvements design

## Problem

Three connected issues with rune's subagent feature today:

1. **Lifecycle output is spammy.** `internal/tui/messages.go:148` (`OnSubagentEvent`) appends a brand-new TUI block on every state transition (pending → running → completed, plus blocked for dependency chains). A single delegation produces 2–4 stacked blocks; three subagents = ~12 lines of pure chatter. There's no coalescing — tool calls get `collectToolRun` grouping, but subagent events don't.

2. **The model rarely picks `spawn_subagent`.** `internal/agent/system.go:13` (`BasePrompt`) only mentions subagents defensively (line 37: "when you start one, let it run"). No positive "when to delegate" guidance. The `spawn_subagent` tool spec is one generic sentence with no per-agent-type "when to use" hints. The model has no nudge to delegate over doing the work itself.

3. **When the model does delegate, it duplicates work or busy-polls.** The agent loop in `internal/agent/loop.go:188` emits `TurnDone` and exits as soon as the model produces no tool calls. So if the model:
   - Spawns a subagent and stops → `TurnDone` → results sit until the user types something
   - Spawns a subagent and keeps going → duplicates work the subagent will do
   - Spawns a subagent and calls `get_subagent_result` repeatedly → wastes turns busy-polling

   The auto-resume mechanism is *almost* in place: `injectCompletedSubagentSummaries` (loop.go:38) already drains completed summaries into the next turn as user-role messages. The missing piece is that the loop doesn't wait for in-flight subagents before ending the turn.

## Goal

Make subagents both more likely to run and less annoying when they do — by teaching the model when to delegate, keeping the turn alive while subagents finish, and collapsing TUI lifecycle noise into a single per-task block.

## Non-goals

- No new subagent types.
- No change to `get_subagent_result` semantics. It stays useful for status checks; auto-injection on completion is additive.
- No yield-and-resume architecture (Approach B in brainstorming). User-can-interject-during-wait is a separate future feature.
- No change to the read-only / full tool modes.

## Design

### 1. Auto-wait for in-flight subagents (`internal/agent/loop.go`, `internal/agent/subagents.go`)

In `runTurn`, before emitting `TurnDone`, check if any subagents are still in pending / blocked / running state. If yes, block on a "any completion" signal, then `continue` the loop. The next iteration's call to `injectCompletedSubagentSummaries` delivers the summary as a user-role message and re-prompts the model.

New supervisor methods:

```go
// HasInFlight reports whether any tracked subagent is pending, blocked, or running.
func (s *SubagentSupervisor) HasInFlight() bool

// AnyCompletion returns a channel that closes on the next time any subagent
// transitions to a terminal state (completed / failed / cancelled). Re-armed
// after each fire; callers obtain a fresh channel before each wait.
func (s *SubagentSupervisor) AnyCompletion() <-chan struct{}
```

`AnyCompletion` is implemented as a single-shot channel held under `s.mu`. `finish` closes the current one and replaces it with a fresh one for the next wait.

Loop change in `runTurn`:

```go
if len(calls) == 0 && len(invalidCallNames) == 0 {
    if a.subagents != nil && a.subagents.HasInFlight() {
        ch := a.subagents.AnyCompletion()
        select {
        case <-ch:
            continue // re-enter loop; top will inject completed summary
        case <-ctx.Done():
            out <- TurnAborted{}
            return
        }
    }
    out <- TurnDone{Reason: doneRsn}
    return
}
```

User interruption via context cancel (Esc) already works — the wait listens on `ctx.Done()`. Subagent timeouts already exist in `SubagentSupervisor.runTask`, so a hung subagent eventually transitions to `Failed`, which signals completion and the loop resumes.

### 2. Prompt nudges (`internal/agent/system.go`, `internal/tools/subagents.go`)

Replace `BasePrompt` line 37 with:

> When a task is self-contained and would take many tool calls to investigate (broad code search, design across multiple files, end-to-end review), prefer spawning a background subagent over doing the work yourself. After spawning, end your turn — the system will resume you automatically when the subagent finishes and inject its summary. Do not call `get_subagent_result` to poll. Do not duplicate the subagent's work in the meantime.

Expand `SpawnSubagent.Spec().Description` to include per-type "when to use" hints:

> Start a specialized subagent with isolated context. After spawning, end your turn — the system resumes you and injects the summary when the subagent finishes. When to use each type: **code-explorer**: investigating an unfamiliar feature area (>3 file reads or broad grep). **code-architect**: designing a non-trivial feature touching multiple files. **code-reviewer**: verifying a substantial change before reporting it complete. **exploration**: broader read-only discovery. **validator**: sanity-checking a plan before implementation. **general**: catch-all for self-contained delegated work. Custom subagents from `~/.rune/agents` and `./.rune/agents` are also available.

### 3. TUI lifecycle coalescing (`internal/tui/messages.go`)

Add a `taskID` field to the `block` struct. Change `OnSubagentEvent` to look up the existing block for that task ID and update it in place; only append on first sighting:

```go
func (m *Messages) OnSubagentEvent(ev agent.SubagentEvent) {
    m.FinalizeStreamingThinking(time.Now())
    m.streamingAsstIdx = -1
    text := renderSubagentEventText(ev)
    for i := range m.blocks {
        if m.blocks[i].kind == bkSubagent && m.blocks[i].taskID == ev.Task.ID {
            m.blocks[i].meta = string(ev.Status)
            m.blocks[i].text = text
            return
        }
    }
    m.blocks = append(m.blocks, block{kind: bkSubagent, meta: string(ev.Status), text: text, taskID: ev.Task.ID})
}
```

Trim flavor text in `renderSubagentEventText` to short labels (keeping `familiarLabel` for per-task identity like `Nyx, familiar of code-explorer-task`):

- Blocked: `◌ <label> waiting on dependencies`
- Pending: `◌ summoning <label>`
- Running: `◐ <label> working…`
- Completed (with summary): `✓ <label> returned (N lines)`
- Completed (no summary): `✓ <label> returned`
- Failed: `✗ <label> failed: <error>`
- Cancelled: `⊘ <label> dismissed`

## Testing

- `internal/agent/loop_test.go` — test that spawning a background subagent that finishes after a short delay keeps the loop alive: no `TurnDone` until the summary is injected and the model is re-prompted at least once.
- `internal/agent/subagents_test.go` — tests for `HasInFlight` (true while running, false after terminal) and `AnyCompletion` (basic fire, fresh channel per wait, race-safety when multiple subagents complete back-to-back).
- `internal/tui/messages_test.go` — three sequential events for the same `task_id` produce exactly one block whose text reflects the latest state; events for different task IDs produce separate blocks.
- Manual: run rune, ask a question that should trigger `spawn_subagent` (e.g. "explore how feature X works"), confirm the UI shows one block updating in place and the model receives the summary without user input.

## Risks

- **Loop never exits if a subagent hangs and the timeout is too long.** Subagents already have per-task timeouts (`SubagentSupervisor.runTask`), so a hung subagent eventually transitions to `Failed`, signalling completion and unblocking the loop. User can also cancel via Esc.
- **Prompt change doesn't change behavior.** The tool-description change reinforces the instruction at the point of use, which is more reliable than system prompt alone. If behavior is still off, follow-up could make `get_subagent_result` on an in-flight task return a "you don't need to poll" hint.
- **Existing TUI tests may assume one block per event.** Update them when changing `OnSubagentEvent`.

## Out of scope (potential follow-ups)

- Yield-and-resume architecture (Approach B) so the user can type during a wait.
- `get_subagent_result` returning a special "auto-delivery pending" status when called on an in-flight task, to further discourage polling.
- Per-subagent-type quiet/verbose mode in TUI settings.
