package modal

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type SecretInputResult struct {
	Action string
	Value  string
}

type SecretInput struct {
	title  string
	action string
	input  textinput.Model
	err    string
}

func NewSecretInput(title, action string) Modal {
	ti := textinput.New()
	ti.Focus()
	ti.EchoMode = textinput.EchoPassword
	ti.EchoCharacter = '•'
	return &SecretInput{title: title, action: action, input: ti}
}

func (s *SecretInput) Init() tea.Cmd { return textinput.Blink }

func (s *SecretInput) Update(msg tea.Msg) (Modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyEnter:
			return s, Result(SecretInputResult{Action: s.action, Value: s.input.Value()})
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

func (s *SecretInput) View(width, height int) string {
	var b strings.Builder
	if s.title != "" {
		b.WriteString(s.title)
	} else {
		b.WriteString("Secret")
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
