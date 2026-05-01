package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/codex"
	"github.com/khang859/rune/internal/ai/groq"
	"github.com/khang859/rune/internal/ai/oauth"
	"github.com/khang859/rune/internal/ai/ollama"
	"github.com/khang859/rune/internal/ai/runpod"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/providers"
)

type providerSelection struct {
	Provider  string
	ProfileID string
	Model     string
	AI        ai.Provider
}

func buildProvider(ctx context.Context, providerOverride, modelOverride string) (providerSelection, error) {
	settings, _ := config.LoadSettings(config.SettingsPath())
	resolved := providers.Resolve(settings, providers.ResolveOptions{ProviderOverride: providerOverride, ModelOverride: modelOverride})
	provider := strings.TrimSpace(resolved.Provider)
	if provider == "" {
		return providerSelection{}, fmt.Errorf("no active provider configured (use `rune --provider <id>` or choose one in /providers)")
	}
	model := resolved.Model

	switch provider {
	case providers.Groq:
		endpoint := resolved.Endpoint
		key, err := config.NewSecretStore(config.SecretsPath()).GroqAPIKey()
		if err != nil {
			return providerSelection{}, err
		}
		return providerSelection{Provider: provider, ProfileID: resolved.ProfileID, Model: model, AI: groq.New(endpoint, key)}, nil
	case providers.Ollama:
		endpoint := resolved.Endpoint
		store := config.NewSecretStore(config.SecretsPath())
		key, err := config.OllamaEnvAPIKey()
		if err != nil {
			return providerSelection{}, err
		}
		if key == "" && resolved.ProfileID != "" {
			key, err = store.ProfileAPIKey(resolved.ProfileID)
			if err != nil {
				return providerSelection{}, err
			}
		}
		if key == "" {
			key, err = store.OllamaAPIKey()
			if err != nil {
				return providerSelection{}, err
			}
		}
		return providerSelection{Provider: provider, ProfileID: resolved.ProfileID, Model: model, AI: ollama.New(endpoint, key)}, nil
	case providers.Runpod:
		endpoint := resolved.Endpoint
		key, err := config.NewSecretStore(config.SecretsPath()).RunpodAPIKey()
		if err != nil {
			return providerSelection{}, err
		}
		return providerSelection{Provider: provider, ProfileID: resolved.ProfileID, Model: model, AI: runpod.New(endpoint, key)}, nil
	default:
		endpoint := oauth.CodexResponsesBaseURL + oauth.CodexResponsesPath
		if v := os.Getenv("RUNE_CODEX_ENDPOINT"); v != "" {
			endpoint = v
		}
		tokenURL := oauth.CodexTokenURL
		if v := os.Getenv("RUNE_OAUTH_TOKEN_URL"); v != "" {
			tokenURL = v
		}
		store := oauth.NewStore(config.AuthPath())
		src := &oauth.CodexSource{Store: store, TokenURL: tokenURL}
		if _, err := src.Token(ctx); err != nil {
			return providerSelection{}, fmt.Errorf("not logged in: %w (run `rune login codex`)", err)
		}
		return providerSelection{Provider: provider, ProfileID: resolved.ProfileID, Model: model, AI: codex.New(endpoint, src)}, nil
	}
}
