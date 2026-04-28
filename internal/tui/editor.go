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
