# rune Plan 01 — Foundation + Headless Agent

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the headless core of rune — types, sessions, built-in tools, agent loop — driven end-to-end by a scriptable fake provider. No UI, no real API calls.

**Architecture:** Single Go module with `internal/` packages. The `ai` package defines provider/message/event types and ships a `faux` provider for tests. The `agent` package owns the turn loop. The `session` package stores a branching message tree as JSON. The `tools` package implements `read`/`write`/`edit`/`bash`. A minimal `cmd/rune` binary can run a turn from a JSON script for end-to-end smoke testing.

**Tech Stack:** Go 1.22+, stdlib only for v1 of this plan (no third-party deps yet — Bubble Tea and friends arrive in Plan 03).

**Spec:** `docs/superpowers/specs/2026-04-28-rune-coding-agent-design.md`

---

## File Structure

```
rune/
├── cmd/rune/main.go                    # entrypoint, --script smoke runner
├── internal/
│   ├── config/paths.go                 # ~/.rune layout helpers
│   ├── ai/
│   │   ├── types.go                    # Provider, Message, Event, ToolSpec
│   │   └── faux/faux.go                # scriptable test provider
│   ├── tools/
│   │   ├── tool.go                     # Tool, Result, Registry
│   │   ├── read.go
│   │   ├── write.go
│   │   ├── edit.go
│   │   └── bash.go
│   ├── session/
│   │   ├── session.go                  # Session, Node, ops
│   │   └── persist.go                  # atomic JSON save/load
│   └── agent/
│       ├── agent.go                    # Agent struct, New
│       ├── events.go                   # Event types
│       └── loop.go                     # Run() implementation
├── go.mod
├── go.sum
└── .gitignore
```

Test files are colocated with each package (`tool_test.go`, `loop_test.go`, etc.) per Go convention.

---

## Type contract (locked across all 7 plans)

These are the public types established in this plan. All later plans reference them.

```go
// internal/ai/types.go

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

type Role string
const (
    RoleUser       Role = "user"
    RoleAssistant  Role = "assistant"
    RoleToolResult Role = "tool_result"
)

type Message struct {
    Role       Role
    Content    []ContentBlock
    ToolCallID string // populated when Role == RoleToolResult
}

type ContentBlock interface{ isContentBlock() }
type TextBlock       struct{ Text string }
type ImageBlock      struct{ Data []byte; MimeType string }
type ToolUseBlock    struct{ ID, Name string; Args json.RawMessage }
type ToolResultBlock struct{ ToolCallID string; Output string; IsError bool }

type ToolSpec struct {
    Name        string
    Description string
    Schema      json.RawMessage // JSON Schema for arguments
}

type ReasoningConfig struct {
    Effort string // "minimal" | "low" | "medium" | "high"
}

type Event interface{ isEvent() }
type TextDelta   struct{ Text string }
type Thinking    struct{ Text string }
type ToolCall    struct{ ID, Name string; Args json.RawMessage }
type Usage       struct{ Input, Output, CacheRead int }
type StreamError struct{ Err error; Retryable bool }
type Done        struct{ Reason string } // "stop" | "tool_use" | "max_tokens" | "context_overflow"
```

```go
// internal/tools/tool.go

type Tool interface {
    Spec() ai.ToolSpec
    Run(ctx context.Context, args json.RawMessage) (Result, error)
}

type Result struct {
    Output  string
    IsError bool
}

type Registry struct{ /* ... */ }

func NewRegistry() *Registry
func (r *Registry) Register(t Tool)
func (r *Registry) Specs() []ai.ToolSpec
func (r *Registry) Run(ctx context.Context, call ai.ToolCall) (Result, error)
```

```go
// internal/session/session.go

type Session struct {
    ID, Name string
    Created  time.Time
    Model    string
    Root     *Node
    Active   *Node
    path     string // ~/.rune/sessions/<id>.json
}

type Node struct {
    ID       string
    Parent   *Node       `json:"-"` // reconstructed on load
    Children []*Node
    Message  ai.Message
    Usage    ai.Usage
    Created  time.Time
}

func New(model string) *Session
func Load(path string) (*Session, error)
func (s *Session) Append(msg ai.Message) *Node
func (s *Session) Fork(node *Node)
func (s *Session) Clone() *Session
func (s *Session) PathToActive() []ai.Message
func (s *Session) Save() error
```

```go
// internal/agent/events.go

type Event interface{ isEvent() }
type AssistantText   struct{ Delta string }
type ThinkingText    struct{ Delta string }
type ToolStarted     struct{ Call ai.ToolCall }
type ToolFinished    struct{ Call ai.ToolCall; Result tools.Result }
type TurnUsage       struct{ Usage ai.Usage; Cost float64 }
type ContextOverflow struct{}
type TurnAborted     struct{}
type TurnDone        struct{ Reason string }
type TurnError       struct{ Err error }
```

```go
// internal/agent/agent.go

type Agent struct { /* unexported */ }

func New(p ai.Provider, t *tools.Registry, s *session.Session, systemPrompt string) *Agent
func (a *Agent) Run(ctx context.Context, userMsg ai.Message) <-chan Event
```

---

## Task 1: Bootstrap the Go module

**Files:**
- Create: `go.mod`
- Create: `.gitignore`
- Create: `cmd/rune/main.go`

- [ ] **Step 1: Initialize module**

```bash
cd /Users/khangnguyen/Development/rune
go mod init github.com/khang859/rune
```

- [ ] **Step 2: Write .gitignore**

```
# rune
/rune
/rune.exe
*.test
*.out
coverage.txt

# Editor / OS
.DS_Store
.vscode/
.idea/
```

(append to existing `.gitignore`; do not overwrite the `reference` line that's already there.)

- [ ] **Step 3: Write minimal cmd/rune/main.go**

```go
package main

import "fmt"

const version = "0.0.0-dev"

func main() {
    fmt.Println("rune", version)
}
```

- [ ] **Step 4: Verify build**

Run: `go build -o rune ./cmd/rune && ./rune`
Expected: prints `rune 0.0.0-dev`

- [ ] **Step 5: Commit**

```bash
git add go.mod .gitignore cmd/rune/main.go
git commit -m "chore: bootstrap go module and rune binary skeleton"
```

---

## Task 2: Config paths

**Files:**
- Create: `internal/config/paths.go`
- Create: `internal/config/paths_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/config/paths_test.go
package config

import (
    "os"
    "path/filepath"
    "strings"
    "testing"
)

func TestRuneDir_DefaultsToHomeRune(t *testing.T) {
    t.Setenv("HOME", "/tmp/fakehome")
    t.Setenv("RUNE_DIR", "")
    got := RuneDir()
    want := filepath.Join("/tmp/fakehome", ".rune")
    if got != want {
        t.Fatalf("RuneDir() = %q, want %q", got, want)
    }
}

func TestRuneDir_RespectsRUNE_DIR(t *testing.T) {
    t.Setenv("RUNE_DIR", "/custom/path")
    if got := RuneDir(); got != "/custom/path" {
        t.Fatalf("RuneDir() = %q, want %q", got, "/custom/path")
    }
}

func TestSessionsDir_IsUnderRuneDir(t *testing.T) {
    t.Setenv("RUNE_DIR", "/r")
    if got := SessionsDir(); !strings.HasSuffix(got, "/sessions") || !strings.HasPrefix(got, "/r") {
        t.Fatalf("SessionsDir() = %q, want under /r ending /sessions", got)
    }
}

func TestEnsureRuneDir_CreatesDir(t *testing.T) {
    dir := t.TempDir()
    t.Setenv("RUNE_DIR", filepath.Join(dir, "nested"))
    if err := EnsureRuneDir(); err != nil {
        t.Fatal(err)
    }
    if _, err := os.Stat(RuneDir()); err != nil {
        t.Fatal(err)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/...`
Expected: FAIL — package does not compile (RuneDir undefined)

- [ ] **Step 3: Write minimal implementation**

```go
// internal/config/paths.go
package config

import (
    "os"
    "path/filepath"
)

// RuneDir returns ~/.rune (or $RUNE_DIR if set).
func RuneDir() string {
    if d := os.Getenv("RUNE_DIR"); d != "" {
        return d
    }
    home, err := os.UserHomeDir()
    if err != nil {
        return ".rune"
    }
    return filepath.Join(home, ".rune")
}

func SessionsDir() string { return filepath.Join(RuneDir(), "sessions") }
func AuthPath()    string { return filepath.Join(RuneDir(), "auth.json") }
func SkillsDir()   string { return filepath.Join(RuneDir(), "skills") }
func MCPConfig()   string { return filepath.Join(RuneDir(), "mcp.json") }
func LogPath()     string { return filepath.Join(RuneDir(), "log") }

// EnsureRuneDir creates the rune dir tree if missing.
func EnsureRuneDir() error {
    return os.MkdirAll(SessionsDir(), 0o755)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat(config): rune dir layout helpers"
```

---

## Task 3: AI types

**Files:**
- Create: `internal/ai/types.go`
- Create: `internal/ai/types_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/ai/types_test.go
package ai

import (
    "encoding/json"
    "testing"
)

func TestMessage_RoundTripJSON(t *testing.T) {
    m := Message{
        Role: RoleAssistant,
        Content: []ContentBlock{
            TextBlock{Text: "hello"},
            ToolUseBlock{ID: "t1", Name: "read", Args: json.RawMessage(`{"path":"x"}`)},
        },
    }
    b, err := json.Marshal(m)
    if err != nil {
        t.Fatal(err)
    }
    var got Message
    if err := json.Unmarshal(b, &got); err != nil {
        t.Fatal(err)
    }
    if got.Role != RoleAssistant {
        t.Fatalf("role = %q", got.Role)
    }
    if len(got.Content) != 2 {
        t.Fatalf("content len = %d", len(got.Content))
    }
    if tx, ok := got.Content[0].(TextBlock); !ok || tx.Text != "hello" {
        t.Fatalf("content[0] = %#v", got.Content[0])
    }
    if tu, ok := got.Content[1].(ToolUseBlock); !ok || tu.Name != "read" {
        t.Fatalf("content[1] = %#v", got.Content[1])
    }
}

func TestToolResultMessage_HasToolCallID(t *testing.T) {
    m := Message{
        Role:       RoleToolResult,
        ToolCallID: "t1",
        Content:    []ContentBlock{ToolResultBlock{ToolCallID: "t1", Output: "ok"}},
    }
    b, _ := json.Marshal(m)
    var got Message
    if err := json.Unmarshal(b, &got); err != nil {
        t.Fatal(err)
    }
    if got.ToolCallID != "t1" {
        t.Fatalf("toolCallID = %q", got.ToolCallID)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/...`
Expected: FAIL — package does not compile

- [ ] **Step 3: Write minimal implementation**

```go
// internal/ai/types.go
package ai

import (
    "context"
    "encoding/json"
    "fmt"
)

type Provider interface {
    Stream(ctx context.Context, req Request) (<-chan Event, error)
}

type Request struct {
    Model     string          `json:"model"`
    System    string          `json:"system,omitempty"`
    Messages  []Message       `json:"messages"`
    Tools     []ToolSpec      `json:"tools,omitempty"`
    Reasoning ReasoningConfig `json:"reasoning,omitempty"`
}

type Role string

const (
    RoleUser       Role = "user"
    RoleAssistant  Role = "assistant"
    RoleToolResult Role = "tool_result"
)

type Message struct {
    Role       Role           `json:"role"`
    Content    []ContentBlock `json:"content"`
    ToolCallID string         `json:"tool_call_id,omitempty"`
}

type ContentBlock interface{ isContentBlock() }

type TextBlock struct {
    Text string `json:"text"`
}

func (TextBlock) isContentBlock() {}

type ImageBlock struct {
    Data     []byte `json:"data"`
    MimeType string `json:"mime_type"`
}

func (ImageBlock) isContentBlock() {}

type ToolUseBlock struct {
    ID   string          `json:"id"`
    Name string          `json:"name"`
    Args json.RawMessage `json:"args"`
}

func (ToolUseBlock) isContentBlock() {}

type ToolResultBlock struct {
    ToolCallID string `json:"tool_call_id"`
    Output     string `json:"output"`
    IsError    bool   `json:"is_error,omitempty"`
}

func (ToolResultBlock) isContentBlock() {}

type ToolSpec struct {
    Name        string          `json:"name"`
    Description string          `json:"description"`
    Schema      json.RawMessage `json:"schema"`
}

type ReasoningConfig struct {
    Effort string `json:"effort,omitempty"`
}

// Custom JSON for Message so we can deserialize the polymorphic Content slice.
type messageWire struct {
    Role       Role            `json:"role"`
    Content    []contentWire   `json:"content"`
    ToolCallID string          `json:"tool_call_id,omitempty"`
}

type contentWire struct {
    Type       string          `json:"type"`
    Text       string          `json:"text,omitempty"`
    Data       []byte          `json:"data,omitempty"`
    MimeType   string          `json:"mime_type,omitempty"`
    ID         string          `json:"id,omitempty"`
    Name       string          `json:"name,omitempty"`
    Args       json.RawMessage `json:"args,omitempty"`
    ToolCallID string          `json:"tool_call_id,omitempty"`
    Output     string          `json:"output,omitempty"`
    IsError    bool            `json:"is_error,omitempty"`
}

func (m Message) MarshalJSON() ([]byte, error) {
    w := messageWire{Role: m.Role, ToolCallID: m.ToolCallID}
    for _, c := range m.Content {
        switch v := c.(type) {
        case TextBlock:
            w.Content = append(w.Content, contentWire{Type: "text", Text: v.Text})
        case ImageBlock:
            w.Content = append(w.Content, contentWire{Type: "image", Data: v.Data, MimeType: v.MimeType})
        case ToolUseBlock:
            w.Content = append(w.Content, contentWire{Type: "tool_use", ID: v.ID, Name: v.Name, Args: v.Args})
        case ToolResultBlock:
            w.Content = append(w.Content, contentWire{Type: "tool_result", ToolCallID: v.ToolCallID, Output: v.Output, IsError: v.IsError})
        default:
            return nil, fmt.Errorf("unknown content block: %T", c)
        }
    }
    return json.Marshal(w)
}

func (m *Message) UnmarshalJSON(b []byte) error {
    var w messageWire
    if err := json.Unmarshal(b, &w); err != nil {
        return err
    }
    m.Role = w.Role
    m.ToolCallID = w.ToolCallID
    m.Content = nil
    for _, c := range w.Content {
        switch c.Type {
        case "text":
            m.Content = append(m.Content, TextBlock{Text: c.Text})
        case "image":
            m.Content = append(m.Content, ImageBlock{Data: c.Data, MimeType: c.MimeType})
        case "tool_use":
            m.Content = append(m.Content, ToolUseBlock{ID: c.ID, Name: c.Name, Args: c.Args})
        case "tool_result":
            m.Content = append(m.Content, ToolResultBlock{ToolCallID: c.ToolCallID, Output: c.Output, IsError: c.IsError})
        default:
            return fmt.Errorf("unknown content type: %q", c.Type)
        }
    }
    return nil
}

type Event interface{ isEvent() }

type TextDelta struct{ Text string }

func (TextDelta) isEvent() {}

type Thinking struct{ Text string }

func (Thinking) isEvent() {}

type ToolCall struct {
    ID   string
    Name string
    Args json.RawMessage
}

func (ToolCall) isEvent() {}

type Usage struct {
    Input     int
    Output    int
    CacheRead int
}

func (Usage) isEvent() {}

type StreamError struct {
    Err       error
    Retryable bool
}

func (StreamError) isEvent() {}

type Done struct{ Reason string }

func (Done) isEvent() {}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ai/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai/
git commit -m "feat(ai): core types — Provider, Message, Event, ToolSpec"
```

---

## Task 4: Faux provider

**Files:**
- Create: `internal/ai/faux/faux.go`
- Create: `internal/ai/faux/faux_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/ai/faux/faux_test.go
package faux

import (
    "context"
    "encoding/json"
    "testing"

    "github.com/khang859/rune/internal/ai"
)

func collect(t *testing.T, ch <-chan ai.Event) []ai.Event {
    t.Helper()
    var out []ai.Event
    for e := range ch {
        out = append(out, e)
    }
    return out
}

func TestFaux_TextThenDone(t *testing.T) {
    f := New().Reply("hello world").Done()
    ch, err := f.Stream(context.Background(), ai.Request{})
    if err != nil {
        t.Fatal(err)
    }
    evs := collect(t, ch)
    // Expect: TextDelta("hello world"), Usage, Done
    if len(evs) != 3 {
        t.Fatalf("events = %d, want 3: %#v", len(evs), evs)
    }
    if td, ok := evs[0].(ai.TextDelta); !ok || td.Text != "hello world" {
        t.Fatalf("evs[0] = %#v", evs[0])
    }
    if d, ok := evs[2].(ai.Done); !ok || d.Reason != "stop" {
        t.Fatalf("evs[2] = %#v", evs[2])
    }
}

func TestFaux_ToolCallThenDone(t *testing.T) {
    f := New().CallTool("read", `{"path":"foo"}`).Done()
    ch, _ := f.Stream(context.Background(), ai.Request{})
    evs := collect(t, ch)
    var foundCall bool
    for _, e := range evs {
        if c, ok := e.(ai.ToolCall); ok {
            foundCall = true
            if c.Name != "read" {
                t.Fatalf("name = %q", c.Name)
            }
            var args map[string]string
            if err := json.Unmarshal(c.Args, &args); err != nil {
                t.Fatal(err)
            }
            if args["path"] != "foo" {
                t.Fatalf("args = %v", args)
            }
        }
    }
    if !foundCall {
        t.Fatal("no ToolCall emitted")
    }
}

func TestFaux_TurnsAdvanceAcrossStreamCalls(t *testing.T) {
    f := New().
        Reply("first turn").Done().
        Reply("second turn").Done()
    // First call returns turn 1.
    ch1, _ := f.Stream(context.Background(), ai.Request{})
    e1 := collect(t, ch1)
    if td := e1[0].(ai.TextDelta); td.Text != "first turn" {
        t.Fatal("turn 1 wrong")
    }
    // Second call returns turn 2.
    ch2, _ := f.Stream(context.Background(), ai.Request{})
    e2 := collect(t, ch2)
    if td := e2[0].(ai.TextDelta); td.Text != "second turn" {
        t.Fatal("turn 2 wrong")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/ai/faux/...`
Expected: FAIL — package does not compile

- [ ] **Step 3: Write minimal implementation**

```go
// internal/ai/faux/faux.go
package faux

import (
    "context"
    "encoding/json"
    "sync"

    "github.com/khang859/rune/internal/ai"
)

// Faux is a scriptable ai.Provider for tests.
//
// Build a script with chained methods; each Done() ends one turn.
// Stream returns the next turn's events in order.
type Faux struct {
    mu    sync.Mutex
    turns [][]ai.Event
    next  int
}

func New() *Faux { return &Faux{} }

func (f *Faux) Reply(text string) *Faux {
    f.cur().push(ai.TextDelta{Text: text})
    return f
}

func (f *Faux) Thinking(text string) *Faux {
    f.cur().push(ai.Thinking{Text: text})
    return f
}

func (f *Faux) CallTool(name, jsonArgs string) *Faux {
    f.cur().push(ai.ToolCall{
        ID:   randID(),
        Name: name,
        Args: json.RawMessage(jsonArgs),
    })
    return f
}

func (f *Faux) Usage(in, out int) *Faux {
    f.cur().push(ai.Usage{Input: in, Output: out})
    return f
}

// Done finishes the current turn (default reason "stop"; "tool_use" if any tool calls in this turn).
func (f *Faux) Done() *Faux {
    f.mu.Lock()
    defer f.mu.Unlock()
    if len(f.turns) == 0 {
        f.turns = append(f.turns, nil)
    }
    cur := f.turns[len(f.turns)-1]
    reason := "stop"
    for _, e := range cur {
        if _, ok := e.(ai.ToolCall); ok {
            reason = "tool_use"
            break
        }
    }
    // Always emit a usage event before Done so consumers see token counts.
    cur = append(cur, ai.Usage{Input: 1, Output: 1})
    cur = append(cur, ai.Done{Reason: reason})
    f.turns[len(f.turns)-1] = cur
    f.turns = append(f.turns, nil) // start a new turn buffer for next chained call
    return f
}

// DoneOverflow finishes the current turn with reason "context_overflow".
func (f *Faux) DoneOverflow() *Faux {
    f.mu.Lock()
    defer f.mu.Unlock()
    if len(f.turns) == 0 {
        f.turns = append(f.turns, nil)
    }
    cur := f.turns[len(f.turns)-1]
    cur = append(cur, ai.Done{Reason: "context_overflow"})
    f.turns[len(f.turns)-1] = cur
    f.turns = append(f.turns, nil)
    return f
}

func (f *Faux) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
    f.mu.Lock()
    if f.next >= len(f.turns) || len(f.turns[f.next]) == 0 {
        f.mu.Unlock()
        // Empty turn: return a closed channel after a single Done.
        out := make(chan ai.Event, 1)
        out <- ai.Done{Reason: "stop"}
        close(out)
        return out, nil
    }
    events := f.turns[f.next]
    f.next++
    f.mu.Unlock()

    out := make(chan ai.Event, len(events))
    go func() {
        defer close(out)
        for _, e := range events {
            select {
            case <-ctx.Done():
                return
            case out <- e:
            }
        }
    }()
    return out, nil
}

// --- helpers ---

type turnBuf struct{ events *[]ai.Event }

func (f *Faux) cur() turnBuf {
    f.mu.Lock()
    defer f.mu.Unlock()
    if len(f.turns) == 0 {
        f.turns = append(f.turns, nil)
    }
    last := len(f.turns) - 1
    return turnBuf{events: &f.turns[last]}
}

func (b turnBuf) push(e ai.Event) {
    *b.events = append(*b.events, e)
}

var idCounter int

func randID() string {
    idCounter++
    return string(rune('a'+idCounter%26)) + string(rune('0'+(idCounter/26)%10))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/ai/faux/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/ai/faux/
git commit -m "feat(ai/faux): scriptable test provider"
```

---

## Task 5: Tools registry

**Files:**
- Create: `internal/tools/tool.go`
- Create: `internal/tools/tool_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tools/tool_test.go
package tools

import (
    "context"
    "encoding/json"
    "errors"
    "strings"
    "testing"

    "github.com/khang859/rune/internal/ai"
)

type stubTool struct{ name string }

func (s stubTool) Spec() ai.ToolSpec {
    return ai.ToolSpec{Name: s.name, Description: "stub", Schema: json.RawMessage(`{}`)}
}
func (s stubTool) Run(ctx context.Context, args json.RawMessage) (Result, error) {
    return Result{Output: "ran " + s.name}, nil
}

func TestRegistry_RegisterAndRun(t *testing.T) {
    r := NewRegistry()
    r.Register(stubTool{name: "x"})
    res, err := r.Run(context.Background(), ai.ToolCall{Name: "x", Args: json.RawMessage(`{}`)})
    if err != nil {
        t.Fatal(err)
    }
    if res.Output != "ran x" {
        t.Fatalf("output = %q", res.Output)
    }
}

func TestRegistry_UnknownTool(t *testing.T) {
    r := NewRegistry()
    _, err := r.Run(context.Background(), ai.ToolCall{Name: "missing"})
    if err == nil || !strings.Contains(err.Error(), "unknown tool") {
        t.Fatalf("err = %v", err)
    }
}

func TestRegistry_Specs(t *testing.T) {
    r := NewRegistry()
    r.Register(stubTool{name: "a"})
    r.Register(stubTool{name: "b"})
    specs := r.Specs()
    if len(specs) != 2 {
        t.Fatalf("specs len = %d", len(specs))
    }
}

func TestRegistry_DoesNotSwallowToolErrors(t *testing.T) {
    r := NewRegistry()
    r.Register(errTool{})
    _, err := r.Run(context.Background(), ai.ToolCall{Name: "boom"})
    if !errors.Is(err, errBoom) {
        t.Fatalf("err = %v", err)
    }
}

var errBoom = errors.New("boom")

type errTool struct{}

func (errTool) Spec() ai.ToolSpec {
    return ai.ToolSpec{Name: "boom", Schema: json.RawMessage(`{}`)}
}
func (errTool) Run(ctx context.Context, args json.RawMessage) (Result, error) {
    return Result{}, errBoom
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tools/...`
Expected: FAIL — package does not compile

- [ ] **Step 3: Write minimal implementation**

```go
// internal/tools/tool.go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    "sort"

    "github.com/khang859/rune/internal/ai"
)

type Tool interface {
    Spec() ai.ToolSpec
    Run(ctx context.Context, args json.RawMessage) (Result, error)
}

type Result struct {
    Output  string
    IsError bool
}

type Registry struct {
    tools map[string]Tool
}

func NewRegistry() *Registry {
    return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(t Tool) {
    r.tools[t.Spec().Name] = t
}

func (r *Registry) Specs() []ai.ToolSpec {
    var names []string
    for n := range r.tools {
        names = append(names, n)
    }
    sort.Strings(names)
    out := make([]ai.ToolSpec, 0, len(names))
    for _, n := range names {
        out = append(out, r.tools[n].Spec())
    }
    return out
}

func (r *Registry) Run(ctx context.Context, call ai.ToolCall) (Result, error) {
    t, ok := r.tools[call.Name]
    if !ok {
        return Result{}, fmt.Errorf("unknown tool: %q", call.Name)
    }
    return t.Run(ctx, call.Args)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tools/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tools/
git commit -m "feat(tools): registry and Tool interface"
```

---

## Task 6: read tool

**Files:**
- Create: `internal/tools/read.go`
- Create: `internal/tools/read_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tools/read_test.go
package tools

import (
    "context"
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
)

func TestRead_File(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "foo.txt")
    if err := os.WriteFile(p, []byte("hello\nworld\n"), 0o644); err != nil {
        t.Fatal(err)
    }
    args, _ := json.Marshal(map[string]any{"path": p})
    res, err := (Read{}).Run(context.Background(), args)
    if err != nil {
        t.Fatal(err)
    }
    if res.IsError {
        t.Fatalf("unexpected error: %s", res.Output)
    }
    if res.Output != "hello\nworld\n" {
        t.Fatalf("output = %q", res.Output)
    }
}

func TestRead_Missing(t *testing.T) {
    args, _ := json.Marshal(map[string]any{"path": "/does/not/exist"})
    res, err := (Read{}).Run(context.Background(), args)
    if err != nil {
        t.Fatalf("unexpected go error: %v", err)
    }
    if !res.IsError {
        t.Fatalf("expected IsError=true, got %#v", res)
    }
}

func TestRead_BadArgs(t *testing.T) {
    res, err := (Read{}).Run(context.Background(), json.RawMessage(`not-json`))
    if err != nil {
        t.Fatal(err) // we want IsError=true, not a Go error
    }
    if !res.IsError {
        t.Fatal("expected IsError=true")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tools/ -run TestRead`
Expected: FAIL — Read undefined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/tools/read.go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    "os"

    "github.com/khang859/rune/internal/ai"
)

type Read struct{}

func (Read) Spec() ai.ToolSpec {
    return ai.ToolSpec{
        Name:        "read",
        Description: "Read the contents of a file at an absolute path.",
        Schema: json.RawMessage(`{
            "type":"object",
            "properties":{"path":{"type":"string"}},
            "required":["path"]
        }`),
    }
}

func (Read) Run(ctx context.Context, args json.RawMessage) (Result, error) {
    var a struct{ Path string `json:"path"` }
    if err := json.Unmarshal(args, &a); err != nil {
        return Result{Output: fmt.Sprintf("invalid args: %v", err), IsError: true}, nil
    }
    b, err := os.ReadFile(a.Path)
    if err != nil {
        return Result{Output: err.Error(), IsError: true}, nil
    }
    return Result{Output: string(b)}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tools/ -run TestRead`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tools/read.go internal/tools/read_test.go
git commit -m "feat(tools): read tool"
```

---

## Task 7: write tool

**Files:**
- Create: `internal/tools/write.go`
- Create: `internal/tools/write_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tools/write_test.go
package tools

import (
    "context"
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
)

func TestWrite_NewFile(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "out.txt")
    args, _ := json.Marshal(map[string]any{"path": p, "content": "hi"})
    res, err := (Write{}).Run(context.Background(), args)
    if err != nil {
        t.Fatal(err)
    }
    if res.IsError {
        t.Fatalf("error: %s", res.Output)
    }
    b, _ := os.ReadFile(p)
    if string(b) != "hi" {
        t.Fatalf("content = %q", b)
    }
}

func TestWrite_OverwritesExisting(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "x.txt")
    _ = os.WriteFile(p, []byte("old"), 0o644)
    args, _ := json.Marshal(map[string]any{"path": p, "content": "new"})
    if _, err := (Write{}).Run(context.Background(), args); err != nil {
        t.Fatal(err)
    }
    b, _ := os.ReadFile(p)
    if string(b) != "new" {
        t.Fatalf("content = %q", b)
    }
}

func TestWrite_CreatesParents(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "a", "b", "c.txt")
    args, _ := json.Marshal(map[string]any{"path": p, "content": "ok"})
    if _, err := (Write{}).Run(context.Background(), args); err != nil {
        t.Fatal(err)
    }
    if _, err := os.Stat(p); err != nil {
        t.Fatal(err)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tools/ -run TestWrite`
Expected: FAIL — Write undefined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/tools/write.go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"

    "github.com/khang859/rune/internal/ai"
)

type Write struct{}

func (Write) Spec() ai.ToolSpec {
    return ai.ToolSpec{
        Name:        "write",
        Description: "Write content to a file. Creates parent directories. Overwrites existing files.",
        Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "path":{"type":"string"},
                "content":{"type":"string"}
            },
            "required":["path","content"]
        }`),
    }
}

func (Write) Run(ctx context.Context, args json.RawMessage) (Result, error) {
    var a struct {
        Path    string `json:"path"`
        Content string `json:"content"`
    }
    if err := json.Unmarshal(args, &a); err != nil {
        return Result{Output: fmt.Sprintf("invalid args: %v", err), IsError: true}, nil
    }
    if err := os.MkdirAll(filepath.Dir(a.Path), 0o755); err != nil {
        return Result{Output: err.Error(), IsError: true}, nil
    }
    if err := os.WriteFile(a.Path, []byte(a.Content), 0o644); err != nil {
        return Result{Output: err.Error(), IsError: true}, nil
    }
    return Result{Output: fmt.Sprintf("wrote %d bytes to %s", len(a.Content), a.Path)}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tools/ -run TestWrite`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tools/write.go internal/tools/write_test.go
git commit -m "feat(tools): write tool"
```

---

## Task 8: edit tool

**Files:**
- Create: `internal/tools/edit.go`
- Create: `internal/tools/edit_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tools/edit_test.go
package tools

import (
    "context"
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
)

func TestEdit_ReplacesUniqueString(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "x.txt")
    _ = os.WriteFile(p, []byte("alpha BETA gamma"), 0o644)
    args, _ := json.Marshal(map[string]string{"path": p, "old_string": "BETA", "new_string": "delta"})
    res, err := (Edit{}).Run(context.Background(), args)
    if err != nil {
        t.Fatal(err)
    }
    if res.IsError {
        t.Fatal(res.Output)
    }
    b, _ := os.ReadFile(p)
    if string(b) != "alpha delta gamma" {
        t.Fatalf("content = %q", b)
    }
}

func TestEdit_FailsOnAmbiguous(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "x.txt")
    _ = os.WriteFile(p, []byte("foo foo"), 0o644)
    args, _ := json.Marshal(map[string]string{"path": p, "old_string": "foo", "new_string": "bar"})
    res, _ := (Edit{}).Run(context.Background(), args)
    if !res.IsError {
        t.Fatal("expected IsError=true on ambiguous match")
    }
}

func TestEdit_FailsWhenNotFound(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "x.txt")
    _ = os.WriteFile(p, []byte("hello"), 0o644)
    args, _ := json.Marshal(map[string]string{"path": p, "old_string": "missing", "new_string": "nope"})
    res, _ := (Edit{}).Run(context.Background(), args)
    if !res.IsError {
        t.Fatal("expected IsError=true on no match")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tools/ -run TestEdit`
Expected: FAIL — Edit undefined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/tools/edit.go
package tools

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "strings"

    "github.com/khang859/rune/internal/ai"
)

type Edit struct{}

func (Edit) Spec() ai.ToolSpec {
    return ai.ToolSpec{
        Name:        "edit",
        Description: "Replace a unique exact-string occurrence in a file. Fails if old_string is missing or appears more than once.",
        Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "path":{"type":"string"},
                "old_string":{"type":"string"},
                "new_string":{"type":"string"}
            },
            "required":["path","old_string","new_string"]
        }`),
    }
}

func (Edit) Run(ctx context.Context, args json.RawMessage) (Result, error) {
    var a struct {
        Path      string `json:"path"`
        OldString string `json:"old_string"`
        NewString string `json:"new_string"`
    }
    if err := json.Unmarshal(args, &a); err != nil {
        return Result{Output: fmt.Sprintf("invalid args: %v", err), IsError: true}, nil
    }
    b, err := os.ReadFile(a.Path)
    if err != nil {
        return Result{Output: err.Error(), IsError: true}, nil
    }
    src := string(b)
    count := strings.Count(src, a.OldString)
    if count == 0 {
        return Result{Output: fmt.Sprintf("old_string not found in %s", a.Path), IsError: true}, nil
    }
    if count > 1 {
        return Result{Output: fmt.Sprintf("old_string appears %d times in %s — must be unique", count, a.Path), IsError: true}, nil
    }
    out := strings.Replace(src, a.OldString, a.NewString, 1)
    if err := os.WriteFile(a.Path, []byte(out), 0o644); err != nil {
        return Result{Output: err.Error(), IsError: true}, nil
    }
    return Result{Output: fmt.Sprintf("edited %s", a.Path)}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tools/ -run TestEdit`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tools/edit.go internal/tools/edit_test.go
git commit -m "feat(tools): edit tool"
```

---

## Task 9: bash tool

**Files:**
- Create: `internal/tools/bash.go`
- Create: `internal/tools/bash_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tools/bash_test.go
package tools

import (
    "context"
    "encoding/json"
    "strings"
    "testing"
    "time"
)

func TestBash_RunsCommand(t *testing.T) {
    args, _ := json.Marshal(map[string]any{"command": "echo hello"})
    res, err := (Bash{}).Run(context.Background(), args)
    if err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(res.Output, "hello") {
        t.Fatalf("output = %q", res.Output)
    }
    if res.IsError {
        t.Fatal("expected success")
    }
}

func TestBash_NonzeroExitIsErrorButOutputIncluded(t *testing.T) {
    args, _ := json.Marshal(map[string]any{"command": "echo nope; exit 7"})
    res, _ := (Bash{}).Run(context.Background(), args)
    if !res.IsError {
        t.Fatal("expected IsError=true on nonzero exit")
    }
    if !strings.Contains(res.Output, "nope") {
        t.Fatalf("output should include stdout: %q", res.Output)
    }
    if !strings.Contains(res.Output, "7") {
        t.Fatalf("output should mention exit code: %q", res.Output)
    }
}

func TestBash_ContextCancelKillsProcess(t *testing.T) {
    ctx, cancel := context.WithCancel(context.Background())
    args, _ := json.Marshal(map[string]any{"command": "sleep 5"})
    done := make(chan struct{})
    go func() {
        _, _ = (Bash{}).Run(ctx, args)
        close(done)
    }()
    time.Sleep(100 * time.Millisecond)
    cancel()
    select {
    case <-done:
    case <-time.After(2 * time.Second):
        t.Fatal("bash did not exit on ctx cancel")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tools/ -run TestBash`
Expected: FAIL — Bash undefined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/tools/bash.go
package tools

import (
    "bytes"
    "context"
    "encoding/json"
    "fmt"
    "os/exec"

    "github.com/khang859/rune/internal/ai"
)

type Bash struct{}

func (Bash) Spec() ai.ToolSpec {
    return ai.ToolSpec{
        Name:        "bash",
        Description: "Run a shell command. Returns combined stdout+stderr. Nonzero exit is an error result, not a Go error.",
        Schema: json.RawMessage(`{
            "type":"object",
            "properties":{"command":{"type":"string"}},
            "required":["command"]
        }`),
    }
}

func (Bash) Run(ctx context.Context, args json.RawMessage) (Result, error) {
    var a struct{ Command string `json:"command"` }
    if err := json.Unmarshal(args, &a); err != nil {
        return Result{Output: fmt.Sprintf("invalid args: %v", err), IsError: true}, nil
    }
    cmd := exec.CommandContext(ctx, "bash", "-lc", a.Command)
    var buf bytes.Buffer
    cmd.Stdout = &buf
    cmd.Stderr = &buf
    err := cmd.Run()
    out := buf.String()
    if err != nil {
        if ctx.Err() != nil {
            return Result{Output: out + "\n(canceled)", IsError: true}, nil
        }
        if ee, ok := err.(*exec.ExitError); ok {
            return Result{Output: fmt.Sprintf("%s\n(exit %d)", out, ee.ExitCode()), IsError: true}, nil
        }
        return Result{Output: out + "\n" + err.Error(), IsError: true}, nil
    }
    return Result{Output: out}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tools/ -run TestBash`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/tools/bash.go internal/tools/bash_test.go
git commit -m "feat(tools): bash tool"
```

---

## Task 10: Session — types and tree ops

**Files:**
- Create: `internal/session/session.go`
- Create: `internal/session/session_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/session/session_test.go
package session

import (
    "testing"

    "github.com/khang859/rune/internal/ai"
)

func userMsg(text string) ai.Message {
    return ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}}
}

func asstMsg(text string) ai.Message {
    return ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}}
}

func TestSession_AppendBuildsLinearPath(t *testing.T) {
    s := New("gpt-5")
    s.Append(userMsg("hi"))
    s.Append(asstMsg("hello"))
    s.Append(userMsg("again"))
    path := s.PathToActive()
    if len(path) != 3 {
        t.Fatalf("path len = %d", len(path))
    }
    if got := path[0].Content[0].(ai.TextBlock).Text; got != "hi" {
        t.Fatalf("path[0] text = %q", got)
    }
    if got := path[2].Content[0].(ai.TextBlock).Text; got != "again" {
        t.Fatalf("path[2] text = %q", got)
    }
}

func TestSession_ForkCreatesBranch(t *testing.T) {
    s := New("gpt-5")
    s.Append(userMsg("a"))
    a := s.Active
    s.Append(asstMsg("first reply"))
    s.Append(userMsg("b1"))
    // Fork back to "a" and add a different child.
    s.Fork(a)
    s.Append(asstMsg("second reply"))
    if len(a.Children) != 2 {
        t.Fatalf("a.Children len = %d", len(a.Children))
    }
    path := s.PathToActive()
    last := path[len(path)-1].Content[0].(ai.TextBlock).Text
    if last != "second reply" {
        t.Fatalf("last on active branch = %q", last)
    }
}

func TestSession_CloneCopiesActiveBranch(t *testing.T) {
    s := New("gpt-5")
    s.Append(userMsg("a"))
    s.Append(asstMsg("b"))
    c := s.Clone()
    if c.ID == s.ID {
        t.Fatal("cloned session must have a different ID")
    }
    if len(c.PathToActive()) != 2 {
        t.Fatalf("cloned path len = %d", len(c.PathToActive()))
    }
    // Mutating original must not affect clone.
    s.Append(userMsg("c"))
    if len(c.PathToActive()) != 2 {
        t.Fatal("clone aliased to original")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/session/...`
Expected: FAIL — package does not compile

- [ ] **Step 3: Write minimal implementation**

```go
// internal/session/session.go
package session

import (
    "crypto/rand"
    "encoding/hex"
    "time"

    "github.com/khang859/rune/internal/ai"
)

type Session struct {
    ID      string
    Name    string
    Created time.Time
    Model   string
    Root    *Node
    Active  *Node
    path    string
}

type Node struct {
    ID       string
    Parent   *Node `json:"-"`
    Children []*Node
    Message  ai.Message
    Usage    ai.Usage
    Created  time.Time
}

func New(model string) *Session {
    root := &Node{ID: newID(), Created: time.Now()}
    return &Session{
        ID:      newID(),
        Created: time.Now(),
        Model:   model,
        Root:    root,
        Active:  root,
    }
}

// SetPath assigns the file path used by Save. Callers in cmd/rune use this
// to place sessions under ~/.rune/sessions; tests use it to point at a temp dir.
func (s *Session) SetPath(p string) { s.path = p }
func (s *Session) Path() string     { return s.path }

func (s *Session) Append(msg ai.Message) *Node {
    n := &Node{
        ID:      newID(),
        Parent:  s.Active,
        Message: msg,
        Created: time.Now(),
    }
    s.Active.Children = append(s.Active.Children, n)
    s.Active = n
    return n
}

func (s *Session) Fork(target *Node) {
    s.Active = target
}

func (s *Session) Clone() *Session {
    nc := New(s.Model)
    nc.Name = s.Name
    // Copy the active path: walk up to root, reverse, replay Append.
    var msgs []ai.Message
    for n := s.Active; n != nil && n.Parent != nil; n = n.Parent {
        msgs = append([]ai.Message{n.Message}, msgs...)
    }
    for _, m := range msgs {
        nc.Append(m)
    }
    return nc
}

// PathToActive returns the messages from the first child of root down to Active (excluding root).
func (s *Session) PathToActive() []ai.Message {
    var msgs []ai.Message
    for n := s.Active; n != nil && n.Parent != nil; n = n.Parent {
        msgs = append([]ai.Message{n.Message}, msgs...)
    }
    return msgs
}

func newID() string {
    b := make([]byte, 8)
    _, _ = rand.Read(b)
    return hex.EncodeToString(b)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/session/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/session/session.go internal/session/session_test.go
git commit -m "feat(session): branching tree with append/fork/clone"
```

---

## Task 11: Session — atomic JSON persistence

**Files:**
- Create: `internal/session/persist.go`
- Create: `internal/session/persist_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/session/persist_test.go
package session

import (
    "os"
    "path/filepath"
    "testing"

    "github.com/khang859/rune/internal/ai"
)

func TestSave_AndLoad_RoundTrip(t *testing.T) {
    dir := t.TempDir()
    s := New("gpt-5")
    s.Name = "demo"
    s.SetPath(filepath.Join(dir, s.ID+".json"))
    s.Append(userMsg("hi"))
    s.Append(asstMsg("hello"))

    if err := s.Save(); err != nil {
        t.Fatal(err)
    }

    loaded, err := Load(s.path)
    if err != nil {
        t.Fatal(err)
    }
    if loaded.ID != s.ID {
        t.Fatalf("id mismatch")
    }
    if loaded.Name != "demo" {
        t.Fatalf("name = %q", loaded.Name)
    }
    if got := len(loaded.PathToActive()); got != 2 {
        t.Fatalf("path len = %d", got)
    }
    // Parent pointers must be reconstructed.
    if loaded.Active.Parent == nil || loaded.Active.Parent.Parent == nil {
        t.Fatal("parent pointers not reconstructed")
    }
    if loaded.Active.Parent.Parent != loaded.Root {
        t.Fatal("parent pointers do not chain to root")
    }
}

func TestSave_IsAtomic(t *testing.T) {
    // After Save, the temp file must be gone — only the final file exists.
    dir := t.TempDir()
    s := New("gpt-5")
    s.SetPath(filepath.Join(dir, s.ID+".json"))
    s.Append(userMsg("hi"))
    if err := s.Save(); err != nil {
        t.Fatal(err)
    }
    entries, _ := os.ReadDir(dir)
    if len(entries) != 1 {
        t.Fatalf("expected 1 file, got %d", len(entries))
    }
}

// keep ai import alive for test compilation
var _ = ai.RoleUser
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/session/ -run TestSave`
Expected: FAIL — `Save`/`Load` undefined

- [ ] **Step 3: Write minimal implementation**

```go
// internal/session/persist.go
package session

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"

    "github.com/khang859/rune/internal/ai"
)

type wireSession struct {
    ID       string     `json:"id"`
    Name     string     `json:"name,omitempty"`
    Created  string     `json:"created"`
    Model    string     `json:"model"`
    RootID   string     `json:"root_id"`
    ActiveID string     `json:"active_id"`
    Nodes    []wireNode `json:"nodes"`
}

type wireNode struct {
    ID         string     `json:"id"`
    ParentID   string     `json:"parent_id,omitempty"`
    ChildIDs   []string   `json:"children,omitempty"`
    Message    ai.Message `json:"message,omitempty"`
    HasMessage bool       `json:"has_message"`
    Usage      ai.Usage   `json:"usage,omitempty"`
    Created    string     `json:"created"`
}

func (s *Session) Save() error {
    if s.path == "" {
        return fmt.Errorf("session path is empty; set with SetPath or Load")
    }
    if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
        return err
    }
    w := wireSession{
        ID:       s.ID,
        Name:     s.Name,
        Created:  s.Created.Format("2006-01-02T15:04:05Z07:00"),
        Model:    s.Model,
        RootID:   s.Root.ID,
        ActiveID: s.Active.ID,
    }
    walk(s.Root, func(n *Node) {
        wn := wireNode{
            ID:      n.ID,
            Usage:   n.Usage,
            Created: n.Created.Format("2006-01-02T15:04:05Z07:00"),
        }
        if n.Parent != nil {
            wn.ParentID = n.Parent.ID
        }
        for _, c := range n.Children {
            wn.ChildIDs = append(wn.ChildIDs, c.ID)
        }
        if n != s.Root {
            wn.Message = n.Message
            wn.HasMessage = true
        }
        w.Nodes = append(w.Nodes, wn)
    })
    b, err := json.MarshalIndent(w, "", "  ")
    if err != nil {
        return err
    }
    tmp := s.path + ".tmp"
    f, err := os.Create(tmp)
    if err != nil {
        return err
    }
    if _, err := f.Write(b); err != nil {
        f.Close()
        os.Remove(tmp)
        return err
    }
    if err := f.Sync(); err != nil {
        f.Close()
        os.Remove(tmp)
        return err
    }
    if err := f.Close(); err != nil {
        os.Remove(tmp)
        return err
    }
    return os.Rename(tmp, s.path)
}

func Load(path string) (*Session, error) {
    b, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var w wireSession
    if err := json.Unmarshal(b, &w); err != nil {
        return nil, err
    }
    nodes := map[string]*Node{}
    for _, wn := range w.Nodes {
        n := &Node{ID: wn.ID, Usage: wn.Usage}
        if wn.HasMessage {
            n.Message = wn.Message
        }
        nodes[wn.ID] = n
    }
    for _, wn := range w.Nodes {
        n := nodes[wn.ID]
        if wn.ParentID != "" {
            n.Parent = nodes[wn.ParentID]
        }
        for _, cid := range wn.ChildIDs {
            n.Children = append(n.Children, nodes[cid])
        }
    }
    return &Session{
        ID:     w.ID,
        Name:   w.Name,
        Model:  w.Model,
        Root:   nodes[w.RootID],
        Active: nodes[w.ActiveID],
        path:   path,
    }, nil
}

func walk(n *Node, fn func(*Node)) {
    fn(n)
    for _, c := range n.Children {
        walk(c, fn)
    }
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/session/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/session/persist.go internal/session/persist_test.go
git commit -m "feat(session): atomic JSON persistence with parent reconstruction"
```

---

## Task 12: Agent — single-turn no-tools loop

**Files:**
- Create: `internal/agent/events.go`
- Create: `internal/agent/agent.go`
- Create: `internal/agent/loop.go`
- Create: `internal/agent/loop_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/agent/loop_test.go
package agent

import (
    "context"
    "testing"

    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/ai/faux"
    "github.com/khang859/rune/internal/session"
    "github.com/khang859/rune/internal/tools"
)

func userMsg(s string) ai.Message {
    return ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: s}}}
}

func collect(t *testing.T, ch <-chan Event) []Event {
    t.Helper()
    var out []Event
    for e := range ch {
        out = append(out, e)
    }
    return out
}

func TestRun_TextOnlyTurn(t *testing.T) {
    f := faux.New().Reply("hi there").Done()
    s := session.New("gpt-5")
    a := New(f, tools.NewRegistry(), s, "system")
    evs := collect(t, a.Run(context.Background(), userMsg("hello")))

    var sawText, sawDone bool
    for _, e := range evs {
        switch v := e.(type) {
        case AssistantText:
            if v.Delta == "hi there" {
                sawText = true
            }
        case TurnDone:
            if v.Reason == "stop" {
                sawDone = true
            }
        }
    }
    if !sawText {
        t.Fatal("missing AssistantText")
    }
    if !sawDone {
        t.Fatal("missing TurnDone")
    }
    // Session must contain user msg + assistant msg.
    if got := len(s.PathToActive()); got != 2 {
        t.Fatalf("path len = %d", got)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/...`
Expected: FAIL — package does not compile

- [ ] **Step 3: Write minimal implementations**

```go
// internal/agent/events.go
package agent

import (
    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/tools"
)

type Event interface{ isEvent() }

type AssistantText   struct{ Delta string }
type ThinkingText    struct{ Delta string }
type ToolStarted     struct{ Call ai.ToolCall }
type ToolFinished    struct{ Call ai.ToolCall; Result tools.Result }
type TurnUsage       struct{ Usage ai.Usage; Cost float64 }
type ContextOverflow struct{}
type TurnAborted     struct{}
type TurnDone        struct{ Reason string }
type TurnError       struct{ Err error }

func (AssistantText) isEvent()   {}
func (ThinkingText) isEvent()    {}
func (ToolStarted) isEvent()     {}
func (ToolFinished) isEvent()    {}
func (TurnUsage) isEvent()       {}
func (ContextOverflow) isEvent() {}
func (TurnAborted) isEvent()     {}
func (TurnDone) isEvent()        {}
func (TurnError) isEvent()       {}
```

```go
// internal/agent/agent.go
package agent

import (
    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/session"
    "github.com/khang859/rune/internal/tools"
)

type Agent struct {
    provider ai.Provider
    tools    *tools.Registry
    session  *session.Session
    system   string
}

func New(p ai.Provider, t *tools.Registry, s *session.Session, systemPrompt string) *Agent {
    return &Agent{provider: p, tools: t, session: s, system: systemPrompt}
}
```

```go
// internal/agent/loop.go
package agent

import (
    "context"
    "errors"
    "strings"

    "github.com/khang859/rune/internal/ai"
)

const eventBuffer = 64

func (a *Agent) Run(ctx context.Context, userMsg ai.Message) <-chan Event {
    out := make(chan Event, eventBuffer)
    a.session.Append(userMsg)
    go func() {
        defer close(out)
        defer func() {
            if r := recover(); r != nil {
                out <- TurnError{Err: panicErr(r)}
            }
        }()
        a.runTurn(ctx, out)
    }()
    return out
}

func (a *Agent) runTurn(ctx context.Context, out chan<- Event) {
    for {
        req := ai.Request{
            Model:    a.session.Model,
            System:   a.system,
            Messages: a.session.PathToActive(),
            Tools:    a.tools.Specs(),
        }
        events, err := a.provider.Stream(ctx, req)
        if err != nil {
            out <- TurnError{Err: err}
            return
        }

        var (
            assistantText strings.Builder
            toolCalls     []ai.ToolCall
            usage         ai.Usage
        )

        for ev := range events {
            switch v := ev.(type) {
            case ai.TextDelta:
                assistantText.WriteString(v.Text)
                out <- AssistantText{Delta: v.Text}
            case ai.Thinking:
                out <- ThinkingText{Delta: v.Text}
            case ai.ToolCall:
                toolCalls = append(toolCalls, v)
            case ai.Usage:
                usage = v
                out <- TurnUsage{Usage: v}
            case ai.StreamError:
                out <- TurnError{Err: v.Err}
                return
            case ai.Done:
                if v.Reason == "context_overflow" {
                    out <- ContextOverflow{}
                    // For now: end the turn. Auto-compact lands in Plan 05.
                    out <- TurnDone{Reason: "context_overflow"}
                    return
                }
                a.persistAssistant(assistantText.String(), toolCalls, usage)
                if len(toolCalls) == 0 {
                    out <- TurnDone{Reason: v.Reason}
                    return
                }
                if err := a.runTools(ctx, toolCalls, out); err != nil {
                    if errors.Is(err, context.Canceled) {
                        out <- TurnAborted{}
                        return
                    }
                    out <- TurnError{Err: err}
                    return
                }
                // continue outer loop for next provider call
            }
        }
        if ctx.Err() != nil {
            out <- TurnAborted{}
            return
        }
    }
}

func (a *Agent) persistAssistant(text string, calls []ai.ToolCall, usage ai.Usage) {
    var content []ai.ContentBlock
    if text != "" {
        content = append(content, ai.TextBlock{Text: text})
    }
    for _, c := range calls {
        content = append(content, ai.ToolUseBlock{ID: c.ID, Name: c.Name, Args: c.Args})
    }
    n := a.session.Append(ai.Message{Role: ai.RoleAssistant, Content: content})
    n.Usage = usage
}

func (a *Agent) runTools(ctx context.Context, calls []ai.ToolCall, out chan<- Event) error {
    for _, call := range calls {
        out <- ToolStarted{Call: call}
        res, err := a.tools.Run(ctx, call)
        if err != nil {
            return err
        }
        a.session.Append(ai.Message{
            Role:       ai.RoleToolResult,
            ToolCallID: call.ID,
            Content:    []ai.ContentBlock{ai.ToolResultBlock{ToolCallID: call.ID, Output: res.Output, IsError: res.IsError}},
        })
        out <- ToolFinished{Call: call, Result: res}
    }
    return nil
}

func panicErr(r any) error {
    if e, ok := r.(error); ok {
        return e
    }
    return errors.New("agent panic")
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/...`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/agent/
git commit -m "feat(agent): single-turn loop with text + session persistence"
```

---

## Task 13: Agent — multi-turn tool dispatch

**Files:**
- Modify: `internal/agent/loop_test.go` (add tests)

- [ ] **Step 1: Add the failing tests**

```go
// append to internal/agent/loop_test.go

import "encoding/json"

func TestRun_DispatchesToolThenContinues(t *testing.T) {
    f := faux.New().
        CallTool("read", `{"path":"/tmp/x"}`).Done().
        Reply("file said hi").Done()
    s := session.New("gpt-5")
    reg := tools.NewRegistry()
    reg.Register(stubReadTool{output: "hi"})
    a := New(f, reg, s, "")
    evs := collect(t, a.Run(context.Background(), userMsg("look at /tmp/x")))

    var started, finished, doneStop bool
    for _, e := range evs {
        switch v := e.(type) {
        case ToolStarted:
            if v.Call.Name == "read" {
                started = true
            }
        case ToolFinished:
            if v.Result.Output == "hi" {
                finished = true
            }
        case TurnDone:
            if v.Reason == "stop" {
                doneStop = true
            }
        }
    }
    if !started || !finished || !doneStop {
        t.Fatalf("missing events: started=%v finished=%v done=%v", started, finished, doneStop)
    }

    // Session: user, assistant(tool_use), tool_result, assistant(text)
    path := s.PathToActive()
    if len(path) != 4 {
        t.Fatalf("path len = %d, want 4", len(path))
    }
    if path[1].Role != ai.RoleAssistant {
        t.Fatalf("path[1] role = %s", path[1].Role)
    }
    if path[2].Role != ai.RoleToolResult {
        t.Fatalf("path[2] role = %s", path[2].Role)
    }
}

type stubReadTool struct{ output string }

func (stubReadTool) Spec() ai.ToolSpec {
    return ai.ToolSpec{Name: "read", Schema: json.RawMessage(`{}`)}
}
func (s stubReadTool) Run(ctx context.Context, args json.RawMessage) (tools.Result, error) {
    return tools.Result{Output: s.output}, nil
}
```

- [ ] **Step 2: Run tests to verify they pass**

Run: `go test ./internal/agent/...`
Expected: PASS — already wired in Task 12, this is a regression test for the multi-turn path.

- [ ] **Step 3: Commit**

```bash
git add internal/agent/loop_test.go
git commit -m "test(agent): multi-turn tool dispatch end-to-end"
```

---

## Task 14: Agent — abort via ctx cancel

**Files:**
- Modify: `internal/agent/loop_test.go` (add test)

- [ ] **Step 1: Add the failing test**

```go
// append to internal/agent/loop_test.go
import (
    "time"
)

type slowProvider struct{}

func (slowProvider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
    out := make(chan ai.Event)
    go func() {
        defer close(out)
        select {
        case <-ctx.Done():
            return
        case <-time.After(5 * time.Second):
            out <- ai.Done{Reason: "stop"}
        }
    }()
    return out, nil
}

func TestRun_AbortViaCtx(t *testing.T) {
    s := session.New("gpt-5")
    a := New(slowProvider{}, tools.NewRegistry(), s, "")
    ctx, cancel := context.WithCancel(context.Background())
    ch := a.Run(ctx, userMsg("anything"))

    go func() {
        time.Sleep(50 * time.Millisecond)
        cancel()
    }()

    deadline := time.After(2 * time.Second)
    var sawAbort bool
    for {
        select {
        case e, ok := <-ch:
            if !ok {
                if !sawAbort {
                    t.Fatal("channel closed without TurnAborted")
                }
                return
            }
            if _, ok := e.(TurnAborted); ok {
                sawAbort = true
            }
        case <-deadline:
            t.Fatal("agent did not abort within deadline")
        }
    }
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./internal/agent/ -run TestRun_AbortViaCtx -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/agent/loop_test.go
git commit -m "test(agent): abort on ctx cancel"
```

---

## Task 15: Agent — context overflow surfaces ContextOverflow event

**Files:**
- Modify: `internal/agent/loop_test.go` (add test)

- [ ] **Step 1: Add the failing test**

```go
// append to internal/agent/loop_test.go

func TestRun_ContextOverflow(t *testing.T) {
    f := faux.New().Reply("partial").DoneOverflow()
    s := session.New("gpt-5")
    a := New(f, tools.NewRegistry(), s, "")
    evs := collect(t, a.Run(context.Background(), userMsg("hi")))

    var sawOverflow, sawDoneOverflow bool
    for _, e := range evs {
        switch v := e.(type) {
        case ContextOverflow:
            sawOverflow = true
        case TurnDone:
            if v.Reason == "context_overflow" {
                sawDoneOverflow = true
            }
        }
    }
    if !sawOverflow || !sawDoneOverflow {
        t.Fatalf("overflow=%v doneOverflow=%v", sawOverflow, sawDoneOverflow)
    }
}
```

- [ ] **Step 2: Run test to verify it passes**

Run: `go test ./internal/agent/ -run TestRun_ContextOverflow -v`
Expected: PASS — already wired in Task 12.

- [ ] **Step 3: Commit**

```bash
git add internal/agent/loop_test.go
git commit -m "test(agent): context overflow surfaces ContextOverflow event"
```

---

## Task 16: Headless smoke runner (`rune --script`)

**Files:**
- Modify: `cmd/rune/main.go`
- Create: `cmd/rune/script.go`
- Create: `cmd/rune/script_test.go`
- Create: `cmd/rune/testdata/hello.script.json`

- [ ] **Step 1: Write the script-runner test**

```go
// cmd/rune/script_test.go
package main

import (
    "bytes"
    "context"
    "encoding/json"
    "os"
    "path/filepath"
    "strings"
    "testing"

    "github.com/khang859/rune/internal/ai/faux"
)

func TestRunScript_FauxTextTurn(t *testing.T) {
    dir := t.TempDir()
    sessPath := filepath.Join(dir, "s.json")

    sc := scriptFile{
        Provider: "faux",
        Session:  sessPath,
        Model:    "gpt-5",
        Faux: []fauxStep{
            {Reply: "hello back"},
            {Done: true},
        },
        UserMessage: "hi",
    }
    b, _ := json.Marshal(sc)
    scriptPath := filepath.Join(dir, "in.json")
    _ = os.WriteFile(scriptPath, b, 0o644)

    var out bytes.Buffer
    if err := runScript(context.Background(), scriptPath, &out, faux.New()); err != nil {
        t.Fatal(err)
    }
    if !strings.Contains(out.String(), "hello back") {
        t.Fatalf("output missing assistant text: %q", out.String())
    }
    if _, err := os.Stat(sessPath); err != nil {
        t.Fatalf("session file not written: %v", err)
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./cmd/rune/...`
Expected: FAIL — runScript / scriptFile / fauxStep undefined.

- [ ] **Step 3: Implement script.go**

```go
// cmd/rune/script.go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "os"

    "github.com/khang859/rune/internal/agent"
    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/ai/faux"
    "github.com/khang859/rune/internal/session"
    "github.com/khang859/rune/internal/tools"
)

type scriptFile struct {
    Provider    string     `json:"provider"`     // "faux" only in plan 01
    Session     string     `json:"session"`      // path to session JSON
    Model       string     `json:"model"`
    Faux        []fauxStep `json:"faux"`
    UserMessage string     `json:"user_message"`
}

type fauxStep struct {
    Reply    string          `json:"reply,omitempty"`
    Thinking string          `json:"thinking,omitempty"`
    Tool     *fauxToolCall   `json:"tool,omitempty"`
    Done     bool            `json:"done,omitempty"`
    Overflow bool            `json:"overflow,omitempty"`
}

type fauxToolCall struct {
    Name string          `json:"name"`
    Args json.RawMessage `json:"args"`
}

// runScript drives one turn from the script and writes a transcript to w.
// fauxBase is injected for testability; pass faux.New() in main.
func runScript(ctx context.Context, path string, w io.Writer, _ *faux.Faux) error {
    b, err := os.ReadFile(path)
    if err != nil {
        return err
    }
    var sc scriptFile
    if err := json.Unmarshal(b, &sc); err != nil {
        return err
    }

    f := faux.New()
    for _, st := range sc.Faux {
        switch {
        case st.Reply != "":
            f.Reply(st.Reply)
        case st.Thinking != "":
            f.Thinking(st.Thinking)
        case st.Tool != nil:
            f.CallTool(st.Tool.Name, string(st.Tool.Args))
        case st.Overflow:
            f.DoneOverflow()
        case st.Done:
            f.Done()
        }
    }

    sess := session.New(sc.Model)
    if sc.Session != "" {
        sess.SetPath(sc.Session)
    }
    reg := tools.NewRegistry()
    reg.Register(tools.Read{})
    reg.Register(tools.Write{})
    reg.Register(tools.Edit{})
    reg.Register(tools.Bash{})

    a := agent.New(f, reg, sess, "")
    msg := ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: sc.UserMessage}}}
    for ev := range a.Run(ctx, msg) {
        switch v := ev.(type) {
        case agent.AssistantText:
            fmt.Fprint(w, v.Delta)
        case agent.ToolStarted:
            fmt.Fprintf(w, "\n[tool start: %s]", v.Call.Name)
        case agent.ToolFinished:
            fmt.Fprintf(w, "\n[tool done: %s -> %q]", v.Call.Name, truncate(v.Result.Output, 80))
        case agent.TurnError:
            fmt.Fprintf(w, "\n[error: %v]", v.Err)
        case agent.TurnDone:
            fmt.Fprintf(w, "\n[done: %s]", v.Reason)
        }
    }
    if sc.Session != "" {
        if err := sess.Save(); err != nil {
            return fmt.Errorf("save session: %w", err)
        }
    }
    return nil
}

func truncate(s string, n int) string {
    if len(s) <= n {
        return s
    }
    return s[:n] + "…"
}
```

- [ ] **Step 4: Wire `--script` flag into main**

```go
// cmd/rune/main.go (replace)
package main

import (
    "context"
    "flag"
    "fmt"
    "os"

    "github.com/khang859/rune/internal/ai/faux"
)

const version = "0.0.0-dev"

func main() {
    script := flag.String("script", "", "run a JSON script (headless smoke runner)")
    flag.Parse()

    if *script != "" {
        if err := runScript(context.Background(), *script, os.Stdout, faux.New()); err != nil {
            fmt.Fprintln(os.Stderr, "error:", err)
            os.Exit(1)
        }
        return
    }
    fmt.Println("rune", version)
}
```

- [ ] **Step 5: Run tests + smoke**

```bash
go test ./...
```
Expected: all PASS.

```bash
mkdir -p cmd/rune/testdata
cat > cmd/rune/testdata/hello.script.json <<'EOF'
{
  "provider":"faux",
  "session":"/tmp/rune-smoke.json",
  "model":"gpt-5",
  "user_message":"hi",
  "faux":[
    {"reply":"hello back"},
    {"done":true}
  ]
}
EOF
go run ./cmd/rune --script cmd/rune/testdata/hello.script.json
```
Expected: prints `hello back` followed by `[done: stop]`.

- [ ] **Step 6: Commit**

```bash
git add cmd/rune/ go.mod go.sum
git commit -m "feat(cmd): headless --script runner via faux provider"
```

---

## Task 17: Final CI gate

**Files:**
- Create: `Makefile`

- [ ] **Step 1: Write the Makefile**

```makefile
.PHONY: test vet fmt build all

build:
	go build -o rune ./cmd/rune

test:
	go test ./...

vet:
	go vet ./...

fmt:
	gofmt -l .

all: vet fmt test build
```

- [ ] **Step 2: Run the full gate**

Run: `make all`
Expected: empty `gofmt -l` output, all tests pass, binary builds.

- [ ] **Step 3: Commit**

```bash
git add Makefile
git commit -m "chore: makefile with vet/fmt/test/build gate"
```

---

## End state

After Plan 01, rune has:

- A buildable `rune` binary that responds to `--script <path>`.
- `internal/ai` types covering messages, tools, events, providers.
- `internal/ai/faux` for end-to-end testing without network.
- `internal/tools` with `read`, `write`, `edit`, `bash` (each with isolated tests).
- `internal/session` with branching tree, atomic JSON persistence, fork/clone/path-to-active.
- `internal/agent` with the full turn loop: text streaming, tool dispatch, abort via `ctx`, context-overflow event, panic recovery.
- A Makefile gate (`make all`).

Plan 02 swaps `faux` for a real Codex provider while keeping the same agent loop and tests.
