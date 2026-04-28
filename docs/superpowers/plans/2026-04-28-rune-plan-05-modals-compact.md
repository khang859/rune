# rune Plan 05 — Modals + Compaction

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the rest of pi's slash commands and the modal overlays they open: `/model`, `/tree`, `/resume`, `/settings`, plus session management commands `/new`, `/name`, `/fork`, `/clone`, `/session`, `/reload`, `/copy`, `/hotkeys`. Add `/compact` (manual) and auto-compact on context overflow.

**Architecture:** A `Modal` interface lets the root model swap the editor for a fullscreen-area component. Each modal is its own type with `Init/Update/View`. They emit a `ModalResultMsg` when dismissed (with optional payload — selected model id, picked node id, etc.). Compaction is implemented in `internal/session` as `Compact(ctx, instructions, summarizer)` where `summarizer` is a callback that takes the messages-to-replace and returns a single summary message.

**Tech Stack:** Same as Plan 04. Adds `golang.design/x/clipboard` for `/copy`. No new heavy deps.

**Spec:** `docs/superpowers/specs/2026-04-28-rune-coding-agent-design.md`

---

## File Structure

```
internal/tui/modal/
├── modal.go              # interface + dismissed message
├── model_picker.go       # /model
├── model_picker_test.go
├── tree.go               # /tree
├── tree_test.go
├── resume.go             # /resume
├── resume_test.go
├── settings.go           # /settings
├── settings_test.go
└── hotkeys.go            # /hotkeys (read-only display)
internal/session/
├── compact.go            # Compact(ctx, instructions, fn)
├── compact_test.go
└── browse.go             # ListSessions for /resume
└── browse_test.go
internal/tui/
└── root.go               # extended for modals + auto-compact + clipboard
internal/agent/
└── compact.go            # bridge: agent.Compact uses provider as summarizer
└── compact_test.go
```

---

## Task 1: Modal interface

**Files:**
- Create: `internal/tui/modal/modal.go`

- [ ] **Step 1: Implement**

```go
// internal/tui/modal/modal.go
package modal

import tea "github.com/charmbracelet/bubbletea"

type Modal interface {
    Init() tea.Cmd
    Update(msg tea.Msg) (Modal, tea.Cmd)
    View(width, height int) string
}

// ResultMsg is sent by a modal to dismiss itself with an optional payload.
// Payload is type-asserted by the caller based on which modal was open.
type ResultMsg struct {
    Payload any
    Cancel  bool
}

func Result(payload any) tea.Cmd {
    return func() tea.Msg { return ResultMsg{Payload: payload} }
}

func Cancel() tea.Cmd {
    return func() tea.Msg { return ResultMsg{Cancel: true} }
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/modal/modal.go
git commit -m "feat(tui/modal): modal interface and result message"
```

---

## Task 2: Model picker

**Files:**
- Create: `internal/tui/modal/model_picker.go`
- Create: `internal/tui/modal/model_picker_test.go`

> v1 ships with a fixed list of Codex-supported model IDs. The list is `gpt-5`, `gpt-5-codex`, `gpt-5.1-codex-mini`, plus whatever else `pi-mono` exposes — but for this plan we only need the picker itself; the source list is a constant.

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/modal/model_picker_test.go
package modal

import (
    "testing"

    tea "github.com/charmbracelet/bubbletea"
)

func TestModelPicker_PicksHighlighted(t *testing.T) {
    p := NewModelPicker([]string{"gpt-5", "gpt-5-codex"}, "gpt-5")
    p.(*ModelPicker).Down() // highlight gpt-5-codex
    next, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
    _ = next
    msg := cmd().(ResultMsg)
    if got := msg.Payload.(string); got != "gpt-5-codex" {
        t.Fatalf("payload = %q", got)
    }
}

func TestModelPicker_EscCancels(t *testing.T) {
    p := NewModelPicker([]string{"gpt-5"}, "gpt-5")
    _, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
    msg := cmd().(ResultMsg)
    if !msg.Cancel {
        t.Fatal("expected cancel")
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/tui/modal/model_picker.go
package modal

import (
    "strings"

    tea "github.com/charmbracelet/bubbletea"
)

type ModelPicker struct {
    items []string
    sel   int
}

func NewModelPicker(items []string, current string) Modal {
    sel := 0
    for i, it := range items {
        if it == current {
            sel = i
            break
        }
    }
    return &ModelPicker{items: items, sel: sel}
}

func (m *ModelPicker) Init() tea.Cmd { return nil }

func (m *ModelPicker) Update(msg tea.Msg) (Modal, tea.Cmd) {
    if k, ok := msg.(tea.KeyMsg); ok {
        switch k.Type {
        case tea.KeyUp:
            m.Up()
        case tea.KeyDown:
            m.Down()
        case tea.KeyEnter:
            return m, Result(m.items[m.sel])
        case tea.KeyEsc:
            return m, Cancel()
        }
    }
    return m, nil
}

func (m *ModelPicker) Up() {
    if m.sel > 0 { m.sel-- }
}
func (m *ModelPicker) Down() {
    if m.sel < len(m.items)-1 { m.sel++ }
}

func (m *ModelPicker) View(width, height int) string {
    var sb strings.Builder
    sb.WriteString("Select model (↑/↓, Enter, Esc):\n")
    for i, it := range m.items {
        if i == m.sel {
            sb.WriteString("  > " + it + "\n")
        } else {
            sb.WriteString("    " + it + "\n")
        }
    }
    return sb.String()
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/tui/modal/ -run TestModelPicker`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/modal/model_picker.go internal/tui/modal/model_picker_test.go
git commit -m "feat(tui/modal): /model picker"
```

---

## Task 3: Session — list saved sessions

**Files:**
- Create: `internal/session/browse.go`
- Create: `internal/session/browse_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/session/browse_test.go
package session

import (
    "encoding/json"
    "os"
    "path/filepath"
    "testing"
)

func TestListSessions_ReadsSummaries(t *testing.T) {
    dir := t.TempDir()
    s := New("gpt-5")
    s.Name = "demo"
    s.SetPath(filepath.Join(dir, s.ID+".json"))
    s.Append(userMsg("hi"))
    if err := s.Save(); err != nil {
        t.Fatal(err)
    }

    summaries, err := ListSessions(dir)
    if err != nil {
        t.Fatal(err)
    }
    if len(summaries) != 1 {
        t.Fatalf("len = %d", len(summaries))
    }
    if summaries[0].Name != "demo" {
        t.Fatalf("name = %q", summaries[0].Name)
    }
    if summaries[0].ID != s.ID {
        t.Fatalf("id = %q", summaries[0].ID)
    }
    if summaries[0].MessageCount < 1 {
        t.Fatalf("message_count = %d", summaries[0].MessageCount)
    }
}

func TestListSessions_SkipsBadFiles(t *testing.T) {
    dir := t.TempDir()
    _ = os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not json"), 0o644)
    summaries, err := ListSessions(dir)
    if err != nil {
        t.Fatalf("unexpected err: %v", err)
    }
    if len(summaries) != 0 {
        t.Fatalf("expected 0, got %d", len(summaries))
    }
    var unused json.RawMessage
    _ = unused
}
```

- [ ] **Step 2: Implement**

```go
// internal/session/browse.go
package session

import (
    "encoding/json"
    "os"
    "path/filepath"
    "sort"
    "time"
)

type Summary struct {
    ID            string
    Name          string
    Created       time.Time
    Path          string
    MessageCount  int
    Model         string
}

func ListSessions(dir string) ([]Summary, error) {
    entries, err := os.ReadDir(dir)
    if err != nil {
        if os.IsNotExist(err) {
            return nil, nil
        }
        return nil, err
    }
    var out []Summary
    for _, e := range entries {
        if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
            continue
        }
        p := filepath.Join(dir, e.Name())
        b, err := os.ReadFile(p)
        if err != nil {
            continue
        }
        var w wireSession
        if err := json.Unmarshal(b, &w); err != nil {
            continue
        }
        ts, _ := time.Parse(time.RFC3339, w.Created)
        msgCount := 0
        for _, n := range w.Nodes {
            if n.HasMessage {
                msgCount++
            }
        }
        out = append(out, Summary{
            ID:           w.ID,
            Name:         w.Name,
            Created:      ts,
            Path:         p,
            MessageCount: msgCount,
            Model:        w.Model,
        })
    }
    sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
    return out, nil
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/session/ -run TestListSessions`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/session/browse.go internal/session/browse_test.go
git commit -m "feat(session): list saved session summaries"
```

---

## Task 4: Resume modal

**Files:**
- Create: `internal/tui/modal/resume.go`
- Create: `internal/tui/modal/resume_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/modal/resume_test.go
package modal

import (
    "testing"
    "time"

    tea "github.com/charmbracelet/bubbletea"

    "github.com/khang859/rune/internal/session"
)

func TestResume_PicksHighlighted(t *testing.T) {
    items := []session.Summary{
        {ID: "a", Name: "old",   Created: time.Now().Add(-time.Hour)},
        {ID: "b", Name: "newer", Created: time.Now()},
    }
    r := NewResume(items)
    _, cmd := r.Update(tea.KeyMsg{Type: tea.KeyEnter})
    msg := cmd().(ResultMsg)
    if got := msg.Payload.(session.Summary); got.ID != "a" {
        t.Fatalf("payload id = %q", got.ID)
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/tui/modal/resume.go
package modal

import (
    "fmt"
    "strings"

    tea "github.com/charmbracelet/bubbletea"

    "github.com/khang859/rune/internal/session"
)

type Resume struct {
    items []session.Summary
    sel   int
}

func NewResume(items []session.Summary) Modal {
    return &Resume{items: items}
}

func (r *Resume) Init() tea.Cmd { return nil }

func (r *Resume) Update(msg tea.Msg) (Modal, tea.Cmd) {
    if k, ok := msg.(tea.KeyMsg); ok {
        switch k.Type {
        case tea.KeyUp:
            if r.sel > 0 { r.sel-- }
        case tea.KeyDown:
            if r.sel < len(r.items)-1 { r.sel++ }
        case tea.KeyEnter:
            if len(r.items) == 0 {
                return r, Cancel()
            }
            return r, Result(r.items[r.sel])
        case tea.KeyEsc:
            return r, Cancel()
        }
    }
    return r, nil
}

func (r *Resume) View(width, height int) string {
    if len(r.items) == 0 {
        return "(no saved sessions)"
    }
    var sb strings.Builder
    sb.WriteString("Resume session (↑/↓, Enter, Esc):\n")
    for i, it := range r.items {
        marker := "  "
        if i == r.sel {
            marker = "> "
        }
        name := it.Name
        if name == "" {
            name = "(unnamed)"
        }
        sb.WriteString(fmt.Sprintf("%s%s — %d msgs — %s\n", marker, name, it.MessageCount, it.Created.Format("2006-01-02 15:04")))
    }
    return sb.String()
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/tui/modal/ -run TestResume`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/modal/resume.go internal/tui/modal/resume_test.go
git commit -m "feat(tui/modal): /resume picker"
```

---

## Task 5: Tree modal — navigate session nodes

**Files:**
- Create: `internal/tui/modal/tree.go`
- Create: `internal/tui/modal/tree_test.go`

> Renders the session as an indented tree. Up/Down moves selection in DFS order; Enter picks a node. The root model uses the picked node id as the new `Active`.

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/modal/tree_test.go
package modal

import (
    "testing"

    tea "github.com/charmbracelet/bubbletea"

    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/session"
)

func TestTree_PicksNodeID(t *testing.T) {
    s := session.New("gpt-5")
    n1 := s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "a"}}})
    s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "x"}}})
    s.Fork(n1)
    s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "y"}}})

    tr := NewTree(s)
    // Pick whatever is highlighted; just verify a node id flows through.
    _, cmd := tr.Update(tea.KeyMsg{Type: tea.KeyEnter})
    msg := cmd().(ResultMsg)
    id := msg.Payload.(string)
    if id == "" {
        t.Fatal("empty id")
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/tui/modal/tree.go
package modal

import (
    "fmt"
    "strings"

    tea "github.com/charmbracelet/bubbletea"

    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/session"
)

type Tree struct {
    sess   *session.Session
    flat   []treeRow
    sel    int
}

type treeRow struct {
    Node  *session.Node
    Depth int
}

func NewTree(s *session.Session) Modal {
    t := &Tree{sess: s}
    t.flatten(s.Root, 0)
    // Default selection: the node currently active.
    for i, r := range t.flat {
        if r.Node == s.Active {
            t.sel = i
            break
        }
    }
    return t
}

func (t *Tree) flatten(n *session.Node, depth int) {
    if n != nil && n.Parent != nil { // skip root sentinel
        t.flat = append(t.flat, treeRow{Node: n, Depth: depth})
    }
    for _, c := range n.Children {
        t.flatten(c, depth+1)
    }
}

func (t *Tree) Init() tea.Cmd { return nil }

func (t *Tree) Update(msg tea.Msg) (Modal, tea.Cmd) {
    if k, ok := msg.(tea.KeyMsg); ok {
        switch k.Type {
        case tea.KeyUp:
            if t.sel > 0 { t.sel-- }
        case tea.KeyDown:
            if t.sel < len(t.flat)-1 { t.sel++ }
        case tea.KeyEnter:
            if len(t.flat) == 0 {
                return t, Cancel()
            }
            return t, Result(t.flat[t.sel].Node.ID)
        case tea.KeyEsc:
            return t, Cancel()
        }
    }
    return t, nil
}

func (t *Tree) View(width, height int) string {
    var sb strings.Builder
    sb.WriteString("Pick a node to continue from (↑/↓, Enter, Esc):\n")
    for i, r := range t.flat {
        marker := "  "
        if i == t.sel {
            marker = "> "
        }
        snippet := previewMessage(r.Node.Message)
        sb.WriteString(fmt.Sprintf("%s%s%s %s\n", marker, indent(r.Depth), prefix(r.Node.Message.Role), snippet))
    }
    return sb.String()
}

func indent(d int) string { return strings.Repeat("  ", d) }

func prefix(role ai.Role) string {
    switch role {
    case ai.RoleUser: return "U:"
    case ai.RoleAssistant: return "A:"
    case ai.RoleToolResult: return "T:"
    }
    return "?:"
}

func previewMessage(m ai.Message) string {
    for _, c := range m.Content {
        if t, ok := c.(ai.TextBlock); ok {
            line := t.Text
            if len(line) > 60 {
                line = line[:60] + "…"
            }
            return strings.ReplaceAll(line, "\n", " ")
        }
        if t, ok := c.(ai.ToolUseBlock); ok {
            return "(tool " + t.Name + ")"
        }
        if t, ok := c.(ai.ToolResultBlock); ok {
            line := t.Output
            if len(line) > 40 {
                line = line[:40] + "…"
            }
            return "(result) " + line
        }
    }
    return ""
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/tui/modal/ -run TestTree`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/modal/tree.go internal/tui/modal/tree_test.go
git commit -m "feat(tui/modal): /tree node navigation"
```

---

## Task 6: Hotkeys modal (read-only)

**Files:**
- Create: `internal/tui/modal/hotkeys.go`

- [ ] **Step 1: Implement**

```go
// internal/tui/modal/hotkeys.go
package modal

import tea "github.com/charmbracelet/bubbletea"

type Hotkeys struct{}

func NewHotkeys() Modal { return Hotkeys{} }

func (Hotkeys) Init() tea.Cmd { return nil }

func (h Hotkeys) Update(msg tea.Msg) (Modal, tea.Cmd) {
    if _, ok := msg.(tea.KeyMsg); ok {
        return h, Cancel()
    }
    return h, nil
}

func (Hotkeys) View(width, height int) string {
    return `Hotkeys:
  Enter           submit
  Shift+Enter     newline
  Esc             cancel turn / close modal
  Ctrl+C ×2       quit
  Ctrl+L          /model
  Tab             path completion
  @               file picker
  /               command menu
  !cmd            run shell, send output
  !!cmd           run shell, do not send

(any key to close)`
}
```

- [ ] **Step 2: Commit**

```bash
git add internal/tui/modal/hotkeys.go
git commit -m "feat(tui/modal): /hotkeys reference"
```

---

## Task 7: Settings modal

**Files:**
- Create: `internal/tui/modal/settings.go`
- Create: `internal/tui/modal/settings_test.go`

> v1 settings cover: thinking effort (`minimal`/`low`/`medium`/`high`). Theme/transport/etc. land later.

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/modal/settings_test.go
package modal

import (
    "testing"

    tea "github.com/charmbracelet/bubbletea"
)

func TestSettings_CyclesEffort(t *testing.T) {
    s := NewSettings(Settings{Effort: "low"}).(*SettingsModal)
    s.Update(tea.KeyMsg{Type: tea.KeyDown})
    s.Update(tea.KeyMsg{Type: tea.KeyDown})
    _, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
    res := cmd().(ResultMsg).Payload.(Settings)
    if res.Effort != "high" {
        t.Fatalf("effort = %q", res.Effort)
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/tui/modal/settings.go
package modal

import (
    "fmt"
    "strings"

    tea "github.com/charmbracelet/bubbletea"
)

type Settings struct {
    Effort string
}

type SettingsModal struct {
    cur     Settings
    options []string
    sel     int
}

func NewSettings(cur Settings) Modal {
    options := []string{"minimal", "low", "medium", "high"}
    sel := 0
    for i, o := range options {
        if o == cur.Effort {
            sel = i
            break
        }
    }
    return &SettingsModal{cur: cur, options: options, sel: sel}
}

func (s *SettingsModal) Init() tea.Cmd { return nil }

func (s *SettingsModal) Update(msg tea.Msg) (Modal, tea.Cmd) {
    if k, ok := msg.(tea.KeyMsg); ok {
        switch k.Type {
        case tea.KeyUp:
            if s.sel > 0 { s.sel-- }
        case tea.KeyDown:
            if s.sel < len(s.options)-1 { s.sel++ }
        case tea.KeyEnter:
            return s, Result(Settings{Effort: s.options[s.sel]})
        case tea.KeyEsc:
            return s, Cancel()
        }
    }
    return s, nil
}

func (s *SettingsModal) View(width, height int) string {
    var sb strings.Builder
    sb.WriteString("Settings — thinking effort:\n")
    for i, o := range s.options {
        m := "  "
        if i == s.sel { m = "> " }
        sb.WriteString(fmt.Sprintf("%s%s\n", m, o))
    }
    sb.WriteString("\n(Enter to apply, Esc to cancel)")
    return sb.String()
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/tui/modal/ -run TestSettings`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/modal/settings.go internal/tui/modal/settings_test.go
git commit -m "feat(tui/modal): /settings — thinking effort"
```

---

## Task 8: Compaction in session package

**Files:**
- Create: `internal/session/compact.go`
- Create: `internal/session/compact_test.go`

> Compact replaces history above a chosen cut point with a single synthetic assistant message containing a summary. The cut point is the most recent user message — everything strictly before it gets summarized; that user message and onward are preserved unchanged.

- [ ] **Step 1: Write the failing test**

```go
// internal/session/compact_test.go
package session

import (
    "context"
    "strings"
    "testing"

    "github.com/khang859/rune/internal/ai"
)

func TestCompact_ReplacesPreCutWithSummary(t *testing.T) {
    s := New("gpt-5")
    s.Append(userMsg("u1"))
    s.Append(asstMsg("a1"))
    s.Append(userMsg("u2"))
    s.Append(asstMsg("a2"))

    summarizer := func(ctx context.Context, msgs []ai.Message, instructions string) (string, error) {
        var b strings.Builder
        for _, m := range msgs {
            for _, c := range m.Content {
                if tx, ok := c.(ai.TextBlock); ok {
                    b.WriteString(tx.Text + " ")
                }
            }
        }
        return "SUMMARY: " + strings.TrimSpace(b.String()), nil
    }

    if err := s.Compact(context.Background(), "be brief", summarizer); err != nil {
        t.Fatal(err)
    }

    path := s.PathToActive()
    // Expect: [summary, u2, a2]
    if len(path) != 3 {
        t.Fatalf("path len after compact = %d", len(path))
    }
    if !strings.Contains(path[0].Content[0].(ai.TextBlock).Text, "SUMMARY") {
        t.Fatalf("first msg not a summary: %#v", path[0])
    }
    if path[1].Content[0].(ai.TextBlock).Text != "u2" {
        t.Fatalf("second msg should be u2: %#v", path[1])
    }
    if path[2].Content[0].(ai.TextBlock).Text != "a2" {
        t.Fatalf("third msg should be a2: %#v", path[2])
    }
}

func TestCompact_NoCutPoint_ReturnsNoOp(t *testing.T) {
    s := New("gpt-5")
    summarizer := func(ctx context.Context, msgs []ai.Message, _ string) (string, error) { return "x", nil }
    if err := s.Compact(context.Background(), "", summarizer); err != nil {
        t.Fatal(err)
    }
    if got := len(s.PathToActive()); got != 0 {
        t.Fatalf("path len = %d", got)
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/session/compact.go
package session

import (
    "context"
    "time"

    "github.com/khang859/rune/internal/ai"
)

type Summarizer func(ctx context.Context, history []ai.Message, instructions string) (string, error)

// Compact replaces the active path's prefix up to (but not including) the most recent
// user message with a single synthetic assistant summary message.
func (s *Session) Compact(ctx context.Context, instructions string, summarize Summarizer) error {
    path := s.PathToActive()
    if len(path) == 0 {
        return nil
    }
    cut := lastUserIndex(path)
    if cut <= 0 {
        return nil // nothing to compact (no prior user msgs before the most recent)
    }
    summary, err := summarize(ctx, path[:cut], instructions)
    if err != nil {
        return err
    }
    // Build a new branch off root: [summary, path[cut:]...]
    s.Active = s.Root
    s.Append(ai.Message{
        Role:    ai.RoleAssistant,
        Content: []ai.ContentBlock{ai.TextBlock{Text: summary}},
    })
    for _, m := range path[cut:] {
        n := s.Append(m)
        n.Created = time.Now()
    }
    return nil
}

func lastUserIndex(path []ai.Message) int {
    for i := len(path) - 1; i >= 0; i-- {
        if path[i].Role == ai.RoleUser {
            return i
        }
    }
    return -1
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/session/ -run TestCompact`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/session/compact.go internal/session/compact_test.go
git commit -m "feat(session): compact() replaces pre-cut history with summary"
```

---

## Task 9: agent.Compact bridge

**Files:**
- Create: `internal/agent/compact.go`
- Create: `internal/agent/compact_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/agent/compact_test.go
package agent

import (
    "context"
    "strings"
    "testing"

    "github.com/khang859/rune/internal/ai"
    "github.com/khang859/rune/internal/ai/faux"
    "github.com/khang859/rune/internal/session"
    "github.com/khang859/rune/internal/tools"
)

func TestCompact_UsesProviderForSummary(t *testing.T) {
    s := session.New("gpt-5")
    s.Append(userMsg("u1"))
    s.Append(asstMsg("a1"))
    s.Append(userMsg("u2"))

    f := faux.New().Reply("here is a summary").Done()
    a := New(f, tools.NewRegistry(), s, "")
    if err := a.Compact(context.Background(), ""); err != nil {
        t.Fatal(err)
    }
    path := s.PathToActive()
    if len(path) < 2 {
        t.Fatalf("path len = %d", len(path))
    }
    if !strings.Contains(path[0].Content[0].(ai.TextBlock).Text, "summary") {
        t.Fatalf("first msg not summary: %#v", path[0])
    }
}

// reuse helpers from loop_test.go (same package)
func asstMsg(text string) ai.Message {
    return ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}}
}
```

- [ ] **Step 2: Implement**

```go
// internal/agent/compact.go
package agent

import (
    "context"
    "strings"

    "github.com/khang859/rune/internal/ai"
)

const compactSystemPrompt = "You are a context compactor. Produce a concise (~300 token) summary of the conversation so far. Preserve decisions, file paths, and unresolved questions. Output the summary only."

func (a *Agent) Compact(ctx context.Context, userInstructions string) error {
    return a.session.Compact(ctx, userInstructions, func(ctx context.Context, history []ai.Message, instr string) (string, error) {
        sys := compactSystemPrompt
        if instr != "" {
            sys += "\n\nUser instructions: " + instr
        }
        // Build a single Request that summarizes `history`.
        req := ai.Request{
            Model:    a.session.Model,
            System:   sys,
            Messages: history,
        }
        ch, err := a.provider.Stream(ctx, req)
        if err != nil {
            return "", err
        }
        var b strings.Builder
        for ev := range ch {
            switch v := ev.(type) {
            case ai.TextDelta:
                b.WriteString(v.Text)
            case ai.StreamError:
                return "", v.Err
            case ai.Done:
                return strings.TrimSpace(b.String()), nil
            }
        }
        return strings.TrimSpace(b.String()), nil
    })
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/agent/ -run TestCompact`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/agent/compact.go internal/agent/compact_test.go
git commit -m "feat(agent): compact bridges session.Compact through provider"
```

---

## Task 10: Auto-compact on context overflow

**Files:**
- Modify: `internal/agent/loop.go`
- Modify: `internal/agent/loop_test.go`

> When the agent receives `Done{Reason: "context_overflow"}`, instead of ending the turn we run `Compact` and **retry the turn**. We bound retries to 1 to avoid loops.

- [ ] **Step 1: Update the failing test**

```go
// internal/agent/loop_test.go (replace TestRun_ContextOverflow)

func TestRun_AutoCompactOnOverflow(t *testing.T) {
    f := faux.New().
        DoneOverflow().                              // first call hits overflow
        Reply("compacted summary text").Done().      // compact summarizer
        Reply("post-compact reply").Done()           // retry of original turn
    s := session.New("gpt-5")
    s.Append(userMsg("u1"))
    s.Append(asstMsg("a1"))
    a := New(f, tools.NewRegistry(), s, "")

    evs := collect(t, a.Run(context.Background(), userMsg("u2")))

    var sawOverflow, sawDone bool
    for _, e := range evs {
        switch v := e.(type) {
        case ContextOverflow:
            sawOverflow = true
        case TurnDone:
            if v.Reason == "stop" {
                sawDone = true
            }
        }
    }
    if !sawOverflow {
        t.Fatal("missing ContextOverflow event")
    }
    if !sawDone {
        t.Fatal("missing TurnDone after retry")
    }
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestRun_AutoCompactOnOverflow`
Expected: FAIL — current code ends the turn instead of compacting.

- [ ] **Step 3: Update loop.go**

Replace the overflow branch in `runTurn`:

```go
case ai.Done:
    if v.Reason == "context_overflow" {
        out <- ContextOverflow{}
        // attempt one auto-compact then retry
        if err := a.Compact(ctx, ""); err != nil {
            out <- TurnError{Err: err}
            return
        }
        // restart this turn from scratch
        continue // outer loop
    }
    a.persistAssistant(...)
    ...
```

Concretely, restructure runTurn so the outer `for` is labeled and `continue` restarts:

```go
func (a *Agent) runTurn(ctx context.Context, out chan<- Event) {
    autoCompactRemaining := 1
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
                    if autoCompactRemaining <= 0 {
                        out <- TurnDone{Reason: "context_overflow"}
                        return
                    }
                    autoCompactRemaining--
                    if err := a.Compact(ctx, ""); err != nil {
                        out <- TurnError{Err: err}
                        return
                    }
                    goto restart
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
                goto next
            }
        }
        if ctx.Err() != nil {
            out <- TurnAborted{}
            return
        }
    next:
        continue
    restart:
        continue
    }
}
```

(The duplicate `goto next` / `goto restart` exists to keep loop structure readable; both fall back to the outer `for` at the bottom of the iteration.)

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/agent/...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/agent/loop.go internal/agent/loop_test.go
git commit -m "feat(agent): auto-compact on context overflow with one retry"
```

---

## Task 11: RootModel — modal stack + slash dispatch

**Files:**
- Modify: `internal/tui/root.go`
- Add deps: `golang.design/x/clipboard`

- [ ] **Step 1: Add dep**

```bash
go get golang.design/x/clipboard
```

- [ ] **Step 2: Extend RootModel**

```go
// internal/tui/root.go (add fields and dispatch)
import (
    "github.com/khang859/rune/internal/tui/modal"
    "github.com/khang859/rune/internal/config"
    "github.com/khang859/rune/internal/session"
)

type RootModel struct {
    // existing fields...
    modal       modal.Modal
    settings    modal.Settings
}

// Update branch additions:
case modal.ResultMsg:
    cur := m.modal
    m.modal = nil
    if v.Cancel {
        return m, nil
    }
    return m, m.applyModalResult(cur, v.Payload)
```

```go
// inside Update — modal absorbs all key/text input when active:
if m.modal != nil {
    next, cmd := m.modal.Update(msg)
    m.modal = next
    return m, cmd
}
```

```go
// View additions:
if m.modal != nil {
    return m.modal.View(m.width, m.height)
}
```

- [ ] **Step 3: Wire slash commands**

Replace `handleSlashCommand` with full dispatch:

```go
func (m *RootModel) handleSlashCommand(cmd string) tea.Cmd {
    switch cmd {
    case "/quit":
        return tea.Quit
    case "/model":
        m.modal = modal.NewModelPicker([]string{"gpt-5", "gpt-5-codex", "gpt-5.1-codex-mini"}, m.sess.Model)
    case "/tree":
        m.modal = modal.NewTree(m.sess)
    case "/resume":
        items, _ := session.ListSessions(config.SessionsDir())
        m.modal = modal.NewResume(items)
    case "/settings":
        m.modal = modal.NewSettings(m.settings)
    case "/hotkeys":
        m.modal = modal.NewHotkeys()
    case "/new":
        m.startNewSession()
    case "/name":
        // simple inline prompt: Plan 06 may turn this into a small modal.
        m.msgs.OnTurnError(fmt.Errorf("(use /settings or future inline prompt)"))
    case "/session":
        m.msgs.OnTurnError(fmt.Errorf("session id=%s name=%q model=%s", m.sess.ID, m.sess.Name, m.sess.Model))
    case "/fork":
        m.modal = modal.NewTree(m.sess) // same UI, different result handling: see applyModalResult
        m.pendingForkMode = true
    case "/clone":
        nc := m.sess.Clone()
        nc.SetPath(filepath.Join(config.SessionsDir(), nc.ID+".json"))
        _ = nc.Save()
        m.swapSession(nc)
    case "/copy":
        last := lastAssistantText(m.sess)
        if last != "" {
            clipboard.Write(clipboard.FmtText, []byte(last))
        }
    case "/compact":
        return m.startCompact()
    case "/reload":
        // re-walk AGENTS.md, refresh skills (Plan 06) — for now just rebuild prompt.
        m.refreshSystemPrompt()
    }
    return nil
}

func (m *RootModel) applyModalResult(cur modal.Modal, payload any) tea.Cmd {
    switch cur.(type) {
    case *modal.ModelPicker:
        if id, ok := payload.(string); ok {
            m.sess.Model = id
            m.footer.Model = id
        }
    case *modal.SettingsModal:
        if s, ok := payload.(modal.Settings); ok {
            m.settings = s
        }
    case *modal.Resume:
        if sum, ok := payload.(session.Summary); ok {
            ns, err := session.Load(sum.Path)
            if err == nil {
                m.swapSession(ns)
            }
        }
    case *modal.Tree:
        if id, ok := payload.(string); ok {
            target := findNode(m.sess.Root, id)
            if target != nil {
                if m.pendingForkMode {
                    m.sess.Fork(target)
                    m.pendingForkMode = false
                } else {
                    m.sess.Active = target
                }
                m.rebuildMessagesFromSession()
            }
        }
    }
    return nil
}

func findNode(n *session.Node, id string) *session.Node {
    if n == nil { return nil }
    if n.ID == id { return n }
    for _, c := range n.Children {
        if r := findNode(c, id); r != nil { return r }
    }
    return nil
}

func lastAssistantText(s *session.Session) string {
    for n := s.Active; n != nil && n.Parent != nil; n = n.Parent {
        if n.Message.Role == ai.RoleAssistant {
            for _, c := range n.Message.Content {
                if t, ok := c.(ai.TextBlock); ok {
                    return t.Text
                }
            }
        }
    }
    return ""
}

func (m *RootModel) startNewSession() {
    nc := session.New(m.sess.Model)
    nc.SetPath(filepath.Join(config.SessionsDir(), nc.ID+".json"))
    m.swapSession(nc)
}

func (m *RootModel) swapSession(s *session.Session) {
    m.sess = s
    m.footer.Session = s.Name
    m.footer.Model = s.Model
    m.rebuildMessagesFromSession()
    // Rebuild agent for new session reference.
    m.agent = agent.New(m.agent.Provider(), m.agent.Tools(), s, m.agent.System())
}

func (m *RootModel) rebuildMessagesFromSession() {
    m.msgs = NewMessages(m.width)
    for _, msg := range m.sess.PathToActive() {
        switch msg.Role {
        case ai.RoleUser:
            for _, c := range msg.Content {
                if t, ok := c.(ai.TextBlock); ok {
                    m.msgs.AppendUser(t.Text)
                }
            }
        case ai.RoleAssistant:
            for _, c := range msg.Content {
                if t, ok := c.(ai.TextBlock); ok {
                    m.msgs.OnAssistantDelta(t.Text)
                }
            }
            m.msgs.OnTurnDone("stop")
        }
    }
    m.refreshViewport()
}

func (m *RootModel) startCompact() tea.Cmd {
    return func() tea.Msg {
        ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
        defer cancel()
        _ = m.agent.Compact(ctx, "")
        return compactDoneMsg{}
    }
}

type compactDoneMsg struct{}
```

> The agent struct needs three small accessors so root model can rebuild it on session swap. Add to `internal/agent/agent.go`:

```go
func (a *Agent) Provider() ai.Provider { return a.provider }
func (a *Agent) Tools() *tools.Registry { return a.tools }
func (a *Agent) System() string { return a.system }
```

Add `m.refreshSystemPrompt()` stub (real impl filled by Plan 06):
```go
func (m *RootModel) refreshSystemPrompt() {
    cwd, _ := os.Getwd()
    home, _ := os.UserHomeDir()
    sys := defaultSystemPromptForRoot() + "\n\n" + agent.LoadAgentsMD(cwd, home)
    m.agent = agent.New(m.agent.Provider(), m.agent.Tools(), m.sess, sys)
}

func defaultSystemPromptForRoot() string {
    return "You are rune, a coding agent. Use the available tools."
}
```

- [ ] **Step 4: Update slash menu in editor.New() with the full set**

In `cmd/rune/interactive.go`, replace the cmds slice when constructing the editor. Since the editor is built inside the TUI module's NewRootModel, modify `NewRootModel` directly:

```go
cmds := []string{
    "/quit", "/model", "/tree", "/resume", "/settings",
    "/new", "/name", "/session", "/fork", "/clone", "/copy",
    "/compact", "/reload", "/hotkeys",
}
m.editor = editor.New(cwd, cmds)
```

- [ ] **Step 5: Run all tests**

Run: `make all`
Expected: PASS.

- [ ] **Step 6: Manual smoke (optional)**

```bash
go run ./cmd/rune
```
Expected: `/model` opens model picker; `/tree` shows the message tree; `/resume` lists saved sessions; `/compact` runs in the background and shrinks history; `/copy` puts last assistant text on clipboard; `/quit` exits.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/root.go internal/agent/agent.go cmd/rune/interactive.go go.mod go.sum
git commit -m "feat(tui): modals + full slash command dispatch + auto-compact wired"
```

---

## End state

After Plan 05, rune is feature-complete on the editor + sessions side:

- `/model`, `/tree`, `/resume`, `/settings`, `/hotkeys` modals.
- `/new`, `/name`, `/fork`, `/clone`, `/session`, `/copy`, `/compact`, `/reload` commands.
- Auto-compact on context overflow (one retry).
- Manual compact via `/compact`.
- `/copy` writes to system clipboard.

Plan 06 adds the extension story: markdown skills under `~/.rune/skills/` and MCP plugin clients.
