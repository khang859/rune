# Rune Improvement Research

Date: 2026-05-23

This note summarizes online research into current terminal coding-agent features and how Rune could improve. It compares Rune with Claude Code, Codex CLI, Aider, OpenCode, Gemini CLI, and Amp.

## Current Rune baseline

Rune already covers many modern coding-agent basics:

- Interactive and one-shot CLI usage.
- Plan Mode with enforced read-only tooling.
- Branching/resumable sessions and compaction.
- Built-in read/write/edit/bash, web, git-inspection, and code-index tools.
- MCP plugin support.
- Skills and custom subagents.
- Guided feature-development workflow.
- Multiple providers, including Codex, Groq, and Ollama.

The strongest opportunities are therefore not just “add more tools,” but improving **safe autonomy**: checkpoints, verification loops, permissions, hooks, richer skills, review workflows, and better automation/IDE integration.

## Research sources

- Claude Code docs — agent loop, tools, memories, MCP, skills, subagents, checkpoints, permissions: https://code.claude.com/docs/en/how-claude-code-works
- Codex CLI docs — MCP, approvals, sandboxing, subagents, web search, cloud tasks: https://developers.openai.com/codex/cli
- Codex hooks docs — lifecycle hooks such as `PreToolUse`, `PostToolUse`, `UserPromptSubmit`, `PreCompact`, and `Stop`: https://developers.openai.com/codex/hooks
- Codex skills docs — packaged task-specific capabilities: https://developers.openai.com/codex/skills
- Aider homepage — repo maps, git integration, image/web page context, voice, auto lint/test: https://aider.chat/
- OpenCode agents docs — primary agents, subagents, per-tool permissions, LSP/MCP/skill permissions: https://opencode.ai/docs/agents/
- Gemini CLI docs — ReAct loop, built-in tools, local/remote MCP servers, web search/fetch, IDE agent mode: https://developers.google.com/gemini-code-assist/docs/gemini-cli
- Amp manual — modes, AGENTS.md, subagents, oracle/librarian, skills, MCP, plugins, code review, thread sharing, JSON streaming: https://ampcode.com/manual

## Highest-leverage improvements

| Priority | Idea | Why it matters | Rune-fit MVP |
|---|---|---|---|
| 1 | Filesystem checkpoints and `/undo` | Users trust autonomous edits more when they can quickly revert agent changes. Claude Code emphasizes checkpoints and permissions as safety primitives. | Before every mutating turn, snapshot `git diff --binary` plus created-file metadata. Add `/checkpoint`, `/undo`, and `/checkpoints`. |
| 2 | Configurable auto-verify loop | Aider strongly markets automatic lint/test after edits and self-fixing failures. | Add project/user config for verify commands such as `make test`, `go test ./...`, or `npm test`. Add `/verify`, and optionally “run after edits” with approval. |
| 3 | Fine-grained permissions | OpenCode supports per-tool `allow` / `ask` / `deny`, glob-scoped permissions, and agent-specific permissions. | Extend Rune’s Plan/Act split into profiles: `readonly`, `ask`, `workspace-write`, and `full-auto`. Add per-tool and path-glob policy in settings. |
| 4 | Hooks / lightweight plugin system | Codex, Claude Code, and Amp expose hooks/plugins for policy, automation, and custom workflows. | Start with command hooks: `pre_tool`, `post_tool`, `user_prompt_submit`, `turn_done`, `pre_compact`. Let hooks block, allow, or append context. |
| 5 | LSP/code intelligence tools | Claude Code and OpenCode both expose code-intelligence/LSP capabilities. | Add tools for diagnostics, definitions, references, and symbols using installed language servers. Rune already has Tree-sitter; LSP would improve correctness after edits. |

## Strong next-tier ideas

### 6. Persistent memory

Claude Code loads project/user memory automatically. Rune has `AGENTS.md`, but not auto-learned memory.

MVP:

- Add opt-in `~/.rune/MEMORY.md` and `.rune/MEMORY.md`.
- Add `/remember` and `/forget`.
- Require explicit user approval before writing memory.
- Load only a bounded prefix, similar to Claude Code’s documented memory limits.

### 7. Skills v2

Rune skills are intentionally simple markdown files. Competitors are moving toward richer skill packages with frontmatter, bundled resources, scripts, and sometimes MCP servers.

MVP:

```text
.rune/skills/deploy/
  SKILL.md
  scripts/check-staging.sh
  mcp.json
```

Features to consider:

- Skill frontmatter: name, description, tools, model, trigger hints.
- On-demand loading of full skill content to reduce context bloat.
- Bundled resource files and scripts.
- Optional skill-local MCP config.

### 8. MCP polish

Rune already supports MCP and Plan Mode allowlists. Amp recommends bundling MCP servers inside skills, filtering exposed tools, and requiring explicit trust for workspace MCP servers.

MVP improvements:

- Remote MCP OAuth flow.
- Per-tool include/exclude filters.
- Workspace MCP trust prompts.
- MCP bundled in skills.
- Better `/mcp` visibility for tool descriptions and side-effect hints.

### 9. Diff review mode

Amp has `amp review` and configurable path-scoped checks. Rune already has `code-reviewer` subagents.

MVP:

- Add `/review` to review `git diff` or staged changes.
- Support configurable checks, for example:

```text
.rune/checks/security.md
.rune/checks/performance.md
.rune/checks/go-style.md
```

This would make Rune useful even when the user wrote the code manually.

### 10. Better non-interactive/automation output

Amp supports streaming JSON for execute mode. Rune has `--prompt` and `--script`, but JSONL event streaming would make it easier to integrate with CI, editors, and dashboards.

MVP:

```sh
rune --prompt "fix failing tests" --json
```

Emit events for:

- assistant text
- tool start/end
- file changed
- approval needed
- error
- final summary

## Bigger bets

### 11. Worktree/background task runner

Codex CLI advertises launching cloud tasks and applying resulting diffs from the terminal. Rune could do a local-first version using git worktrees.

Possible commands:

```sh
rune task start "upgrade dependency x" --worktree
rune task list
rune task apply <id>
```

Benefits:

- Parallel isolated tasks.
- Safer experimentation.
- Easier diff review before applying changes.

### 12. IDE bridge

Aider and Amp integrate with editors/IDEs. Rune could start with a small local socket bridge or VS Code extension.

Useful context to send Rune:

- Active file.
- Selected text.
- Diagnostics.
- Open tabs.

Useful actions Rune could request:

- Apply edit through IDE APIs for undo support.
- Reveal file/line.
- Read diagnostics after edits.

### 13. Multimodal/UI workflow

Aider supports images/web pages and voice. Amp supports image attachment and browser/UI-oriented workflows.

Potential Rune features:

- Image attachment to prompts.
- Screenshot paste support.
- Browser/devtools MCP skill.
- `/ui-review localhost:3000`.

This would be especially useful for frontend tasks.

### 14. Thread/session sharing

Amp makes thread sharing a first-class team workflow. Rune has local session persistence and branching, so export/share would be a natural next step.

MVP:

```sh
rune session export --html
rune session export --json
```

Later:

- Optional encrypted sync.
- Share links.
- Redaction before export.

### 15. Agent modes

Amp exposes mode presets such as smart/deep/rush. Rune could add mode presets independent of provider/model.

Possible modes:

- `rush`: cheap/fast, limited tools, low max steps.
- `smart`: default balanced mode.
- `deep`: stronger model, more planning, validator subagent.
- `review`: read-only, diff-focused.

## Recommended roadmap

Recommended order by effort-to-impact ratio:

1. Checkpoints + `/undo`.
2. Auto-verify commands + `/verify`.
3. Fine-grained permissions / approval profiles.
4. Hooks.
5. LSP diagnostics/references.
6. Skills v2 + MCP-in-skills.
7. `/review` with configurable checks.
8. JSONL automation mode.
9. Local worktree background tasks.
10. IDE bridge.

## Key takeaway

The strongest tools are converging on the same pattern: **safer autonomy**. They combine powerful models with guardrails and feedback loops: checkpoints, approvals, hooks, tests, review, memory, isolated/background execution, and focused subagents.

Rune already has a solid agent core. The next improvements should make longer autonomous work easier to trust, inspect, and recover from.
