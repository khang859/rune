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

func TestConfigFromModalSettingsPreservesProviderFields(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	if err := config.SaveSettings(config.SettingsPath(), config.Settings{
		OllamaModel:    "custom:latest",
		OllamaEndpoint: "http://127.0.0.1:11434/v1/chat/completions",
		RunpodModel:    "runpod/model",
		RunpodEndpoint: "private-endpoint",
	}); err != nil {
		t.Fatal(err)
	}
	s := configFromModalSettings(modal.Settings{Provider: "ollama"})
	if s.Provider != "ollama" || s.OllamaModel != "custom:latest" || s.OllamaEndpoint != "http://127.0.0.1:11434/v1/chat/completions" || s.RunpodModel != "runpod/model" || s.RunpodEndpoint != "private-endpoint" {
		t.Fatalf("settings = %+v", s)
	}
}

func TestModalSettingsFromConfigIncludesOllamaAPIKeyStatus(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	s := modalSettingsFromConfig(config.Settings{}, false, false)
	if s.OllamaAPIKeyStatus != "optional — Enter to set" {
		t.Fatalf("OllamaAPIKeyStatus = %q", s.OllamaAPIKeyStatus)
	}
	if err := config.NewSecretStore(config.SecretsPath()).SetOllamaAPIKey("ollama-token"); err != nil {
		t.Fatal(err)
	}
	s = modalSettingsFromConfig(config.Settings{}, false, false)
	if s.OllamaAPIKeyStatus != "configured — Enter to replace" {
		t.Fatalf("OllamaAPIKeyStatus = %q", s.OllamaAPIKeyStatus)
	}
	t.Setenv("RUNE_OLLAMA_API_KEY", "env-token")
	s = modalSettingsFromConfig(config.Settings{}, false, false)
	if s.OllamaAPIKeyStatus != "env override active" {
		t.Fatalf("OllamaAPIKeyStatus = %q", s.OllamaAPIKeyStatus)
	}
}

func TestModalSettingsFromConfigIncludesEndpointStatuses(t *testing.T) {
	s := modalSettingsFromConfig(config.Settings{
		OllamaEndpoint: "http://remote:11434/v1/chat/completions",
		RunpodEndpoint: "private-endpoint",
	}, false, false)
	if s.OllamaEndpointStatus != "custom — Enter to edit" {
		t.Fatalf("OllamaEndpointStatus = %q", s.OllamaEndpointStatus)
	}
	if s.RunpodEndpointStatus != "custom — Enter to edit" {
		t.Fatalf("RunpodEndpointStatus = %q", s.RunpodEndpointStatus)
	}

	t.Setenv("RUNE_OLLAMA_ENDPOINT", "http://env:11434/v1/chat/completions")
	t.Setenv("RUNE_RUNPOD_ENDPOINT", "env-endpoint")
	s = modalSettingsFromConfig(config.Settings{}, false, false)
	if s.OllamaEndpointStatus != "env override active" {
		t.Fatalf("OllamaEndpointStatus = %q", s.OllamaEndpointStatus)
	}
	if s.RunpodEndpointStatus != "env override active" {
		t.Fatalf("RunpodEndpointStatus = %q", s.RunpodEndpointStatus)
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
