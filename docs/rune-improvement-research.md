# Rune Improvement Research

Research accessed: 2026-06-18  
Last updated: 2026-06-18

This document summarizes current leading coding-agent harness patterns and what Rune should change or improve to stay competitive. It focuses on agent harness/product capabilities rather than model benchmarks.

## Current Rune baseline

Rune already covers more than a minimal terminal coding agent. From the current docs and README, Rune includes:

- Local read/write/edit/bash tools.
- Local file search/listing.
- Tree-sitter code indexing and repo maps.
- Read-only git/GitHub inspection tools.
- Branching sessions and `/compact`.
- Safe `/plan` mode with mutating tools and arbitrary bash disabled.
- MCP support with Plan Mode/read-only allowlists.
- Built-in and custom subagents.
- Markdown skills.
- Optional web search/fetch tools.
- Image/PDF attachments.
- Multiple providers, including Codex/OAuth, Groq, Ollama, Runpod, and OpenRouter.

The biggest gaps versus leading harnesses are therefore not basic coding-agent capabilities. The main opportunities are in **trust, safety, reversibility, workflow automation, observability, context/memory UX, and ecosystem polish**.

## Leading harnesses reviewed

### Claude Code / Anthropic Agent SDK

Sources:

- https://code.claude.com/docs/en/agent-sdk/overview
- https://code.claude.com/docs/en/agent-sdk/subagents
- https://code.claude.com/docs/en/agent-sdk/permissions
- https://code.claude.com/docs/en/agent-sdk/file-checkpointing
- https://code.claude.com/docs/en/agent-sdk/tool-search

Observed patterns:

- Agent SDK exposes the same core loop, tools, and context-management concepts as Claude Code.
- Subagents are built around context isolation, specialized instructions, parallelization, and tool restrictions.
- Permissions are multi-layered: hooks, deny rules, ask rules, permission modes, allow rules, and runtime callbacks.
- Permission modes include variants such as accepting edits, not asking, bypassing permissions, and planning.
- Docs surface checkpointing/rewind, tool search, cost tracking, OpenTelemetry, sessions, skills, plugins, and todo lists.

Implication for Rune:

Rune already has Plan Mode and subagents. The major gaps are **permission layering, file checkpointing, tool search, hooks, and observability**.

### OpenAI Codex / Agents SDK / Responses tools

Sources:

- https://developers.openai.com/codex/cli
- https://developers.openai.com/codex/agent-approvals-security
- https://developers.openai.com/api/docs/guides/agents
- https://developers.openai.com/api/docs/guides/tools

Observed patterns:

- Codex product docs expose sandboxing, approvals/security, `AGENTS.md`, memories, subagents, hooks, MCP, plugins, skills, automations, GitHub/Slack/Linear integrations, worktrees, in-app browser, computer use, and auto-review.
- Approval/security docs describe OS-enforced sandboxing and policies such as `read-only`, `workspace-write`, and `danger-full-access`.
- `workspace-write` commonly disables network by default unless explicitly configured.
- OpenAI Agents/Responses docs emphasize orchestration, guardrails, approvals, evals, MCP/connectors, web search, shell, computer use, file search, tool search, code interpreter, background mode, compaction, and prompt caching.

Implication for Rune:

OpenAI is productizing **sandbox profiles, approval policies, managed configuration, hooks, memory, automations, integrations, and eval loops** as first-class harness concerns.

### Cursor

Sources:

- https://cursor.com/docs
- https://cursor.com/changelog/1-0

Observed patterns:

- Cursor docs cover Agent, Rules, MCP, Skills, and CLI.
- Cursor 1.0 introduced or highlighted Bugbot PR review, Background Agent, Memories beta, one-click MCP plus OAuth, Jupyter notebook agent editing, Mermaid/table rendering, usage analytics, and parallel tool calls.

Implication for Rune:

Cursor's product edge is **developer workflow integration**: PR review, background agents, memory, one-click MCP, notebook support, analytics, and richer UI rendering.

### Aider

Sources:

- https://aider.chat/docs/repomap.html
- https://aider.chat/docs/config/options.html

Observed patterns:

- Aider's repo map is a concise whole-repo map with important classes/functions/signatures.
- It selects relevant repo-map sections using graph ranking and token-budget controls.
- Options include architect mode, prompt caching, map token controls, auto-commits, dirty commits, diff display, lint/test commands, auto-lint, auto-test, voice, browser/web, watch files, and model metadata.

Implication for Rune:

Rune already has code indexing/repo maps. Aider suggests useful additions around **git commit/undo workflows, lint/test automation, prompt-cache/model metadata controls, and explicit repo-map budgeting UX**.

### Cline / Roo Code

Sources:

- https://docs.cline.bot/cline-overview
- https://roocodeinc.github.io/Roo-Code/features/auto-approving-actions/

Observed patterns:

- Cline can read/write files, run terminal commands, use a browser, and require explicit action approval.
- Cline includes SDK, CLI, Kanban, VS Code/JetBrains apps, and multi-agent parallel workflows with per-card worktrees, auto-commit, and dependency chains.
- Enterprise positioning includes SSO/RBAC and OpenTelemetry integrations.
- Roo Code exposes granular permission tiles: read files, edit files, execute approved commands, browser use, MCP servers, mode switching, subtasks, and follow-up questions.
- Roo also documents workspace-boundary protection, outside-workspace flags, protected-file controls, MCP per-tool “always allow,” command allow/deny prefixes, diagnostics delays after writes, codebase indexing, checkpoints, context condensing, modes, worktrees, diagnostics integration, skills, slash commands, `.rooignore`, and task todo lists.

Implication for Rune:

These tools highlight **granular user control and UX affordances**: approval categories, command allowlists/denylists, workspace boundaries, checkpoints, worktrees, diagnostics, and multi-agent task boards.

### Goose

Source:

- https://goose-docs.ai

Observed patterns:

- Goose positions itself as Desktop + CLI + API.
- It advertises 70+ MCP extensions, 15+ providers, ACP support, recipes as portable YAML workflows, MCP Apps with interactive UIs, subagents, prompt-injection detection, tool permission controls, sandbox mode, and an adversary reviewer.

Implication for Rune:

Goose's edge is an **open plugin/workflow ecosystem**: MCP extension catalog, recipes, ACP interoperability, security review, and interactive extension UIs.

### OpenHands / SWE-agent

Sources:

- https://docs.openhands.dev/openhands/usage/run-openhands/github-action
- https://docs.openhands.dev/openhands/usage/use-cases/code-review
- https://openreview.net/forum?id=OJd3ayDDoF
- https://swe-agent.com/latest/installation/changelog/

Observed patterns:

- OpenHands GitHub Actions can run issue/PR automation triggered by labels or comments.
- OpenHands code-review workflows can post inline PR comments.
- OpenHands agents write code, use a command line, browse the web, interact with sandboxed execution environments, and support evaluation benchmarks.
- SWE-agent v1.0 added SWE-ReX for fast parallel code execution, local/cloud execution, retry mechanisms, flexible tool bundles, LiteLLM model support, and trajectory inspection.
- SWE-agent v1.1.0 added trajectory-format changes and SWE-smith/multimodal/multilingual compatibility.

Implication for Rune:

Academic/open harnesses emphasize **sandbox runtimes, reproducible trajectories, batch/eval execution, GitHub issue/PR automation, and trajectory inspection**.

### Other relevant harnesses from earlier research

Sources:

- OpenCode agents docs: https://opencode.ai/docs/agents/
- Gemini CLI docs: https://developers.google.com/gemini-code-assist/docs/gemini-cli
- Amp manual: https://ampcode.com/manual

Observed patterns:

- OpenCode highlights primary agents, subagents, per-tool permissions, and LSP/MCP/skill permissions.
- Gemini CLI emphasizes a ReAct loop, built-in tools, local/remote MCP servers, web search/fetch, and IDE agent mode.
- Amp emphasizes modes, `AGENTS.md`, subagents, oracle/librarian agents, skills, MCP, plugins, code review, thread sharing, and JSON streaming.

Implication for Rune:

These reinforce the same direction: **agent modes, LSP/code intelligence, review workflows, JSON automation output, IDE integration, and shareable sessions**.

## Highest-priority recommendations

### 1. Add file checkpoints, rewind, and undo

This is the highest-trust improvement.

Rune has branching sessions, but no documented first-class file-level checkpoint/rewind system. Leading tools increasingly treat checkpointing, undo, or git-backed recovery as table stakes.

Recommended shape:

- Create a checkpoint before each mutating turn or mutating tool batch.
- Add commands:
  - `/checkpoint`
  - `/checkpoints`
  - `/undo`
  - `/rewind`
  - `/rewind-files`
- Show changed files and diff before rewind.
- Use git when available.
- Fall back to content snapshots for untracked or non-git repos.
- Preserve user work by separating pre-existing dirty state from agent edits.
- Track created/deleted files as well as modified files.

Why it matters:

Users will let the agent do more if they can easily recover from bad edits.

### 2. Replace binary Plan/Act safety with named permission profiles

Rune's Plan Mode is strong, but leading tools expose more granular policy than Plan vs Act.

Recommended profiles:

- `read-only`: no writes or mutating bash.
- `ask`: ask before edits, bash, and mutating MCP tools.
- `accept-edits`: auto-apply file edits, ask for bash/network.
- `workspace-write`: allow edits only inside the workspace; no network by default.
- `full` or `danger-full-access`: explicit risky mode.

Recommended policy controls:

- Per-tool allow/ask/deny.
- Bash command prefix allow/deny.
- Outside-workspace read/write controls.
- Network controls for bash separately from `web_fetch`.
- Per-MCP tool “always allow” with read/write side-effect hints.
- Separate policies for main agents, subagents, background agents, and headless/CI mode.

Why it matters:

This enables safer autonomy without forcing users into either fully manual Plan Mode or fully permissive Act Mode.

### 3. Add OS/container sandboxing for bash and autonomous runs

Tool-registry enforcement is useful, but mature autonomous agents need OS-level or container-level boundaries.

Recommended implementation:

- macOS: explore `sandbox-exec`, with clear documentation if limitations apply.
- Linux: support bubblewrap, namespaces, or Docker fallback.
- Default sandbox profile:
  - workspace read/write
  - no outside-workspace writes
  - network disabled unless explicitly approved
- Separate sandbox profiles for:
  - Plan Mode
  - Act Mode
  - subagents
  - background tasks
  - CI/headless mode
- Use sandboxed worktrees for background/headless tasks where possible.

Why it matters:

Sandboxing is required for higher-confidence autonomous execution, especially if Rune runs tests, package managers, scripts, or third-party MCP tools.

### 4. Add git-native implementation workflows

Rune currently has read-only git/GitHub inspection. Leading harnesses go further with commits, PRs, PR reviews, and issue automation.

Recommended commands/workflows:

- `/commit`: generate commit message, show diff, ask approval.
- `/pr`: create or update a pull request after approval.
- `/review`: review local `git diff` or staged changes.
- `/review-pr`: inspect a PR diff and produce review comments.
- `rune fix-issue <url>`: headless issue resolver.
- GitHub Action mode triggered by labels or comments.
- Optional agent-authored commit trailers.

Recommended safeguards:

- Do not commit, push, or post comments without approval by default.
- Allow autonomous CI behavior only with explicit config.
- Keep mutation/posting policy separate from read-only GitHub inspection.

Why it matters:

This turns Rune from a terminal assistant into a development workflow agent.

### 5. Build structured traces, replay, evals, and observability

Leading harnesses increasingly rely on trajectories, evals, and telemetry to improve systematically.

Recommended additions:

- JSONL trace export for every turn/tool call/model response.
- Include structured events for:
  - assistant text
  - model request/response metadata
  - tool start/end
  - tool errors
  - approvals
  - denials
  - retries
  - sandbox denials
  - file changes
  - token/cost/time stats
- Add commands:
  - `rune trace inspect`
  - `rune trace replay`
  - `rune eval`
- Optional OpenTelemetry export.
- TUI-visible token/cost/time stats.
- Replay/inspect UI for trajectories.

Why it matters:

This lets Rune improve by measurement rather than anecdote and makes failed agent runs debuggable.

## Strong next-tier recommendations

### 6. Improve context management and memory UX

Rune's repo map/code index is a strength. The next improvement is making context visible, controllable, and reusable.

Recommended additions:

- Context budget/status display in the TUI.
- Show which files/symbols are currently in context.
- Repo-map token budget controls, similar in spirit to Aider's map-token settings.
- Relevance explanations for selected files/symbols.
- Cached repo maps and symbol graphs across sessions.
- Opt-in memory commands:
  - `/memory`
  - `/memories`
  - `/remember`
  - `/forget`
- User-editable and auditable memory files.
- Never silently store secrets.
- Dynamic tool exposure or “tool search” when MCP/tool count grows.

### 7. Expand subagents into orchestration

Rune already supports built-in and custom subagents. The next step is managed orchestration.

Recommended additions:

- Parallel subagent execution UI with progress and cancellation.
- Parent/child provenance IDs in session traces.
- Optional per-subagent worktrees.
- Background subagent sessions that can be resumed.
- Automatic subagent selection based on descriptions.
- A task board or queue for multiple independent tasks.
- Dependency chains between subagent tasks.

### 8. Add hooks and managed policy extension points

Hooks are now common across leading harnesses because they provide deterministic policy and automation outside the model prompt.

Suggested hook events:

- `turn_start`
- `turn_end`
- `before_tool_call`
- `after_tool_call`
- `before_edit`
- `after_edit`
- `before_bash`
- `after_bash`
- `plan_approved`
- `session_compact`
- `checkpoint_created`

Use cases:

- Run formatters/tests after edits.
- Block risky commands.
- Enforce project or enterprise policy.
- Emit telemetry.
- Auto-create checkpoints.
- Append project-specific context.

### 9. Improve MCP and plugin ecosystem polish

Rune has MCP support, but leading tools make MCP discoverable, installable, and governable.

Recommended additions:

- `rune mcp marketplace` or curated registry.
- One-command or deeplink MCP install.
- OAuth credential lifecycle for MCP servers.
- Per-tool permission UI.
- MCP health diagnostics.
- Workspace MCP trust prompts.
- Tool include/exclude filters.
- Side-effect hints in MCP tool listings.
- Portable recipes combining:
  - prompt
  - tools
  - model
  - permissions
  - env requirements
  - validation commands
- Optional ACP/editor integration mode.

### 10. Add optional browser/computer-use tools later

This is useful but lower priority than safety, permissions, sandboxing, evals, and git workflows.

Potential additions:

- Playwright browser automation tool.
- Screenshot capture.
- DOM inspection.
- Local app preview capture.
- Approval-gated browser actions.
- Browser/network controls integrated with permission profiles.
- Optional Jupyter notebook editing if a clear use case emerges.

## Additional ideas from earlier research

### Auto-verify loop

Aider strongly emphasizes auto-lint/auto-test. Rune should support explicit verification commands.

MVP:

- Project/user config for verify commands such as `go test ./...`, `npm test`, or `make test`.
- `/verify` command.
- Optional “run after edits” behavior gated by approval/profile.
- Agent self-fix loop when verification fails.

### LSP/code intelligence tools

Tree-sitter code indexing is useful, but LSP can improve correctness after edits.

MVP tools:

- diagnostics
- definitions
- references
- rename support
- workspace symbols

### JSONL automation mode

For CI, editors, and dashboards, Rune should expose structured non-interactive output.

Example:

```sh
rune --prompt "fix failing tests" --json
```

Events should include assistant text, tool start/end, file changed, approval needed, errors, and final summary.

### IDE bridge

An IDE bridge could provide:

- active file
- selected text
- open tabs
- diagnostics
- apply-edit through IDE undo APIs
- reveal file/line

### Session export/sharing

Rune already has local sessions and branching. Add:

```sh
rune session export --html
rune session export --json
```

Later, consider encrypted sync, share links, and redaction controls.

### Agent modes

Add provider/model-independent mode presets:

- `rush`: fast/cheap, limited tools, low max steps.
- `smart`: balanced default.
- `deep`: stronger model, more planning, validator subagent.
- `review`: read-only and diff-focused.

## Recommended roadmap

### Top five investments

If Rune only does five things next, prioritize:

1. **Checkpoints/rewind** — immediate trust and safety win.
2. **Permission profiles and approval UI** — moves beyond binary Plan/Act.
3. **Sandboxed bash/workspace-write mode** — required for safer autonomy.
4. **Structured traces and eval runner** — lets Rune improve measurably.
5. **Git/PR workflows** — turns Rune into a development workflow agent.

### Suggested implementation sequence

1. **Checkpoints + `/undo`**
   - Verify with tests that modified, created, deleted, and untracked files restore correctly.
2. **Permission profiles**
   - Extend the existing tool registry permission system before adding OS sandboxing.
3. **Sandboxed bash**
   - Start with workspace-write/no-network on the most supportable platform, then expand.
4. **Trace JSONL**
   - Save events first; build replay/eval tooling after the event schema is stable.
5. **Git workflows**
   - Add `/review`, then `/commit`, then `/pr`/GitHub Action automation.
6. **Context/memory UX**
   - Make repo-map selection visible and add opt-in memory.
7. **Hooks**
   - Add stable hook events around tool calls, edits, bash, checkpoints, and compaction.
8. **MCP ecosystem polish**
   - Add registry/install/OAuth/permission improvements.
9. **Subagent orchestration**
   - Add worktrees, provenance, task queues, and background resumability.
10. **Browser/computer-use**
   - Add after the safety and permission model is mature.

## Key takeaway

The strongest agent harnesses are converging on **safer autonomy**. They combine powerful models with guardrails and feedback loops: checkpoints, permissions, sandboxing, hooks, verification, review, memory, isolated/background execution, structured traces, and focused subagents.

Rune already has a solid agent core. The next improvements should make longer autonomous work easier to **trust, inspect, recover from, and measure**.
