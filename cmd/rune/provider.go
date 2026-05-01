package main

import (
	"context"
	"fmt"
	"os"

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
	Provider string
	Model    string
	AI       ai.Provider
}

func buildProvider(ctx context.Context, providerOverride, modelOverride string) (providerSelection, error) {
	settings, _ := config.LoadSettings(config.SettingsPath())
	provider := settings.Provider
	if v := os.Getenv("RUNE_PROVIDER"); v != "" {
		provider = v
	}
	if providerOverride != "" {
		provider = providerOverride
	}
	provider = providers.Normalize(provider)

	model := modelOverride
	if model == "" {
		switch provider {
		case providers.Groq:
			model = os.Getenv("RUNE_GROQ_MODEL")
		case providers.Ollama:
			model = os.Getenv("RUNE_OLLAMA_MODEL")
		case providers.Runpod:
			model = os.Getenv("RUNE_RUNPOD_MODEL")
		default:
			model = os.Getenv("RUNE_CODEX_MODEL")
		}
	}
	if model == "" {
		switch provider {
		case providers.Groq:
			model = settings.GroqModel
		case providers.Ollama:
			model = settings.OllamaModel
		case providers.Runpod:
			model = settings.RunpodModel
		default:
			model = settings.CodexModel
		}
	}
	if model == "" {
		model = providers.DefaultModel(provider)
	}

	switch provider {
	case providers.Groq:
		endpoint := groq.DefaultEndpoint
		if v := os.Getenv("RUNE_GROQ_ENDPOINT"); v != "" {
			endpoint = v
		}
		key, err := config.NewSecretStore(config.SecretsPath()).GroqAPIKey()
		if err != nil {
			return providerSelection{}, err
		}
		return providerSelection{Provider: provider, Model: model, AI: groq.New(endpoint, key)}, nil
	case providers.Ollama:
		endpoint := settings.OllamaEndpoint
		if endpoint == "" {
			endpoint = ollama.DefaultEndpoint
		}
		if v := os.Getenv("RUNE_OLLAMA_ENDPOINT"); v != "" {
			endpoint = v
		}
		return providerSelection{Provider: provider, Model: model, AI: ollama.New(endpoint)}, nil
	case providers.Runpod:
		endpoint := runpod.EndpointForModel(model)
		if v := os.Getenv("RUNE_RUNPOD_ENDPOINT"); v != "" {
			endpoint = v
		}
		key, err := config.NewSecretStore(config.SecretsPath()).RunpodAPIKey()
		if err != nil {
			return providerSelection{}, err
		}
		return providerSelection{Provider: provider, Model: model, AI: runpod.New(endpoint, key)}, nil
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
		return providerSelection{Provider: provider, Model: model, AI: codex.New(endpoint, src)}, nil
	}
}
