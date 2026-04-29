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

func TestSettings_CanChangeSubagentSettings(t *testing.T) {
	s := NewSettings(Settings{Subagents: "on", SubagentMaxConcurrent: "4", SubagentTimeout: "600s", SubagentRetain: "100"}).(*SettingsModal)
	for range 10 {
		s.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	s.Update(tea.KeyMsg{Type: tea.KeyLeft}) // subagents: on -> off
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s.Update(tea.KeyMsg{Type: tea.KeyRight}) // max concurrent: 4 -> 8
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s.Update(tea.KeyMsg{Type: tea.KeyRight}) // timeout: 600s -> 30s
	s.Update(tea.KeyMsg{Type: tea.KeyDown})
	s.Update(tea.KeyMsg{Type: tea.KeyRight}) // retain: 100 -> 250

	_, cmd := s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	res := cmd().(ResultMsg).Payload.(Settings)
	if res.Subagents != "off" {
		t.Fatalf("subagents = %q, want off", res.Subagents)
	}
	if res.SubagentMaxConcurrent != "8" {
		t.Fatalf("max concurrent = %q, want 8", res.SubagentMaxConcurrent)
	}
	if res.SubagentTimeout != "30s" {
		t.Fatalf("timeout = %q, want 30s", res.SubagentTimeout)
	}
	if res.SubagentRetain != "250" {
		t.Fatalf("retain = %q, want 250", res.SubagentRetain)
	}
}

func TestSettings_ViewShowsNewRows(t *testing.T) {
	s := NewSettings(Settings{Effort: "medium", IconMode: "nerd", ActivityMode: "arcane"}).(*SettingsModal)
	out := s.View(80, 24)
	for _, want := range []string{
		"Grimoire of Settings",
		"✧ Mind",
		"thinking effort",
		"medium",
		"✧ Interface",
		"icon mode",
		"nerd",
		"activity indicator",
		"arcane",
		"✧ Memory",
		"auto compact",
		"compact threshold",
		"✧ Subagents",
		"subagents",
		"max concurrent",
		"default timeout",
		"keep recent",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("settings view missing %q:\n%s", want, out)
		}
	}
}
