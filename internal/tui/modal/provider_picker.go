package modal

import tea "github.com/charmbracelet/bubbletea"

type ProviderChoice struct {
	ID        string
	ProfileID string
	Label     string
	Value     string
}

type ProviderPicker struct {
	items []ProviderChoice
	sel   int
}

func NewProviderPicker(items []string, current string) Modal {
	choices := make([]ProviderChoice, 0, len(items))
	for _, it := range items {
		choices = append(choices, ProviderChoice{ID: it, Label: it})
	}
	return NewProviderProfilePicker(choices, current, "")
}

func NewProviderProfilePicker(items []ProviderChoice, currentProvider, currentProfile string) Modal {
	sel := 0
	for i, it := range items {
		if currentProfile != "" && it.ProfileID == currentProfile {
			sel = i
			break
		}
		if currentProfile == "" && it.ID == currentProvider && it.ProfileID == "" {
			sel = i
		}
	}
	return &ProviderPicker{items: items, sel: sel}
}

func (m *ProviderPicker) Init() tea.Cmd { return nil }

func (m *ProviderPicker) Update(msg tea.Msg) (Modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyUp:
			if m.sel > 0 {
				m.sel--
			}
		case tea.KeyDown:
			if m.sel < len(m.items)-1 {
				m.sel++
			}
		case tea.KeyEnter:
			return m, Result(m.items[m.sel])
		case tea.KeyEsc:
			return m, Cancel()
		}
	}
	return m, nil
}

func (m *ProviderPicker) View(width, height int) string {
	rows := make([]choiceRow, len(m.items))
	for i, it := range m.items {
		label := it.Label
		if label == "" {
			label = it.ID
		}
		rows[i] = choiceRow{Label: label, Value: it.Value}
	}
	return renderChoiceModal(width, height, "✦ Provider Selection ✦", "Source", "↑/↓ choose provider · Enter bind · Esc dismiss", rows, m.sel)
}
