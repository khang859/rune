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

type ShellMode int

const (
	ShellModeNone   ShellMode = iota
	ShellModeSend             // "!cmd": run shell, send output to AI
	ShellModeInsert           // "!!cmd": run shell, insert output locally
)

// defaultMaxEditorRows is the fallback cap before the host calls SetMaxRows.
// The host (root layout) overrides this based on terminal height.
const defaultMaxEditorRows = 8

// rowsFor returns the raw number of visual rows the value needs at the given
// outer width. Each \n-separated line contributes ceil(visualWidth / wrapWidth)
// visual rows. Callers cap with the editor's maxRows.
func rowsFor(value string, width int) int {
	wrapWidth := wrapWidthFor(width)
	n := 0
	for _, line := range strings.Split(value, "\n") {
		n += visualRowsForLine(line, wrapWidth)
	}
	if n < 1 {
		n = 1
	}
	return n
}

func wrapWidthFor(width int) int {
	w := width - promptWidth
	if w < 1 {
		w = 1
	}
	return w
}

func visualRowsForLine(line string, wrapWidth int) int {
	w := lipgloss.Width(line)
	rows := 1 + (w-1)/wrapWidth
	if rows < 1 {
		rows = 1
	}
	return rows
}

type Editor struct {
	ta      textarea.Model
	mode    Mode
	cwd     string
	width   int
	maxRows int

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
		maxRows:   defaultMaxEditorRows,
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

// Reset clears the textarea and any open overlay (file picker, slash menu),
// and resets history navigation so the next Up arrow starts from the latest
// entry. Used by the host when surfacing a "discard input" affordance like
// Ctrl+C's first press.
func (e *Editor) Reset() {
	e.ta.Reset()
	e.closeOverlay()
	if e.hist != nil {
		e.hist.Reset()
	}
	e.updateHeight()
}
func (e *Editor) Rows() int    { return e.cap(rowsFor(e.ta.Value(), e.width)) }
func (e *Editor) RawRows() int { return rowsFor(e.ta.Value(), e.width) }

func (e *Editor) SetMaxRows(n int) {
	if n < 1 {
		n = 1
	}
	if e.maxRows == n {
		return
	}
	e.maxRows = n
	e.updateHeight()
}

func (e *Editor) cap(n int) int {
	if e.maxRows > 0 && n > e.maxRows {
		return e.maxRows
	}
	return n
}

// ScrollState reports how many wrapped rows are hidden above and below the
// visible editor viewport. Both zero when all content fits.
func (e *Editor) ScrollState() (above, below int) {
	visible := e.Rows()
	total := e.RawRows()
	if total <= visible {
		return 0, 0
	}
	cursor := e.cursorVisualRow()
	offset := 0
	if cursor >= visible {
		offset = cursor - visible + 1
	}
	if max := total - visible; offset > max {
		offset = max
	}
	return offset, total - visible - offset
}

func (e *Editor) cursorVisualRow() int {
	wrapWidth := wrapWidthFor(e.width)
	val := e.ta.Value()
	line := e.ta.Line()
	row := 0
	lines := strings.Split(val, "\n")
	for i := 0; i < line && i < len(lines); i++ {
		row += visualRowsForLine(lines[i], wrapWidth)
	}
	return row + e.ta.LineInfo().RowOffset
}

func (e *Editor) Mode() Mode { return e.mode }
func (e *Editor) ShellMode() ShellMode {
	v := strings.TrimSpace(e.ta.Value())
	switch {
	case strings.HasPrefix(v, "!!"):
		return ShellModeInsert
	case strings.HasPrefix(v, "!"):
		return ShellModeSend
	default:
		return ShellModeNone
	}
}
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
		// Only navigate history when the input is empty (so a draft is never
		// destroyed) or when already mid-navigation (so Up/Down keep walking
		// the same recall sequence). Down also still requires the existing
		// Navigating() guard.
		if k.Type == tea.KeyUp && e.hist != nil && e.atFirstVisualRow() &&
			(e.ta.Value() == "" || e.hist.Navigating()) {
			if text, ok := e.hist.Prev(e.ta.Value()); ok {
				e.setValueFromHistory(text)
				return Result{}, nil, true
			}
		}
		if k.Type == tea.KeyDown && e.hist != nil && e.hist.Navigating() && e.atLastVisualRow() {
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

// updateHeight keeps the textarea sized to its full allowed budget (maxRows)
// rather than shrinking to current content. The textarea's internal viewport
// scrolls to follow the cursor when its height is smaller than content, and
// in v1.0.0 of bubbles/textarea the YOffset is not reset when height grows
// back. By keeping height fixed at maxRows, the textarea never needs to
// scroll until content actually exceeds maxRows. We crop the rendered view
// to the current row count in View() so the editor box still hugs content.
func (e *Editor) updateHeight() {
	if e.maxRows > 0 {
		e.ta.SetHeight(e.maxRows)
	}
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

func (e *Editor) atFirstVisualRow() bool {
	return e.ta.Line() == 0 && e.ta.LineInfo().RowOffset == 0
}

func (e *Editor) atLastVisualRow() bool {
	if e.ta.Line() != e.ta.LineCount()-1 {
		return false
	}
	info := e.ta.LineInfo()
	if info.Height <= 0 {
		return true
	}
	return info.RowOffset >= info.Height-1
}

func (e *Editor) setValueFromHistory(text string) {
	e.ta.SetValue(text)
	e.updateHeight()
}

func (e *Editor) View(width int) string {
	raw := e.ta.View()
	visible := e.Rows()
	if visible <= 0 {
		visible = 1
	}
	lines := strings.Split(raw, "\n")
	if len(lines) <= visible {
		return raw
	}
	return strings.Join(lines[:visible], "\n")
}

func (e *Editor) AddAttachmentPath(p string) error    { return e.atts.AddFromPath(p) }
func (e *Editor) AddAttachmentDataURI(s string) error { return e.atts.AddFromDataURI(s) }
