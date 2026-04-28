// internal/tui/modal/model_picker.go
package modal

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

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
	var sb strings.Builder
	sb.WriteString("Select model (↑/↓, Enter, Esc):\n")
	for i, it := range m.items {
		if i == m.sel {
			sb.WriteString("  > " + it + "\n")
		} else {
			sb.WriteString("    " + it + "\n")
		}
	}
	return sb.String()
}
