// internal/tui/modal/settings.go
package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type Settings struct {
	Effort                string
	IconMode              string
	ActivityMode          string
	WebFetch              string
	FetchPrivateURLs      string
	WebSearch             string
	SearchProvider        string
	Subagents             string
	SubagentMaxConcurrent string
	SubagentTimeout       string
	SubagentRetain        string
	BraveAPIKeyStatus     string
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
	section string
	label   string
	options []string
	value   int
	action  string
	status  string
}

const (
	settingsRowEffort = iota
	settingsRowIconMode
	settingsRowActivityMode
	settingsRowWebFetch
	settingsRowFetchPrivateURLs
	settingsRowWebSearch
	settingsRowSearchProvider
	settingsRowBraveAPIKey
	settingsRowSubagents
	settingsRowSubagentMaxConcurrent
	settingsRowSubagentTimeout
	settingsRowSubagentRetain
)

func NewSettings(cur Settings) Modal {
	cur = normalizeSettings(cur)
	return &SettingsModal{cur: cur, rows: []settingsRow{
		newSettingsRow("Mind", "thinking effort", []string{"none", "low", "medium", "high", "xhigh"}, cur.Effort),
		newSettingsRow("Interface", "icon mode", []string{"auto", "nerd", "unicode", "ascii"}, cur.IconMode),
		newSettingsRow("Interface", "activity indicator", []string{"off", "simple", "arcane"}, cur.ActivityMode),
		newSettingsRow("Web Scrying", "web fetch", []string{"off", "on"}, cur.WebFetch),
		newSettingsRow("Web Scrying", "fetch private urls", []string{"off", "on"}, cur.FetchPrivateURLs),
		newSettingsRow("Web Scrying", "web search", []string{"auto", "off", "on"}, cur.WebSearch),
		newSettingsRow("Web Scrying", "search provider", []string{"auto", "brave", "searxng"}, cur.SearchProvider),
		{kind: settingsRowAction, section: "Web Scrying", label: "brave api key", action: "brave_api_key", status: cur.BraveAPIKeyStatus},
		newSettingsRow("Subagents", "subagents", []string{"off", "on"}, cur.Subagents),
		newSettingsRow("Subagents", "max concurrent", []string{"1", "2", "4", "8"}, cur.SubagentMaxConcurrent),
		newSettingsRow("Subagents", "default timeout", []string{"30s", "60s", "120s", "300s", "600s"}, cur.SubagentTimeout),
		newSettingsRow("Subagents", "keep recent", []string{"25", "50", "100", "250"}, cur.SubagentRetain),
	}}
}

func normalizeSettings(s Settings) Settings {
	if s.Effort == "" {
		s.Effort = "medium"
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
	if s.Subagents == "" {
		s.Subagents = "on"
	}
	if s.SubagentMaxConcurrent == "" {
		s.SubagentMaxConcurrent = "4"
	}
	if s.SubagentTimeout == "" {
		s.SubagentTimeout = "600s"
	}
	if s.SubagentRetain == "" {
		s.SubagentRetain = "100"
	}
	if s.BraveAPIKeyStatus == "" {
		s.BraveAPIKeyStatus = "missing — Enter to set"
	}
	return s
}

func newSettingsRow(section, label string, options []string, current string) settingsRow {
	row := settingsRow{kind: settingsRowEnum, section: section, label: label, options: options}
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
		Effort:                s.rows[settingsRowEffort].options[s.rows[settingsRowEffort].value],
		IconMode:              s.rows[settingsRowIconMode].options[s.rows[settingsRowIconMode].value],
		ActivityMode:          s.rows[settingsRowActivityMode].options[s.rows[settingsRowActivityMode].value],
		WebFetch:              s.rows[settingsRowWebFetch].options[s.rows[settingsRowWebFetch].value],
		FetchPrivateURLs:      s.rows[settingsRowFetchPrivateURLs].options[s.rows[settingsRowFetchPrivateURLs].value],
		WebSearch:             s.rows[settingsRowWebSearch].options[s.rows[settingsRowWebSearch].value],
		SearchProvider:        s.rows[settingsRowSearchProvider].options[s.rows[settingsRowSearchProvider].value],
		Subagents:             s.rows[settingsRowSubagents].options[s.rows[settingsRowSubagents].value],
		SubagentMaxConcurrent: s.rows[settingsRowSubagentMaxConcurrent].options[s.rows[settingsRowSubagentMaxConcurrent].value],
		SubagentTimeout:       s.rows[settingsRowSubagentTimeout].options[s.rows[settingsRowSubagentTimeout].value],
		SubagentRetain:        s.rows[settingsRowSubagentRetain].options[s.rows[settingsRowSubagentRetain].value],
		BraveAPIKeyStatus:     s.cur.BraveAPIKeyStatus,
	}
}

func (s *SettingsModal) View(width, height int) string {
	var sb strings.Builder
	styles := settingsStyles()

	contentWidth := modalContentWidth(width)

	sb.WriteString(styles.Title.Render("✦ Grimoire of Settings ✦"))
	sb.WriteByte('\n')
	sb.WriteString(styles.Divider.Width(contentWidth).Render(""))
	sb.WriteString("\n\n")

	lastSection := ""
	for i, row := range s.rows {
		if row.section != lastSection {
			if lastSection != "" {
				sb.WriteByte('\n')
			}
			sb.WriteString(styles.Section.Render("✧ " + row.section))
			sb.WriteByte('\n')
			lastSection = row.section
		}

		selected := i == s.sel
		selector := styles.Gutter.Render("  ")
		label := styles.Label.Render(row.label)
		value := ""
		if row.kind == settingsRowAction {
			value = row.status
		} else {
			value = row.options[row.value]
		}

		valueStyle := styles.Value
		if selected {
			selector = styles.Selector.Render("➤ ")
			label = styles.SelectedLabel.Render(row.label)
			valueStyle = styles.SelectedValue
		}

		fmt.Fprintf(&sb, "%s%s %s\n", selector, label, valueStyle.Render(value))
	}

	sb.WriteByte('\n')
	sb.WriteString(styles.Help.Render("↑/↓ choose rune · ←/→ alter enchantment · Enter bind · Esc dismiss"))

	return centeredModal(width, height, contentWidth, sb.String())
}
