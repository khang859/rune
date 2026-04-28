# rune Plan 03 — TUI MVP

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up the Bubble Tea interactive REPL — viewport that renders streaming agent events, plain-text editor with multi-line, footer with model/tokens/cwd. Single-turn flow works end-to-end against real Codex. No `@`, `/`, message queue, or modals yet (those land in Plans 04 and 05).

**Architecture:** A Bubble Tea program owns the screen. `RootModel` composes `messages` (a `bubbles/viewport`), `editor` (a `bubbles/textarea`), and `footer` (a custom small component). A long-running `tea.Cmd` pumps events from the agent's `<-chan agent.Event` into `tea.Msg`s; the model's `Update` routes those to render functions that mutate the viewport's content.

**Tech Stack:** `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/bubbles`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/x/exp/teatest` for tests.

**Spec:** `docs/superpowers/specs/2026-04-28-rune-coding-agent-design.md`

---

## File Structure

```
internal/tui/
├── tui.go                  # Run() entrypoint: wires agent + program
├── root.go                 # RootModel: Init/Update/View
├── messages.go             # message-list rendering (assistant text, tool blocks, errors)
├── editor.go               # editor wrapper around bubbles/textarea
├── footer.go               # footer rendering
├── styles.go               # lipgloss styles (one place)
├── events.go               # bridge between agent.Event and tea.Msg
├── root_test.go            # teatest snapshots
└── messages_test.go
cmd/rune/
└── interactive.go          # `rune` (no flags) → tui.Run(...)
```

---

## Task 1: Add Bubble Tea dependencies

- [ ] **Step 1: Add deps**

```bash
go get github.com/charmbracelet/bubbletea
go get github.com/charmbracelet/bubbles
go get github.com/charmbracelet/lipgloss
go get github.com/charmbracelet/x/exp/teatest
```

- [ ] **Step 2: Verify**

Run: `go build ./...`
Expected: succeeds.

- [ ] **Step 3: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add bubble tea dependencies"
```

---

## Task 2: Styles

**Files:**
- Create: `internal/tui/styles.go`

- [ ] **Step 1: Implement**

```go
// internal/tui/styles.go
package tui

import "github.com/charmbracelet/lipgloss"

type Styles struct {
    User       lipgloss.Style
    Assistant  lipgloss.Style
    Thinking   lipgloss.Style
    ToolCall   lipgloss.Style
    ToolResult lipgloss.Style
    ToolError  lipgloss.Style
    Footer     lipgloss.Style
    EditorBox  lipgloss.Style
}

func DefaultStyles() Styles {
    return Styles{
        User:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
        Assistant:  lipgloss.NewStyle().Foreground(lipgloss.Color("15")),
        Thinking:   lipgloss.NewStyle().Faint(true).Italic(true),
        ToolCall:   lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
        ToolResult: lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
        ToolError:  lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
        Footer:     lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Reverse(true).Padding(0, 1),
        EditorBox:  lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/styles.go
git commit -m "feat(tui): default lipgloss styles"
```

---

## Task 3: Footer component

**Files:**
- Create: `internal/tui/footer.go`
- Create: `internal/tui/footer_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/footer_test.go
package tui

import (
    "strings"
    "testing"
)

func TestFooter_RendersAllFields(t *testing.T) {
    f := Footer{
        Cwd:        "/home/x/proj",
        Session:    "demo",
        Model:      "gpt-5",
        Tokens:     1234,
        ContextPct: 42,
        Width:      120,
    }
    out := f.Render(DefaultStyles())
    for _, want := range []string{"/home/x/proj", "demo", "gpt-5", "1234", "42%"} {
        if !strings.Contains(out, want) {
            t.Fatalf("footer missing %q:\n%s", want, out)
        }
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestFooter`
Expected: FAIL.

- [ ] **Step 3: Implement**

```go
// internal/tui/footer.go
package tui

import "fmt"

type Footer struct {
    Cwd        string
    Session    string
    Model      string
    Tokens     int
    ContextPct int
    Width      int
}

func (f Footer) Render(s Styles) string {
    line := fmt.Sprintf(" %s | %s | %s | %d tok | %d%% ctx ",
        f.Cwd, f.Session, f.Model, f.Tokens, f.ContextPct)
    return s.Footer.Width(f.Width).Render(line)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestFooter`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/footer.go internal/tui/footer_test.go
git commit -m "feat(tui): footer component"
```

---

## Task 4: Editor wrapper

**Files:**
- Create: `internal/tui/editor.go`

- [ ] **Step 1: Implement**

```go
// internal/tui/editor.go
package tui

import (
    "github.com/charmbracelet/bubbles/textarea"
    tea "github.com/charmbracelet/bubbletea"
)

type Editor struct {
    ta textarea.Model
}

func NewEditor() Editor {
    ta := textarea.New()
    ta.Placeholder = "type a message…"
    ta.Prompt = "› "
    ta.CharLimit = 0
    ta.SetWidth(80)
    ta.SetHeight(3)
    ta.ShowLineNumbers = false
    ta.Focus()
    return Editor{ta: ta}
}

func (e *Editor) SetWidth(w int)  { e.ta.SetWidth(w) }
func (e *Editor) SetHeight(h int) { e.ta.SetHeight(h) }
func (e *Editor) Value() string   { return e.ta.Value() }
func (e *Editor) Reset()          { e.ta.Reset() }
func (e *Editor) Focus()          { e.ta.Focus() }
func (e *Editor) Blur()           { e.ta.Blur() }

func (e *Editor) Update(msg tea.Msg) tea.Cmd {
    var cmd tea.Cmd
    e.ta, cmd = e.ta.Update(msg)
    return cmd
}

func (e *Editor) View(s Styles) string {
    return s.EditorBox.Render(e.ta.View())
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/editor.go
git commit -m "feat(tui): editor wrapper around textarea"
```

---

## Task 5: Message renderer

**Files:**
- Create: `internal/tui/messages.go`
- Create: `internal/tui/messages_test.go`

> The viewport holds rendered text. As agent events arrive, the renderer appends to a buffer and asks the viewport to update its content. A live-streaming assistant block is mutable until `TurnDone` finalizes it.

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/messages_test.go
package tui

import (
    "strings"
    "testing"

    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/agent"
    "github.com/khang859/rune/internal/tools"
)

func TestMessages_AppendUser(t *testing.T) {
    m := NewMessages(80)
    m.AppendUser("hi there")
    if !strings.Contains(m.Render(DefaultStyles()), "hi there") {
        t.Fatal("user text missing")
    }
}

func TestMessages_StreamingAssistantText(t *testing.T) {
    m := NewMessages(80)
    m.OnAssistantDelta("hel")
    m.OnAssistantDelta("lo")
    m.OnTurnDone("stop")
    if !strings.Contains(m.Render(DefaultStyles()), "hello") {
        t.Fatal("streamed text not rendered")
    }
}

func TestMessages_ToolCallAndResult(t *testing.T) {
    m := NewMessages(80)
    call := ai.ToolCall{ID: "t1", Name: "read", Args: []byte(`{"path":"/x"}`)}
    m.OnToolStarted(call)
    m.OnToolFinished(agent.ToolFinished{Call: call, Result: tools.Result{Output: "file content"}})
    out := m.Render(DefaultStyles())
    if !strings.Contains(out, "read") {
        t.Fatalf("tool name missing:\n%s", out)
    }
    if !strings.Contains(out, "file content") {
        t.Fatalf("tool output missing:\n%s", out)
    }
}

func TestMessages_TurnError(t *testing.T) {
    m := NewMessages(80)
    m.OnTurnError(errString("bad thing"))
    if !strings.Contains(m.Render(DefaultStyles()), "bad thing") {
        t.Fatal("error not rendered")
    }
}

type errString string

func (e errString) Error() string { return string(e) }
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestMessages`
Expected: FAIL — `Messages` undefined.

- [ ] **Step 3: Implement**

```go
// internal/tui/messages.go
package tui

import (
    "fmt"
    "strings"

    "github.com/khang859/rune/internal/agent"
    "github.com/khang859/rune/internal/ai"
)

type Messages struct {
    width int
    blocks []block
    // when streaming an assistant turn, we mutate the last block in place.
    streamingAsst *block
}

type blockKind int

const (
    bkUser blockKind = iota
    bkAssistant
    bkThinking
    bkToolCall
    bkToolResult
    bkError
)

type block struct {
    kind blockKind
    text string
    meta string // e.g. tool name
}

func NewMessages(width int) *Messages { return &Messages{width: width} }

func (m *Messages) SetWidth(w int) { m.width = w }

func (m *Messages) AppendUser(text string) {
    m.blocks = append(m.blocks, block{kind: bkUser, text: text})
    m.streamingAsst = nil
}

func (m *Messages) OnAssistantDelta(delta string) {
    if m.streamingAsst == nil {
        m.blocks = append(m.blocks, block{kind: bkAssistant})
        m.streamingAsst = &m.blocks[len(m.blocks)-1]
    }
    m.streamingAsst.text += delta
}

func (m *Messages) OnThinkingDelta(delta string) {
    last := len(m.blocks)
    if last > 0 && m.blocks[last-1].kind == bkThinking {
        m.blocks[last-1].text += delta
        return
    }
    m.blocks = append(m.blocks, block{kind: bkThinking, text: delta})
}

func (m *Messages) OnToolStarted(call ai.ToolCall) {
    m.streamingAsst = nil
    m.blocks = append(m.blocks, block{
        kind: bkToolCall,
        meta: call.Name,
        text: string(call.Args),
    })
}

func (m *Messages) OnToolFinished(f agent.ToolFinished) {
    kind := bkToolResult
    if f.Result.IsError {
        kind = bkError
    }
    m.blocks = append(m.blocks, block{
        kind: kind,
        meta: f.Call.Name,
        text: f.Result.Output,
    })
}

func (m *Messages) OnTurnDone(reason string) {
    m.streamingAsst = nil
    if reason != "" && reason != "stop" {
        m.blocks = append(m.blocks, block{kind: bkThinking, text: fmt.Sprintf("(turn ended: %s)", reason)})
    }
}

func (m *Messages) OnTurnError(err error) {
    m.streamingAsst = nil
    m.blocks = append(m.blocks, block{kind: bkError, text: err.Error()})
}

func (m *Messages) Render(s Styles) string {
    var sb strings.Builder
    for i, b := range m.blocks {
        if i > 0 {
            sb.WriteString("\n\n")
        }
        switch b.kind {
        case bkUser:
            sb.WriteString(s.User.Render("user> ") + b.text)
        case bkAssistant:
            sb.WriteString(s.Assistant.Render(b.text))
        case bkThinking:
            sb.WriteString(s.Thinking.Render(b.text))
        case bkToolCall:
            sb.WriteString(s.ToolCall.Render(fmt.Sprintf("· %s(%s)", b.meta, b.text)))
        case bkToolResult:
            sb.WriteString(s.ToolResult.Render(fmt.Sprintf("← %s\n%s", b.meta, b.text)))
        case bkError:
            sb.WriteString(s.ToolError.Render("error: " + b.text))
        }
    }
    return sb.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestMessages`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/messages.go internal/tui/messages_test.go
git commit -m "feat(tui): message renderer for streaming events"
```

---

## Task 6: Event bridge — agent.Event → tea.Msg

**Files:**
- Create: `internal/tui/events.go`

- [ ] **Step 1: Implement**

```go
// internal/tui/events.go
package tui

import (
    tea "github.com/charmbracelet/bubbletea"

    "github.com/khang859/rune/internal/agent"
)

type AgentEventMsg struct{ Event agent.Event }
type AgentChannelDoneMsg struct{}

// nextEventCmd returns a tea.Cmd that reads ONE event from ch.
// If the channel closes, it sends AgentChannelDoneMsg.
func nextEventCmd(ch <-chan agent.Event) tea.Cmd {
    return func() tea.Msg {
        e, ok := <-ch
        if !ok {
            return AgentChannelDoneMsg{}
        }
        return AgentEventMsg{Event: e}
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/events.go
git commit -m "feat(tui): event-channel bridge to tea.Msg"
```

---

## Task 7: RootModel — wiring it all together

**Files:**
- Create: `internal/tui/root.go`
- Create: `internal/tui/root_test.go`

- [ ] **Step 1: Implement**

```go
// internal/tui/root.go
package tui

import (
    "context"
    "fmt"
    "os"
    "strings"

    tea "github.com/charmbracelet/bubbletea"

    "github.com/khang859/rune/internal/agent"
    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/session"

    "github.com/charmbracelet/bubbles/viewport"
)

type RootModel struct {
    agent      *agent.Agent
    sess       *session.Session
    styles     Styles
    msgs       *Messages
    viewport   viewport.Model
    editor     Editor
    footer     Footer

    width  int
    height int

    streaming bool
    eventCh   <-chan agent.Event
    cancel    context.CancelFunc

    totalTokens int
}

func NewRootModel(a *agent.Agent, sess *session.Session) *RootModel {
    cwd, _ := os.Getwd()
    home, _ := os.UserHomeDir()
    if strings.HasPrefix(cwd, home) {
        cwd = "~" + strings.TrimPrefix(cwd, home)
    }
    return &RootModel{
        agent:    a,
        sess:     sess,
        styles:   DefaultStyles(),
        msgs:     NewMessages(80),
        viewport: viewport.New(80, 20),
        editor:   NewEditor(),
        footer:   Footer{Cwd: cwd, Session: sess.Name, Model: sess.Model},
    }
}

func (m *RootModel) Init() tea.Cmd { return nil }

func (m *RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd

    switch v := msg.(type) {
    case tea.WindowSizeMsg:
        m.width, m.height = v.Width, v.Height
        m.layout()
        m.refreshViewport()
        return m, nil

    case tea.KeyMsg:
        if m.streaming {
            switch v.Type {
            case tea.KeyEsc:
                if m.cancel != nil {
                    m.cancel()
                }
            }
            return m, nil
        }
        switch v.Type {
        case tea.KeyCtrlC:
            return m, tea.Quit
        case tea.KeyEnter:
            if !v.Alt && v.Type == tea.KeyEnter && !isShiftEnter(v) {
                text := strings.TrimSpace(m.editor.Value())
                if text == "" {
                    return m, nil
                }
                m.editor.Reset()
                m.msgs.AppendUser(text)
                m.refreshViewport()
                return m, m.startTurn(text)
            }
        }
        if cmd := m.editor.Update(msg); cmd != nil {
            cmds = append(cmds, cmd)
        }
        return m, tea.Batch(cmds...)

    case AgentEventMsg:
        m.handleEvent(v.Event)
        m.refreshViewport()
        // Pump the next event.
        return m, nextEventCmd(m.eventCh)

    case AgentChannelDoneMsg:
        m.streaming = false
        m.eventCh = nil
        m.cancel = nil
        m.editor.Focus()
        return m, nil
    }

    if cmd := m.editor.Update(msg); cmd != nil {
        cmds = append(cmds, cmd)
    }
    return m, tea.Batch(cmds...)
}

func (m *RootModel) View() string {
    msgArea := m.viewport.View()
    edArea := m.editor.View(m.styles)
    foot := m.footer.Render(m.styles)
    return msgArea + "\n" + edArea + "\n" + foot
}

func (m *RootModel) layout() {
    if m.width == 0 || m.height == 0 {
        return
    }
    footerH := 1
    editorH := 5
    msgH := m.height - footerH - editorH
    if msgH < 3 {
        msgH = 3
    }
    m.viewport.Width = m.width
    m.viewport.Height = msgH
    m.editor.SetWidth(m.width - 2)
    m.editor.SetHeight(editorH - 2)
    m.footer.Width = m.width
    m.msgs.SetWidth(m.width)
}

func (m *RootModel) refreshViewport() {
    m.viewport.SetContent(m.msgs.Render(m.styles))
    m.viewport.GotoBottom()
}

func (m *RootModel) startTurn(text string) tea.Cmd {
    ctx, cancel := context.WithCancel(context.Background())
    m.cancel = cancel
    m.streaming = true
    m.editor.Blur()
    msg := ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}}
    ch := m.agent.Run(ctx, msg)
    m.eventCh = ch
    return nextEventCmd(ch)
}

func (m *RootModel) handleEvent(e agent.Event) {
    switch v := e.(type) {
    case agent.AssistantText:
        m.msgs.OnAssistantDelta(v.Delta)
    case agent.ThinkingText:
        m.msgs.OnThinkingDelta(v.Delta)
    case agent.ToolStarted:
        m.msgs.OnToolStarted(v.Call)
    case agent.ToolFinished:
        m.msgs.OnToolFinished(v)
    case agent.TurnUsage:
        m.totalTokens += v.Usage.Input + v.Usage.Output
        m.footer.Tokens = m.totalTokens
        m.footer.ContextPct = ctxPct(m.totalTokens)
    case agent.ContextOverflow:
        // visible feedback; auto-compact lands in Plan 05
        m.msgs.OnTurnError(fmt.Errorf("context overflow — manual /compact recommended"))
    case agent.TurnError:
        m.msgs.OnTurnError(v.Err)
    case agent.TurnAborted:
        m.msgs.OnTurnError(fmt.Errorf("(aborted)"))
    case agent.TurnDone:
        m.msgs.OnTurnDone(v.Reason)
    }
}

func ctxPct(tokens int) int {
    const cap = 200000
    p := tokens * 100 / cap
    if p > 100 {
        return 100
    }
    return p
}

// Detect shift+enter via the bubbletea key encoding.
// In bubbletea v0.25+, shift+enter is reported as v.Type == tea.KeyEnter with v.Shift, but
// versions vary; treat alt+enter as the multi-line marker as well.
func isShiftEnter(k tea.KeyMsg) bool {
    s := k.String()
    return s == "shift+enter"
}
```

- [ ] **Step 2: Write a teatest snapshot**

```go
// internal/tui/root_test.go
package tui

import (
    "context"
    "encoding/json"
    "strings"
    "testing"
    "time"

    tea "github.com/charmbracelet/bubbletea"
    "github.com/charmbracelet/x/exp/teatest"

    "github.com/khang859/rune/internal/agent"
    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/ai/faux"
    "github.com/khang859/rune/internal/session"
    "github.com/khang859/rune/internal/tools"
)

func TestRoot_TextOnlyTurnRendersAssistantText(t *testing.T) {
    f := faux.New().Reply("hello back").Done()
    s := session.New("gpt-5")
    a := agent.New(f, tools.NewRegistry(), s, "")

    m := NewRootModel(a, s)
    tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

    tm.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
    typeText(tm, "hi")
    tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

    teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
        return strings.Contains(string(b), "hello back")
    }, teatest.WithDuration(2*time.Second))

    tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
    tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func typeText(tm *teatest.TestModel, s string) {
    for _, r := range s {
        tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
    }
}

// Keep imports tidy
var _ = ai.RoleUser
var _ = json.Valid
var _ = context.Background
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestRoot`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go
git commit -m "feat(tui): root model with single-turn streaming"
```

---

## Task 8: tui.Run — package entry point

**Files:**
- Create: `internal/tui/tui.go`

- [ ] **Step 1: Implement**

```go
// internal/tui/tui.go
package tui

import (
    tea "github.com/charmbracelet/bubbletea"

    "github.com/khang859/rune/internal/agent"
    "github.com/khang859/rune/internal/session"
)

func Run(a *agent.Agent, s *session.Session) error {
    p := tea.NewProgram(NewRootModel(a, s), tea.WithAltScreen())
    _, err := p.Run()
    return err
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/tui.go
git commit -m "feat(tui): Run entrypoint"
```

---

## Task 9: Wire `rune` (no flags) into the TUI

**Files:**
- Create: `cmd/rune/interactive.go`
- Modify: `cmd/rune/main.go`

- [ ] **Step 1: Implement interactive.go**

```go
// cmd/rune/interactive.go
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
    "github.com/khang859/rune/internal/session"
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

    cwd, _ := os.Getwd()
    home, _ := os.UserHomeDir()
    system := defaultSystemPrompt() + "\n\n" + agent.LoadAgentsMD(cwd, home)
    a := agent.New(p, reg, sess, system)

    return tui.Run(a, sess)
}
```

- [ ] **Step 2: Wire into main**

```go
// cmd/rune/main.go (replace the trailing "rune <version>" branch)

if *prompt != "" {
    if err := runPrompt(ctx, *prompt, os.Stdout); err != nil {
        fmt.Fprintln(os.Stderr, "error:", err)
        os.Exit(1)
    }
    return
}
if *script != "" {
    // already handled above
}
// default: interactive
if err := runInteractive(ctx); err != nil {
    fmt.Fprintln(os.Stderr, "error:", err)
    os.Exit(1)
}
```

- [ ] **Step 3: Run all tests**

Run: `make all`
Expected: PASS.

- [ ] **Step 4: Manual smoke (optional, requires login)**

```bash
go run ./cmd/rune
```
Expected: full-screen REPL; type "hi" + Enter; see streamed response; Ctrl+C exits.

- [ ] **Step 5: Commit**

```bash
git add cmd/rune/interactive.go cmd/rune/main.go
git commit -m "feat(cmd): interactive mode wires agent into bubble tea TUI"
```

---

## End state

After Plan 03, rune is interactive:

- `rune` (no flags) opens a full-screen REPL.
- The user can type, send, and watch streamed responses; tool calls render inline; errors show as inline blocks; the footer tracks tokens and context %.
- Esc cancels an in-flight turn (ctx propagates through agent → provider → tools).
- Ctrl+C quits.

Plan 04 fills in the editor: `@` file refs, `/` command menu, message queue, `!command`, image paste, Tab path completion.
