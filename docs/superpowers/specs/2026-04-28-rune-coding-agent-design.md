# rune — Go Port of pi-mono Coding Agent — Design

**Date:** 2026-04-28
**Reference:** `reference/pi-mono` (TypeScript monorepo, badlogic/pi-mono)
**Target binary:** `rune`
**Config dir:** `~/.rune/`

## Goal

Build `rune`: a Go re-implementation of pi-mono's coding agent. v1 targets feature-parity with pi's interactive coding agent on a single provider (OpenAI Codex via ChatGPT Pro/Plus OAuth), preserving pi's distinguishing capabilities — branching sessions, rich editor, full-screen TUI, and skills — while accepting a few intentional cuts that don't affect the core experience.

## Scope

### In scope (v1)

- **Single provider:** OpenAI Codex (ChatGPT subscription via OAuth PKCE, `chatgpt.com/backend-api/codex/responses`).
- **Interactive mode only.** No `--print`, no JSON mode, no RPC mode, no SDK consumers.
- **Full-screen TUI** built on Bubble Tea (charmbracelet) with editor, messages viewport, footer, and modal overlays.
- **Built-in tools:** `read`, `write`, `edit`, `bash` — same surface as pi.
- **Editor parity with pi:** `@` fuzzy file refs, Tab path completion, Shift+Enter multi-line, Ctrl+V image paste / drag-drop, `!command` (run + send output), `!!command` (run, don't send), `/` command menu with autocomplete, message queue while LLM is responding.
- **Branching sessions:** message tree (every user message can fork), `/tree` navigation, `/fork`, `/clone`, `/compact` (manual + auto on context overflow), `/resume`, `/new`, `/name`.
- **Slash commands:** `/login`, `/logout`, `/model`, `/settings`, `/resume`, `/new`, `/name`, `/session`, `/tree`, `/fork`, `/clone`, `/compact`, `/copy`, `/reload`, `/hotkeys`, `/quit`. Skills surface as `/skill:<name>`.
- **Skills:** markdown files in `~/.rune/skills/*.md` and `./.rune/skills/*.md`. Each becomes a `/skill:name` command; body is injected as a prefix into the next user message. No code execution beyond what tools already do.
- **MCP plugins:** stdio JSON-RPC clients spawned per `~/.rune/mcp.json`. Their tools register into the agent's tool registry alongside built-ins. This is the executable extension story.
- **Project context:** walk from cwd up to `$HOME` collecting `AGENTS.md`, concat into the system prompt.
- **Auto-compact on context overflow:** detect overflow, run compact silently, retry the turn.

### Out of scope (v1)

- All other providers (Anthropic, Google, Bedrock, etc.) and their auth flows.
- `/share` (GitHub gist upload) and `/export` (HTML export).
- Custom themes, prompt templates as separate first-class entities (skills cover the common case).
- Native Go extensions / dynamic plugin loading.
- SDK / library consumers — `internal/` packages only.

## Non-goals

- **No backwards compatibility with pi session files.** rune session format is fresh.
- **No multi-module Go workspace.** Single module, `internal/` packages.
- **No real-API tests in CI.** All tests are deterministic and offline.

## Architecture

Five layers; lower layers don't know about higher ones.

```
┌─────────────────────────────────────────────────────────────┐
│  cmd/rune                                                    │
│  • CLI entrypoint, mode dispatch (interactive only)          │
└──────────────────────────┬──────────────────────────────────┘
                           │
┌──────────────────────────▼──────────────────────────────────┐
│  internal/tui  (Bubble Tea)                                  │
│  • RootModel composes: messages viewport, editor, footer,    │
│    modal overlays                                            │
│  • Subscribes to Agent events via tea.Cmd → tea.Msg          │
│  • Renders streaming text, tool calls, errors                │
└──────────────────────────┬──────────────────────────────────┘
                  commands ↓     events ↑
┌──────────────────────────▼──────────────────────────────────┐
│  internal/agent  (the loop)                                  │
│  • Owns the turn: user msg → model → tools → loop            │
│  • Streams events on a channel                               │
│  • Tool dispatch: built-ins + MCP                            │
│  • Context-overflow detection → auto-compact                 │
│  • Abort via context.Context                                 │
└──────┬───────────────────┬───────────────────┬──────────────┘
       │                   │                   │
┌──────▼─────────┐ ┌───────▼────────┐ ┌────────▼──────────────┐
│ internal/ai    │ │ internal/tools │ │ internal/mcp          │
│ • Codex        │ │ • read/write/  │ │ • stdio JSON-RPC      │
│   provider     │ │   edit/bash    │ │ • dynamic tools       │
│ • OAuth (PKCE) │ │                │ │   from servers        │
│ • Streaming    │ │                │ │                       │
│   Responses    │ │                │ │                       │
│   API parser   │ │                │ │                       │
└────────────────┘ └────────────────┘ └───────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  internal/session                                            │
│  • Message tree (each user msg is a node, can fork)          │
│  • Persist as ~/.rune/sessions/<id>.json (atomic)            │
│  • compact / fork / clone / tree navigation                  │
└─────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────┐
│  internal/skill                                              │
│  • Walk ~/.rune/skills/*.md + project ./.rune/skills/*.md    │
│  • Each becomes a /skill:name slash command                  │
│  • Body is injected as prefix to next user message           │
└─────────────────────────────────────────────────────────────┘
```

**Key shape:** the agent runs on its own goroutine and emits events on a buffered channel. The TUI subscribes via a `tea.Cmd` that pulls from the channel and emits `tea.Msg`s. This keeps the UI responsive during streaming and makes abort just a `context.CancelFunc()`.

**`context.Context` is the single end-to-end abort mechanism.** Editor → TUI → agent → provider HTTP → tool subprocess. No custom cancel signals.

## Repository layout

```
rune/
├── cmd/rune/main.go            # entrypoint
├── internal/
│   ├── ai/                     # provider client
│   │   ├── codex/              # Codex provider impl
│   │   ├── oauth/              # PKCE flow, token refresh
│   │   ├── faux/               # scriptable fake provider for tests
│   │   └── types.go            # Provider, Request, Event, Message
│   ├── agent/                  # agent loop, event types
│   ├── session/                # branching tree, persistence, compact
│   ├── tools/                  # read/write/edit/bash + Registry
│   ├── tui/                    # Bubble Tea root model + components
│   ├── skill/                  # markdown skill loader
│   ├── mcp/                    # stdio JSON-RPC client
│   └── config/                 # ~/.rune layout, settings
├── docs/superpowers/specs/     # design docs (this file lives here)
├── go.mod
└── README.md
```

Single Go module. `internal/` blocks external imports. We can lift packages out into a multi-module workspace later if SDK consumers ever appear; that is not a v1 problem.

## Components

### `internal/ai` — provider client

```go
type Provider interface {
    Stream(ctx context.Context, req Request) (<-chan Event, error)
}

type Request struct {
    Model     string
    System    string
    Messages  []Message
    Tools     []ToolSpec
    Reasoning ReasoningConfig
}

type Event interface{ isEvent() }
type TextDelta   struct{ Text string }
type ToolCall    struct{ ID, Name string; Args json.RawMessage }
type Thinking    struct{ Text string }
type Usage       struct{ Input, Output, CacheRead int }
type StreamError struct{ Err error; Retryable bool }
type Done        struct{ Reason string } // "stop" | "tool_use" | "max_tokens" | "context_overflow"
```

- `internal/ai/codex/`: hits `https://chatgpt.com/backend-api/codex/responses`, streams SSE, parses Responses API events, retries on 429/5xx with exponential backoff (3 attempts; 1s/2s/4s + jitter).
- `internal/ai/oauth/codex.go`: PKCE flow. Spin up `localhost:1455`, open browser, exchange code, decode JWT, persist `~/.rune/auth.json`, refresh on expiry with file lock. Mirrors pi-mono's `packages/ai/src/utils/oauth/openai-codex.ts`.
- `internal/ai/faux/`: scriptable fake `Provider` for tests.

### `internal/agent` — the loop

```go
type Agent struct {
    provider ai.Provider
    tools    *tools.Registry
    session  *session.Session
}

func (a *Agent) Run(ctx context.Context, userMsg Message) <-chan Event { ... }

type Event interface{ isEvent() }
type AssistantText    struct{ Delta string }
type ThinkingText     struct{ Delta string }
type ToolStarted      struct{ Call ToolCall }
type ToolFinished     struct{ Call ToolCall; Result ToolResult }
type TurnUsage        struct{ Usage ai.Usage; Cost float64 } // Cost is 0 for subscription auth
type ContextOverflow  struct{}
type TurnAborted      struct{}
type TurnDone         struct{ Reason string }
type TurnError        struct{ Err error }
```

The loop:

1. Build `ai.Request` from the active session path + system prompt + tool specs.
2. Call `provider.Stream(ctx, req)`.
3. For each provider event: forward text/thinking deltas; buffer tool calls; accumulate usage.
4. On `Done`: if no tool calls, emit `TurnDone` and close. Otherwise dispatch each tool, append `tool_result` to session, emit `ToolStarted`/`ToolFinished`, loop back to step 1.
5. On `Done{Reason: "context_overflow"}`: emit `ContextOverflow`, run compact, restart the turn.

### `internal/session` — branching tree

```go
type Session struct {
    ID, Name string
    Created  time.Time
    Root     *Node
    Active   *Node
    Model    string
}

type Node struct {
    ID       string
    Parent   *Node
    Children []*Node
    Message  Message              // user | assistant | tool_result
    Usage    ai.Usage
    Created  time.Time
}
```

Operations:

- `Append(msg)` — adds a child to `Active`, advances `Active`.
- `Fork(node)` — moves `Active` to an existing node; subsequent `Append` creates a new branch.
- `Clone()` — duplicates the active branch into a new session.
- `Compact(ctx, instructions)` — replaces the history above a chosen point with an LLM-generated summary node.

Persistence: one JSON file per session at `~/.rune/sessions/<id>.json`. Atomic writes via `temp file → fsync → rename`. Debounced ~250ms after node mutations.

### `internal/tools` — built-in tools

`Registry` holds named tools. Built-ins: `read`, `write`, `edit`, `bash`. Each tool exposes:

```go
type Tool interface {
    Spec() ToolSpec                                  // JSON schema for the model
    Run(ctx context.Context, args json.RawMessage) (Result, error)
}

type Result struct {
    Output  string
    IsError bool
}
```

MCP-loaded tools register into the same `Registry`; the agent doesn't care about the source.

### `internal/tui` — Bubble Tea

`RootModel` composes:

- **`messages`** — viewport with scrollback. Renders streaming text, tool-call blocks (expandable), errors as inline blocks.
- **`editor`** — `textarea` with overlay autocompletes:
  - `@` → fuzzy file picker
  - `/` → command menu (built-ins + skills + extensions)
  - `!command` → run bash and append output to the user message
  - `!!command` → run bash, do not send
  - Tab → path completion
  - Shift+Enter → newline
  - Ctrl+V / drag-drop → paste image
  - Message queue: while a turn is active, Enter queues; queue drains on `TurnDone`.
- **`footer`** — cwd, session name, total tokens, context %, current model. Cost is omitted under OAuth subscription (no metered per-token cost) and shown when an API-keyed provider lands later.
- **`modals`** — `/model`, `/tree`, `/resume`, `/settings`. One overlay at a time; Esc dismisses.

Subscribes to agent events via a `tea.Cmd` that drains the channel and emits one `tea.Msg` per event.

### `internal/skill`

```go
type Loader struct{ Roots []string }   // ~/.rune/skills, ./.rune/skills
type Skill  struct{ Name string; Body string; Path string }

func (l *Loader) Load() ([]Skill, error)
```

`/reload` re-runs `Load()`. Body is prepended (with a separator) to the next user message.

### `internal/mcp`

Stdio JSON-RPC client. On startup, reads `~/.rune/mcp.json` (server name → command + args + env), spawns each, lists tools, registers them in `tools.Registry` with name prefix (`<server>:<tool>`). Per-tool timeout default 60s.

## Data flow

### Normal turn

```
User types in editor + Enter
  → TUI: append user message node, render it
  → TUI: spawn agent.Run(ctx, msg) → <-chan agent.Event
  → TUI: tea.Cmd reads next event, returns as tea.Msg

agent.Run goroutine:
  loop:
    1. build ai.Request from session.Active path + system prompt + tools
    2. provider.Stream(ctx, req) → <-chan ai.Event
    3. for each ai.Event:
         TextDelta  → emit AssistantText
         Thinking   → emit ThinkingText
         ToolCall   → buffer
         Usage      → accumulate
         Done       → break
    4. if no tool calls: emit TurnDone, close, return
    5. for each tool call:
         emit ToolStarted
         result := tools.Registry.Run(ctx, call)
         append tool_result to session
         emit ToolFinished
    6. goto 1

TUI on each tea.Msg:
  AssistantText   → append to current assistant block
  ToolStarted     → render tool-call block
  ToolFinished    → render result inline (collapsible)
  TurnUsage       → update footer
  ContextOverflow → trigger /compact silently, then re-run
  TurnDone        → release editor, drain queue
  TurnError       → render error block, release editor

session: every node append → debounced atomic save
```

### Abort

```
User hits Esc:
  TUI: cancel turn ctx
  → provider HTTP cancels
  → tools.Run cancels (bash kills child via ctx)
  → agent emits TurnAborted, closes channel

TUI: render "(aborted)", release editor
session: keep partial assistant text; mark turn aborted
```

### Streaming guarantees

- Agent channel buffered (cap 64). On full buffer, agent blocks on send — events are never dropped.
- Single writer per session: only `agent.Run` writes. TUI reads only.
- On quit: cancel root ctx, wait for agent goroutine, flush session.

## Error handling

### Provider errors (`internal/ai`)

- **Network / 5xx / 429:** retry with exponential backoff (3 attempts, 1s/2s/4s + jitter). Emit `StreamError{Retryable: true}` only after retries exhausted.
- **401 / token expired:** catch, attempt `oauth.Refresh()`, retry once. If refresh fails → `StreamError{Retryable: false}` with message `"login expired, run /login"`.
- **Context overflow:** emit `Done{Reason: "context_overflow"}`. Agent layer auto-compacts and retries.
- **Malformed SSE:** `StreamError{Retryable: false}`, log raw chunk.
- **Abort (`ctx.Done()`):** not an error. Stream goroutine returns silently, channel closes.

### Tool errors (`internal/tools`)

Tool errors are **expected output, not exceptions.** Every tool returns `Result{Output, IsError}`. Nonzero bash exits, "file not found", permission denied — all become tool_result messages the model sees. The model decides what to do.

The only thing that propagates as a Go `error` is "tool not found in registry" or "args fail schema validation" — those crash the turn (programming bug or malicious model output) and the TUI shows an error block.

### OAuth errors (`internal/ai/oauth`)

- **No `auth.json`:** agent layer asks TUI to show `/login` overlay before first turn.
- **Refresh fails:** same as 401 — surface as "login expired".
- **Concurrent refresh:** file-lock `~/.rune/auth.json` so two rune processes don't both hit the token endpoint and clobber each other.

### MCP errors (`internal/mcp`)

- **Server fails to spawn / dies mid-session:** log, mark its tools unavailable, agent continues. One flaky MCP server does not crash the session.
- **Bad JSON / protocol violation:** kill that server, log, surface as a notification. Other MCP servers keep working.
- **Tool call timeout:** default 60s, configurable. Returns `Result{IsError: true, Output: "tool timed out"}`.

### Session persistence

- **Disk full / write fails:** log, surface as a notification, keep session in memory. Don't crash.
- **Corrupt JSON on load:** show error, let user start fresh from a copy of the file.
- **Atomic writes:** temp + fsync + rename. Corruption only happens to in-flight writes, never to persisted state.

### Cross-cutting

- TUI never panics on agent errors — every error becomes a rendered error block.
- TUI exits only on `/quit`, double `Ctrl+C`, or unrecoverable terminal error.
- A `recover()` in the agent goroutine converts panics to `TurnError` and logs panic + stack to `~/.rune/log`.

## Testing

Three layers, all offline, no API keys.

### Layer 1 — unit tests per package

- **`internal/ai/codex`:** SSE parsing. Feed canned event streams (captured + scrubbed) and assert `ai.Event` sequences. Cover text deltas, tool calls, thinking, usage, retry-on-429, malformed chunks.
- **`internal/ai/oauth`:** PKCE round-trip with stub authorize/token server. JWT decode. Token refresh under expiry. File lock under concurrent processes.
- **`internal/session`:** tree ops (append, fork, clone, compact-replaces-history). Persist + reload round-trips. Atomic write under simulated crash.
- **`internal/tools`:** each tool against `t.TempDir()`. Error cases (permission denied, file too big, bash timeout).
- **`internal/skill`:** loading from fake skill dirs, slash command resolution, `/reload`.
- **`internal/mcp`:** spawn a stub MCP server (Go binary built per-test), exercise tool listing + invocation + crash recovery.

### Layer 2 — agent integration tests with `faux` provider

```go
faux := faux.New().
    Reply("I'll read the file.").
    CallTool("read", `{"path":"foo.go"}`).
    Reply("It says hello.").
    Done()

agent := agent.New(faux, tools, session)
events := drain(agent.Run(ctx, userMsg("look at foo.go")))

assertEventSeq(t, events, ...)
```

Tests the whole agent loop end-to-end — provider → tool dispatch → session writes → events out — with zero network and deterministic timing. Covers tool calls, multi-turn loops, abort mid-stream, context overflow → auto-compact, error propagation.

### Layer 3 — TUI tests via `teatest`

`github.com/charmbracelet/x/exp/teatest`. Targets:

- Editor: `@` opens file picker, `/` opens command menu, multi-line, message queue.
- Streaming render: feed canned agent events, snapshot viewport at each step.
- Modals: `/model`, `/tree`, `/resume` open and dismiss.
- Aborts: Esc cancels active turn, footer/state correct after.

Snapshots for visual output, asserts for state. Focused on user flows, not pixel parity.

### Out of CI scope

- Real Codex API calls. There is a `make test-real` target gated on `RUNE_REAL_TESTS=1` + a populated `~/.rune/auth.json` for occasional manual verification.
- Real third-party MCP servers — stubs only.

### Test data

Captured-and-scrubbed responses live in `internal/ai/codex/testdata/`. Refreshing fixtures is a manual scripted task, not automated.

### CI gate

`go test ./...` + `go vet ./...` + `staticcheck ./...` + `gofmt -l` (must be empty).

## Dependencies (Go modules)

- `github.com/charmbracelet/bubbletea` — TUI runtime
- `github.com/charmbracelet/bubbles` — viewport, textarea, list components
- `github.com/charmbracelet/lipgloss` — styling
- `github.com/charmbracelet/glamour` — markdown rendering for assistant text
- `github.com/charmbracelet/x/exp/teatest` — TUI testing
- `github.com/spf13/cobra` *or* stdlib `flag` — CLI parsing (lean toward stdlib unless cobra earns its weight)
- `github.com/sahilm/fuzzy` — `@` file picker fuzzy match
- Standard library for HTTP, JSON, OAuth (PKCE), file locks, exec.

No SDK for OpenAI — Codex hits a custom endpoint with a hand-rolled streaming parser.

## Open questions deferred to implementation

- Exact wire format for the Responses API stream — pin while implementing `internal/ai/codex` against a captured fixture from pi-mono's `packages/ai/src/providers/openai-codex-responses.ts`.
- Whether the `messages` viewport uses Glamour for assistant text live-streaming or only post-stream (Glamour is not designed for partial input). Most likely: render plain text during streaming, re-render with Glamour on `TurnDone`.
- Concrete shape of MCP config (`~/.rune/mcp.json`) — adopt Claude Desktop's format if compatible, otherwise a minimal subset.

## Build sequence (high level — implementation plan will refine)

1. Repo skeleton + `go.mod` + minimal `cmd/rune` that prints version.
2. `internal/session` (in-memory tree, JSON persistence, no UI).
3. `internal/ai/faux` + `internal/ai/types.go` — types and a stub provider.
4. `internal/tools` built-ins, no UI.
5. `internal/agent` agent loop, driven by `faux`. End-to-end tests pass before any UI.
6. `internal/ai/codex` provider + `internal/ai/oauth` — agent now works against real Codex from a script.
7. `internal/tui` minimum viable: editor, viewport, footer, agent wired in. Single-turn flow works.
8. Editor features incrementally: `@`, `/`, message queue, `!command`, image paste, Tab.
9. Modals: `/model`, `/tree`, `/resume`, `/settings`.
10. Compact (manual + auto on overflow).
11. Skills loader + `/skill:name` registration.
12. MCP client + tool registration.
13. Polish, docs, README.

Each step ships with its own tests; nothing merges without green CI.
