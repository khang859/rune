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
