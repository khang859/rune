// internal/tui/modal/settings.go
package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Settings struct {
	Effort       string
	IconMode     string
	ActivityMode string
}

type SettingsModal struct {
	cur  Settings
	rows []settingsRow
	sel  int
}

type settingsRow struct {
	label   string
	options []string
	value   int
}

func NewSettings(cur Settings) Modal {
	cur = normalizeSettings(cur)
	return &SettingsModal{cur: cur, rows: []settingsRow{
		newSettingsRow("thinking effort", []string{"minimal", "low", "medium", "high"}, cur.Effort),
		newSettingsRow("icon mode", []string{"auto", "nerd", "unicode", "ascii"}, cur.IconMode),
		newSettingsRow("activity indicator", []string{"off", "simple", "arcane"}, cur.ActivityMode),
	}}
}

func normalizeSettings(s Settings) Settings {
	if s.Effort == "" {
		s.Effort = "minimal"
	}
	if s.IconMode == "" {
		s.IconMode = "unicode"
	}
	if s.ActivityMode == "" {
		s.ActivityMode = "arcane"
	}
	return s
}

func newSettingsRow(label string, options []string, current string) settingsRow {
	row := settingsRow{label: label, options: options}
	for i, opt := range options {
		if opt == current {
			row.value = i
			break
		}
	}
	return row
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
			if s.sel < len(s.rows)-1 {
				s.sel++
			}
		case tea.KeyLeft:
			s.cycleSelected(-1)
		case tea.KeyRight:
			s.cycleSelected(1)
		case tea.KeyEnter:
			return s, Result(s.selectedSettings())
		case tea.KeyEsc:
			return s, Cancel()
		}
	}
	return s, nil
}

func (s *SettingsModal) cycleSelected(delta int) {
	row := &s.rows[s.sel]
	row.value = (row.value + delta + len(row.options)) % len(row.options)
}

func (s *SettingsModal) selectedSettings() Settings {
	return Settings{
		Effort:       s.rows[0].options[s.rows[0].value],
		IconMode:     s.rows[1].options[s.rows[1].value],
		ActivityMode: s.rows[2].options[s.rows[2].value],
	}
}

func (s *SettingsModal) View(width, height int) string {
	var sb strings.Builder
	sb.WriteString("Settings\n")
	for i, row := range s.rows {
		m := "  "
		if i == s.sel {
			m = "> "
		}
		sb.WriteString(fmt.Sprintf("%s%s: %s\n", m, row.label, row.options[row.value]))
	}
	sb.WriteString("\n(↑/↓ select, ←/→ change, Enter apply, Esc cancel)")
	return sb.String()
}
