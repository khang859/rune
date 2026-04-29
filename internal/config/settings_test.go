package config

import "testing"

func TestDefaultSettingsIncludesSubagents(t *testing.T) {
	s := DefaultSettings()
	if !s.Subagents.EnabledValue() {
		t.Fatal("subagents should be enabled by default")
	}
	if s.Subagents.MaxConcurrent != 4 {
		t.Fatalf("MaxConcurrent = %d, want 4", s.Subagents.MaxConcurrent)
	}
	if s.Subagents.DefaultTimeoutSecs != 600 {
		t.Fatalf("DefaultTimeoutSecs = %d, want 600", s.Subagents.DefaultTimeoutSecs)
	}
	if s.Subagents.MaxCompletedRetain != 100 {
		t.Fatalf("MaxCompletedRetain = %d, want 100", s.Subagents.MaxCompletedRetain)
	}
}

func TestNormalizeSettingsFillsSubagentDefaults(t *testing.T) {
	s := NormalizeSettings(Settings{})
	if !s.Subagents.EnabledValue() {
		t.Fatal("subagents should default to enabled")
	}
	if s.Subagents.MaxConcurrent != 4 {
		t.Fatalf("MaxConcurrent = %d, want 4", s.Subagents.MaxConcurrent)
	}
	if s.Subagents.DefaultTimeoutSecs != 600 {
		t.Fatalf("DefaultTimeoutSecs = %d, want 600", s.Subagents.DefaultTimeoutSecs)
	}
	if s.Subagents.MaxCompletedRetain != 100 {
		t.Fatalf("MaxCompletedRetain = %d, want 100", s.Subagents.MaxCompletedRetain)
	}
}

func TestNormalizeSettingsPreservesSubagentOverrides(t *testing.T) {
	s := NormalizeSettings(Settings{Subagents: SubagentSettings{Enabled: boolPtr(false), MaxConcurrent: 2, DefaultTimeoutSecs: 30, MaxCompletedRetain: 7}})
	if s.Subagents.EnabledValue() {
		t.Fatal("subagents enabled=false override was not preserved")
	}
	if s.Subagents.MaxConcurrent != 2 {
		t.Fatalf("MaxConcurrent = %d, want 2", s.Subagents.MaxConcurrent)
	}
	if s.Subagents.DefaultTimeoutSecs != 30 {
		t.Fatalf("DefaultTimeoutSecs = %d, want 30", s.Subagents.DefaultTimeoutSecs)
	}
	if s.Subagents.MaxCompletedRetain != 7 {
		t.Fatalf("MaxCompletedRetain = %d, want 7", s.Subagents.MaxCompletedRetain)
	}
}
