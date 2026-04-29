package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/skill"
)

func Run(a *agent.Agent, s *session.Session, skills []skill.Skill) error {
	m := NewRootModel(a, s)
	m.SetSkills(skills)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	fmt.Print(ansi.PopKittyKeyboard(1))
	return err
}
