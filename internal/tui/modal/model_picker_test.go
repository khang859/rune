// internal/tui/modal/model_picker_test.go
package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestModelPicker_PicksHighlighted(t *testing.T) {
	p := NewModelPicker([]string{"gpt-5", "gpt-5-codex"}, "gpt-5")
	p.(*ModelPicker).Down() // highlight gpt-5-codex
	next, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = next
	msg := cmd().(ResultMsg)
	if got := msg.Payload.(ModelChoice).Model; got != "gpt-5-codex" {
		t.Fatalf("payload = %q", got)
	}
}

func TestModelPicker_EscCancels(t *testing.T) {
	p := NewModelPicker([]string{"gpt-5"}, "gpt-5")
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEsc})
	msg := cmd().(ResultMsg)
	if !msg.Cancel {
		t.Fatal("expected cancel")
	}
}

func TestCustomModelPickerAppendsCustomAndPrependsCurrent(t *testing.T) {
	p := NewCustomModelPicker([]string{"a", "b"}, "custom/model").(*ModelPicker)
	if len(p.items) != 4 || p.items[0] != "custom/model" || p.items[3] != ModelPickerCustom {
		t.Fatalf("items = %#v", p.items)
	}
}

func TestModelPicker_CyclesToolOverride(t *testing.T) {
	p := NewModelPicker([]string{"qwen3"}, "qwen3")
	_, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'t'}})
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	choice := cmd().(ResultMsg).Payload.(ModelChoice)
	if choice.Tools != "off" {
		t.Fatalf("tools = %q, want off", choice.Tools)
	}
}
