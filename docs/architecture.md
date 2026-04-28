# Architecture

```
cmd/rune              entrypoint, mode dispatch
└── internal/tui      Bubble Tea: root model, editor, modals
    └── internal/agent  turn loop: provider → tools → loop
        ├── internal/ai     provider client (codex), oauth, faux
        ├── internal/tools  read/write/edit/bash + Registry
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

## Persistence

One JSON file per session at `~/.rune/sessions/<id>.json`. Atomic writes
(temp + fsync + rename). Debounced ~250ms after node mutations.

## Auth

`~/.rune/auth.json` under `flock`. Token refresh is single-flight per process,
file-locked across processes.
