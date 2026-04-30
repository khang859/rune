// internal/tui/modal/model_picker.go
package modal

import tea "github.com/charmbracelet/bubbletea"

const ModelPickerCustom = "custom…"

type ModelPicker struct {
	items []string
	sel   int
}

func NewModelPicker(items []string, current string) Modal {
	sel := 0
	for i, it := range items {
		if it == current {
			sel = i
			break
		}
	}
	return &ModelPicker{items: items, sel: sel}
}

func NewOllamaModelPicker(items []string, current string) Modal {
	seen := map[string]bool{}
	var out []string
	for _, it := range items {
		if it != "" && !seen[it] {
			seen[it] = true
			out = append(out, it)
		}
	}
	if current != "" && !seen[current] {
		out = append([]string{current}, out...)
		seen[current] = true
	}
	out = append(out, ModelPickerCustom)
	return NewModelPicker(out, current)
}

func (m *ModelPicker) Init() tea.Cmd { return nil }

func (m *ModelPicker) Update(msg tea.Msg) (Modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyUp:
			m.Up()
		case tea.KeyDown:
			m.Down()
		case tea.KeyEnter:
			return m, Result(m.items[m.sel])
		case tea.KeyEsc:
			return m, Cancel()
		}
	}
	return m, nil
}

func (m *ModelPicker) Up() {
	if m.sel > 0 {
		m.sel--
	}
}

func (m *ModelPicker) Down() {
	if m.sel < len(m.items)-1 {
		m.sel++
	}
}

func (m *ModelPicker) View(width, height int) string {
	rows := make([]choiceRow, len(m.items))
	for i, it := range m.items {
		rows[i] = choiceRow{Label: it}
	}
	return renderChoiceModal(width, height, "✦ Model Selection ✦", "Mind", "↑/↓ choose rune · Enter bind · Esc dismiss", rows, m.sel)
}
