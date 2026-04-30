# Architecture

```
cmd/rune              entrypoint, mode dispatch
└── internal/tui      Bubble Tea: root model, editor, modals
    └── internal/agent  turn loop: provider → tools → loop
        ├── internal/ai     provider client (codex), oauth, faux
        ├── internal/tools  read/write/edit/bash/web_fetch/web_search + Registry
        ├── internal/search web search providers (Brave; SearXNG planned)
        ├── internal/mcp    stdio JSON-RPC client + manager
        └── internal/session branching tree, persist, compact
```

`internal/skill` and `internal/config` are leaf utilities.

## Goroutines and channels

The agent runs in its own goroutine and emits events on a buffered channel
(cap 64). The TUI subscribes via a `tea.Cmd` that drains one event at a time.
Backpressure: the agent blocks on send if the TUI is slow. We never drop events.

## Cancellation

A single `context.Context` per turn cascades through agent → provider → tools.
Esc cancels it; everything propagates: HTTP read aborts, bash subprocess dies.

## Tool permissions and Plan Mode

Tool safety is enforced in `internal/tools.Registry`, not only in prompts.
Interactive Plan Mode sets both the agent mode and registry permission mode:

- `Registry.Specs()` filters denied tools before tool specs are sent to the model.
- `Registry.Run()` also runtime-denies denied calls, returning a normal tool result with `IsError=true` instead of a Go error. This prevents orphan provider tool calls while still enforcing policy.

Plan Mode uses a default-deny policy for mutating and opaque tools:

- Built-in read-only tools are explicitly allowed, including `read`, local file search/listing, read-only git inspection, web search/fetch, and main-agent subagent management.
- Mutating built-ins such as `write`, `edit`, and `bash` are hidden and runtime-denied.
- MCP/external tools are denied by default because rune cannot infer their side effects.

MCP tools can opt in to Plan Mode only through explicit metadata in `~/.rune/mcp.json`:

- `read_only: true` allows all tools from a server.
- `plan_tools: [...]` allows only listed unprefixed tool names from a server.

Internally, external tools can implement `tools.PlanModeTool` to declare Plan Mode availability. MCP tools implement that interface based on their server config metadata.

Subagents inherit a stricter read-only registry via `CloneReadOnly()`: it uses Plan Mode policy and unregisters subagent-management tools so child subagents cannot recursively spawn subagents.

## Persistence

One JSON file per session at `~/.rune/sessions/<id>.json`. Atomic writes
(temp + fsync + rename). Debounced ~250ms after node mutations.

## Auth

`~/.rune/auth.json` under `flock`. Token refresh is single-flight per process,
file-locked across processes.
