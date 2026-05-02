# Plan Mode

Plan Mode is an interactive safety mode for planning before implementation.

Enter it with `/plan`. While active, rune adds stronger planning instructions to the agent and enforces tool permissions in code:

- `write`, `edit`, and `bash` are hidden from the model and denied at runtime.
- MCP/external tools are denied by default unless explicitly marked read-only or allowlisted in MCP config.
- Read-only tools such as `read`, `gh`, `web_search`, and `web_fetch` remain available when configured.
- Subagents remain available to the main agent for read-only exploration and plan validation; child subagents cannot recursively spawn subagents.
- Shell shortcuts (`!cmd` and `!!cmd`) are blocked in the TUI.
- The TUI shows both a footer `plan` indicator and a Plan Mode banner while active.

When you approve a plan, Rune starts implementation in a new session using only the approved plan as the prompt.

## Local codebase discovery

Plan Mode blocks `bash` completely. This is safe, but it also removes common read-only discovery commands such as `rg`, `grep`, `find`, `ls`, `git grep`, and `git status`.

To preserve safe discovery without exposing arbitrary shell execution, Plan Mode allows dedicated read-only local exploration tools:

- `list_files` — recursively list project files with path/glob filters, ignore handling, and result limits.
- `search_files` — literal text search with path/glob filters, context lines, binary/large-file skipping, and result limits.
- `git_status` — inspect repository status without mutating state.
- `git_diff` — inspect unstaged or staged diffs, optionally as a diffstat or filtered to a path, with bounded output.
- `gh` — run selected read-only GitHub CLI commands such as issue/PR/repo/run/release views, searches, and `gh api` GET requests.

These tools use bounded output and do not evaluate shell strings.

A restricted shell tool such as `bash_readonly` is possible, but it is riskier because shell redirection, `find -exec`, command substitution, aliases, and broad command semantics can hide mutations. The `gh` tool instead runs the `gh` executable directly and validates the command/subcommand before execution.

## MCP read-only metadata and allowlists

MCP tools are opaque to rune, so Plan Mode denies them by default. You can opt in only MCP tools you trust to be read-only in `~/.rune/mcp.json`:

```json
{
  "servers": {
    "docs": {
      "type": "http",
      "url": "https://example.com/mcp",
      "read_only": true
    },
    "context7": {
      "type": "http",
      "url": "https://example.com/context7/mcp",
      "plan_tools": ["resolve-library-id", "query-docs"]
    }
  }
}
```

- `read_only: true` allows all tools from that MCP server in Plan Mode.
- `plan_tools` allows only the listed unprefixed MCP tool names from that server.
- If both are omitted, tools from that MCP server remain hidden and denied in Plan Mode.

Only mark tools as read-only if the server cannot mutate files, databases, network state, tickets, issues, or other external resources.

## Commands

- `/plan` — enter Plan Mode.
- `/approve` — approve the latest assistant plan and implement it in a new session.
- `/cancel-plan` — clear pending plan approval state while staying in Plan Mode.

The footer shows `plan` while Plan Mode is active, and the TUI displays this banner:

```text
Plan Mode: edits and bash disabled · MCP tools require read-only allowlist · /approve to implement · Shift+Tab to exit
```

MCP tools with `read_only` or `plan_tools` metadata can be available in Plan Mode; all other MCP tools remain hidden and runtime-denied.

## Planning subagents

The `spawn_subagent` tool supports these read-only subagent types:

- `general` — a default-purpose isolated read-only helper.
- `exploration` — codebase discovery focused on relevant files, functions, tests, risks, and implementation touchpoints.
- `validator` — plan review focused on missing files, unsafe assumptions, sequencing, tests, and edge cases.

A recommended workflow is to use one or more `exploration` subagents to gather evidence, draft the plan, then use a `validator` subagent to review the plan before presenting it for approval.
