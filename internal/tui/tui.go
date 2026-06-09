package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/mcp"
	"github.com/khang859/rune/internal/profile"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/skill"
)

func Run(a *agent.Agent, s *session.Session, skills []skill.Skill, mcpStatuses []mcp.Status, version string) error {
	return RunWithProfile(a, s, "", skills, mcpStatuses, version, nil, "")
}

// RunWithProfile starts the TUI. activeProfile is the active provider profile
// ID (settings/--provider, shown in the footer); worker is the optional
// --profile worker role whose persona and skill set are applied to the agent.
// startupNotice, when non-empty, is shown as an info banner on entry (e.g. a
// provider/auth recovery hint when the configured provider failed to build).
func RunWithProfile(a *agent.Agent, s *session.Session, activeProfile string, skills []skill.Skill, mcpStatuses []mcp.Status, version string, worker *profile.Profile, startupNotice string) error {
	m := NewRootModel(a, s)
	m.SetActiveProfile(activeProfile)
	m.SetWorkerProfile(worker)
	m.SetVersion(version)
	m.SetSkills(skills)
	m.SetMCPStatuses(mcpStatuses)
	if s != nil && len(s.PathToActiveNodes()) > 0 {
		m.rebuildMessagesFromSession()
	}
	m.SetStartupNotice(startupNotice)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseCellMotion())
	defer fmt.Print(ansi.PopKittyKeyboard(1))
	final, err := p.Run()
	if rm, ok := final.(*RootModel); ok {
		if t := rm.Transcript(); t != "" {
			fmt.Println("\n" + rm.styles.Info.Render("── rune transcript ──") + "\n")
			fmt.Println(t)
		}
	}
	return err
}
