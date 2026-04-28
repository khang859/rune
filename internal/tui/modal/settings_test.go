// internal/tui/modal/settings_test.go
package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSettings_CyclesEffort(t *testing.T) {
	s := NewSettings(Settings{Effort: "low"}).(*SettingsModal)
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	res := cmd().(ResultMsg).Payload.(Settings)
	if res.Effort != "high" {
		t.Fatalf("effort = %q", res.Effort)
	}
}
