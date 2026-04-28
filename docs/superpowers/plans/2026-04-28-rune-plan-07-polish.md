# rune Plan 07 — Polish

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship-quality polish on top of a feature-complete v1: README, docs, install instructions, version stamping, structured logging, panic-to-log handler, `/version` command, basic CI workflow, and a `make release` target. No new product features.

**Architecture:** Add an `internal/log` package that wraps `slog` writing to `~/.rune/log` (single file, append-only, rotated when >10 MB). Wire `panic`-recovery handlers to log full stacks. Add a `Version` constant injected at build via `-ldflags "-X main.Version=..."`. Document everything in `README.md` and `docs/`.

**Tech Stack:** stdlib `log/slog` for logging. GitHub Actions for CI.

**Spec:** `docs/superpowers/specs/2026-04-28-rune-coding-agent-design.md`

---

## File Structure

```
internal/log/
├── log.go
└── log_test.go
docs/
├── providers.md            # how to log in; env vars; troubleshooting
├── skills.md               # how to author markdown skills
├── mcp.md                  # how to configure mcp.json; common servers
├── keybindings.md          # full keymap
└── architecture.md         # 1-pager for contributors
.github/
└── workflows/ci.yml
README.md
Makefile                    # extended
cmd/rune/
└── version.go              # version + /version slash command result
```

---

## Task 1: Structured logging

**Files:**
- Create: `internal/log/log.go`
- Create: `internal/log/log_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/log/log_test.go
package log

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestInit_WritesToFile(t *testing.T) {
    p := filepath.Join(t.TempDir(), "log")
    if err := Init(p); err != nil {
        t.Fatal(err)
    }
    Info("hello", "k", "v")
    Close()

    b, _ := os.ReadFile(p)
    if !strings.Contains(string(b), "hello") || !strings.Contains(string(b), "v") {
        t.Fatalf("log missing entries: %q", b)
    }
}

func TestRotate_RotatesAtThreshold(t *testing.T) {
    p := filepath.Join(t.TempDir(), "log")
    rotateThreshold = 200
    if err := Init(p); err != nil { t.Fatal(err) }
    for i := 0; i < 50; i++ {
        Info("filler line", "i", i)
    }
    Close()

    // rotated copy must exist alongside the new log file.
    rotated := p + ".1"
    if _, err := os.Stat(rotated); err != nil {
        t.Fatalf("expected rotated file at %s: %v", rotated, err)
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/log/log.go
package log

import (
    "log/slog"
    "os"
    "sync"
)

var (
    mu       sync.Mutex
    file     *os.File
    handler  *slog.JSONHandler
    logger   *slog.Logger
    rotateThreshold int64 = 10 * 1024 * 1024 // 10 MB
)

func Init(path string) error {
    mu.Lock()
    defer mu.Unlock()
    if err := rotateIfNeeded(path); err != nil {
        return err
    }
    f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
    if err != nil {
        return err
    }
    file = f
    handler = slog.NewJSONHandler(f, &slog.HandlerOptions{Level: slog.LevelInfo})
    logger = slog.New(handler)
    return nil
}

func rotateIfNeeded(path string) error {
    info, err := os.Stat(path)
    if err != nil {
        if os.IsNotExist(err) { return nil }
        return err
    }
    if info.Size() < rotateThreshold {
        return nil
    }
    return os.Rename(path, path+".1")
}

func Close() {
    mu.Lock()
    defer mu.Unlock()
    if file != nil {
        _ = file.Close()
        file = nil
    }
}

func Info(msg string, args ...any)  { if logger != nil { logger.Info(msg, args...) } }
func Warn(msg string, args ...any)  { if logger != nil { logger.Warn(msg, args...) } }
func Error(msg string, args ...any) { if logger != nil { logger.Error(msg, args...) } }
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/log/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/log/
git commit -m "feat(log): structured slog wrapper with size-based rotation"
```

---

## Task 2: Wire log init + panic handler in main

**Files:**
- Modify: `cmd/rune/main.go`

- [ ] **Step 1: Implement**

```go
// cmd/rune/main.go (additions)
package main

import (
    "context"
    "flag"
    "fmt"
    "os"
    "runtime/debug"

    "github.com/khang859/rune/internal/ai/faux"
    "github.com/khang859/rune/internal/config"
    runelog "github.com/khang859/rune/internal/log"
)

var Version = "0.0.0-dev"

func main() {
    flag.Usage = func() {
        fmt.Fprintln(os.Stderr, "usage: rune [--script <file>] [--prompt <text>] [--version] | rune login codex")
        flag.PrintDefaults()
    }
    showVersion := flag.Bool("version", false, "print version and exit")
    script := flag.String("script", "", "run a JSON script (headless)")
    prompt := flag.String("prompt", "", "run a single turn against the configured provider and exit")
    flag.Parse()

    if *showVersion {
        fmt.Println("rune", Version)
        return
    }

    if err := config.EnsureRuneDir(); err == nil {
        _ = runelog.Init(config.LogPath())
        defer runelog.Close()
    }
    defer func() {
        if r := recover(); r != nil {
            runelog.Error("panic", "value", fmt.Sprint(r), "stack", string(debug.Stack()))
            fmt.Fprintln(os.Stderr, "rune crashed; details in", config.LogPath())
            os.Exit(2)
        }
    }()

    ctx := context.Background()
    args := flag.Args()
    if len(args) >= 2 && args[0] == "login" {
        if err := runLogin(ctx, args[1]); err != nil {
            runelog.Error("login", "err", err.Error())
            fmt.Fprintln(os.Stderr, "login error:", err)
            os.Exit(1)
        }
        return
    }
    if *script != "" {
        if err := runScript(ctx, *script, os.Stdout, faux.New()); err != nil {
            fmt.Fprintln(os.Stderr, "error:", err)
            os.Exit(1)
        }
        return
    }
    if *prompt != "" {
        if err := runPrompt(ctx, *prompt, os.Stdout); err != nil {
            fmt.Fprintln(os.Stderr, "error:", err)
            os.Exit(1)
        }
        return
    }
    if err := runInteractive(ctx); err != nil {
        runelog.Error("interactive", "err", err.Error())
        fmt.Fprintln(os.Stderr, "error:", err)
        os.Exit(1)
    }
}
```

- [ ] **Step 2: Run all tests**

Run: `make all`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add cmd/rune/main.go
git commit -m "feat(cmd): version flag, log init, panic-to-log recovery"
```

---

## Task 3: README

**Files:**
- Create: `README.md`

- [ ] **Step 1: Write the README**

```markdown
# rune

A minimal terminal coding agent in Go. Inspired by [pi-mono](https://github.com/badlogic/pi-mono).

rune ships with read/write/edit/bash tools, branching sessions with compaction,
markdown skills, and MCP plugin support. It runs against ChatGPT Pro/Plus
subscriptions via OAuth.

## Quick start

```bash
go install github.com/khang859/rune/cmd/rune@latest

rune login codex          # opens a browser to auth via your ChatGPT account
rune                      # interactive mode
```

## Usage

| Mode | How |
|---|---|
| Interactive | `rune` |
| One-shot | `rune --prompt "fix the test in ./foo_test.go"` |
| Headless smoke | `rune --script script.json` |
| Version | `rune --version` |

## Commands

Type `/` in the editor to see all commands. Highlights:

- `/model` — switch model
- `/tree` — jump to any point in the session
- `/resume` — pick a previous session
- `/compact` — summarize history
- `/skill:<name>` — invoke a markdown skill
- `/quit` — exit

See `docs/keybindings.md` for the full key map and `/hotkeys` for in-app help.

## Customization

- **Skills** — drop a `.md` file into `~/.rune/skills/` or `./.rune/skills/`.
  See `docs/skills.md`.
- **MCP plugins** — configure `~/.rune/mcp.json`. See `docs/mcp.md`.
- **Project context** — rune walks up from the cwd collecting `AGENTS.md`.

## Development

```bash
make all      # vet + fmt + test + build
make test     # tests only
make build    # build binary
```

See `docs/architecture.md`.

## License

MIT.
```

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: README"
```

---

## Task 4: Provider docs

**Files:**
- Create: `docs/providers.md`

- [ ] **Step 1: Write**

```markdown
# Providers

## Codex (ChatGPT Pro/Plus)

The only built-in provider in v1.

### Login

```bash
rune login codex
```

Opens your default browser to OpenAI's auth page. After approval, the local
callback at `http://localhost:1455/auth/callback` exchanges the code for an
access + refresh token. Tokens are stored in `~/.rune/auth.json` (chmod 0600).

### Refresh

rune refreshes the access token automatically when it has less than 5 minutes
remaining. Concurrent rune processes coordinate via a file lock on
`~/.rune/auth.json.lock`, so a single refresh applies to both.

### Models

- `gpt-5` (default)
- `gpt-5-codex`
- `gpt-5.1-codex-mini`

Switch with `/model` or Ctrl+L.

### Reasoning effort

`/settings` exposes `minimal` / `low` / `medium` / `high`. Higher = more
thinking, more cost-equivalent (subscription is unmetered, but slower).

### Troubleshooting

| Symptom | Fix |
|---|---|
| `not logged in` | `rune login codex` |
| `login expired, run /login` | `rune login codex` (refresh token revoked) |
| `429` | rune retries automatically (3 attempts, exp backoff) |
| `context_length_exceeded` | rune auto-compacts and retries once |

### Env overrides (testing)

- `RUNE_CODEX_ENDPOINT` — override the Responses URL.
- `RUNE_OAUTH_TOKEN_URL` — override the token endpoint.
- `RUNE_DIR` — override `~/.rune`.
```

- [ ] **Step 2: Commit**

```bash
git add docs/providers.md
git commit -m "docs: providers (codex)"
```

---

## Task 5: Skills + MCP docs

**Files:**
- Create: `docs/skills.md`
- Create: `docs/mcp.md`

- [ ] **Step 1: skills.md**

```markdown
# Skills

A skill is a markdown file. Drop one into `~/.rune/skills/` (user-global) or
`./.rune/skills/` (project-local; overrides user-global on slug collision).
The filename minus `.md` becomes the slug — `refactor-step.md` → `/skill:refactor-step`.

## Lifecycle

1. rune scans the two skill roots at startup and on `/reload`.
2. Each skill becomes a `/skill:<slug>` command in the slash menu.
3. Selecting a skill **arms** its body. The body is prepended to your next
   submitted message and then cleared.

## Authoring tips

- Be specific. "When refactoring, write a failing test first" beats "be careful".
- One skill per file; don't bury two unrelated workflows in one body.
- The body is just text — there's no schema, no front matter.

## Example

`~/.rune/skills/tdd.md`:

```
Before changing implementation code, write a failing test that captures
the desired behavior. Run the test to confirm it fails. Then implement
the minimal change to make the test pass.
```

After `/skill:tdd` and "add a foo() helper", rune sends both as the user
message.
```

- [ ] **Step 2: mcp.md**

```markdown
# MCP plugins

rune speaks the [Model Context Protocol](https://modelcontextprotocol.io/) over
stdio. Configure servers in `~/.rune/mcp.json`. Their tools register alongside
rune's built-ins as `<server>:<tool>`.

## Config

```json
{
  "servers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/Users/me/work"]
    },
    "sqlite": {
      "command": "uvx",
      "args": ["mcp-server-sqlite", "--db-path", "/tmp/db.sqlite"]
    }
  }
}
```

Each server is spawned at rune startup. If a server fails to spawn, rune logs
the error to `~/.rune/log` and continues — its tools are simply unavailable.

## Lifecycle

- Servers spawn on `rune` startup.
- Servers terminate when rune exits.
- A crash in one server does not affect other servers or rune itself.

## Tool naming

If `filesystem` exposes a tool named `read_file`, rune surfaces it as
`filesystem:read_file`. The model sees the prefixed name in its tool list.

## Per-tool timeout

Default 60s. Override per server:

```json
{
  "servers": {
    "slow_server": {
      "command": "...",
      "timeout_seconds": 180
    }
  }
}
```

(Not all knobs are wired in v1; see source.)
```

- [ ] **Step 3: Commit**

```bash
git add docs/skills.md docs/mcp.md
git commit -m "docs: skills + mcp"
```

---

## Task 6: Keybindings + architecture docs

**Files:**
- Create: `docs/keybindings.md`
- Create: `docs/architecture.md`

- [ ] **Step 1: keybindings.md**

```markdown
# Keybindings

| Key | Action |
|---|---|
| Enter | Submit message |
| Shift+Enter | Newline |
| Esc | Cancel turn / close modal / close overlay |
| Ctrl+C | Quit (twice if interactive editing) |
| Ctrl+L | Open `/model` |
| Tab | Path completion (or accept overlay item) |
| ↑ / ↓ | Navigate overlays / modals |
| `@` | Open file picker |
| `/` | Open command menu |
| `!cmd` | Run shell, send output as message |
| `!!cmd` | Run shell, do not send |
| Ctrl+V | Paste image (where supported) |
```

- [ ] **Step 2: architecture.md**

```markdown
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
```

- [ ] **Step 3: Commit**

```bash
git add docs/keybindings.md docs/architecture.md
git commit -m "docs: keybindings + architecture"
```

---

## Task 7: CI workflow

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Implement**

```yaml
name: ci
on:
  push:
    branches: [main]
  pull_request:
    branches: [main]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: '1.22'
          cache: true
      - name: Install staticcheck
        run: go install honnef.co/go/tools/cmd/staticcheck@latest
      - name: gofmt
        run: |
          out=$(gofmt -l .)
          if [ -n "$out" ]; then
            echo "gofmt issues:"
            echo "$out"
            exit 1
          fi
      - name: vet
        run: go vet ./...
      - name: staticcheck
        run: staticcheck ./...
      - name: test
        run: go test -race ./...
      - name: build
        run: go build ./...
```

- [ ] **Step 2: Run locally to confirm green**

Run: `make all && go vet ./... && go test -race ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add .github/workflows/ci.yml
git commit -m "ci: github actions — fmt, vet, staticcheck, race test, build"
```

---

## Task 8: Makefile — release target

**Files:**
- Modify: `Makefile`

- [ ] **Step 1: Replace Makefile**

```makefile
.PHONY: build test vet fmt lint all release-snapshot release

VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.Version=$(VERSION)

build:
	go build -ldflags "$(LDFLAGS)" -o rune ./cmd/rune

test:
	go test -race ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

lint:
	staticcheck ./...

all: vet fmt test build

# Cross-compile binaries into ./dist for the four common targets.
release-snapshot:
	@mkdir -p dist
	GOOS=darwin  GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/rune-darwin-arm64 ./cmd/rune
	GOOS=darwin  GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/rune-darwin-amd64 ./cmd/rune
	GOOS=linux   GOARCH=amd64 go build -ldflags "$(LDFLAGS)" -o dist/rune-linux-amd64 ./cmd/rune
	GOOS=linux   GOARCH=arm64 go build -ldflags "$(LDFLAGS)" -o dist/rune-linux-arm64 ./cmd/rune

release: release-snapshot
	@echo "Tagging and uploading release $(VERSION)"
	@test -n "$(VERSION)" || (echo "VERSION required"; exit 1)
	gh release create $(VERSION) dist/* --generate-notes
```

- [ ] **Step 2: Run a release-snapshot locally to confirm cross-compile**

Run: `make release-snapshot`
Expected: 4 binaries in `dist/`.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "build: cross-compile + gh release target"
```

---

## Task 9: Final QA — manual smoke run

This task has no code changes — it's a checklist to run before shipping v1.

- [ ] `make all` is green.
- [ ] `rune --version` prints the expected version string.
- [ ] `rune login codex` opens a browser, completes, persists `~/.rune/auth.json`.
- [ ] `rune --prompt "say hi"` streams text and exits cleanly.
- [ ] `rune` opens the TUI; `/model`, `/tree`, `/resume`, `/settings`, `/hotkeys`, `/quit` all work.
- [ ] `@` opens fuzzy file picker; selecting inserts `@path/to/file`.
- [ ] `Tab` completes unique paths.
- [ ] `!ls` runs and sends ls output as a message.
- [ ] A `~/.rune/skills/test.md` shows up as `/skill:test`; selecting it arms the body.
- [ ] Adding an `~/.rune/mcp.json` server registers its tools (`<server>:<tool>` visible to model).
- [ ] Esc cancels an in-flight turn cleanly.
- [ ] Quitting and resuming via `/resume` reproduces the prior conversation tree.

If any step fails, file a bug, fix in a separate commit.

- [ ] **Step 1: Run the checklist**

Run each item; if all pass:

```bash
git tag -a v0.1.0 -m "rune v0.1.0 — initial release"
git push --tags
make release VERSION=v0.1.0
```

(The user runs the release, not the agent.)

---

## End state

After Plan 07, rune v0.1.0 is shippable:

- README + provider/skill/MCP/keybindings/architecture docs.
- Logging to `~/.rune/log` with rotation; panic recovery.
- `--version` flag; LDFLAGS-stamped Version.
- GitHub Actions CI: fmt + vet + staticcheck + race tests + build.
- `make release` cross-compiles four targets and creates a GitHub release.

Plans 08+ would add other providers (Anthropic, etc.), `/share`, `/export`,
themes, and SDK consumers — but those are explicitly out of scope for v1.
