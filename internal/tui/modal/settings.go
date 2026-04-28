// internal/tui/modal/settings.go
package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Settings struct {
	Effort string
}

type SettingsModal struct {
	cur     Settings
	options []string
	sel     int
}

func NewSettings(cur Settings) Modal {
	options := []string{"minimal", "low", "medium", "high"}
	sel := 0
	for i, o := range options {
		if o == cur.Effort {
			sel = i
			break
		}
	}
	return &SettingsModal{cur: cur, options: options, sel: sel}
}

func (s *SettingsModal) Init() tea.Cmd { return nil }

func (s *SettingsModal) Update(msg tea.Msg) (Modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyUp:
			if s.sel > 0 {
				s.sel--
			}
		case tea.KeyDown:
			if s.sel < len(s.options)-1 {
				s.sel++
			}
		case tea.KeyEnter:
			return s, Result(Settings{Effort: s.options[s.sel]})
		case tea.KeyEsc:
			return s, Cancel()
		}
	}
	return s, nil
}

func (s *SettingsModal) View(width, height int) string {
	var sb strings.Builder
	sb.WriteString("Settings — thinking effort:\n")
	for i, o := range s.options {
		m := "  "
		if i == s.sel {
			m = "> "
		}
		sb.WriteString(fmt.Sprintf("%s%s\n", m, o))
	}
	sb.WriteString("\n(Enter to apply, Esc to cancel)")
	return sb.String()
}
