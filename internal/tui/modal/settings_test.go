// internal/tui/modal/settings_test.go
package modal

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSettings_CyclesSelectedRow(t *testing.T) {
	s := NewSettings(Settings{Effort: "low"}).(*SettingsModal)
	s.Update(tea.KeyMsg{Type: tea.KeyRight})
	s.Update(tea.KeyMsg{Type: tea.KeyRight})
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	res := cmd().(ResultMsg).Payload.(Settings)
	if res.Effort != "high" {
		t.Fatalf("effort = %q", res.Effort)
	}
}

func TestSettings_CanChangeIconAndActivityModes(t *testing.T) {
	s := NewSettings(Settings{Effort: "medium", IconMode: "unicode", ActivityMode: "arcane"}).(*SettingsModal)
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s.Update(tea.KeyMsg{Type: tea.KeyRight})
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s.Update(tea.KeyMsg{Type: tea.KeyRight})
	s.Update(tea.KeyMsg{Type: tea.KeyRight})
	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	res := cmd().(ResultMsg).Payload.(Settings)
	if res.IconMode != "ascii" {
		t.Fatalf("icon mode = %q", res.IconMode)
	}
	if res.ActivityMode != "simple" {
		t.Fatalf("activity mode = %q", res.ActivityMode)
	}
}

func TestSettings_ViewShowsNewRows(t *testing.T) {
	s := NewSettings(Settings{Effort: "medium", IconMode: "nerd", ActivityMode: "arcane"}).(*SettingsModal)
	out := s.View(80, 24)
	for _, want := range []string{"Grimoire of Settings", "✧ Mind", "thinking effort", "medium", "✧ Interface", "icon mode", "nerd", "activity indicator", "arcane"} {
		if !strings.Contains(out, want) {
			t.Fatalf("settings view missing %q:\n%s", want, out)
		}
	}
}
