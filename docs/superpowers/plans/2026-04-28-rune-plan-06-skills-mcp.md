# rune Plan 06 — Skills + MCP

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Land the two extension surfaces from the spec: **markdown skills** (drop a `.md` file into `~/.rune/skills/` or `./.rune/skills/`, surfaces as `/skill:name` and prepends the body to the next user message) and **MCP plugins** (stdio JSON-RPC servers configured via `~/.rune/mcp.json`; their tools register alongside built-ins).

**Architecture:** `internal/skill` walks two skill roots and produces a `[]Skill`; the root model registers each as `/skill:<name>` in the slash menu and stores a pending-skill body that prepends to the next user submission. `internal/mcp` is a stdio JSON-RPC 2.0 client (`initialize`, `tools/list`, `tools/call`) per server. Each `mcp.Server` exposes its tools via the same `tools.Tool` interface, so the agent's `tools.Registry` doesn't need to know they're remote. A `Manager` owns the lifecycle: spawn on startup, kill on shutdown, restart on crash (with backoff).

**Tech Stack:** Standard library only (`encoding/json`, `os/exec`, `bufio`). MCP protocol version targeted: `2024-11-05` (current stable).

**Spec:** `docs/superpowers/specs/2026-04-28-rune-coding-agent-design.md`

---

## File Structure

```
internal/skill/
├── skill.go               # Skill type + Loader
├── loader.go              # walks roots
└── skill_test.go
internal/mcp/
├── mcp.go                 # protocol types
├── client.go              # stdio JSON-RPC client
├── client_test.go         # against a stub server (Go binary built per-test)
├── tool.go                # mcp tool wrapper implementing tools.Tool
├── tool_test.go
├── manager.go             # spawn/restart/shutdown
├── manager_test.go
└── testdata/
    └── stub_server.go     # minimal MCP server used by tests
internal/tui/
└── root.go                # registers skills + MCP tools, handles /skill:name
internal/config/
└── paths.go               # SkillsDir, MCPConfig already exist (Plan 01)
```

---

## Task 1: Skill type + loader

**Files:**
- Create: `internal/skill/skill.go`
- Create: `internal/skill/loader.go`
- Create: `internal/skill/skill_test.go`

> A skill file is plain markdown. The slug is derived from the filename: `~/.rune/skills/refactor-step.md` → `refactor-step`. Skills override built-ins on slug collision: project-local (`./.rune/skills`) wins over user-global (`~/.rune/skills`).

- [ ] **Step 1: Write the failing test**

```go
// internal/skill/skill_test.go
package skill

import (
    "os"
    "path/filepath"
    "testing"
)

func TestLoader_FindsAcrossRoots(t *testing.T) {
    home := t.TempDir()
    proj := t.TempDir()
    _ = os.WriteFile(filepath.Join(home, "alpha.md"),     []byte("ALPHA-HOME"), 0o644)
    _ = os.WriteFile(filepath.Join(home, "shared.md"),    []byte("SHARED-HOME"), 0o644)
    _ = os.WriteFile(filepath.Join(proj, "shared.md"),    []byte("SHARED-PROJ"), 0o644)
    _ = os.WriteFile(filepath.Join(proj, "beta.md"),      []byte("BETA-PROJ"), 0o644)

    sks, err := (&Loader{Roots: []string{home, proj}}).Load()
    if err != nil {
        t.Fatal(err)
    }

    by := map[string]string{}
    for _, s := range sks {
        by[s.Slug] = s.Body
    }
    // Project-local "shared" overrides home.
    if by["shared"] != "SHARED-PROJ" {
        t.Fatalf("override failed: shared=%q", by["shared"])
    }
    if by["alpha"] != "ALPHA-HOME" {
        t.Fatalf("home alpha = %q", by["alpha"])
    }
    if by["beta"] != "BETA-PROJ" {
        t.Fatalf("proj beta = %q", by["beta"])
    }
    if len(by) != 3 {
        t.Fatalf("expected 3 unique slugs, got %d: %v", len(by), by)
    }
}

func TestLoader_IgnoresNonMarkdown(t *testing.T) {
    dir := t.TempDir()
    _ = os.WriteFile(filepath.Join(dir, "x.txt"),  []byte("nope"), 0o644)
    _ = os.WriteFile(filepath.Join(dir, "y.md"),   []byte("yes"), 0o644)
    sks, _ := (&Loader{Roots: []string{dir}}).Load()
    if len(sks) != 1 {
        t.Fatalf("len = %d", len(sks))
    }
}

func TestLoader_MissingRootIsNoOp(t *testing.T) {
    sks, err := (&Loader{Roots: []string{"/does/not/exist"}}).Load()
    if err != nil { t.Fatal(err) }
    if len(sks) != 0 { t.Fatalf("len = %d", len(sks)) }
}
```

- [ ] **Step 2: Implement**

```go
// internal/skill/skill.go
package skill

type Skill struct {
    Slug string
    Path string
    Body string
}
```

```go
// internal/skill/loader.go
package skill

import (
    "os"
    "path/filepath"
    "strings"
)

type Loader struct {
    // Roots are walked in order; later roots override earlier ones on slug collision.
    Roots []string
}

func (l *Loader) Load() ([]Skill, error) {
    by := map[string]Skill{}
    for _, root := range l.Roots {
        entries, err := os.ReadDir(root)
        if err != nil {
            if os.IsNotExist(err) { continue }
            return nil, err
        }
        for _, e := range entries {
            if e.IsDir() { continue }
            if filepath.Ext(e.Name()) != ".md" { continue }
            p := filepath.Join(root, e.Name())
            b, err := os.ReadFile(p)
            if err != nil { continue }
            slug := strings.TrimSuffix(e.Name(), ".md")
            by[slug] = Skill{Slug: slug, Path: p, Body: string(b)}
        }
    }
    out := make([]Skill, 0, len(by))
    for _, s := range by {
        out = append(out, s)
    }
    return out, nil
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/skill/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/skill/
git commit -m "feat(skill): markdown skill loader with project-overrides-home"
```

---

## Task 2: MCP wire types

**Files:**
- Create: `internal/mcp/mcp.go`

- [ ] **Step 1: Implement (no test — pure types)**

```go
// internal/mcp/mcp.go
package mcp

import "encoding/json"

const ProtocolVersion = "2024-11-05"

type Request struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      json.RawMessage `json:"id,omitempty"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
    JSONRPC string          `json:"jsonrpc"`
    ID      json.RawMessage `json:"id,omitempty"`
    Result  json.RawMessage `json:"result,omitempty"`
    Error   *Error          `json:"error,omitempty"`
}

type Notification struct {
    JSONRPC string          `json:"jsonrpc"`
    Method  string          `json:"method"`
    Params  json.RawMessage `json:"params,omitempty"`
}

type Error struct {
    Code    int             `json:"code"`
    Message string          `json:"message"`
    Data    json.RawMessage `json:"data,omitempty"`
}

type InitializeParams struct {
    ProtocolVersion string             `json:"protocolVersion"`
    ClientInfo      map[string]string  `json:"clientInfo"`
    Capabilities    map[string]any     `json:"capabilities"`
}

type Tool struct {
    Name        string          `json:"name"`
    Description string          `json:"description,omitempty"`
    InputSchema json.RawMessage `json:"inputSchema"`
}

type ToolsListResult struct {
    Tools []Tool `json:"tools"`
}

type ToolsCallParams struct {
    Name      string          `json:"name"`
    Arguments json.RawMessage `json:"arguments"`
}

type ToolsCallResult struct {
    Content []ContentItem `json:"content"`
    IsError bool          `json:"isError,omitempty"`
}

type ContentItem struct {
    Type string `json:"type"`
    Text string `json:"text,omitempty"`
    Data string `json:"data,omitempty"`
    MIME string `json:"mimeType,omitempty"`
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/mcp/mcp.go
git commit -m "feat(mcp): jsonrpc + protocol types"
```

---

## Task 3: stub MCP server (testdata binary)

**Files:**
- Create: `internal/mcp/testdata/stub_server.go`

> The stub is a tiny standalone Go program that implements `initialize`, `tools/list`, and `tools/call` for a single tool named `echo`. Tests build it with `go build` into `t.TempDir()` and spawn it as the MCP server.

- [ ] **Step 1: Implement**

```go
// internal/mcp/testdata/stub_server.go
//go:build ignore

package main

import (
    "bufio"
    "encoding/json"
    "fmt"
    "os"
)

func main() {
    in := bufio.NewReader(os.Stdin)
    enc := json.NewEncoder(os.Stdout)
    for {
        line, err := in.ReadString('\n')
        if err != nil {
            return
        }
        var req struct {
            JSONRPC string          `json:"jsonrpc"`
            ID      json.RawMessage `json:"id,omitempty"`
            Method  string          `json:"method"`
            Params  json.RawMessage `json:"params,omitempty"`
        }
        if err := json.Unmarshal([]byte(line), &req); err != nil {
            continue
        }
        switch req.Method {
        case "initialize":
            _ = enc.Encode(map[string]any{
                "jsonrpc": "2.0",
                "id":      json.RawMessage(req.ID),
                "result": map[string]any{
                    "protocolVersion": "2024-11-05",
                    "serverInfo":      map[string]string{"name": "stub", "version": "0.0.1"},
                    "capabilities":    map[string]any{"tools": map[string]any{}},
                },
            })
        case "tools/list":
            _ = enc.Encode(map[string]any{
                "jsonrpc": "2.0",
                "id":      json.RawMessage(req.ID),
                "result": map[string]any{
                    "tools": []map[string]any{
                        {
                            "name":        "echo",
                            "description": "echoes back its argument",
                            "inputSchema": map[string]any{
                                "type":       "object",
                                "properties": map[string]any{"text": map[string]any{"type": "string"}},
                                "required":   []string{"text"},
                            },
                        },
                    },
                },
            })
        case "tools/call":
            var p struct {
                Name      string          `json:"name"`
                Arguments json.RawMessage `json:"arguments"`
            }
            _ = json.Unmarshal(req.Params, &p)
            var args struct{ Text string `json:"text"` }
            _ = json.Unmarshal(p.Arguments, &args)
            _ = enc.Encode(map[string]any{
                "jsonrpc": "2.0",
                "id":      json.RawMessage(req.ID),
                "result": map[string]any{
                    "content": []map[string]any{{"type": "text", "text": "echo: " + args.Text}},
                },
            })
        case "notifications/initialized":
            // no-op
        default:
            _ = enc.Encode(map[string]any{
                "jsonrpc": "2.0",
                "id":      json.RawMessage(req.ID),
                "error":   map[string]any{"code": -32601, "message": "method not found: " + req.Method},
            })
        }
        _ = fmt.Sprintf // keep import
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/mcp/testdata/stub_server.go
git commit -m "test(mcp): stub server for client tests"
```

---

## Task 4: MCP client — handshake + tools/list

**Files:**
- Create: `internal/mcp/client.go`
- Create: `internal/mcp/client_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/mcp/client_test.go
package mcp

import (
    "context"
    "os/exec"
    "path/filepath"
    "testing"
    "time"
)

func buildStubServer(t *testing.T) string {
    t.Helper()
    bin := filepath.Join(t.TempDir(), "stub")
    cmd := exec.Command("go", "build", "-o", bin, "./testdata/stub_server.go")
    if out, err := cmd.CombinedOutput(); err != nil {
        t.Fatalf("build stub failed: %v\n%s", err, out)
    }
    return bin
}

func TestClient_InitializeAndListTools(t *testing.T) {
    bin := buildStubServer(t)
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    c, err := Spawn(ctx, "stub", bin, nil, nil)
    if err != nil { t.Fatal(err) }
    defer c.Close()

    if err := c.Initialize(ctx); err != nil { t.Fatal(err) }
    tools, err := c.ListTools(ctx)
    if err != nil { t.Fatal(err) }
    if len(tools) != 1 || tools[0].Name != "echo" {
        t.Fatalf("tools = %#v", tools)
    }
}

func TestClient_CallTool(t *testing.T) {
    bin := buildStubServer(t)
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    c, err := Spawn(ctx, "stub", bin, nil, nil)
    if err != nil { t.Fatal(err) }
    defer c.Close()
    _ = c.Initialize(ctx)

    res, err := c.CallTool(ctx, "echo", []byte(`{"text":"hi"}`))
    if err != nil { t.Fatal(err) }
    if len(res.Content) != 1 || res.Content[0].Text != "echo: hi" {
        t.Fatalf("res = %#v", res)
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/mcp/client.go
package mcp

import (
    "bufio"
    "context"
    "encoding/json"
    "fmt"
    "io"
    "os/exec"
    "sync"
    "sync/atomic"
)

type Client struct {
    name string
    cmd  *exec.Cmd
    in   io.WriteCloser
    out  *bufio.Reader
    enc  *json.Encoder
    nextID atomic.Int64
    mu     sync.Mutex
    pending map[string]chan Response
}

func Spawn(ctx context.Context, name, bin string, args []string, env []string) (*Client, error) {
    cmd := exec.CommandContext(ctx, bin, args...)
    cmd.Env = append(append([]string{}, env...))
    in, err := cmd.StdinPipe()
    if err != nil { return nil, err }
    out, err := cmd.StdoutPipe()
    if err != nil { return nil, err }
    cmd.Stderr = io.Discard
    if err := cmd.Start(); err != nil {
        return nil, err
    }
    c := &Client{
        name:    name,
        cmd:     cmd,
        in:      in,
        out:     bufio.NewReader(out),
        enc:     json.NewEncoder(in),
        pending: map[string]chan Response{},
    }
    go c.readLoop()
    return c, nil
}

func (c *Client) Initialize(ctx context.Context) error {
    params, _ := json.Marshal(InitializeParams{
        ProtocolVersion: ProtocolVersion,
        ClientInfo:      map[string]string{"name": "rune", "version": "0.0.0-dev"},
        Capabilities:    map[string]any{},
    })
    if _, err := c.call(ctx, "initialize", params); err != nil {
        return err
    }
    // Notify initialized.
    notif, _ := json.Marshal(Notification{JSONRPC: "2.0", Method: "notifications/initialized"})
    notif = append(notif, '\n')
    _, err := c.in.Write(notif)
    return err
}

func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
    res, err := c.call(ctx, "tools/list", nil)
    if err != nil { return nil, err }
    var r ToolsListResult
    if err := json.Unmarshal(res, &r); err != nil { return nil, err }
    return r.Tools, nil
}

func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (ToolsCallResult, error) {
    p, _ := json.Marshal(ToolsCallParams{Name: name, Arguments: args})
    res, err := c.call(ctx, "tools/call", p)
    if err != nil { return ToolsCallResult{}, err }
    var r ToolsCallResult
    if err := json.Unmarshal(res, &r); err != nil { return ToolsCallResult{}, err }
    return r, nil
}

func (c *Client) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
    id := fmt.Sprintf("%d", c.nextID.Add(1))
    rawID, _ := json.Marshal(id)
    req := Request{JSONRPC: "2.0", ID: rawID, Method: method, Params: params}
    ch := make(chan Response, 1)
    c.mu.Lock()
    c.pending[id] = ch
    c.mu.Unlock()
    defer func() {
        c.mu.Lock()
        delete(c.pending, id)
        c.mu.Unlock()
    }()
    line, _ := json.Marshal(req)
    line = append(line, '\n')
    if _, err := c.in.Write(line); err != nil {
        return nil, err
    }
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    case resp := <-ch:
        if resp.Error != nil {
            return nil, fmt.Errorf("%s: %s", method, resp.Error.Message)
        }
        return resp.Result, nil
    }
}

func (c *Client) readLoop() {
    for {
        line, err := c.out.ReadBytes('\n')
        if err != nil {
            return
        }
        var resp Response
        if err := json.Unmarshal(line, &resp); err != nil {
            continue
        }
        if len(resp.ID) == 0 {
            continue
        }
        var id string
        _ = json.Unmarshal(resp.ID, &id)
        c.mu.Lock()
        ch := c.pending[id]
        c.mu.Unlock()
        if ch != nil {
            ch <- resp
        }
    }
}

func (c *Client) Close() error {
    _ = c.in.Close()
    return c.cmd.Process.Kill()
}

func (c *Client) Name() string { return c.name }
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/mcp/ -run "TestClient"`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/mcp/client.go internal/mcp/client_test.go
git commit -m "feat(mcp): stdio jsonrpc client with initialize/tools.list/tools.call"
```

---

## Task 5: MCP tool wrapper

**Files:**
- Create: `internal/mcp/tool.go`
- Create: `internal/mcp/tool_test.go`

> Each MCP tool becomes a `tools.Tool` whose name is `<server>:<tool>` (so namespacing avoids collisions with built-ins or other servers). `Run` calls `client.CallTool` and serializes `[]ContentItem` into a flat string.

- [ ] **Step 1: Write the failing test**

```go
// internal/mcp/tool_test.go
package mcp

import (
    "context"
    "encoding/json"
    "strings"
    "testing"
    "time"
)

func TestMCPTool_RunsAndStringifies(t *testing.T) {
    bin := buildStubServer(t)
    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    c, _ := Spawn(ctx, "stub", bin, nil, nil)
    defer c.Close()
    _ = c.Initialize(ctx)

    tt := NewTool(c, Tool{
        Name: "echo", Description: "echo",
        InputSchema: json.RawMessage(`{"type":"object"}`),
    })
    spec := tt.Spec()
    if spec.Name != "stub:echo" {
        t.Fatalf("spec.Name = %q", spec.Name)
    }
    res, err := tt.Run(ctx, json.RawMessage(`{"text":"hello"}`))
    if err != nil { t.Fatal(err) }
    if !strings.Contains(res.Output, "echo: hello") {
        t.Fatalf("output = %q", res.Output)
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/mcp/tool.go
package mcp

import (
    "context"
    "encoding/json"
    "strings"

    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/tools"
)

type MCPTool struct {
    client *Client
    tool   Tool
}

func NewTool(c *Client, t Tool) *MCPTool {
    return &MCPTool{client: c, tool: t}
}

func (m *MCPTool) Spec() ai.ToolSpec {
    return ai.ToolSpec{
        Name:        m.client.Name() + ":" + m.tool.Name,
        Description: m.tool.Description,
        Schema:      m.tool.InputSchema,
    }
}

func (m *MCPTool) Run(ctx context.Context, args json.RawMessage) (tools.Result, error) {
    res, err := m.client.CallTool(ctx, m.tool.Name, args)
    if err != nil {
        return tools.Result{Output: err.Error(), IsError: true}, nil
    }
    var sb strings.Builder
    for i, c := range res.Content {
        if i > 0 { sb.WriteString("\n") }
        switch c.Type {
        case "text":
            sb.WriteString(c.Text)
        case "image":
            sb.WriteString("[image: ")
            sb.WriteString(c.MIME)
            sb.WriteString(" base64=")
            sb.WriteString(c.Data[:min(60, len(c.Data))])
            sb.WriteString("…]")
        default:
            sb.WriteString("[")
            sb.WriteString(c.Type)
            sb.WriteString("]")
        }
    }
    return tools.Result{Output: sb.String(), IsError: res.IsError}, nil
}

func min(a, b int) int { if a < b { return a }; return b }
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/mcp/ -run "TestMCPTool"`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/mcp/tool.go internal/mcp/tool_test.go
git commit -m "feat(mcp): wrap mcp tools to satisfy tools.Tool interface"
```

---

## Task 6: MCP manager — config + lifecycle

**Files:**
- Create: `internal/mcp/manager.go`
- Create: `internal/mcp/manager_test.go`

> Config format `~/.rune/mcp.json`:
> ```json
> {
>   "servers": {
>     "filesystem": { "command": "npx",  "args": ["-y", "@modelcontextprotocol/server-filesystem", "/tmp"] },
>     "stub":       { "command": "/path/to/stub" }
>   }
> }
> ```

- [ ] **Step 1: Write the failing test**

```go
// internal/mcp/manager_test.go
package mcp

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "testing"
    "time"

    "github.com/khang859/rune/internal/tools"
)

func TestManager_RegistersToolsFromAllServers(t *testing.T) {
    bin := buildStubServer(t)

    cfgPath := filepath.Join(t.TempDir(), "mcp.json")
    cfg := fmt.Sprintf(`{"servers":{"stub":{"command":%q}}}`, bin)
    _ = os.WriteFile(cfgPath, []byte(cfg), 0o644)

    reg := tools.NewRegistry()
    mgr := NewManager(cfgPath)

    ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()

    if err := mgr.Start(ctx, reg); err != nil {
        t.Fatal(err)
    }
    defer mgr.Shutdown()

    specs := reg.Specs()
    var found bool
    for _, s := range specs {
        if s.Name == "stub:echo" {
            found = true
        }
    }
    if !found {
        t.Fatalf("stub:echo not registered: %v", specs)
    }

    res, err := reg.Run(ctx, mustToolCall("stub:echo", `{"text":"x"}`))
    if err != nil { t.Fatal(err) }
    if res.IsError || res.Output == "" {
        t.Fatalf("res = %#v", res)
    }

    var unused json.RawMessage
    _ = unused
}

// helper from agent package — recreated locally
func mustToolCall(name, args string) (call /* aliased below */) {
    return call{Name: name, Args: json.RawMessage(args)}
}

// alias to avoid pulling ai import here
type call = struct {
    ID   string
    Name string
    Args json.RawMessage
}
```

> Note on the test: `tools.Registry.Run` takes `ai.ToolCall`. Either import `ai` in this test, or simplify by registering and asserting differently. To keep tests within `internal/mcp`, the test should import `ai`:

```go
// adjust the test to use ai.ToolCall instead of the alias hack:
import "github.com/khang859/rune/internal/ai"
// ...
res, err := reg.Run(ctx, ai.ToolCall{Name: "stub:echo", Args: json.RawMessage(`{"text":"x"}`)})
```

(Apply that simplification in your final test file.)

- [ ] **Step 2: Implement**

```go
// internal/mcp/manager.go
package mcp

import (
    "context"
    "encoding/json"
    "fmt"
    "os"
    "sync"

    "github.com/khang859/rune/internal/tools"
)

type ServerConfig struct {
    Command string            `json:"command"`
    Args    []string          `json:"args,omitempty"`
    Env     map[string]string `json:"env,omitempty"`
}

type Config struct {
    Servers map[string]ServerConfig `json:"servers"`
}

type Manager struct {
    path    string
    clients map[string]*Client
    mu      sync.Mutex
}

func NewManager(path string) *Manager {
    return &Manager{path: path, clients: map[string]*Client{}}
}

// Start reads the config file, spawns all servers, lists their tools,
// and registers each tool into the provided tools.Registry.
// Servers that fail to spawn are logged-and-skipped (do not abort the session).
func (m *Manager) Start(ctx context.Context, reg *tools.Registry) error {
    b, err := os.ReadFile(m.path)
    if err != nil {
        if os.IsNotExist(err) {
            return nil
        }
        return err
    }
    var cfg Config
    if err := json.Unmarshal(b, &cfg); err != nil {
        return fmt.Errorf("mcp.json: %w", err)
    }
    for name, sc := range cfg.Servers {
        env := envSlice(sc.Env)
        c, err := Spawn(ctx, name, sc.Command, sc.Args, env)
        if err != nil {
            fmt.Fprintf(os.Stderr, "[mcp] failed to spawn %s: %v\n", name, err)
            continue
        }
        if err := c.Initialize(ctx); err != nil {
            fmt.Fprintf(os.Stderr, "[mcp] init %s: %v\n", name, err)
            _ = c.Close()
            continue
        }
        ts, err := c.ListTools(ctx)
        if err != nil {
            fmt.Fprintf(os.Stderr, "[mcp] list %s: %v\n", name, err)
            _ = c.Close()
            continue
        }
        for _, t := range ts {
            reg.Register(NewTool(c, t))
        }
        m.mu.Lock()
        m.clients[name] = c
        m.mu.Unlock()
    }
    return nil
}

func (m *Manager) Shutdown() {
    m.mu.Lock()
    defer m.mu.Unlock()
    for _, c := range m.clients {
        _ = c.Close()
    }
    m.clients = map[string]*Client{}
}

func envSlice(m map[string]string) []string {
    out := make([]string, 0, len(m)+len(os.Environ()))
    out = append(out, os.Environ()...)
    for k, v := range m {
        out = append(out, k+"="+v)
    }
    return out
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/mcp/...`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/mcp/manager.go internal/mcp/manager_test.go
git commit -m "feat(mcp): manager — config-driven spawn, register tools, shutdown"
```

---

## Task 7: Wire skills + MCP into TUI startup

**Files:**
- Modify: `cmd/rune/interactive.go`
- Modify: `internal/tui/root.go` (slash menu and skill dispatch)

- [ ] **Step 1: Wire skills + MCP in interactive.go**

```go
// cmd/rune/interactive.go (replace runInteractive body)
package main

import (
    "context"
    "fmt"
    "os"
    "path/filepath"

    "github.com/khang859/rune/internal/agent"
    "github.com/khang859/rune/internal/ai/codex"
    "github.com/khang859/rune/internal/ai/oauth"
    "github.com/khang859/rune/internal/config"
    "github.com/khang859/rune/internal/mcp"
    "github.com/khang859/rune/internal/session"
    "github.com/khang859/rune/internal/skill"
    "github.com/khang859/rune/internal/tools"
    "github.com/khang859/rune/internal/tui"
)

func runInteractive(ctx context.Context) error {
    if err := config.EnsureRuneDir(); err != nil {
        return err
    }
    endpoint := oauth.CodexResponsesBaseURL + oauth.CodexResponsesPath
    if v := os.Getenv("RUNE_CODEX_ENDPOINT"); v != "" {
        endpoint = v
    }
    tokenURL := oauth.CodexTokenURL
    if v := os.Getenv("RUNE_OAUTH_TOKEN_URL"); v != "" {
        tokenURL = v
    }
    store := oauth.NewStore(config.AuthPath())
    src := &oauth.CodexSource{Store: store, TokenURL: tokenURL}
    if _, err := src.Token(ctx); err != nil {
        return fmt.Errorf("not logged in: %w (run `rune login codex`)", err)
    }
    p := codex.New(endpoint, src)

    sess := session.New("gpt-5")
    sess.SetPath(filepath.Join(config.SessionsDir(), sess.ID+".json"))

    reg := tools.NewRegistry()
    reg.Register(tools.Read{})
    reg.Register(tools.Write{})
    reg.Register(tools.Edit{})
    reg.Register(tools.Bash{})

    // MCP plugins
    mgr := mcp.NewManager(config.MCPConfig())
    if err := mgr.Start(ctx, reg); err != nil {
        fmt.Fprintln(os.Stderr, "[mcp] start failed:", err)
    }
    defer mgr.Shutdown()

    // Skills
    cwd, _ := os.Getwd()
    home, _ := os.UserHomeDir()
    skills, _ := (&skill.Loader{
        Roots: []string{
            filepath.Join(home, ".rune", "skills"),
            filepath.Join(cwd, ".rune", "skills"),
        },
    }).Load()

    system := defaultSystemPrompt() + "\n\n" + agent.LoadAgentsMD(cwd, home)
    a := agent.New(p, reg, sess, system)

    return tui.RunWithExtensions(a, sess, skills)
}
```

- [ ] **Step 2: Add `tui.RunWithExtensions` and skill plumbing**

```go
// internal/tui/tui.go
package tui

import (
    tea "github.com/charmbracelet/bubbletea"

    "github.com/khang859/rune/internal/agent"
    "github.com/khang859/rune/internal/session"
    "github.com/khang859/rune/internal/skill"
)

func RunWithExtensions(a *agent.Agent, s *session.Session, skills []skill.Skill) error {
    m := NewRootModel(a, s)
    m.SetSkills(skills)
    p := tea.NewProgram(m, tea.WithAltScreen())
    _, err := p.Run()
    return err
}
```

```go
// internal/tui/root.go (additions)

import "github.com/khang859/rune/internal/skill"

func (m *RootModel) SetSkills(skills []skill.Skill) {
    m.skills = make(map[string]string, len(skills))
    cmds := []string{
        "/quit", "/model", "/tree", "/resume", "/settings",
        "/new", "/name", "/session", "/fork", "/clone", "/copy",
        "/compact", "/reload", "/hotkeys",
    }
    for _, s := range skills {
        m.skills[s.Slug] = s.Body
        cmds = append(cmds, "/skill:"+s.Slug)
    }
    m.editor = editor.New(m.editor.Cwd(), cmds) // see editor.Cwd accessor below
}
```

Add `editor.Cwd()` accessor in `internal/tui/editor/editor.go`:
```go
func (e *Editor) Cwd() string { return e.cwd }
```

Handle `/skill:<slug>` in `handleSlashCommand`:
```go
case strings.HasPrefix(cmd, "/skill:"):
    slug := strings.TrimPrefix(cmd, "/skill:")
    if body, ok := m.skills[slug]; ok {
        m.pendingSkillBody = body
        m.msgs.OnTurnError(fmt.Errorf("(skill %q armed; will be prepended to your next message)", slug))
        m.refreshViewport()
    }
```

> Replace the `switch cmd { ... }` with a `switch` + falls into a small dispatch helper that lets us also match prefix. Concretely:
> ```go
> if strings.HasPrefix(cmd, "/skill:") { ... return nil }
> switch cmd { ... }
> ```

Inject the pending skill body into the next user message in `Update`:
```go
if res.Send {
    text := res.Text
    if m.pendingSkillBody != "" {
        text = m.pendingSkillBody + "\n\n" + text
        m.pendingSkillBody = ""
    }
    // ... rest unchanged
}
```

- [ ] **Step 3: Update `/reload` to re-load skills**

```go
case "/reload":
    home, _ := os.UserHomeDir()
    cwd, _ := os.Getwd()
    sks, _ := (&skill.Loader{Roots: []string{
        filepath.Join(home, ".rune", "skills"),
        filepath.Join(cwd, ".rune", "skills"),
    }}).Load()
    m.SetSkills(sks)
    m.msgs.OnTurnError(fmt.Errorf("(reloaded %d skills)", len(sks)))
    m.refreshSystemPrompt()
```

- [ ] **Step 4: Run all tests + manual smoke**

Run: `make all`
Expected: PASS.

```bash
mkdir -p ~/.rune/skills
cat > ~/.rune/skills/refactor.md <<'EOF'
When refactoring, write a failing test first.
EOF
go run ./cmd/rune
# Type /skill:refactor + Enter, then "rename foo to bar in pkg/x" + Enter.
# The skill body should prepend automatically.
```

- [ ] **Step 5: Commit**

```bash
git add cmd/rune/interactive.go internal/tui/tui.go internal/tui/root.go internal/tui/editor/editor.go
git commit -m "feat(tui): wire skills + mcp into interactive startup; /skill:name dispatch; /reload"
```

---

## End state

After Plan 06, rune is extensible:

- `~/.rune/skills/*.md` and `./.rune/skills/*.md` surface as `/skill:<slug>` commands.
- Selecting a skill arms its body to prepend to the next user message.
- `~/.rune/mcp.json` configures MCP servers; their tools register as `<server>:<tool>` and the agent uses them like built-ins.
- `/reload` re-loads skills (and re-walks AGENTS.md for the system prompt).

Plan 07 is polish: README, docs, install instructions, release prep.
