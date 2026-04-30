package modal

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type TextInputResult struct {
	Action string
	Value  string
}

type TextInput struct {
	title  string
	action string
	input  textinput.Model
	err    string
}

func NewTextInput(title, action, value string) Modal {
	ti := textinput.New()
	ti.Focus()
	ti.SetValue(value)
	return &TextInput{title: title, action: action, input: ti}
}

func (s *TextInput) Init() tea.Cmd { return textinput.Blink }

func (s *TextInput) Update(msg tea.Msg) (Modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyEnter:
			return s, Result(TextInputResult{Action: s.action, Value: strings.TrimSpace(s.input.Value())})
		case tea.KeyEsc:
			return s, Cancel()
		case tea.KeyCtrlU:
			s.input.SetValue("")
			return s, nil
		}
	}
	var cmd tea.Cmd
	s.input, cmd = s.input.Update(msg)
	return s, cmd
}

func (s *TextInput) View(width, height int) string {
	var b strings.Builder
	if s.title != "" {
		b.WriteString(s.title)
	} else {
		b.WriteString("Input")
	}
	b.WriteString("\n")
	b.WriteString(s.input.View())
	b.WriteString("\n")
	if s.err != "" {
		fmt.Fprintf(&b, "%s\n", s.err)
	}
	b.WriteString("\n(Enter save, Esc cancel, Ctrl+U clear)")
	return b.String()
}
