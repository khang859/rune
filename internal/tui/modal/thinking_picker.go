package modal

import tea "github.com/charmbracelet/bubbletea"

type ThinkingPicker struct {
	items []string
	sel   int
}

func NewThinkingPicker(items []string, current string) Modal {
	sel := 0
	for i, it := range items {
		if it == current {
			sel = i
			break
		}
	}
	return &ThinkingPicker{items: append([]string(nil), items...), sel: sel}
}

func (m *ThinkingPicker) Init() tea.Cmd { return nil }

func (m *ThinkingPicker) Update(msg tea.Msg) (Modal, tea.Cmd) {
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
			if len(m.items) == 0 {
				return m, Cancel()
			}
			return m, Result(m.items[m.sel])
		case tea.KeyEsc:
			return m, Cancel()
		}
	}
	return m, nil
}

func (m *ThinkingPicker) View(width, height int) string {
	rows := make([]choiceRow, len(m.items))
	for i, it := range m.items {
		rows[i] = choiceRow{Label: it}
	}
	return renderChoiceModal(width, height, "✦ Thinking Effort ✦", "Mind", "↑/↓ choose rune · Enter bind · Esc dismiss", rows, m.sel)
}
