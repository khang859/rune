# Feature Dev Command Plan

## Goal

Add a self-contained `/feature-dev` workflow to Rune, adapted from Claude Code's `feature-dev` plugin, with three dedicated built-in subagents in addition to Rune's existing built-ins.

Existing built-in subagents:

- `general`
- `exploration`
- `validator`

New built-in subagents to add:

- `code-explorer`
- `code-architect`
- `code-reviewer`

## Source Material Reviewed

Claude Code plugin references:

- `plugins/feature-dev/commands/feature-dev.md`
- `plugins/feature-dev/agents/code-explorer.md`
- `plugins/feature-dev/agents/code-architect.md`
- `plugins/feature-dev/agents/code-reviewer.md`
- `plugins/feature-dev/README.md`

Rune implementation areas inspected:

- `internal/tui/root.go` — built-in slash commands and command handling
- `internal/agent/subagents.go` — built-in subagent types and subagent system prompts
- `internal/tools/subagents.go` — subagent tool descriptions and tool schemas
- `internal/agentdef/agentdef.go` — custom subagent loading and reserved-name behavior
- `cmd/rune/interactive.go` and `cmd/rune/prompt.go` — loading custom subagents into agent config
- `docs/skills.md` — existing skill workflow documentation

## Claude Agent Contents Summary

### `code-explorer`

Purpose: deeply analyze existing codebase features by tracing execution paths, mapping architecture layers, understanding patterns and abstractions, and documenting dependencies.

Key behavior:

- Find entry points such as APIs, UI components, and CLI commands.
- Locate core implementation files.
- Map feature boundaries and configuration.
- Follow call chains from entry to output.
- Trace data transformations and state changes.
- Identify dependencies, integrations, side effects, error handling, edge cases, and performance considerations.
- Return concrete file paths and line numbers.
- Include a list of essential files to read.

Rune adaptation: implement this as a dedicated built-in `code-explorer` subagent with read-only tools by default. Its prompt should emphasize repository evidence, file:line references, and a concise list of essential files for the parent agent to inspect.

### `code-architect`

Purpose: design feature architectures by analyzing existing codebase patterns and conventions, then producing a comprehensive implementation blueprint.

Key behavior:

- Extract existing patterns, conventions, module boundaries, abstraction layers, and project guidelines.
- Find similar features to understand established approaches.
- Design a complete feature architecture.
- Make confident architectural choices.
- Specify every file to create or modify.
- Define component responsibilities, integration points, data flow, and build sequence.
- Include error handling, state management, testing, performance, and security considerations.

Rune adaptation: implement this as a dedicated built-in `code-architect` subagent. In the `/feature-dev` command, it can be invoked with different focuses when multiple approaches are useful, but each architect response should still be concrete and actionable.

### `code-reviewer`

Purpose: review code for bugs, logic errors, security vulnerabilities, code quality issues, and adherence to project conventions using confidence-based filtering.

Key behavior:

- Review unstaged changes from `git diff` by default.
- Check project guideline compliance, usually from `CLAUDE.md` or equivalent.
- Identify real bugs and significant issues only.
- Rate confidence from 0 to 100.
- Report only issues with confidence >= 80.
- Provide file path, line number, explanation, and concrete fix suggestion.
- Group issues by severity.
- If no high-confidence issues exist, confirm the code meets standards.

Rune adaptation: implement this as a dedicated built-in `code-reviewer` subagent. It should prefer high-signal findings and avoid speculative or stylistic nitpicks.

## Implementation Plan

### 1. Add dedicated built-in subagent types

Update `internal/agent/subagents.go`:

- Extend the built-in type list from:

  ```go
  var subagentTypes = []string{"general", "exploration", "validator"}
  ```

  to include:

  ```go
  "code-explorer", "code-architect", "code-reviewer"
  ```

- Ensure `BuiltinSubagentTypeSet()` automatically treats these names as reserved, so user/project custom agents cannot accidentally override them.

### 2. Add specialized instructions for the new subagents

Update `subagentSystemPrompt()` in `internal/agent/subagents.go`:

- Add a `case "code-explorer"` section adapted from Claude's `code-explorer.md`.
- Add a `case "code-architect"` section adapted from Claude's `code-architect.md`.
- Add a `case "code-reviewer"` section adapted from Claude's `code-reviewer.md`.

Prompt adaptation rules:

- Remove Claude-specific front matter and tool names.
- Do not mention `TodoWrite`, `Glob`, `Grep`, `LS`, `NotebookRead`, `KillShell`, or `BashOutput` as required tools.
- Use Rune terminology: read-only tools, subagents, `AGENTS.md`, `CLAUDE.md`, and repository evidence.
- Preserve the important output contracts: file:line references, essential files, implementation map, confidence filtering.
- Keep each subagent's scope narrow and aligned with the parent task.

### 3. Update subagent tool documentation

Update `internal/tools/subagents.go`:

- Change the `spawn_subagent` description to list all six built-in types:
  - `general`
  - `exploration`
  - `validator`
  - `code-explorer`
  - `code-architect`
  - `code-reviewer`

This keeps model-facing tool documentation accurate.

### 4. Add `/feature-dev` as a built-in slash command

Update `internal/tui/root.go`:

- Add `/feature-dev` to `baseSlashCmds`.
- Add command handling in `handleSlashCommand`.

Desired behavior:

- `/feature-dev`
  - Arms the feature-dev workflow prompt.
  - User sends the feature description in the next message.

- `/feature-dev <description>`
  - Starts a turn immediately using the feature-dev workflow prompt plus the supplied description.

The command should be adapted for Rune, not copied verbatim from Claude.

### 5. Make slash-command arguments work for `/feature-dev`

Rune's editor currently treats slash menu selections as exact commands. Supporting `/feature-dev <description>` may require focused argument handling.

Recommended minimal approach:

- Keep slash menu selection behavior unchanged for normal slash commands.
- In `handleSlashCommand`, accept a raw command string and split it into command name + optional argument.
- Ensure `/feature-dev whatever` is recognized as command `/feature-dev` with argument `whatever`.
- Avoid broad editor refactors.

If the existing editor does not pass through slash commands with unmatched arguments, a small editor/root integration change may be needed so exact built-in commands with arguments can submit instead of being treated as normal user text.

### 6. Add Rune-adapted feature-dev prompt

Add a built-in `featureDevPrompt` constant near `skillCreatorPrompt` in `internal/tui/root.go`, or move larger built-in prompts to a dedicated file if that keeps `root.go` readable.

The prompt should instruct the agent to follow these phases:

1. Discovery
   - Understand the requested feature.
   - If unclear, ask exactly one clarifying question at a time.
   - Summarize the goal and constraints.

2. Codebase exploration
   - Spawn 2-3 `code-explorer` subagents with different focuses.
   - Ask each to return essential files to read.
   - Do not immediately duplicate delegated work unless needed.
   - After subagents complete, read key files before designing.

3. Clarifying questions
   - Identify ambiguity, edge cases, integration points, scope boundaries, compatibility, and validation needs.
   - Ask specific questions before architecture design when blocking decisions remain.

4. Architecture design
   - Use `code-architect` subagents for architecture options or focused design analysis.
   - Present approaches and tradeoffs when useful.
   - Make a clear recommendation.
   - Ask the user to approve an implementation plan before editing.

5. Implementation
   - Do not implement without explicit approval.
   - Keep changes surgical.
   - Follow project style.
   - Add or update tests when practical.

6. Quality review
   - Spawn `code-reviewer` subagents after implementation for focused reviews.
   - Ask reviewers to report only high-confidence, actionable issues.
   - Consolidate findings and fix approved issues.

7. Summary
   - Summarize what changed, files modified, validation run, and remaining risks.

Rune-specific constraints to include:

- Respect Plan Mode / Act Mode semantics.
- Do not use mutating tools before approval.
- Prefer repository evidence over assumptions.
- Preserve user work and unrelated changes.
- Do not rely on `TodoWrite`; maintain progress in concise status updates instead.

### 7. Tests

Add or update tests covering:

- Built-in subagent type list includes:
  - `general`
  - `exploration`
  - `validator`
  - `code-explorer`
  - `code-architect`
  - `code-reviewer`

- `BuiltinSubagentTypeSet()` reserves the new names.

- `subagentSystemPrompt()` includes specialized instructions for:
  - `code-explorer`
  - `code-architect`
  - `code-reviewer`

- `spawn_subagent` tool description mentions the new built-ins.

- `baseSlashCmds` includes `/feature-dev`.

- `/feature-dev` with no argument arms the workflow prompt.

- `/feature-dev <description>` starts or queues a turn with the feature-dev prompt and description.

### 8. Documentation

Update docs after implementation:

- Add `/feature-dev` to the README slash command list.
- Consider adding a short `docs/feature-dev.md` usage page.
- Mention the three new specialized subagents in `docs/subagents.md` if that document lists built-ins.

## Recommended Behavior Summary

`/feature-dev` should feel like a contained guided workflow for non-trivial feature development:

- It brings its own specialist agents.
- It asks questions before designing.
- It explores before changing code.
- It presents an implementation plan and waits for approval.
- It validates and reviews after implementation.
- It stays compatible with Rune's existing tool permissions and subagent architecture.

## Open Decision

Whether `/feature-dev <description>` must be supported immediately or whether the first version can support only `/feature-dev` followed by a next user message.

Recommendation: support both if the required editor/root change stays small. If argument support becomes invasive, ship `/feature-dev` arm-first behavior initially and add argument support separately.
