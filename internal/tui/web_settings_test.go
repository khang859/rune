package tui

import (
	"testing"

	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/tui/modal"
)

func TestModalSettingsFromConfigIncludesSubagents(t *testing.T) {
	s := modalSettingsFromConfig(config.Settings{Subagents: config.SubagentSettings{Enabled: boolPtr(false), MaxConcurrent: 2, DefaultTimeoutSecs: 120, MaxCompletedRetain: 50}}, false, false)
	if s.Subagents != "off" {
		t.Fatalf("Subagents = %q, want off", s.Subagents)
	}
	if s.SubagentMaxConcurrent != "2" {
		t.Fatalf("SubagentMaxConcurrent = %q, want 2", s.SubagentMaxConcurrent)
	}
	if s.SubagentTimeout != "120s" {
		t.Fatalf("SubagentTimeout = %q, want 120s", s.SubagentTimeout)
	}
	if s.SubagentRetain != "50" {
		t.Fatalf("SubagentRetain = %q, want 50", s.SubagentRetain)
	}
}

func TestConfigFromModalSettingsPreservesOllamaFields(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	if err := config.SaveSettings(config.SettingsPath(), config.Settings{OllamaModel: "custom:latest", OllamaEndpoint: "http://127.0.0.1:11434/v1/chat/completions"}); err != nil {
		t.Fatal(err)
	}
	s := configFromModalSettings(modal.Settings{Provider: "ollama"})
	if s.Provider != "ollama" || s.OllamaModel != "custom:latest" || s.OllamaEndpoint != "http://127.0.0.1:11434/v1/chat/completions" {
		t.Fatalf("settings = %+v", s)
	}
}

func TestConfigFromModalSettingsIncludesSubagents(t *testing.T) {
	s := configFromModalSettings(modal.Settings{Subagents: "off", SubagentMaxConcurrent: "8", SubagentTimeout: "300s", SubagentRetain: "250"})
	if s.Subagents.EnabledValue() {
		t.Fatal("subagents should be disabled")
	}
	if s.Subagents.MaxConcurrent != 8 {
		t.Fatalf("MaxConcurrent = %d, want 8", s.Subagents.MaxConcurrent)
	}
	if s.Subagents.DefaultTimeoutSecs != 300 {
		t.Fatalf("DefaultTimeoutSecs = %d, want 300", s.Subagents.DefaultTimeoutSecs)
	}
	if s.Subagents.MaxCompletedRetain != 250 {
		t.Fatalf("MaxCompletedRetain = %d, want 250", s.Subagents.MaxCompletedRetain)
	}
}
