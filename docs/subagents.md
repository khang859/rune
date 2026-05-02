# Subagents

rune supports built-in subagents and project/global custom subagent types.

Built-in types:

- `general`
- `exploration`
- `validator`
- `code-explorer` — read-only codebase discovery with file:line evidence and essential files to inspect.
- `code-architect` — architecture planning grounded in existing patterns, files, data flow, and tests.
- `code-reviewer` — high-confidence review findings for bugs, regressions, security issues, and meaningful convention mismatches.

Use the `spawn_subagent` tool with `agent_type` to choose a type.

## Custom subagents

Declare custom subagent types as Markdown files in either:

```text
~/.rune/agents/*.md
./.rune/agents/*.md
```

Global agents are loaded first, then project agents. If both define the same custom name, the project definition wins. Built-in names (`general`, `exploration`, `validator`, `code-explorer`, `code-architect`, `code-reviewer`) are reserved.

Example:

```md
---
name: implementation-agent
description: Makes focused code changes.
model: gpt-5.5
timeout_secs: 1800
tools: full
---

You are an implementation subagent.

You may edit files and run commands when necessary, but keep changes narrow.
Return a concise summary, files changed, tests run, and risks.
```

Frontmatter fields:

- `name` — optional; defaults to the filename without `.md`.
- `description` — optional human-readable description.
- `model` — optional model override using the same provider as the parent session.
- `timeout_secs` — optional default timeout for this agent type.
- `tools` — optional; `readonly` by default, or `full`.

`spawn_subagent.timeout_secs` overrides the timeout declared by the agent file.

## Tool modes

`tools: readonly` uses rune's read-only child registry. Mutating tools such as `write`, `edit`, and `bash` are denied.

`tools: full` gives the subagent a full act-mode clone of the parent registry, including mutating built-ins and act-mode MCP tools. Subagent-management tools are still removed, so subagents cannot recursively spawn subagents in this iteration.
