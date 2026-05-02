package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/mcp"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/skill"
)

func Run(a *agent.Agent, s *session.Session, skills []skill.Skill, mcpStatuses []mcp.Status, version string) error {
	return RunWithProfile(a, s, "", skills, mcpStatuses, version)
}

func RunWithProfile(a *agent.Agent, s *session.Session, activeProfile string, skills []skill.Skill, mcpStatuses []mcp.Status, version string) error {
	m := NewRootModel(a, s)
	m.SetActiveProfile(activeProfile)
	m.SetVersion(version)
	m.SetSkills(skills)
	m.SetMCPStatuses(mcpStatuses)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	defer fmt.Print(ansi.PopKittyKeyboard(1))
	_, err := p.Run()
	return err
}
