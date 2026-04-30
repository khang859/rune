# Subagents and parallel agent orchestration plan

This document captures the planned implementation for adding Claude Code-style subagents and parallel task orchestration to rune.

## Goals

Add first-class support for the main agent loop to start isolated subagents that can run concurrently, report progress, and return compact structured results.

The expected agent workflow is:

1. The main loop identifies work that can be delegated.
2. The main loop starts one or more subagents with narrow prompts and isolated context.
3. Subagents run independently, optionally in parallel.
4. The main loop lists task status or retrieves completed results.
5. The main loop synthesizes results and decides the next action.

Primary benefits:

- Parallel investigation or implementation.
- Context isolation per delegated task.
- Specialized instructions and tool policies per subagent.
- Lower main-context pollution by returning summaries instead of full transcripts.
- Foundation for future agent-team style coordination.

## References

This design is informed by:

- Claude Code subagents docs: https://code.claude.com/docs/en/sub-agents
- Claude Agent SDK subagents docs: https://code.claude.com/docs/en/agent-sdk/subagents
- Anthropic engineering article on the Claude Agent SDK: https://claude.com/blog/building-agents-with-the-claude-agent-sdk
- Microsoft AI agent orchestration patterns: https://learn.microsoft.com/en-us/azure/architecture/ai-ml/guide/ai-agent-design-patterns
- Addy Osmani, "The Code Agent Orchestra": https://addyosmani.com/blog/code-agent-orchestra/

## Current implementation status

Implemented in the first MVP pass:

- `SubagentSupervisor` in `internal/agent/subagents.go`.
- Subagent task lifecycle with `pending`, `running`, `completed`, `failed`, and `cancelled` states.
- Bounded concurrency with default `max_concurrent = 4`.
- Default subagent timeout of `10m`.
- In-memory task registry and recent task listing.
- Isolated child agent/session execution per subagent.
- Read-only child tool registry via `Registry.CloneReadOnly()`.
- Parent-facing subagent tools:
  - `spawn_subagent`
  - `list_subagents`
  - `get_subagent_result`
  - `cancel_subagent`
- Subagent tools registered for both interactive mode and one-shot `rune prompt` mode.
- Initial subagent type support through `agent_type`.
- Shipped `general` as the only built-in subagent type for now.
- Unknown `agent_type` values are rejected with a clear error.
- Subagents cannot recursively spawn subagents in v1 because subagent tools are stripped from the child registry.
- Write-capable tools are stripped from child registries:
  - `write`
  - `edit`
  - `bash`
- Tests cover spawning, default `general` type, unknown type rejection, tool registration, child read-only registry behavior, cancellation of pending tasks, lifecycle event publishing, and TUI rendering/tracking of subagent events.

Implemented in the second MVP pass:

- Push-style subagent lifecycle events through `SubagentSupervisor.Subscribe(ctx)`.
- Lifecycle notifications for queued/pending, running, completed, failed, and cancelled task states.
- Bubble Tea/TUI subscription to subagent events independent of the main agent streaming turn.
- TUI message rendering for subagent status updates:
  - queued
  - working
  - completed with summary
  - failed/cancelled
- Activity-line support for background subagents, for example `◐ 1 subagent working…`.
- Subagent completion is posted to the visible TUI message view without requiring the main agent to poll `get_subagent_result` for user-visible progress.
- `get_subagent_result` remains available for model/tool use and explicit result lookup.
- Completed subagent summaries are injected once into future main-agent context/session history before provider requests, avoiding asynchronous mutation while a turn/tool-call sequence is active.

Implemented in the third MVP pass:

- Settings-backed subagent configuration:
  - `enabled`
  - `max_concurrent`
  - `default_timeout_secs`
  - `max_completed_retain`
- Interactive mode and one-shot `rune prompt` mode construct subagent supervisors from settings.
- When subagents are disabled, parent-facing subagent tools return clear disabled errors instead of starting tasks.

Implemented in the fourth MVP pass:

- Dependency-aware subagent orchestration.
- `dependencies` support on `spawn_subagent`.
- `blocked` task state for tasks waiting on dependencies.
- Automatic unblocking when dependencies complete.
- Dependent task failure when dependencies fail or are cancelled.
- Dependency summaries injected into downstream subagent prompts.
- Cancellation support for blocked tasks.

Implemented in the fifth MVP pass:

- Durable subagent task metadata in session files.
- Save/load of subagent task ID, name, type, status, dependencies, timestamps, summary, and error.
- Loaded session history is seeded into the subagent supervisor and appears in `list_subagents` / `/subagents`.
- Stale non-terminal tasks from prior processes are restored as cancelled with a session-restore error.
- Full prompts/transcripts are intentionally not persisted yet.

Implemented in the sixth MVP pass:

- Custom user-defined subagent type registry loaded from `~/.rune/agents/*.md` and `./.rune/agents/*.md`.
- Markdown/frontmatter custom agent definitions with `name`, `description`, `model`, `timeout_secs`, and `tools`.
- Per-subagent model selection on the current provider.
- Per-subagent default timeout, overridable by `spawn_subagent.timeout_secs`.
- `tools: readonly` and `tools: full` custom subagent modes.
- Full-tool subagents receive an act-mode cloned registry, while subagent-management tools remain stripped to prevent recursive subagents.

Not yet implemented:

- Subagent transcript/artifact persistence beyond durable task metadata.
- Per-subagent model selection.
- Per-subagent provider selection. Custom subagents support model selection on the current provider.
- File scopes and file locks.
- Patch/worktree-based implementation subagents.
- Peer-to-peer subagent messaging or full agent-team mode.

## Non-goals for v1

- Full autonomous agent teams.
- Peer-to-peer messaging between subagents.
- Recursive subagent spawning.
- Shared mutable task board with automatic dependency resolution.
- Unbounded parallelism.
- Letting multiple agents edit overlapping files without coordination.
- Full browser or terminal multiplexing UI.
- Automatically merging subagent code changes without review.

## Design principles

### Isolate context

A subagent should not inherit the full main-agent transcript by default. It should receive:

- A focused task brief.
- Relevant project instructions.
- Explicit constraints.
- Selected context or file references.
- Tool permissions.
- Expected result format.

The parent should receive a compact structured result, not the full child transcript.

### Prefer read-only subagents first

The initial version should focus on research, codebase exploration, analysis, review, and planning. These are high-value and low-risk because they avoid concurrent file edits.

Implementation subagents can be added later behind stricter controls.

### Bound concurrency

The system should prevent runaway task creation and token usage.

Suggested defaults:

```toml
[subagents]
enabled = true
max_concurrent = 4
max_depth = 1
default_timeout_secs = 600
max_tokens_per_subagent = 80000
```

`max_depth = 1` means the main agent can spawn subagents, but subagents cannot spawn additional subagents in v1.

### Make task state explicit

Every subagent should have a task record with a clear lifecycle.

Implemented lifecycle:

```text
pending -> running -> completed
pending -> running -> failed
pending -> running -> cancelled
pending -> cancelled
blocked -> pending -> running -> completed
blocked -> failed
blocked -> cancelled
```

### Return structured results

Subagents should end with a predictable result contract:

```markdown
## Summary
...

## Findings
- ...

## Files inspected
- ...

## Files changed
- ...

## Risks
- ...

## Recommended next steps
- ...
```

For API/tool responses, this should be normalized into a machine-readable result object.

## When to use parallel subagents

Parallel dispatch is appropriate when all of these are true:

- Tasks are independent.
- Tasks have clear boundaries.
- Tasks do not need shared mutable state.
- Tasks do not edit the same files.
- Results can be synthesized by the parent.

Good examples:

- Explore parser, config, and TUI subsystems in parallel.
- Ask separate agents to review correctness, security, and tests.
- Research multiple implementation options concurrently.
- Run one agent to inspect code while another summarizes docs.

Sequential dispatch is better when:

- Task B depends on Task A.
- Multiple tasks touch the same files.
- The architecture is not yet decided.
- The first step is discovery and later steps depend on findings.

## Architecture overview

Add an agent supervision layer between the main loop and subagent execution.

```text
main agent loop
  ├── spawn_subagent tool
  ├── list_subagents tool
  ├── get_subagent_result tool
  └── cancel_subagent tool
        │
        ▼
SubagentSupervisor / TaskManager
  ├── task registry
  ├── bounded worker pool
  ├── cancellation and timeouts
  ├── durable task metadata persistence
  ├── transcript/artifact persistence (planned)
  └── lifecycle progress events
        │
        ├── subagent A isolated context
        ├── subagent B isolated context
        └── subagent C isolated context
```

The `SubagentSupervisor` owns subagent lifecycle, concurrency limits, cancellation, durable task metadata, and result storage.

## Core data model

Illustrative shape:

```go
type SubagentTask struct {
    ID          string
    Name        string
    AgentType   string
    Prompt      string
    Status      SubagentStatus
    CreatedAt   time.Time
    StartedAt   *time.Time
    CompletedAt *time.Time
    Summary     string
    Error       string
}
```

Future expanded shape:

```go
type SubagentTask struct {
    ID             string
    ParentTurnID   string
    Name           string
    Prompt         string
    Status         TaskStatus
    AgentKind      string
    CreatedAt      time.Time
    StartedAt      *time.Time
    CompletedAt    *time.Time
    Dependencies   []string
    AllowedTools   []string
    WorkingDir     string
    FileScope      []string
    Result         *SubagentResult
    Error          *SubagentError
}
```

Task statuses:

```go
type SubagentStatus string

const (
    SubagentBlocked   SubagentStatus = "blocked"
    SubagentPending   SubagentStatus = "pending"
    SubagentRunning   SubagentStatus = "running"
    SubagentCompleted SubagentStatus = "completed"
    SubagentFailed    SubagentStatus = "failed"
    SubagentCancelled SubagentStatus = "cancelled"
)
```

Result shape:

```go
type SubagentResult struct {
    Summary      string
    Findings     []Finding
    FilesInspected []string
    ChangedFiles []string
    Artifacts    []ArtifactRef
    TranscriptRef string
    TokenUsage   TokenUsage
}
```

## Tool design

### `spawn_subagent`

Purpose: start a specialized subagent with isolated context.

Implemented schema:

```json
{
  "name": "string",
  "prompt": "string",
  "agent_type": "general",
  "background": true,
  "dependencies": ["task_id"],
  "timeout_secs": 600
}
```

Planned future schema additions:

```json
{
  "allowed_tools": ["read", "grep", "web_search"],
  "file_scope": ["internal/agent/**", "docs/**"]
}
```

Implemented v1 behavior:

- `name` and `prompt` are required.
- `agent_type` defaults to `general`.
- Only `general` is accepted currently.
- `background` defaults to `true`.
- Child agents receive a conservative read-only tool registry.
- `timeout_secs` can reduce the default timeout, but cannot raise it above the configured/default cap.
- If concurrency is saturated, the task remains `pending` until capacity is available.
- If dependencies are incomplete, the task remains `blocked` until they complete.
- If a dependency fails or is cancelled, dependent blocked tasks fail with a dependency error.
- Dependency summaries are included in downstream subagent context.

Planned future behavior:

- Configurable allowed tools per subagent.
- Explicit file scopes.
- User-defined subagent types.

Example response:

```json
{
  "task_id": "subagent_123",
  "status": "running"
}
```

If `background: false`, the tool may wait for completion and return the final result directly, subject to timeout.

### `list_subagents`

Purpose: show active and recent subagent tasks.

Initial schema:

```json
{}
```

Example response:

```json
{
  "tasks": [
    {
      "task_id": "subagent_123",
      "name": "Inspect parser architecture",
      "status": "running",
      "started_at": "...",
      "token_usage": 12000
    }
  ]
}
```

### `get_subagent_result`

Purpose: retrieve result or current status for a task.

Initial schema:

```json
{
  "task_id": "subagent_123"
}
```

Example completed response:

```json
{
  "task_id": "subagent_123",
  "status": "completed",
  "summary": "...",
  "findings": [],
  "files_inspected": [],
  "changed_files": [],
  "artifacts": []
}
```

If still running:

```json
{
  "task_id": "subagent_123",
  "status": "running",
  "progress": "Inspecting internal/agent loop"
}
```

### `cancel_subagent`

Purpose: cancel a running, pending, or blocked subagent.

Initial schema:

```json
{
  "task_id": "subagent_123"
}
```

Behavior:

- Cancels the subagent context if running.
- Marks task as `cancelled`.
- Preserves partial transcript if available.

## Execution model

1. Main agent calls `spawn_subagent`.
2. Tool validates config, prompt, dependencies, and tool policy.
3. `AgentSupervisor` creates a task record.
4. If runnable and capacity is available, supervisor starts the subagent in a goroutine.
5. Subagent receives a fresh conversation/context with specialized system instructions.
6. Subagent runs using a read-only cloned tool registry.
7. Supervisor stores final summary/error in memory and mirrors durable task metadata into the session.
8. Main agent calls `get_subagent_result` to retrieve status or result.

Planned future additions:

- Transcript/artifact persistence for child sessions and outputs.
- More granular progress events beyond lifecycle transitions.

## Events

Lifecycle events are implemented and emitted to subscribers, including the TUI. Finer-grained progress events may be added later.

Implemented lifecycle events:

```text
subagent_started
subagent_progress
subagent_completed
subagent_failed
subagent_cancelled
subagent_blocked
subagent_unblocked
```

These events allow the UI to show background activity without forcing the main loop to block.

## Context construction

Each subagent context should include:

- Base rune system/developer instructions.
- Project instructions from relevant files, if applicable.
- Subagent-specific role/instructions.
- Task prompt.
- Tool policy.
- File scope.
- Required result format.

It should not include the full parent transcript by default.

Optional context inputs for later versions:

- Explicit parent-selected messages.
- Attached file excerpts.
- Search results.
- Prior subagent results.
- Artifacts from dependency tasks.

## Tool permissions

V1 defaults to a read-only cloned registry.

The parent registry is cloned, then these tools are stripped from child subagents:

- `write`
- `edit`
- `bash`
- `spawn_subagent`
- `list_subagents`
- `get_subagent_result`
- `cancel_subagent`

This keeps read/web tools available when present, while preventing direct mutation and recursive subagent spawning.

Write-capable tools are disabled by default for subagents in v1.

Later implementation subagents may allow:

- `write`
- `edit`
- `bash`

But only with explicit user/main-agent intent, file scopes, and safety controls.

## File editing strategy for later phases

Concurrent editing is the highest-risk part of multi-agent coding. Use progressive controls.

### Option A: file locks

Subagents must declare file scope. The supervisor prevents overlapping write ownership.

Example:

```text
subagent A owns: internal/agent/**
subagent B owns: docs/**
```

If another subagent tries to edit an owned file, the write is denied or queued.

### Option B: patch-only output

Subagents do not directly edit the working tree. They produce patch artifacts, and the parent applies selected patches.

### Option C: git worktrees

Each implementation subagent gets its own git worktree and branch. The parent reviews and merges results.

This is the safest option for larger implementation tasks, but it is more complex.

## Persistence

Task state should be persisted with the session when possible:

- task metadata,
- status,
- result summary,
- transcript references,
- artifact references,
- token usage,
- error state.

Long transcripts should live as referenced artifacts rather than bloating the main session JSON.

## Cancellation and timeout behavior

Each subagent should have its own `context.Context` derived from the parent turn/session context.

Cancellation sources:

- user cancels current turn,
- main agent calls `cancel_subagent`,
- timeout expires,
- session exits,
- supervisor shutdown.

Timeouts should produce a structured error and preserve partial transcript.

## Suggested implementation phases

### Phase 1: read-only parallel subagents — implemented

Implemented:

- `SubagentSupervisor` / task manager.
- `spawn_subagent`.
- `list_subagents`.
- `get_subagent_result`.
- `cancel_subagent`.
- Bounded concurrency.
- Isolated subagent context/session.
- Read-only cloned tool policy.
- `general` subagent type.
- Interactive and one-shot prompt registration.
- Cancellation and timeout support.
- Tests for core behavior.

Still to improve within Phase 1:

- More structured result parsing beyond storing the final assistant summary string.
- Session/transcript artifact references.

This enables safe parallel research and codebase exploration.

### Phase 2: background task UX — mostly implemented

Implemented:

- Lifecycle progress events.
- TUI display for active subagents.
- Result notifications with summaries.
- Background activity indicator.
- Completed summary injection into future main-agent context.

Still to improve within Phase 2:

- Durable task history in session view backed by persisted task metadata.
- More granular progress events beyond lifecycle transitions.

### Phase 3: dependency-aware orchestration — implemented

Implemented:

- `dependencies` field support.
- `blocked` task state.
- Automatic unblock when dependencies complete.
- Passing dependency summaries into downstream subagent context.
- Failing dependent blocked tasks when dependencies fail or are cancelled.
- Cancellation of blocked tasks.

### Phase 4: durable subagent task history/session persistence — partially implemented

Implemented:

- Session-level persisted subagent task metadata.
- Session save/load round-trip for subagent task history.
- Supervisor hydration from loaded session metadata.
- Stale pending/running/blocked tasks restored as cancelled because no child process survives across process restarts.

Still to improve:

- Transcript/artifact references for child sessions.
- More detailed result objects beyond the final summary string.
- Optional TUI affordances for browsing persisted subagent history separately from active tasks.

### Phase 5: controlled implementation subagents

Add:

- Write tool permissions behind explicit policy.
- File scopes.
- File locks or patch-only mode.
- Diff review before final application.

### Phase 6: agent-team mode

Add higher-level coordination primitives:

- Shared task board.
- Team lead role.
- Peer or mediated messaging.
- File ownership visualization.
- Dependency graph UI.
- Optional git worktree execution.

## Open questions

- Should subagents run through the same provider/model as the main agent, or allow per-subagent model selection?
- Should background subagents survive across turns or only within a single active turn?
- How should subagent events appear in the TUI without cluttering the main conversation?
- Should completed results be automatically injected into the main context or only fetched on demand?
- What is the right default tool set for read-only exploration in rune?
- Should implementation subagents produce patches first, or edit directly under file locks?
- How much of the parent context should be selectively inherited?

## MVP status

The implementation remains intentionally conservative:

1. Read-only subagents only.
2. Bounded parallelism.
3. No recursive spawning.
4. No direct file edits.
5. `general` subagent type.
6. Explicit task listing and retrieval.
7. Cancellation and timeout support.
8. Enabled in both interactive and one-shot prompt modes.
9. Settings-backed configuration.
10. TUI visibility for active/completed subagents.
11. Dependency-aware blocked tasks.
12. Durable session-backed subagent task metadata and restore of stale non-terminal tasks as cancelled.

This captures the highest-value part of Claude Code-style subagents: isolated parallel work that returns concise results to the main loop, while avoiding the hardest risks of concurrent code mutation.

Next recommended step: add transcript/artifact references for child sessions, then proceed to controlled implementation subagents with explicit file scopes and patch/worktree safety.
