# rune Plan 04 — Full Editor

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reach editor parity with pi: `@` fuzzy file picker, `/` slash-command menu (with autocomplete), Tab path completion, `!command` (run bash, append output to message), `!!command` (run, do not send), Ctrl+V image paste / drag-drop, and message queue while a turn is active.

**Architecture:** The editor stays a `bubbles/textarea` but is wrapped by a state machine that watches input for trigger characters. Triggers open *overlay* components (file picker, slash menu, path complete). Overlays consume key events while open and either insert text into the editor or close. A `MessageQueue` lives on the root model: while `streaming==true`, Enter appends to the queue; on `TurnDone` the queue drains.

**Tech Stack:** `github.com/sahilm/fuzzy` for fuzzy matching, stdlib `image` for clipboard image decoding fallback. Clipboard: stdlib for OSC52 detection of base64 image pastes (terminal escape sequences). For drag-drop on macOS Terminal/iTerm, the file path arrives as plain text in the editor — we treat any line starting with a recognized image extension as an image attachment.

**Spec:** `docs/superpowers/specs/2026-04-28-rune-coding-agent-design.md`

---

## File Structure

```
internal/tui/editor/
├── editor.go              # rewritten Editor: state machine + textarea
├── editor_test.go
├── filepicker.go          # @ fuzzy overlay
├── filepicker_test.go
├── slashmenu.go           # / overlay
├── slashmenu_test.go
├── pathcomplete.go        # Tab completion
├── pathcomplete_test.go
├── attachments.go         # image attachment buffer
├── attachments_test.go
└── shell.go               # !command / !!command runner
internal/tui/
├── queue.go               # message queue type
├── queue_test.go
├── root.go                # extended for queue + new editor API + viewport scrolling
└── tui.go                 # NewProgram opts extended (mouse cell motion)
```

The existing `internal/tui/editor.go` from Plan 03 is replaced — its callers must move to the new API.

---

## Task 1: Add fuzzy dep

- [ ] **Step 1**

```bash
go get github.com/sahilm/fuzzy
```

- [ ] **Step 2: Commit**

```bash
git add go.mod go.sum
git commit -m "chore: add fuzzy matching dep"
```

---

## Task 2: Message queue

**Files:**
- Create: `internal/tui/queue.go`
- Create: `internal/tui/queue_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/queue_test.go
package tui

import "testing"

func TestQueue_PushAndPop(t *testing.T) {
    q := &Queue{}
    q.Push("a")
    q.Push("b")
    if got, ok := q.Pop(); !ok || got != "a" {
        t.Fatalf("pop = %q, %v", got, ok)
    }
    if got, ok := q.Pop(); !ok || got != "b" {
        t.Fatalf("pop = %q, %v", got, ok)
    }
    if _, ok := q.Pop(); ok {
        t.Fatal("expected empty")
    }
}

func TestQueue_Len(t *testing.T) {
    q := &Queue{}
    q.Push("x")
    q.Push("y")
    if q.Len() != 2 {
        t.Fatalf("len = %d", q.Len())
    }
}
```

- [ ] **Step 2: Implement and run**

```go
// internal/tui/queue.go
package tui

type Queue struct {
    items []string
}

func (q *Queue) Push(s string) { q.items = append(q.items, s) }
func (q *Queue) Pop() (string, bool) {
    if len(q.items) == 0 {
        return "", false
    }
    s := q.items[0]
    q.items = q.items[1:]
    return s, true
}
func (q *Queue) Len() int { return len(q.items) }
```

Run: `go test ./internal/tui/ -run TestQueue`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
git add internal/tui/queue.go internal/tui/queue_test.go
git commit -m "feat(tui): message queue"
```

---

## Task 3: File picker overlay

**Files:**
- Create: `internal/tui/editor/filepicker.go`
- Create: `internal/tui/editor/filepicker_test.go`

> The picker scans the working directory once, then filters with fuzzy match on each keystroke. It does not walk symlinks and skips hidden files unless the query starts with `.`.

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/editor/filepicker_test.go
package editor

import (
    "os"
    "path/filepath"
    "testing"
)

func TestFilePicker_FiltersOnQuery(t *testing.T) {
    dir := t.TempDir()
    paths := []string{"foo.go", "bar.go", "internal/baz.go", "internal/foo.txt"}
    for _, p := range paths {
        full := filepath.Join(dir, p)
        _ = os.MkdirAll(filepath.Dir(full), 0o755)
        _ = os.WriteFile(full, []byte("x"), 0o644)
    }
    fp := NewFilePicker(dir)
    fp.SetQuery("foo")
    got := fp.Items()
    if len(got) == 0 {
        t.Fatal("expected matches for foo")
    }
    // Both foo.go and internal/foo.txt should match.
    seen := map[string]bool{}
    for _, it := range got {
        seen[it] = true
    }
    if !seen["foo.go"] || !seen["internal/foo.txt"] {
        t.Fatalf("unexpected items: %v", got)
    }
}

func TestFilePicker_HiddenFilesExcludedUnlessDotQuery(t *testing.T) {
    dir := t.TempDir()
    _ = os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0o644)
    fp := NewFilePicker(dir)
    fp.SetQuery("hid")
    if items := fp.Items(); len(items) != 0 {
        t.Fatalf("hidden leaked: %v", items)
    }
    fp.SetQuery(".hid")
    if items := fp.Items(); len(items) != 1 {
        t.Fatalf("expected 1 with .hid query, got %v", items)
    }
}

func TestFilePicker_Selection(t *testing.T) {
    dir := t.TempDir()
    _ = os.WriteFile(filepath.Join(dir, "a"), nil, 0o644)
    _ = os.WriteFile(filepath.Join(dir, "b"), nil, 0o644)
    fp := NewFilePicker(dir)
    fp.SetQuery("")
    fp.Down()
    if fp.Selected() == "" {
        t.Fatal("expected selection")
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/tui/editor/filepicker.go
package editor

import (
    "io/fs"
    "path/filepath"
    "strings"

    "github.com/sahilm/fuzzy"
)

const filePickerLimit = 50

type FilePicker struct {
    root  string
    files []string // relative paths
    query string
    items []string
    sel   int
}

func NewFilePicker(root string) *FilePicker {
    fp := &FilePicker{root: root}
    fp.scan()
    return fp
}

func (f *FilePicker) scan() {
    var paths []string
    _ = filepath.WalkDir(f.root, func(p string, d fs.DirEntry, err error) error {
        if err != nil {
            return nil
        }
        rel, _ := filepath.Rel(f.root, p)
        if rel == "." {
            return nil
        }
        // Skip hidden dirs and .git etc unless queried.
        for _, seg := range strings.Split(rel, string(filepath.Separator)) {
            if strings.HasPrefix(seg, ".") {
                if d.IsDir() {
                    return filepath.SkipDir
                }
                return nil
            }
        }
        if d.IsDir() {
            return nil
        }
        paths = append(paths, rel)
        if len(paths) > 5000 {
            return filepath.SkipAll
        }
        return nil
    })
    f.files = paths
}

func (f *FilePicker) SetQuery(q string) {
    f.query = q
    f.sel = 0
    if q == "" {
        if len(f.files) > filePickerLimit {
            f.items = f.files[:filePickerLimit]
        } else {
            f.items = append([]string{}, f.files...)
        }
        return
    }
    if strings.HasPrefix(q, ".") {
        // include hidden
        all := []string{}
        _ = filepath.WalkDir(f.root, func(p string, d fs.DirEntry, _ error) error {
            if d == nil || d.IsDir() {
                return nil
            }
            rel, _ := filepath.Rel(f.root, p)
            all = append(all, rel)
            return nil
        })
        f.items = filterFuzzy(all, q, filePickerLimit)
        return
    }
    f.items = filterFuzzy(f.files, q, filePickerLimit)
}

func filterFuzzy(corpus []string, q string, limit int) []string {
    matches := fuzzy.Find(q, corpus)
    out := make([]string, 0, limit)
    for i, m := range matches {
        if i >= limit {
            break
        }
        out = append(out, m.Str)
    }
    return out
}

func (f *FilePicker) Items() []string { return f.items }
func (f *FilePicker) Selected() string {
    if f.sel < 0 || f.sel >= len(f.items) {
        return ""
    }
    return f.items[f.sel]
}
func (f *FilePicker) Up() {
    if f.sel > 0 {
        f.sel--
    }
}
func (f *FilePicker) Down() {
    if f.sel < len(f.items)-1 {
        f.sel++
    }
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/tui/editor/ -run TestFilePicker`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/editor/filepicker.go internal/tui/editor/filepicker_test.go
git commit -m "feat(tui/editor): @ fuzzy file picker"
```

---

## Task 4: Slash command menu

**Files:**
- Create: `internal/tui/editor/slashmenu.go`
- Create: `internal/tui/editor/slashmenu_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/editor/slashmenu_test.go
package editor

import "testing"

func TestSlashMenu_FiltersOnQuery(t *testing.T) {
    cmds := []string{"/model", "/tree", "/resume", "/login"}
    m := NewSlashMenu(cmds)
    m.SetQuery("re")
    items := m.Items()
    found := map[string]bool{}
    for _, it := range items {
        found[it] = true
    }
    if !found["/resume"] || !found["/tree"] {
        t.Fatalf("expected /resume and /tree, got %v", items)
    }
}

func TestSlashMenu_ExactMatchFirst(t *testing.T) {
    m := NewSlashMenu([]string{"/model", "/modes"})
    m.SetQuery("model")
    if items := m.Items(); len(items) == 0 || items[0] != "/model" {
        t.Fatalf("/model not first: %v", items)
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/tui/editor/slashmenu.go
package editor

import (
    "sort"
    "strings"

    "github.com/sahilm/fuzzy"
)

type SlashMenu struct {
    all   []string
    query string
    items []string
    sel   int
}

func NewSlashMenu(cmds []string) *SlashMenu {
    s := &SlashMenu{all: cmds}
    sort.Strings(s.all)
    s.SetQuery("")
    return s
}

func (s *SlashMenu) SetQuery(q string) {
    s.query = q
    s.sel = 0
    if q == "" {
        s.items = append([]string{}, s.all...)
        return
    }
    matches := fuzzy.Find(q, s.all)
    out := make([]string, 0, len(matches))
    for _, m := range matches {
        out = append(out, m.Str)
    }
    // Promote exact prefix matches.
    sort.SliceStable(out, func(i, j int) bool {
        ip := strings.HasPrefix(strings.TrimPrefix(out[i], "/"), q)
        jp := strings.HasPrefix(strings.TrimPrefix(out[j], "/"), q)
        if ip != jp {
            return ip
        }
        return false
    })
    s.items = out
}

func (s *SlashMenu) Items() []string { return s.items }
func (s *SlashMenu) Selected() string {
    if s.sel < 0 || s.sel >= len(s.items) {
        return ""
    }
    return s.items[s.sel]
}
func (s *SlashMenu) Up() {
    if s.sel > 0 {
        s.sel--
    }
}
func (s *SlashMenu) Down() {
    if s.sel < len(s.items)-1 {
        s.sel++
    }
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/tui/editor/ -run TestSlashMenu`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/editor/slashmenu.go internal/tui/editor/slashmenu_test.go
git commit -m "feat(tui/editor): / command menu"
```

---

## Task 5: Tab path completion

**Files:**
- Create: `internal/tui/editor/pathcomplete.go`
- Create: `internal/tui/editor/pathcomplete_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/editor/pathcomplete_test.go
package editor

import (
    "os"
    "path/filepath"
    "testing"
)

func TestComplete_SinglePrefixUniqueExpands(t *testing.T) {
    dir := t.TempDir()
    _ = os.WriteFile(filepath.Join(dir, "alpha.go"), nil, 0o644)
    _ = os.WriteFile(filepath.Join(dir, "beta.go"), nil, 0o644)

    // Caller passes (current word, cwd). We complete using cwd.
    out, ok := CompletePath("alp", dir)
    if !ok {
        t.Fatal("expected unique completion for 'alp'")
    }
    if out != "alpha.go" {
        t.Fatalf("complete = %q", out)
    }
}

func TestComplete_AmbiguousReturnsFalse(t *testing.T) {
    dir := t.TempDir()
    _ = os.WriteFile(filepath.Join(dir, "alpha.go"), nil, 0o644)
    _ = os.WriteFile(filepath.Join(dir, "almost.go"), nil, 0o644)
    if _, ok := CompletePath("al", dir); ok {
        t.Fatal("expected ambiguity to return ok=false")
    }
}

func TestComplete_DirSlash(t *testing.T) {
    dir := t.TempDir()
    _ = os.MkdirAll(filepath.Join(dir, "internal"), 0o755)
    out, ok := CompletePath("inter", dir)
    if !ok {
        t.Fatal("expected unique dir completion")
    }
    if out != "internal/" {
        t.Fatalf("complete = %q", out)
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/tui/editor/pathcomplete.go
package editor

import (
    "os"
    "path/filepath"
    "strings"
)

// CompletePath returns a single completion if exactly one entry matches.
// `word` is the current "word" the caller has identified at the cursor.
// `cwd` is the directory to complete relative to.
func CompletePath(word, cwd string) (string, bool) {
    base, prefix := splitWord(word)
    dir := filepath.Join(cwd, base)
    entries, err := os.ReadDir(dir)
    if err != nil {
        return "", false
    }
    var matches []string
    for _, e := range entries {
        n := e.Name()
        if !strings.HasPrefix(n, prefix) {
            continue
        }
        if e.IsDir() {
            n += "/"
        }
        matches = append(matches, n)
    }
    if len(matches) != 1 {
        return "", false
    }
    return filepath.Join(base, matches[0]), true
}

func splitWord(w string) (dir, prefix string) {
    i := strings.LastIndex(w, "/")
    if i < 0 {
        return "", w
    }
    return w[:i], w[i+1:]
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/tui/editor/ -run TestComplete`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/editor/pathcomplete.go internal/tui/editor/pathcomplete_test.go
git commit -m "feat(tui/editor): tab path completion (unique-only)"
```

---

## Task 6: !command shell runner

**Files:**
- Create: `internal/tui/editor/shell.go`
- Create: `internal/tui/editor/shell_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/editor/shell_test.go
package editor

import (
    "context"
    "strings"
    "testing"
)

func TestRunShell_CapturesOutput(t *testing.T) {
    out, _ := RunShell(context.Background(), "echo hi")
    if !strings.Contains(out, "hi") {
        t.Fatalf("out = %q", out)
    }
}

func TestRunShell_ErrorIncluded(t *testing.T) {
    out, err := RunShell(context.Background(), "exit 5")
    if err == nil {
        t.Fatal("expected error")
    }
    if !strings.Contains(out, "5") && !strings.Contains(err.Error(), "5") {
        t.Fatalf("expected exit code in output, got out=%q err=%v", out, err)
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/tui/editor/shell.go
package editor

import (
    "bytes"
    "context"
    "fmt"
    "os/exec"
)

func RunShell(ctx context.Context, cmd string) (string, error) {
    c := exec.CommandContext(ctx, "bash", "-lc", cmd)
    var buf bytes.Buffer
    c.Stdout = &buf
    c.Stderr = &buf
    err := c.Run()
    if err != nil {
        if ee, ok := err.(*exec.ExitError); ok {
            return buf.String(), fmt.Errorf("exit %d", ee.ExitCode())
        }
        return buf.String(), err
    }
    return buf.String(), nil
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/tui/editor/ -run TestRunShell`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/editor/shell.go internal/tui/editor/shell_test.go
git commit -m "feat(tui/editor): !command shell runner"
```

---

## Task 7: Image attachment buffer

**Files:**
- Create: `internal/tui/editor/attachments.go`
- Create: `internal/tui/editor/attachments_test.go`

> The editor accumulates pending image attachments while the user composes. On send, attachments become `ai.ImageBlock`s in the user message. Two ingestion paths:
> 1. **File path drop**: a line starting with an absolute path ending in `.png|.jpg|.jpeg|.gif|.webp` is consumed and the line is removed.
> 2. **Base64 paste (OSC 52)**: Ctrl+V on iTerm/Kitty often delivers the image as a base64 chunk; we accept any sequence matching `data:image/<type>;base64,<...>`.

- [ ] **Step 1: Write the failing test**

```go
// internal/tui/editor/attachments_test.go
package editor

import (
    "encoding/base64"
    "os"
    "path/filepath"
    "testing"
)

func TestAttachments_AddFromPath(t *testing.T) {
    dir := t.TempDir()
    p := filepath.Join(dir, "x.png")
    _ = os.WriteFile(p, []byte{0x89, 0x50, 0x4E, 0x47}, 0o644)

    a := NewAttachments()
    if err := a.AddFromPath(p); err != nil {
        t.Fatal(err)
    }
    items := a.Drain()
    if len(items) != 1 {
        t.Fatalf("len = %d", len(items))
    }
    if items[0].MimeType != "image/png" {
        t.Fatalf("mime = %q", items[0].MimeType)
    }
}

func TestAttachments_AddFromDataURI(t *testing.T) {
    raw := []byte{0xff, 0xd8, 0xff} // jpeg magic
    uri := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)
    a := NewAttachments()
    if err := a.AddFromDataURI(uri); err != nil {
        t.Fatal(err)
    }
    items := a.Drain()
    if len(items) != 1 || items[0].MimeType != "image/jpeg" {
        t.Fatalf("items = %#v", items)
    }
    if string(items[0].Data) != string(raw) {
        t.Fatal("decoded bytes mismatch")
    }
}

func TestAttachments_DrainEmptiesBuffer(t *testing.T) {
    a := NewAttachments()
    _ = a.AddFromDataURI("data:image/png;base64,YQ==")
    _ = a.Drain()
    if got := a.Drain(); len(got) != 0 {
        t.Fatalf("expected empty after drain, got %d", len(got))
    }
}
```

- [ ] **Step 2: Implement**

```go
// internal/tui/editor/attachments.go
package editor

import (
    "encoding/base64"
    "fmt"
    "os"
    "path/filepath"
    "strings"

    "github.com/khang859/rune/internal/ai"
)

type Attachments struct {
    items []ai.ImageBlock
}

func NewAttachments() *Attachments { return &Attachments{} }

func (a *Attachments) AddFromPath(p string) error {
    b, err := os.ReadFile(p)
    if err != nil {
        return err
    }
    mime := mimeFromExt(filepath.Ext(p))
    if mime == "" {
        return fmt.Errorf("not an image: %s", p)
    }
    a.items = append(a.items, ai.ImageBlock{Data: b, MimeType: mime})
    return nil
}

func (a *Attachments) AddFromDataURI(s string) error {
    if !strings.HasPrefix(s, "data:image/") {
        return fmt.Errorf("not a data: URI")
    }
    semi := strings.Index(s, ";base64,")
    if semi < 0 {
        return fmt.Errorf("only base64 supported")
    }
    mime := s[5:semi]
    enc := s[semi+len(";base64,"):]
    raw, err := base64.StdEncoding.DecodeString(enc)
    if err != nil {
        return err
    }
    a.items = append(a.items, ai.ImageBlock{Data: raw, MimeType: mime})
    return nil
}

func (a *Attachments) Pending() int { return len(a.items) }

func (a *Attachments) Drain() []ai.ImageBlock {
    out := a.items
    a.items = nil
    return out
}

func mimeFromExt(ext string) string {
    switch strings.ToLower(ext) {
    case ".png":  return "image/png"
    case ".jpg", ".jpeg": return "image/jpeg"
    case ".gif":  return "image/gif"
    case ".webp": return "image/webp"
    }
    return ""
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/tui/editor/ -run TestAttachments`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/editor/attachments.go internal/tui/editor/attachments_test.go
git commit -m "feat(tui/editor): image attachment buffer (paths + data: URIs)"
```

---

## Task 8: New Editor — state machine wiring overlays

**Files:**
- Replace: `internal/tui/editor/editor.go` (and remove old `internal/tui/editor.go`)
- Create: `internal/tui/editor/editor_test.go`

> The new Editor exposes:
>   - `Update(msg) (handled bool, sendText string, attachments []ai.ImageBlock, slashCmd string, runShell *string, doSend bool)`
> Returning `doSend=true` tells the root model to deliver `sendText` + `attachments` as the user message. `slashCmd` non-empty means the user picked a slash command and the root should dispatch it (no LLM call). `runShell` non-nil means execute that string and either insert the output or send (the editor handles `!` vs `!!`).

- [ ] **Step 1: Implement**

```go
// internal/tui/editor/editor.go
package editor

import (
    "context"
    "os"
    "strings"

    "github.com/charmbracelet/bubbles/textarea"
    tea "github.com/charmbracelet/bubbletea"

    "github.com/khang859/rune/internal/ai"
)

type Mode int

const (
    ModeNormal Mode = iota
    ModeFilePicker
    ModeSlashMenu
)

type Editor struct {
    ta      textarea.Model
    mode    Mode
    cwd     string

    fp        *FilePicker
    slash     *SlashMenu
    slashCmds []string
    atts      *Attachments
}

func New(cwd string, slashCmds []string) *Editor {
    ta := textarea.New()
    ta.Placeholder = "type a message…"
    ta.Prompt = "› "
    ta.SetWidth(80)
    ta.SetHeight(3)
    ta.ShowLineNumbers = false
    ta.Focus()
    return &Editor{
        ta:        ta,
        cwd:       cwd,
        slashCmds: slashCmds,
        atts:      NewAttachments(),
    }
}

type Result struct {
    Send         bool
    Text         string
    Images       []ai.ImageBlock
    SlashCommand string
    InsertText   string
}

func (e *Editor) SetWidth(w int)  { e.ta.SetWidth(w) }
func (e *Editor) SetHeight(h int) { e.ta.SetHeight(h) }
func (e *Editor) Focus()          { e.ta.Focus() }
func (e *Editor) Blur()           { e.ta.Blur() }

func (e *Editor) Mode() Mode { return e.mode }
func (e *Editor) FilePicker() *FilePicker { return e.fp }
func (e *Editor) SlashMenu() *SlashMenu  { return e.slash }
func (e *Editor) PendingImages() int      { return e.atts.Pending() }

func (e *Editor) Update(msg tea.Msg) (Result, tea.Cmd) {
    if k, ok := msg.(tea.KeyMsg); ok {
        if r, cmd, handled := e.handleKey(k); handled {
            return r, cmd
        }
    }
    var cmd tea.Cmd
    e.ta, cmd = e.ta.Update(msg)
    e.maybeOpenOverlay()
    return Result{}, cmd
}

func (e *Editor) handleKey(k tea.KeyMsg) (Result, tea.Cmd, bool) {
    switch e.mode {
    case ModeFilePicker:
        switch k.Type {
        case tea.KeyEsc:
            e.closeOverlay()
            return Result{}, nil, true
        case tea.KeyUp:
            e.fp.Up()
            return Result{}, nil, true
        case tea.KeyDown:
            e.fp.Down()
            return Result{}, nil, true
        case tea.KeyEnter, tea.KeyTab:
            sel := e.fp.Selected()
            if sel != "" {
                e.replaceCurrentRefWith("@" + sel + " ")
            }
            e.closeOverlay()
            return Result{}, nil, true
        case tea.KeyRunes, tea.KeyBackspace, tea.KeySpace:
            // fall through to textarea so it edits, then re-derive query
            var cmd tea.Cmd
            e.ta, cmd = e.ta.Update(k)
            e.fp.SetQuery(e.currentRefQuery())
            return Result{}, cmd, true
        }
    case ModeSlashMenu:
        switch k.Type {
        case tea.KeyEsc:
            e.closeOverlay()
            return Result{}, nil, true
        case tea.KeyUp:
            e.slash.Up()
            return Result{}, nil, true
        case tea.KeyDown:
            e.slash.Down()
            return Result{}, nil, true
        case tea.KeyEnter, tea.KeyTab:
            sel := e.slash.Selected()
            e.closeOverlay()
            e.ta.Reset()
            return Result{SlashCommand: sel}, nil, true
        case tea.KeyRunes, tea.KeyBackspace, tea.KeySpace:
            var cmd tea.Cmd
            e.ta, cmd = e.ta.Update(k)
            e.slash.SetQuery(strings.TrimPrefix(e.currentLine(), "/"))
            return Result{}, cmd, true
        }
    case ModeNormal:
        if k.Type == tea.KeyTab {
            cur := e.currentWord()
            if cur != "" {
                if exp, ok := CompletePath(cur, e.cwd); ok {
                    e.replaceCurrentWordWith(exp)
                    return Result{}, nil, true
                }
            }
        }
        if k.Type == tea.KeyEnter && !isShiftEnter(k) {
            return e.submit(), nil, true
        }
    }
    return Result{}, nil, false
}

func (e *Editor) submit() Result {
    text := strings.TrimRight(e.ta.Value(), "\n")
    e.ta.Reset()
    if strings.HasPrefix(text, "!!") {
        cmd := strings.TrimPrefix(text, "!!")
        out, _ := RunShell(context.Background(), cmd)
        return Result{InsertText: out}
    }
    if strings.HasPrefix(text, "!") {
        cmd := strings.TrimPrefix(text, "!")
        out, _ := RunShell(context.Background(), cmd)
        text = "I ran `" + cmd + "` and it produced:\n```\n" + out + "\n```"
    }
    text = e.consumeImagePathsInline(text)
    return Result{Send: true, Text: text, Images: e.atts.Drain()}
}

// consumeImagePathsInline removes lines that are bare image paths and adds them as attachments.
func (e *Editor) consumeImagePathsInline(text string) string {
    var keep []string
    for _, line := range strings.Split(text, "\n") {
        trimmed := strings.TrimSpace(line)
        if isImageFile(trimmed) {
            if _, err := os.Stat(trimmed); err == nil {
                if err := e.atts.AddFromPath(trimmed); err == nil {
                    continue
                }
            }
        }
        keep = append(keep, line)
    }
    return strings.Join(keep, "\n")
}

func isImageFile(s string) bool {
    if s == "" || !strings.ContainsAny(s, "/\\") {
        return false
    }
    return mimeFromExt(extOf(s)) != ""
}

func extOf(s string) string {
    if i := strings.LastIndex(s, "."); i >= 0 {
        return s[i:]
    }
    return ""
}

func (e *Editor) maybeOpenOverlay() {
    line := e.currentLine()
    word := e.currentWord()
    switch {
    case strings.HasPrefix(word, "@"):
        if e.fp == nil {
            e.fp = NewFilePicker(e.cwd)
        }
        e.fp.SetQuery(strings.TrimPrefix(word, "@"))
        e.mode = ModeFilePicker
    case strings.HasPrefix(line, "/"):
        if e.slash == nil {
            e.slash = NewSlashMenu(e.slashCmds)
        }
        e.slash.SetQuery(strings.TrimPrefix(line, "/"))
        e.mode = ModeSlashMenu
    default:
        e.mode = ModeNormal
    }
}

func (e *Editor) closeOverlay() {
    e.mode = ModeNormal
    e.fp = nil
    e.slash = nil
}

func (e *Editor) currentLine() string {
    val := e.ta.Value()
    if i := strings.LastIndex(val, "\n"); i >= 0 {
        return val[i+1:]
    }
    return val
}

func (e *Editor) currentWord() string {
    line := e.currentLine()
    if i := strings.LastIndex(line, " "); i >= 0 {
        return line[i+1:]
    }
    return line
}

func (e *Editor) currentRefQuery() string {
    w := e.currentWord()
    return strings.TrimPrefix(w, "@")
}

func (e *Editor) replaceCurrentRefWith(s string) {
    val := e.ta.Value()
    cur := e.currentWord()
    if cur == "" {
        return
    }
    idx := strings.LastIndex(val, cur)
    if idx < 0 {
        return
    }
    e.ta.SetValue(val[:idx] + s)
}

func (e *Editor) replaceCurrentWordWith(s string) {
    e.replaceCurrentRefWith(s)
}

func isShiftEnter(k tea.KeyMsg) bool {
    return k.String() == "shift+enter" || k.String() == "alt+enter"
}

func (e *Editor) View(width int) string {
    return e.ta.View()
}

func (e *Editor) AddAttachmentPath(p string) error      { return e.atts.AddFromPath(p) }
func (e *Editor) AddAttachmentDataURI(s string) error    { return e.atts.AddFromDataURI(s) }
```

- [ ] **Step 2: Add a small unit test for submit + slash + tab**

```go
// internal/tui/editor/editor_test.go
package editor

import (
    "testing"

    tea "github.com/charmbracelet/bubbletea"
)

func TestEditor_EnterSendsText(t *testing.T) {
    e := New(t.TempDir(), nil)
    for _, r := range "hi" {
        e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
    }
    res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
    if !res.Send || res.Text != "hi" {
        t.Fatalf("unexpected res: %#v", res)
    }
}

func TestEditor_SlashMenuOpensAndCommitsCommand(t *testing.T) {
    e := New(t.TempDir(), []string{"/model", "/tree"})
    e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
    e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
    res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
    if res.SlashCommand == "" {
        t.Fatal("expected SlashCommand result")
    }
    if res.SlashCommand != "/model" && res.SlashCommand != "/tree" {
        t.Fatalf("slash = %q", res.SlashCommand)
    }
}
```

- [ ] **Step 3: Run test to verify it passes**

Run: `go test ./internal/tui/editor/ -run TestEditor`
Expected: PASS.

- [ ] **Step 4: Delete the old editor file**

```bash
rm internal/tui/editor.go
```

- [ ] **Step 5: Commit**

```bash
git add internal/tui/editor/editor.go internal/tui/editor/editor_test.go
git rm internal/tui/editor.go
git commit -m "feat(tui/editor): full editor — overlays, !cmd, attachments, tab"
```

---

## Task 9: RootModel — wire new editor + queue + slash dispatch

**Files:**
- Modify: `internal/tui/root.go`

> The root model now:
> - holds a `*Queue` and pushes user submissions while `streaming==true`.
> - drains the queue on `TurnDone`.
> - handles `Result.SlashCommand`: in this plan, only `/quit`, `/copy`, `/hotkeys` (placeholder), `/new` are wired. The rest land in Plan 05.
> - renders the editor with overlays.

- [ ] **Step 1: Replace the root model body** (key parts shown; preserve existing code that hasn't changed)

```go
// internal/tui/root.go (relevant changes only)

import (
    "github.com/khang859/rune/internal/tui/editor"
)

type RootModel struct {
    // ...existing fields...
    editor    *editor.Editor
    queue     *Queue
    pending   pendingMsg
}

type pendingMsg struct {
    text   string
    images []ai.ImageBlock
}

func NewRootModel(a *agent.Agent, sess *session.Session) *RootModel {
    cwd, _ := os.Getwd()
    cmds := []string{"/quit", "/new", "/copy", "/hotkeys"}
    return &RootModel{
        agent:    a,
        sess:     sess,
        styles:   DefaultStyles(),
        msgs:     NewMessages(80),
        viewport: viewport.New(80, 20),
        editor:   editor.New(cwd, cmds),
        footer:   Footer{Cwd: cwd, Session: sess.Name, Model: sess.Model},
        queue:    &Queue{},
    }
}

func (m *RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch v := msg.(type) {
    case tea.WindowSizeMsg:
        m.width, m.height = v.Width, v.Height
        m.layout()
        m.refreshViewport()
        return m, nil

    case AgentEventMsg:
        m.handleEvent(v.Event)
        m.refreshViewport()
        return m, nextEventCmd(m.eventCh)

    case AgentChannelDoneMsg:
        m.streaming = false
        m.eventCh = nil
        m.cancel = nil
        m.editor.Focus()
        if text, ok := m.queue.Pop(); ok {
            return m, m.startTurn(text, nil)
        }
        return m, nil
    }

    if k, ok := msg.(tea.KeyMsg); ok {
        if m.streaming && k.Type == tea.KeyEsc && m.cancel != nil {
            m.cancel()
            return m, nil
        }
        if !m.streaming && k.Type == tea.KeyCtrlC {
            return m, tea.Quit
        }
    }

    res, cmd := m.editor.Update(msg)
    if res.Send {
        text := res.Text
        if m.streaming {
            m.queue.Push(text)
            m.msgs.OnTurnError(infoQueued(m.queue.Len()))
            m.refreshViewport()
            return m, cmd
        }
        m.msgs.AppendUser(text)
        m.refreshViewport()
        return m, m.startTurn(text, res.Images)
    }
    if res.InsertText != "" {
        // !! command: just show the result inline (don't send)
        m.msgs.OnTurnDone("") // no-op marker
        m.msgs.AppendUser("!" + res.InsertText)
        m.refreshViewport()
    }
    if res.SlashCommand != "" {
        m.handleSlashCommand(res.SlashCommand)
    }
    return m, cmd
}

func infoQueued(n int) error {
    return fmt.Errorf("queued (%d in queue)", n)
}

func (m *RootModel) startTurn(text string, images []ai.ImageBlock) tea.Cmd {
    ctx, cancel := context.WithCancel(context.Background())
    m.cancel = cancel
    m.streaming = true
    m.editor.Blur()
    content := []ai.ContentBlock{ai.TextBlock{Text: text}}
    for _, im := range images {
        content = append(content, im)
    }
    msg := ai.Message{Role: ai.RoleUser, Content: content}
    ch := m.agent.Run(ctx, msg)
    m.eventCh = ch
    return nextEventCmd(ch)
}

func (m *RootModel) handleSlashCommand(cmd string) {
    switch cmd {
    case "/quit":
        // signal quit through tea by using teaCmdQuit
        // Tea will handle on next tick.
        // For simplicity: panic-trap or use os.Exit; here we set a flag and quit on next Update.
        // A clean way: return tea.Quit from Update — see wrapper below.
    case "/new":
        // Switch to a fresh session — Plan 05 implements this fully; here we just reset messages/viewport.
        m.msgs = NewMessages(m.width)
        m.refreshViewport()
    case "/copy":
        // copy last assistant text to clipboard — Plan 05.
    case "/hotkeys":
        m.msgs.OnTurnError(fmt.Errorf("(hotkeys list lands in Plan 05)"))
        m.refreshViewport()
    }
}
```

> Quitting from a slash command: the cleanest fix is for `handleSlashCommand` to return a `tea.Cmd`. Refactor accordingly:

```go
func (m *RootModel) handleSlashCommand(cmd string) tea.Cmd {
    switch cmd {
    case "/quit":
        return tea.Quit
    case "/new":
        m.msgs = NewMessages(m.width)
        m.refreshViewport()
    case "/hotkeys":
        m.msgs.OnTurnError(fmt.Errorf("(hotkeys list lands in Plan 05)"))
        m.refreshViewport()
    }
    return nil
}
```

And in Update:
```go
if res.SlashCommand != "" {
    if c := m.handleSlashCommand(res.SlashCommand); c != nil {
        return m, c
    }
}
```

- [ ] **Step 2: View must render the overlay (file picker / slash menu) below the editor**

```go
func (m *RootModel) View() string {
    msgArea := m.viewport.View()
    edArea := m.styles.EditorBox.Render(m.editor.View(m.width))
    overlay := ""
    switch m.editor.Mode() {
    case editor.ModeFilePicker:
        overlay = renderList("files", m.editor.FilePicker().Items())
    case editor.ModeSlashMenu:
        overlay = renderList("commands", m.editor.SlashMenu().Items())
    }
    foot := m.footer.Render(m.styles)
    if overlay != "" {
        return msgArea + "\n" + edArea + "\n" + overlay + "\n" + foot
    }
    return msgArea + "\n" + edArea + "\n" + foot
}

func renderList(title string, items []string) string {
    if len(items) == 0 {
        return "(no " + title + ")"
    }
    out := title + ":"
    for i, it := range items {
        if i >= 8 {
            out += "\n  …"
            break
        }
        out += "\n  " + it
    }
    return out
}
```

- [ ] **Step 3: Run all tests**

Run: `make all`
Expected: PASS.

- [ ] **Step 4: Manual smoke (optional)**

```bash
go run ./cmd/rune
```
Expected: type `@`, watch file picker open. Type `/`, watch command menu. Tab on a partial path expands. `!ls` runs ls and sends output as message.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/root.go
git commit -m "feat(tui): wire new editor with overlays, queue, slash dispatch"
```

---

## Task 10: Editor auto-grow

**Files:**
- Modify: `internal/tui/editor/editor.go`
- Modify: `internal/tui/editor/editor_test.go`

> Plan 03 used a fixed 3-row textarea, which wastes vertical space on a typical one-liner and is too small for a long pasted message. After this task the editor starts at one row and grows row-per-newline, capped so the message viewport always has room.

- [ ] **Step 1: Add a pure helper + max constant**

In `internal/tui/editor/editor.go`, near the top of the file:

```go
// maxEditorRows caps the auto-grown editor so the viewport always has room.
const maxEditorRows = 8

// rowsFor returns the number of rows the textarea should occupy for the given value.
// At least 1, at most maxEditorRows. Counts only newlines — wrapping is handled
// by the textarea itself and not modeled here.
func rowsFor(value string) int {
    n := 1 + strings.Count(value, "\n")
    if n < 1 {
        n = 1
    }
    if n > maxEditorRows {
        n = maxEditorRows
    }
    return n
}
```

In `New(...)`, change `ta.SetHeight(3)` to `ta.SetHeight(1)`.

In `(e *Editor) Update(msg tea.Msg)`, after the textarea has consumed `msg` and before returning, recompute height:

```go
e.ta.SetHeight(rowsFor(e.ta.Value()))
```

(Apply the same recompute inside any `handleKey` branch that mutates the textarea — `KeyRunes`, `KeyBackspace`, `KeySpace`, the textarea-fallthrough cases.)

- [ ] **Step 2: Write tests for `rowsFor`**

In `internal/tui/editor/editor_test.go`:

```go
func TestRowsFor(t *testing.T) {
    cases := []struct {
        in   string
        want int
    }{
        {"", 1},
        {"hi", 1},
        {"a\nb", 2},
        {"a\nb\nc", 3},
        {strings.Repeat("x\n", 50), maxEditorRows},
    }
    for _, c := range cases {
        if got := rowsFor(c.in); got != c.want {
            t.Errorf("rowsFor(%q) = %d, want %d", c.in, got, c.want)
        }
    }
}
```

- [ ] **Step 3: Run tests**

Run: `go test ./internal/tui/editor/ -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/editor/editor.go internal/tui/editor/editor_test.go
git commit -m "feat(tui): editor auto-grows with newlines up to maxEditorRows"
```

---

## Task 11: Viewport scrolling

**Files:**
- Modify: `internal/tui/tui.go`
- Modify: `internal/tui/root.go`
- Modify: `internal/tui/root_test.go`

> Plan 03 mounted a viewport but never routed scroll events to it, and `refreshViewport` always snapped to the bottom — so even if the user could scroll, the next streamed event would yank them back. Wire mouse-wheel scrolling, route PgUp/PgDn/Home/End to the viewport, and only auto-scroll if the user is already at the bottom.

- [ ] **Step 1: Enable mouse cell motion**

In `internal/tui/tui.go`, change:

```go
p := tea.NewProgram(NewRootModel(a, s), tea.WithAltScreen())
```

to:

```go
p := tea.NewProgram(NewRootModel(a, s), tea.WithAltScreen(), tea.WithMouseCellMotion())
```

- [ ] **Step 2: Sticky-bottom in `refreshViewport`**

In `internal/tui/root.go`, replace `refreshViewport`:

```go
func (m *RootModel) refreshViewport() {
    atBottom := m.viewport.AtBottom()
    m.viewport.SetContent(m.msgs.Render(m.styles))
    if atBottom {
        m.viewport.GotoBottom()
    }
}
```

- [ ] **Step 3: Forward mouse + scroll keys to viewport**

In `(m *RootModel) Update`, add a `tea.MouseMsg` case alongside the existing `case tea.KeyMsg:` etc. — before the trailing fallback:

```go
case tea.MouseMsg:
    var cmd tea.Cmd
    m.viewport, cmd = m.viewport.Update(msg)
    return m, cmd
```

Inside the non-streaming `tea.KeyMsg` branch, before delegating remaining keys to the editor, intercept scroll keys:

```go
switch v.Type {
case tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
    var cmd tea.Cmd
    m.viewport, cmd = m.viewport.Update(msg)
    return m, cmd
}
```

(Order matters: this `switch` goes after the `case tea.KeyEnter:` arm and the editor-overlay handling but before the editor's general-input fallthrough, so PgUp/PgDn never reach the editor.)

- [ ] **Step 4: Regression test — refresh during scrollback does not snap**

In `internal/tui/root_test.go`:

```go
func TestRoot_RefreshDoesNotJumpWhenScrolledUp(t *testing.T) {
    s := session.New("gpt-5")
    a := agent.New(faux.New(), tools.NewRegistry(), s, "")
    m := NewRootModel(a, s)
    m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

    for i := 0; i < 50; i++ {
        m.msgs.AppendUser(fmt.Sprintf("line %d", i))
    }
    m.refreshViewport()
    m.viewport.GotoTop()
    if m.viewport.AtBottom() {
        t.Fatal("expected viewport not at bottom after GotoTop")
    }
    m.msgs.AppendUser("incoming streamed line")
    m.refreshViewport()
    if m.viewport.AtBottom() {
        t.Fatal("refresh snapped to bottom while user was scrolled up")
    }
}
```

- [ ] **Step 5: Run all tests**

Run: `make all`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/tui.go internal/tui/root.go internal/tui/root_test.go
git commit -m "feat(tui): viewport scrolling — mouse wheel, PgUp/PgDn, sticky bottom"
```

---

## End state

After Plan 04, rune editor matches pi:

- `@` opens fuzzy file picker; selection inserts `@path/to/file` reference.
- `/` opens slash command menu (only `/quit`, `/new`, `/copy`, `/hotkeys` wired in this plan; the others fill in during Plan 05).
- Tab completes unique path prefixes.
- `!cmd` runs and sends output as a message; `!!cmd` runs without sending.
- Image attachments via inline file path or `data:image/...` paste.
- Message queue: pressing Enter mid-stream queues; queue drains on turn end.
- Editor auto-grows from 1 row up to `maxEditorRows` as the user adds newlines.
- Message viewport scrolls with the mouse wheel and PgUp/PgDn/Home/End; streamed events only auto-scroll when the user is already at the bottom (no jump-back during scrollback).

Plan 05 lands `/model`, `/tree`, `/resume`, `/settings` modals plus auto-compact on context overflow.
