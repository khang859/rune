package config

import "testing"

func TestDefaultSettingsIncludesProvider(t *testing.T) {
	s := DefaultSettings()
	if s.Provider != "" {
		t.Fatalf("provider = %q, want no active provider", s.Provider)
	}
	if s.CodexModel != "gpt-5.5" {
		t.Fatalf("codex model = %q", s.CodexModel)
	}
	if s.GroqModel != "llama-3.3-70b-versatile" {
		t.Fatalf("groq model = %q", s.GroqModel)
	}
	if s.OllamaModel != "llama3.2" {
		t.Fatalf("ollama model = %q", s.OllamaModel)
	}
	if s.RunpodModel != "openai/gpt-oss-120b" {
		t.Fatalf("runpod model = %q", s.RunpodModel)
	}
	if s.OllamaEndpoint == "" {
		t.Fatal("ollama endpoint should be set")
	}
}

func TestDefaultSettingsIncludesAutoCompact(t *testing.T) {
	s := DefaultSettings()
	if !s.AutoCompact.EnabledValue() {
		t.Fatal("auto compact should be enabled by default")
	}
	if s.AutoCompact.ThresholdPct != 80 {
		t.Fatalf("auto compact threshold = %d, want 80", s.AutoCompact.ThresholdPct)
	}
}

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

func TestNormalizeSettingsFillsProviderDefaults(t *testing.T) {
	s := NormalizeSettings(Settings{})
	if s.Provider != "" || s.CodexModel == "" || s.GroqModel == "" || s.OllamaModel == "" || s.RunpodModel == "" || s.OllamaEndpoint == "" {
		t.Fatalf("settings = %+v", s)
	}
}

func TestNormalizeSettingsPreservesProfiles(t *testing.T) {
	s := NormalizeSettings(Settings{Provider: "ollama", ActiveProfile: "gpu", Profiles: []ProviderProfile{{ID: "gpu", Name: "GPU", Provider: "ollama", Endpoint: "http://gpu:11434/v1/chat/completions", Model: "qwen3:4b"}}})
	if s.ActiveProfile != "gpu" || s.Provider != "ollama" || len(s.Profiles) != 1 {
		t.Fatalf("settings = %+v", s)
	}
}

func TestNormalizeSettingsDropsInvalidActiveProfile(t *testing.T) {
	s := NormalizeSettings(Settings{Provider: "ollama", ActiveProfile: "missing", Profiles: []ProviderProfile{{ID: "gpu", Provider: "ollama"}}})
	if s.ActiveProfile != "" {
		t.Fatalf("active profile = %q, want empty", s.ActiveProfile)
	}
}

func TestNormalizeSettingsPreservesOllama(t *testing.T) {
	s := NormalizeSettings(Settings{Provider: "ollama", OllamaModel: "custom:latest", OllamaEndpoint: "http://127.0.0.1:11434/v1/chat/completions"})
	if s.Provider != "ollama" || s.OllamaModel != "custom:latest" || s.OllamaEndpoint != "http://127.0.0.1:11434/v1/chat/completions" {
		t.Fatalf("settings = %+v", s)
	}
}

func TestNormalizeSettingsPreservesRunpod(t *testing.T) {
	s := NormalizeSettings(Settings{Provider: "runpod", RunpodModel: "custom/model", RunpodEndpoint: "private-endpoint"})
	if s.Provider != "runpod" || s.RunpodModel != "custom/model" || s.RunpodEndpoint != "private-endpoint" {
		t.Fatalf("settings = %+v", s)
	}
}

func TestNormalizeSettingsFillsAutoCompactDefaults(t *testing.T) {
	s := NormalizeSettings(Settings{})
	if !s.AutoCompact.EnabledValue() {
		t.Fatal("auto compact should default to enabled")
	}
	if s.AutoCompact.ThresholdPct != 80 {
		t.Fatalf("auto compact threshold = %d, want 80", s.AutoCompact.ThresholdPct)
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
