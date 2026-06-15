package providers

import (
	"path/filepath"
	"testing"

	"github.com/khang859/rune/internal/config"
)

func TestResolveUsesActiveProfile(t *testing.T) {
	s := config.NormalizeSettings(config.Settings{
		Provider:      Ollama,
		ActiveProfile: "gpu",
		Profiles: []config.ProviderProfile{{
			ID:       "gpu",
			Name:     "GPU",
			Provider: Ollama,
			Endpoint: "http://gpu:11434/v1/chat/completions",
			Model:    "qwen3:4b",
		}},
	})
	got := Resolve(s, ResolveOptions{})
	if got.Provider != Ollama || got.ProfileID != "gpu" || got.Model != "qwen3:4b" || got.Endpoint != "http://gpu:11434/v1/chat/completions" {
		t.Fatalf("resolved = %+v", got)
	}
}

func TestResolveExplicitNoProfileBypassesActiveProfile(t *testing.T) {
	s := config.NormalizeSettings(config.Settings{
		Provider:      Ollama,
		ActiveProfile: "gpu",
		OllamaModel:   "base-model",
		Profiles:      []config.ProviderProfile{{ID: "gpu", Provider: Ollama, Model: "profile-model", Endpoint: "http://gpu"}},
	})
	got := Resolve(s, ResolveOptions{ProviderOverride: Ollama, ProfileOverride: NoProfile()})
	if got.ProfileID != "" || got.Model != "base-model" {
		t.Fatalf("resolved = %+v", got)
	}
}

func TestSaveResolvedSelectionClearsActiveProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := config.NormalizeSettings(config.Settings{Provider: Ollama, ActiveProfile: "gpu", Profiles: []config.ProviderProfile{{ID: "gpu", Provider: Ollama, Model: "profile-model"}}})
	if err := SaveResolvedSelection(path, s, ResolvedProvider{Provider: Groq, Model: "groq-model"}); err != nil {
		t.Fatal(err)
	}
	saved, err := config.LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Provider != Groq || saved.ActiveProfile != "" {
		t.Fatalf("saved = %+v", saved)
	}
}

func TestSaveResolvedSelectionDoesNotPersistResolvedEndpoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := config.NormalizeSettings(config.Settings{Provider: Runpod, RunpodModel: DefaultRunpodModel})
	if err := SaveResolvedSelection(path, s, Resolve(s, ResolveOptions{ProviderOverride: Runpod})); err != nil {
		t.Fatal(err)
	}
	saved, err := config.LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.RunpodEndpoint != "" {
		t.Fatalf("runpod endpoint persisted = %q", saved.RunpodEndpoint)
	}
}

func TestResolveOpenRouterDefault(t *testing.T) {
	s := config.NormalizeSettings(config.Settings{Provider: OpenRouter})
	got := Resolve(s, ResolveOptions{})
	if got.Provider != OpenRouter || got.Model != DefaultOpenRouterModel || got.Endpoint != DefaultOpenRouterEndpoint {
		t.Fatalf("resolved = %+v", got)
	}
}

func TestResolveOpenRouterProviderFromProfile(t *testing.T) {
	s := config.NormalizeSettings(config.Settings{
		Provider:           OpenRouter,
		OpenRouterProvider: "settings-provider",
		ActiveProfile:      "or",
		Profiles: []config.ProviderProfile{{
			ID:                 "or",
			Provider:           OpenRouter,
			Model:              "profile/model",
			OpenRouterProvider: "profile-provider",
		}},
	})
	got := Resolve(s, ResolveOptions{})
	if got.OpenRouterProvider != "profile-provider" {
		t.Fatalf("openrouter provider = %q, want profile-provider", got.OpenRouterProvider)
	}
}

func TestResolveOpenRouterEnvOverrides(t *testing.T) {
	t.Setenv("RUNE_OPENROUTER_MODEL", "env/model")
	t.Setenv("RUNE_OPENROUTER_ENDPOINT", "https://env.test/v1/chat/completions")
	t.Setenv("RUNE_OPENROUTER_PROVIDER", "env-provider")
	s := config.NormalizeSettings(config.Settings{Provider: OpenRouter, OpenRouterModel: "settings/model", OpenRouterProvider: "settings-provider", OpenRouterEndpoint: "https://settings.test/v1", ActiveProfile: "or", Profiles: []config.ProviderProfile{{ID: "or", Provider: OpenRouter, Model: "profile/model", OpenRouterProvider: "profile-provider", Endpoint: "https://profile.test/v1"}}})
	got := Resolve(s, ResolveOptions{})
	if got.Model != "env/model" || got.Endpoint != "https://env.test/v1/chat/completions" || got.OpenRouterProvider != "env-provider" {
		t.Fatalf("resolved = %+v", got)
	}
}

func TestSaveResolvedSelectionPersistsOpenRouterModelOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := config.NormalizeSettings(config.Settings{Provider: OpenRouter})
	if err := SaveResolvedSelection(path, s, ResolvedProvider{Provider: OpenRouter, Model: "anthropic/claude-sonnet-4.5", Endpoint: DefaultOpenRouterEndpoint}); err != nil {
		t.Fatal(err)
	}
	saved, err := config.LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.OpenRouterModel != "anthropic/claude-sonnet-4.5" {
		t.Fatalf("openrouter model = %q", saved.OpenRouterModel)
	}
	if saved.OpenRouterEndpoint != "" {
		t.Fatalf("openrouter endpoint persisted = %q", saved.OpenRouterEndpoint)
	}
}

func TestSaveResolvedSelectionPersistsOpenRouterProvider(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := config.NormalizeSettings(config.Settings{Provider: OpenRouter})
	if err := SaveResolvedSelection(path, s, ResolvedProvider{Provider: OpenRouter, Model: "anthropic/claude-sonnet-4.5", OpenRouterProvider: "anthropic"}); err != nil {
		t.Fatal(err)
	}
	saved, err := config.LoadSettings(path)
	if err != nil {
		t.Fatal(err)
	}
	if saved.OpenRouterProvider != "anthropic" {
		t.Fatalf("openrouter provider = %q", saved.OpenRouterProvider)
	}
}

func TestResolveEnvModelOverridesProfile(t *testing.T) {
	t.Setenv("RUNE_OLLAMA_MODEL", "env-model")
	s := config.NormalizeSettings(config.Settings{
		Provider:      Ollama,
		ActiveProfile: "gpu",
		Profiles:      []config.ProviderProfile{{ID: "gpu", Provider: Ollama, Model: "profile-model"}},
	})
	got := Resolve(s, ResolveOptions{})
	if got.Model != "env-model" {
		t.Fatalf("model = %q, want env-model", got.Model)
	}
}
