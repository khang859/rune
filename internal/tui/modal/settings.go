// internal/tui/modal/settings.go
package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Settings struct {
	Effort            string
	IconMode          string
	ActivityMode      string
	WebFetch          string
	FetchPrivateURLs  string
	WebSearch         string
	SearchProvider    string
	BraveAPIKeyStatus string
}

type SettingsAction struct {
	Action   string
	Settings Settings
}

type SettingsModal struct {
	cur  Settings
	rows []settingsRow
	sel  int
}

type settingsRowKind int

const (
	settingsRowEnum settingsRowKind = iota
	settingsRowAction
)

type settingsRow struct {
	kind    settingsRowKind
	label   string
	options []string
	value   int
	action  string
	status  string
}

func NewSettings(cur Settings) Modal {
	cur = normalizeSettings(cur)
	return &SettingsModal{cur: cur, rows: []settingsRow{
		newSettingsRow("thinking effort", []string{"minimal", "low", "medium", "high"}, cur.Effort),
		newSettingsRow("icon mode", []string{"auto", "nerd", "unicode", "ascii"}, cur.IconMode),
		newSettingsRow("activity indicator", []string{"off", "simple", "arcane"}, cur.ActivityMode),
		newSettingsRow("web fetch", []string{"off", "on"}, cur.WebFetch),
		newSettingsRow("fetch private urls", []string{"off", "on"}, cur.FetchPrivateURLs),
		newSettingsRow("web search", []string{"auto", "off", "on"}, cur.WebSearch),
		newSettingsRow("search provider", []string{"auto", "brave", "searxng"}, cur.SearchProvider),
		{kind: settingsRowAction, label: "brave api key", action: "brave_api_key", status: cur.BraveAPIKeyStatus},
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
	if s.WebFetch == "" {
		s.WebFetch = "on"
	}
	if s.FetchPrivateURLs == "" {
		s.FetchPrivateURLs = "off"
	}
	if s.WebSearch == "" {
		s.WebSearch = "auto"
	}
	if s.SearchProvider == "" {
		s.SearchProvider = "auto"
	}
	if s.BraveAPIKeyStatus == "" {
		s.BraveAPIKeyStatus = "missing — Enter to set"
	}
	return s
}

func newSettingsRow(label string, options []string, current string) settingsRow {
	row := settingsRow{kind: settingsRowEnum, label: label, options: options}
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
			row := s.rows[s.sel]
			if row.kind == settingsRowAction {
				return s, Result(SettingsAction{Action: row.action, Settings: s.selectedSettings()})
			}
			return s, Result(s.selectedSettings())
		case tea.KeyEsc:
			return s, Cancel()
		}
	}
	return s, nil
}

func (s *SettingsModal) cycleSelected(delta int) {
	row := &s.rows[s.sel]
	if row.kind != settingsRowEnum || len(row.options) == 0 {
		return
	}
	row.value = (row.value + delta + len(row.options)) % len(row.options)
}

func (s *SettingsModal) selectedSettings() Settings {
	return Settings{
		Effort:            s.rows[0].options[s.rows[0].value],
		IconMode:          s.rows[1].options[s.rows[1].value],
		ActivityMode:      s.rows[2].options[s.rows[2].value],
		WebFetch:          s.rows[3].options[s.rows[3].value],
		FetchPrivateURLs:  s.rows[4].options[s.rows[4].value],
		WebSearch:         s.rows[5].options[s.rows[5].value],
		SearchProvider:    s.rows[6].options[s.rows[6].value],
		BraveAPIKeyStatus: s.cur.BraveAPIKeyStatus,
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
		if row.kind == settingsRowAction {
			sb.WriteString(fmt.Sprintf("%s%s: %s\n", m, row.label, row.status))
			continue
		}
		sb.WriteString(fmt.Sprintf("%s%s: %s\n", m, row.label, row.options[row.value]))
	}
	sb.WriteString("\n(↑/↓ select, ←/→ change, Enter apply/action, Esc cancel)")
	return sb.String()
}
