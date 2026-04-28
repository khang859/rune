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
	if got := msg.Payload.(string); got != "gpt-5-codex" {
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
