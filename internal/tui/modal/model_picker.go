// internal/tui/modal/model_picker.go
package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

const ModelPickerCustom = "custom…"

type ModelChoice struct {
	Model string
	Tools string
}

type ModelPicker struct {
	items []string
	sel   int
	tools map[string]string
}

func NewModelPicker(items []string, current string) Modal {
	return NewModelPickerWithCapabilities(items, current, nil)
}

func NewModelPickerWithCapabilities(items []string, current string, tools map[string]string) Modal {
	sel := 0
	for i, it := range items {
		if it == current {
			sel = i
			break
		}
	}
	return &ModelPicker{items: items, sel: sel, tools: normalizeToolOverrides(tools)}
}

func NewCustomModelPicker(items []string, current string) Modal {
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

func NewCustomModelPickerWithCapabilities(items []string, current string, tools map[string]string) Modal {
	p := NewCustomModelPicker(items, current).(*ModelPicker)
	p.tools = normalizeToolOverrides(tools)
	return p
}

func NewOllamaModelPicker(items []string, current string) Modal {
	return NewCustomModelPicker(items, current)
}

func NewOllamaModelPickerWithCapabilities(items []string, current string, tools map[string]string) Modal {
	return NewCustomModelPickerWithCapabilities(items, current, tools)
}

func normalizeToolOverrides(in map[string]string) map[string]string {
	out := map[string]string{}
	for model, value := range in {
		model = strings.TrimSpace(model)
		value = strings.ToLower(strings.TrimSpace(value))
		if model == "" || (value != "auto" && value != "on" && value != "off") {
			continue
		}
		out[model] = value
	}
	return out
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
			model := m.items[m.sel]
			return m, Result(ModelChoice{Model: model, Tools: m.toolOverride(model)})
		case tea.KeyRunes:
			if strings.EqualFold(k.String(), "t") {
				m.cycleTools()
			}
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

func (m *ModelPicker) toolOverride(model string) string {
	if m.tools == nil || model == ModelPickerCustom {
		return "auto"
	}
	if v := m.tools[model]; v != "" {
		return v
	}
	return "auto"
}

func (m *ModelPicker) cycleTools() {
	model := m.items[m.sel]
	if model == ModelPickerCustom {
		return
	}
	if m.tools == nil {
		m.tools = map[string]string{}
	}
	switch m.toolOverride(model) {
	case "auto":
		m.tools[model] = "off"
	case "off":
		m.tools[model] = "on"
	default:
		delete(m.tools, model)
	}
}

func (m *ModelPicker) View(width, height int) string {
	rows := make([]choiceRow, len(m.items))
	for i, it := range m.items {
		rows[i] = choiceRow{Label: it}
		if it != ModelPickerCustom {
			rows[i].Value = fmt.Sprintf("tools: %s", m.toolOverride(it))
		}
	}
	return renderChoiceModal(width, height, "✦ Model Selection ✦", "Mind", "↑/↓ choose rune · t tools auto/off/on · Enter bind · Esc dismiss", rows, m.sel)
}
