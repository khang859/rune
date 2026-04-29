package editor

import (
	"context"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/khang859/rune/internal/ai"
)

// promptWidth is the cell width of the textarea Prompt ("› ").
const promptWidth = 2

type Mode int

const (
	ModeNormal Mode = iota
	ModeFilePicker
	ModeSlashMenu
)

// maxEditorRows caps the auto-grown editor so the viewport always has room.
// Beyond this the textarea's internal viewport scrolls.
const maxEditorRows = 8

// rowsFor returns the number of rows the textarea should occupy for the given value
// at the given outer width. At least 1, at most maxEditorRows. Each \n-separated
// line contributes ceil(visualWidth / wrapWidth) visual rows.
func rowsFor(value string, width int) int {
	wrapWidth := width - promptWidth
	if wrapWidth < 1 {
		wrapWidth = 1
	}
	n := 0
	for _, line := range strings.Split(value, "\n") {
		w := lipgloss.Width(line)
		rows := 1 + (w-1)/wrapWidth
		if rows < 1 {
			rows = 1
		}
		n += rows
	}
	if n < 1 {
		n = 1
	}
	if n > maxEditorRows {
		n = maxEditorRows
	}
	return n
}

type Editor struct {
	ta    textarea.Model
	mode  Mode
	cwd   string
	width int

	fp        *FilePicker
	slash     *SlashMenu
	slashCmds []string
	atts      *Attachments
	hist      *History
}

func New(cwd string, slashCmds []string) *Editor {
	ta := textarea.New()
	ta.KeyMap.InsertNewline = key.NewBinding(
		key.WithKeys("shift+enter", "alt+enter", "ctrl+j"),
		key.WithHelp("shift+enter", "insert newline"),
	)
	ta.Placeholder = "type a message…"
	ta.Prompt = "› "
	ta.SetWidth(80)
	ta.SetHeight(1)
	ta.ShowLineNumbers = false
	ta.Focus()
	return &Editor{
		ta:        ta,
		cwd:       cwd,
		width:     80,
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
	RanCommand   string
}

func (e *Editor) SetWidth(w int) {
	e.width = w
	e.ta.SetWidth(w)
	e.updateHeight()
}
func (e *Editor) SetHeight(h int) { e.ta.SetHeight(h) }
func (e *Editor) Focus()          { e.ta.Focus() }
func (e *Editor) Blur()           { e.ta.Blur() }
func (e *Editor) Rows() int       { return rowsFor(e.ta.Value(), e.width) }

func (e *Editor) Mode() Mode                 { return e.mode }
func (e *Editor) FilePicker() *FilePicker    { return e.fp }
func (e *Editor) SlashMenu() *SlashMenu      { return e.slash }
func (e *Editor) PendingImages() int         { return e.atts.Pending() }
func (e *Editor) SetSlashCmds(cmds []string) { e.slashCmds = cmds }
func (e *Editor) SetHistory(h *History)      { e.hist = h }

func (e *Editor) Update(msg tea.Msg) (Result, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		if r, cmd, handled := e.handleKey(k); handled {
			return r, cmd
		}
	}
	// The textarea scrolls its internal viewport during Update based on its
	// current height. Since this wrapper auto-grows after edits, a newline typed
	// while the editor is one row tall can make the textarea scroll to the new
	// blank line, hiding the existing content. Pre-grow for Shift+Enter so the
	// textarea can keep both lines visible while it processes the key.
	if k, ok := msg.(tea.KeyMsg); ok && isShiftEnter(k) {
		e.ta.SetHeight(rowsFor(e.ta.Value()+"\n", e.width))
	}
	var cmd tea.Cmd
	e.ta, cmd = e.ta.Update(msg)
	e.maybeOpenOverlay()
	e.updateHeight()
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
			// fall through to textarea so it edits, then re-derive mode/query —
			// deleting the leading '@' must close the overlay.
			var cmd tea.Cmd
			e.ta, cmd = e.ta.Update(k)
			e.maybeOpenOverlay()
			e.updateHeight()
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
			if sel == "" {
				e.closeOverlay()
				if k.Type == tea.KeyEnter && !isShiftEnter(k) {
					return e.submit(), nil, true
				}
				return Result{}, nil, true
			}
			e.closeOverlay()
			e.ta.Reset()
			e.updateHeight()
			return Result{SlashCommand: sel}, nil, true
		case tea.KeyRunes, tea.KeyBackspace, tea.KeySpace:
			var cmd tea.Cmd
			e.ta, cmd = e.ta.Update(k)
			e.maybeOpenOverlay()
			e.updateHeight()
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
		if k.Type == tea.KeyUp && e.hist != nil && e.atFirstLine() {
			if text, ok := e.hist.Prev(e.ta.Value()); ok {
				e.setValueFromHistory(text)
				return Result{}, nil, true
			}
		}
		if k.Type == tea.KeyDown && e.hist != nil && e.hist.Navigating() && e.atLastLine() {
			if text, ok := e.hist.Next(); ok {
				e.setValueFromHistory(text)
				return Result{}, nil, true
			}
		}
		if k.Type == tea.KeyEnter && !isShiftEnter(k) {
			return e.submit(), nil, true
		}
	}
	return Result{}, nil, false
}

func (e *Editor) submit() Result {
	text := strings.TrimSpace(e.ta.Value())
	e.ta.Reset()
	e.updateHeight()
	if text == "" && e.atts.Pending() == 0 {
		if e.hist != nil {
			e.hist.Reset()
		}
		return Result{}
	}
	if e.hist != nil {
		e.hist.Push(text)
	}
	if strings.HasPrefix(text, "!!") {
		cmd := strings.TrimPrefix(text, "!!")
		out, _ := RunShell(context.Background(), cmd)
		return Result{RanCommand: cmd, InsertText: out}
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

func (e *Editor) updateHeight() {
	e.ta.SetHeight(rowsFor(e.ta.Value(), e.width))
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
	e.updateHeight()
}

func (e *Editor) replaceCurrentWordWith(s string) {
	e.replaceCurrentRefWith(s)
}

func isShiftEnter(k tea.KeyMsg) bool {
	return k.String() == "shift+enter" || k.String() == "alt+enter" || k.Type == tea.KeyCtrlJ
}

func (e *Editor) atFirstLine() bool { return e.ta.Line() == 0 }

func (e *Editor) atLastLine() bool { return e.ta.Line() == e.ta.LineCount()-1 }

func (e *Editor) setValueFromHistory(text string) {
	e.ta.SetValue(text)
	e.updateHeight()
}

func (e *Editor) View(width int) string {
	return e.ta.View()
}

func (e *Editor) AddAttachmentPath(p string) error    { return e.atts.AddFromPath(p) }
func (e *Editor) AddAttachmentDataURI(s string) error { return e.atts.AddFromDataURI(s) }
