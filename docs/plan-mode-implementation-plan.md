# Plan Mode Implementation Plan

## MVP goal

Add a safe, enforced Plan Mode for interactive Rune:

- `/plan` enters Plan Mode.
- `/act` exits Plan Mode.
- `/approve` exits Plan Mode after user accepts the plan.
- `/cancel-plan` clears pending plan state.
- Plan Mode adds stronger system instructions.
- Plan Mode hides and runtime-denies mutating tools.
- Plan Mode can still use read-only exploration and subagents safely.
- The UI clearly shows when Plan Mode is active.

## Guiding principles

- Enforce safety in code, not only in prompts.
- Tool spec filtering is helpful but insufficient; runtime tool execution must also enforce permissions.
- Deny opaque external/MCP tools by default in Plan Mode until capability metadata or an allowlist exists.
- Avoid changing permissions mid-turn. Mode changes should be blocked while streaming or applied only at safe turn boundaries.
- Do not automatically implement after `/approve`; require an explicit follow-up user message.
- Keep the MVP focused. Add specialized validator/exploration subagent types as a follow-up.

## Implementation status

Status: **MVP implemented**.

Implemented in this pass:

- Added registry permission modes in `internal/tools/tool.go`:
  - `PermissionModeAct`
  - `PermissionModePlan`
- `Registry.Specs()` now hides Plan Mode-denied tools.
- `Registry.Run()` now runtime-denies Plan Mode-denied tools with a normal `Result{IsError: true}` instead of a Go error.
- Plan Mode allows read-only/core-safe tools:
  - `read`
  - `web_search`
  - `web_fetch`
  - `spawn_subagent`
  - `list_subagents`
  - `get_subagent_result`
  - `cancel_subagent`
- Plan Mode denies by default:
  - `write`
  - `edit`
  - `bash`
  - MCP/external tools and any other tool not explicitly allowed
- `CloneReadOnly()` now uses Plan Mode policy and still unregisters child subagent tools so child subagents cannot recursively spawn subagents.
- Added agent modes in `internal/agent/agent.go`:
  - `ModeAct`
  - `ModePlan`
- `Agent.SetMode()` propagates to the tool registry permission mode.
- Agent turns append `PlanModePrompt()` while in Plan Mode.
- Added TUI slash commands:
  - `/plan`
  - `/act`
  - `/approve`
  - `/cancel-plan`
- Mode-changing slash commands are blocked while streaming or compacting.
- `/approve` returns to Act Mode and does **not** automatically start implementation.
- Added a compact footer `plan` indicator.
- Refactored shell shortcuts so the editor reports shell intent and `RootModel` executes or blocks them.
- `!cmd` and `!!cmd` are blocked in Plan Mode.
- Added user docs:
  - `docs/plan-mode.md`
  - README command entries
  - keybinding notes
  - `/hotkeys` modal entries
- Added/updated tests for tool permissions, agent prompt/mode behavior, denied tool calls, subagent read-only policy, TUI commands, shell shortcut blocking, and footer rendering.

Validation completed:

```sh
go test ./internal/tools ./internal/agent ./internal/tui ./internal/tui/editor
go test ./...
```

## Phase 1: Tool permission enforcement

### Files

- `internal/tools/tool.go`
- `internal/tools/tool_test.go`
- Possibly `internal/mcp/tool.go`

### Changes

Add a registry-level permission policy, for example:

```go
type PermissionMode string

const (
	PermissionModeAct  PermissionMode = "act"
	PermissionModePlan PermissionMode = "plan"
)
```

Extend `Registry` with a mode field and methods:

```go
func (r *Registry) SetPermissionMode(mode PermissionMode)
func (r *Registry) PermissionMode() PermissionMode
```

Update these methods:

```go
func (r *Registry) Specs() []ai.ToolSpec
func (r *Registry) Run(ctx context.Context, call ai.ToolCall) (Result, error)
```

In Plan Mode:

- Hide denied tools from `Specs()`.
- Deny denied tools in `Run()` even if the model calls them.

Denied in the MVP:

- `write`
- `edit`
- `bash`
- all MCP/external tools by default

Allowed in the MVP:

- `read`
- `web_search`
- `web_fetch`
- `spawn_subagent`
- `list_subagents`
- `get_subagent_result`
- likely `cancel_subagent`, for cleanup

Permission denial should return a normal tool result, not a Go error:

```go
Result{
	Output:  `tool "write" is disabled in Plan Mode`,
	IsError: true,
}
```

This avoids orphan tool-call issues in the agent loop.

### Subagent implication

Update `CloneReadOnly()` so it returns a registry with read-only/plan policy, not just a name-filtered clone. Keep existing unregistering of child subagent tools so children cannot recursively spawn agents.

## Phase 2: Agent mode and prompt overlay

### Files

- `internal/agent/agent.go`
- `internal/agent/loop.go`
- `internal/agent/system.go`
- `internal/agent/system_test.go`
- `internal/agent/loop_test.go`

### Changes

Add an agent mode:

```go
type Mode string

const (
	ModeAct  Mode = "act"
	ModePlan Mode = "plan"
)
```

Add methods:

```go
func (a *Agent) SetMode(mode Mode)
func (a *Agent) Mode() Mode
```

When mode changes, set the tool registry permission mode too.

In the turn loop, append Plan Mode instructions when mode is Plan:

```md
You are in PLAN MODE.

- Do not edit, write, delete, or run shell commands.
- Use read-only tools and read-only subagents for exploration.
- Research the codebase before proposing implementation.
- Ask clarifying questions when needed.
- Produce a concise, reviewable implementation plan.
- End by asking the user to approve before implementation.
```

Add a helper in `system.go`, such as:

```go
func PlanModePrompt() string
```

This keeps prompt text centralized.

## Phase 3: TUI slash commands and UI state

### Files

- `internal/tui/root.go`
- `internal/tui/footer.go`
- `internal/tui/styles.go`
- `internal/tui/root_test.go`
- `internal/tui/editor/editor_test.go`
- `internal/tui/modal/hotkeys.go`

### Changes

Add commands to `baseSlashCmds`:

```go
"/plan", "/act", "/approve", "/cancel-plan"
```

Add TUI state if needed:

```go
planPending bool
```

### `/plan`

- Block if streaming.
- Set agent mode to Plan.
- Set footer mode to `plan`.
- Show an informational message:

```text
plan mode: edits, bash, and MCP tools disabled
```

### `/act`

- Block if streaming.
- Set agent mode to Act.
- Clear `planPending`.
- Show an informational message:

```text
act mode: implementation tools enabled
```

### `/approve`

MVP behavior should stay simple and safe:

- Block if streaming.
- Set agent mode to Act.
- Clear `planPending`.
- Show an informational message:

```text
plan approved; act mode enabled — send your next message to implement
```

Do not automatically start implementation in the MVP. The user should send the implementation instruction after approval, or type something like "go ahead".

### `/cancel-plan`

- Block if streaming.
- Clear pending plan state.
- Recommended MVP behavior: keep Plan Mode active and only cancel the pending approval state.
- Show an informational message:

```text
plan cancelled; still in plan mode
```

### Footer

Add a compact mode indicator to `Footer`, for example:

```go
Mode string
```

Render as either:

```text
plan
```

or:

```text
act
```

To reduce clutter, it may be enough to show only `plan` when Plan Mode is active.

### Banner

Optional for MVP but recommended:

```text
Plan Mode: edits, bash, and MCP tools disabled · /approve or /act to implement
```

If a banner is added, update `layout()` row accounting.

## Phase 4: Shell shortcut safety

### Files

- `internal/tui/editor/editor.go`
- Possibly `internal/tui/root.go`
- Tests in `internal/tui/editor/editor_test.go` or `internal/tui/root_test.go`

The editor supports `!cmd` and `!!cmd`. These can bypass Plan Mode if they execute shell commands directly.

MVP options:

1. Block shell shortcuts in Plan Mode. This is the recommended safety behavior.
2. Warn and require `/act`.
3. Document them as user-controlled bypasses. This is the weakest option.

Recommended behavior: block shell shortcuts in Plan Mode with a message like:

```text
shell shortcuts are disabled in Plan Mode; use /act to run commands
```

Implementation details depend on where shell shortcut execution is triggered. If the editor returns a shell command result to `RootModel`, block in `RootModel` when `agent.Mode() == ModePlan`.

## Phase 5: Subagent integration for Plan Mode

### Current state

Subagents already provide useful building blocks:

- isolated session
- child registry via `CloneReadOnly()`
- dependency support
- background lifecycle events
- result injection

### MVP

Keep only the existing `general` subagent type, but update the Plan Mode prompt to encourage the main agent to spawn read-only exploration subagents.

Do not add `exploration` and `validator` types in the MVP unless the implementation scope is intentionally expanded.

### Follow-up

Add first-class subagent types:

- `exploration`
- `validator`

Likely files:

- `internal/agent/subagents.go`
- `internal/tools/subagents.go`
- `internal/agent/subagents_test.go`
- `internal/tools/subagents_test.go`

Suggested workflow:

```text
exploration subagents → draft plan → validator subagent → revise plan → present to user
```

## Phase 6: Documentation

### Files

- `README.md`
- `docs/keybindings.md`
- New `docs/plan-mode.md`, if desired
- `docs/architecture.md`, if the permission system deserves architecture notes
- `internal/tui/modal/hotkeys.go`

Document:

- What Plan Mode does.
- Commands:
  - `/plan`
  - `/act`
  - `/approve`
  - `/cancel-plan`
- Tools disabled in Plan Mode.
- Subagents can be used for read-only exploration.
- Whether Plan Mode is interactive/TUI-only for the MVP.

## Tests

### Tool registry

- Normal mode exposes and runs `write`, `edit`, and `bash`.
- Plan mode hides `write`, `edit`, and `bash` from `Specs()`.
- Plan mode denies `write`, `edit`, and `bash` in `Run()`.
- Unknown-tool behavior remains unchanged.
- A fake MCP/external tool is denied in Plan Mode.

### Agent loop

- Plan Mode adds the plan-specific system prompt.
- Plan Mode request excludes denied tools.
- A denied tool call returns a tool result with `IsError=true` and does not create orphan tool calls.

### TUI commands

- `/plan` sets Plan Mode and updates footer/message.
- `/act` exits Plan Mode.
- `/approve` exits Plan Mode but does not automatically start a turn.
- `/cancel-plan` clears pending state.
- Commands while streaming are blocked, or explicitly next-turn-only. Blocking is recommended for MVP.

### Slash menu

- New commands appear and can be selected.

### Subagents

- `CloneReadOnly()` still denies `write`, `edit`, and `bash`.
- Child subagents cannot spawn subagents.
- Child subagents inherit stricter read-only policy.

### Footer/view/layout

- Footer mode indicator renders correctly.
- If a Plan Mode banner is added, layout accounts for it.
- There are no viewport overlaps or regressions with activity line, copy-mode banner, editor scroll hint, or Ctrl+C notice.

## Validation commands

Fast iteration:

```sh
go test ./internal/tools
go test ./internal/agent
go test ./internal/tui
go test ./internal/tui/editor
```

Full validation:

```sh
go test ./...
make all
```

## Recommended MVP scope

Implemented:

1. Registry permission mode with runtime denial.
2. Agent mode with Plan Mode prompt overlay.
3. `/plan`, `/act`, `/approve`, `/cancel-plan`.
4. Footer indicator.
5. Block shell shortcuts in Plan Mode.
6. Tests.
7. Docs.

Deferred from MVP:

1. Plan Mode banner. The MVP uses a footer indicator only.
2. First-class `exploration` and `validator` subagent types.
3. Automatic Plan Mode exploration orchestration.
4. Validator subagent before showing plans.
5. MCP allowlist/read-only classification.
6. Session persistence of plan state.

## Recommended follow-ups

### 1. Manual TUI smoke test before release

Run `rune` interactively and verify:

- `/plan` enters Plan Mode and footer shows `plan`.
- A planning prompt can still use read-only exploration tools.
- `!cmd` and `!!cmd` are blocked with the Plan Mode safety message.
- Requests that would trigger `write`, `edit`, `bash`, or MCP tools are hidden from specs and runtime-denied if called anyway.
- `/cancel-plan` clears pending plan state but stays in Plan Mode.
- `/approve` returns to Act Mode and does not automatically start an implementation turn.
- A follow-up message like “go ahead” can implement in Act Mode.

### 2. Run full repo validation

`go test ./...` passed during implementation. Before merging, also run:

```sh
make all
```

### 3. Add a Plan Mode banner

The footer indicator is intentionally compact. A banner would make Plan Mode harder to miss:

```text
Plan Mode: edits, bash, and MCP tools disabled · /approve or /act to implement
```

If added, update `layout()` row accounting and viewport overlap tests.

### 4. Add MCP capability metadata or an allowlist

Current MVP denies all MCP/external tools in Plan Mode because their capabilities are opaque. A follow-up could support:

- MCP tool metadata declaring read-only vs mutating behavior.
- User/project allowlists for known-safe MCP tools.
- A conservative default-deny fallback when metadata is absent.

### 5. Add first-class planning subagent types

Add specialized types such as:

- `exploration` — read-only codebase discovery.
- `validator` — reviews the drafted plan for gaps, safety issues, and test coverage.

Suggested workflow:

```text
exploration subagents → draft plan → validator subagent → revise plan → present to user
```

### 6. Persist mode state if desired

The MVP treats Plan Mode as interactive UI state, not durable session state. Consider persisting mode/pending-plan state only if users expect Plan Mode to survive reload/resume.

### 7. Architecture docs

If the permission model expands, document it in `docs/architecture.md`, including:

- Tool spec filtering vs runtime denial.
- Default-deny Plan Mode policy.
- MCP capability classification.
- Subagent read-only inheritance.
