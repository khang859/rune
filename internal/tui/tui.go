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
