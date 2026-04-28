# Thinking-step display for reasoning models

Date: 2026-04-28
Status: Approved (pending implementation)

## Problem

rune already has plumbing for thinking events end-to-end: `ai.Thinking{Text}` →
`agent.ThinkingText{Delta}` → `Messages.OnThinkingDelta`, rendered as
faint+italic text in the TUI viewport. But no thinking ever surfaces in the UI.
Two underlying gaps:

1. The codex SSE handler (`internal/ai/codex/sse.go::dispatchEvent`) has no
   case for OpenAI Responses API reasoning events
   (`response.reasoning_summary_text.delta` and friends). They are silently
   dropped.
2. The agent never sets `req.Reasoning.Effort` when building a turn request
   (`internal/agent/loop.go:31`). The codex payload only includes the reasoning
   block when effort is non-empty (`internal/ai/codex/request.go:51`), so the
   API is not asked to emit reasoning summaries in the first place. The
   `/settings` modal stores `Settings.Effort` on `RootModel` but it is never
   threaded into the request.

The user wants thinking surfaced for reasoning models, presented like a
pi-style coding agent: a collapsed `▸ thinking… (Ns)` header that the user can
toggle open globally with a keybinding.

All three currently-available models (`gpt-5`, `gpt-5-codex`,
`gpt-5.1-codex-mini`) are reasoning models, so no explicit "is this a thinking
model" gate is required — display is implicitly gated on whether reasoning
events arrive.

## UX

- **Default state:** every thinking block is collapsed, shown as a single
  header line.
- **Streaming, collapsed:** `▸ thinking… (3s)` — elapsed time updates once per
  second.
- **Finalized, collapsed:** `▸ thought for 5s` — fixed once the next event
  finalizes the block.
- **Expanded:** `▾` prefix in place of `▸`, with the faint-italic body
  underneath. The toggle is global (one boolean for the whole transcript), so
  flipping it expands every thinking block in the viewport at once.
- **Toggle key:** `Ctrl+T` flips the global flag.
- **No persistence.** Thinking is view-only and ephemeral. Reloading via
  `/resume` or scrolling history shows no thinking from past turns. This is a
  deliberate v1 simplification.
- **Multiple blocks per turn** happen naturally when reasoning interleaves with
  tool calls: each contiguous run of reasoning becomes its own block, finalized
  by the next assistant text or tool call.

## Architecture

Three concerns line up with three change points along the existing pipeline.
No goroutine or channel-shape changes.

### 1. Request side: thread effort into the per-turn request

`Agent` gains an in-memory `effort string` field plus a
`SetReasoningEffort(string)` setter. `agent.New` initializes it to `medium` so
the very first turn already requests reasoning summaries; the user never has
to open `/settings` to get thinking output. `runTurn` populates
`req.Reasoning.Effort` from that field. `RootModel.applyModalResult` calls
`m.agent.SetReasoningEffort(s.Effort)` when the settings modal closes.

Effort lives in process memory only; it survives `/model`, `/new`, `/fork`,
and `/resume` within a session, but resets to default on `/quit`. Acceptable
for v1.

### 2. Provider: parse OpenAI reasoning summary SSE events

Add cases in `internal/ai/codex/sse.go::dispatchEvent`:

- `response.reasoning_summary_text.delta` → `ai.Thinking{Text: <delta>}`
- `response.reasoning_summary_part.added` → `ai.Thinking{Text: "\n\n"}` so
  multi-part summaries render with a blank line between parts.

Other reasoning event names stay ignored, consistent with how the dispatcher
silently drops every unknown event type today.

### 3. TUI: collapsed-by-default block with global toggle and live timer

The thinking block in `internal/tui/messages.go` grows two timestamps:
`startedAt` (set on first delta) and `endedAt` (set when something else —
assistant delta, tool call, turn done — implicitly finalizes the block).
`Messages.Render` takes a `showThinking bool` and `now time.Time` so it can
format the header from real elapsed time without coupling to wall clock.

`RootModel` gains `showThinking bool` and a 1-second `tea.Tick` driver. The
ticker schedules itself only while at least one in-progress thinking block
exists; once every thinking block is finalized, the next tick handler returns
no new command and the ticker stops naturally. `Ctrl+T` flips
`m.showThinking` and triggers a viewport refresh.

## File-level changes

| File | Change |
|---|---|
| `internal/agent/agent.go` | Add `effort string` field; add `SetReasoningEffort(string)` setter |
| `internal/agent/loop.go` | In `runTurn`, populate `req.Reasoning.Effort = a.effort` |
| `internal/ai/codex/sse.go` | Handle `response.reasoning_summary_text.delta` → `ai.Thinking{Text: delta}`; handle `response.reasoning_summary_part.added` → `ai.Thinking{Text: "\n\n"}` separator |
| `internal/tui/messages.go` | Thinking block tracks `startedAt`/`endedAt`; `OnAssistantDelta`/`OnToolStarted`/`OnTurnDone`/`OnTurnError` finalize the prior thinking block; `Render` takes `showThinking bool` and `now time.Time`; `OnTurnDone`'s `(turn ended: …)` notice moves from `bkThinking` to `bkInfo` |
| `internal/tui/styles.go` | Add `ThinkingHeader` style (faint, non-italic, slightly more visible than the body) |
| `internal/tui/root.go` | Add `showThinking bool`; intercept `Ctrl+T` in the root `Update` loop **before** the key falls through to the editor (same precedence as the existing `Ctrl+C` / `Esc` interceptors); on first `ThinkingText` of a turn, schedule a 1s `tickMsg`; on tick, refresh and reschedule if any in-progress block remains; in `applyModalResult` for `*modal.SettingsModal`, call `m.agent.SetReasoningEffort(s.Effort)`; pass `m.showThinking` and `time.Now()` into `m.msgs.Render` |
| `internal/tui/modal/hotkeys.go` | Add `Ctrl+T   toggle thinking` to the rendered list |

## Edge cases

- **`OnTurnDone` collision.** `messages.go:85` currently emits the
  `(turn ended: …)` notice as `bkThinking`. That is a status note, not
  reasoning. Move it to `bkInfo` so `bkThinking` has a single, clean meaning.
- **Effort = `minimal`.** The API may emit no reasoning summary. No
  `ai.Thinking` arrives, no block is created, no header. Correct by
  construction.
- **`Ctrl+T` mid-stream.** All state mutations live inside the bubble tea
  update loop, so flipping `m.showThinking` is race-free — the next
  `View()` call rerenders.
- **Tick after stream end.** The tick handler checks for any in-progress
  thinking block; if none, returns no new command and the ticker stops.
- **Session swap mid-stream.** `stopActiveTurn` already cancels the turn; the
  new session has no in-progress thinking block, so the next tick (if any)
  finds nothing to do and stops.
- **Subsecond reasoning.** Render `(0s)` — matches what happened.

## Testing

- `internal/ai/codex/sse_test.go` — feed a `reasoning_summary_text.delta` SSE
  frame; assert `ai.Thinking{Text: "..."}`. Feed
  `reasoning_summary_part.added` between two deltas; assert a separator
  `ai.Thinking` is emitted.
- `internal/tui/messages_test.go` — assert collapsed header for streaming
  (`▸ thinking… (Ns)`) and finalized (`▸ thought for Ns`) using an injected
  `now`; assert that `OnAssistantDelta` after a thinking block sets
  `endedAt`; assert the global toggle changes the rendered output.
- `internal/agent/loop_test.go` — faux provider that captures the request;
  turn with `agent.SetReasoningEffort("medium")` sends
  `Reasoning.Effort = "medium"`.
- No new tests for `root.go` ticker plumbing — covered by manual run.

## Out of scope (deliberate)

- Persisting thinking text to session JSON. Reloaded sessions will not show
  thinking from past turns.
- Per-block focus / expand UI. Single global toggle only.
- Detecting reasoning-vs-non-reasoning models. No non-reasoning model exists
  in the picker today; the gate is implicit.
- Footer hint indicating the toggle exists. The keybinding is listed in
  `/hotkeys` and nowhere else.
