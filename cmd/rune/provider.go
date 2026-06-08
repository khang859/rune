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
	"github.com/khang859/rune/internal/ai/openrouter"
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
	// Carry the resolved provider on every failure path so callers can craft an
	// accurate, provider-aware recovery notice (see startupRecoveryNotice).
	fail := func(err error) (providerSelection, error) {
		return providerSelection{Provider: provider, ProfileID: resolved.ProfileID, Model: model}, err
	}

	switch provider {
	case providers.Groq:
		endpoint := resolved.Endpoint
		key, err := config.NewSecretStore(config.SecretsPath()).GroqAPIKey()
		if err != nil {
			return fail(err)
		}
		return providerSelection{Provider: provider, ProfileID: resolved.ProfileID, Model: model, AI: groq.New(endpoint, key)}, nil
	case providers.Ollama:
		endpoint := resolved.Endpoint
		store := config.NewSecretStore(config.SecretsPath())
		key, err := config.OllamaEnvAPIKey()
		if err != nil {
			return fail(err)
		}
		if key == "" && resolved.ProfileID != "" {
			key, err = store.ProfileAPIKey(resolved.ProfileID)
			if err != nil {
				return fail(err)
			}
		}
		if key == "" {
			key, err = store.OllamaAPIKey()
			if err != nil {
				return fail(err)
			}
		}
		return providerSelection{Provider: provider, ProfileID: resolved.ProfileID, Model: model, AI: ollama.NewWithOptions(ollama.Options{
			Endpoint: endpoint,
			APIKey:   key,
			NumCtx:   resolved.OllamaNumCtx,
			Think:    resolved.OllamaThink,
		})}, nil
	case providers.Runpod:
		endpoint := resolved.Endpoint
		key, err := config.NewSecretStore(config.SecretsPath()).RunpodAPIKey()
		if err != nil {
			return fail(err)
		}
		return providerSelection{Provider: provider, ProfileID: resolved.ProfileID, Model: model, AI: runpod.New(endpoint, key)}, nil
	case providers.OpenRouter:
		endpoint := resolved.Endpoint
		key, err := config.NewSecretStore(config.SecretsPath()).OpenRouterAPIKey()
		if err != nil {
			return fail(err)
		}
		return providerSelection{Provider: provider, ProfileID: resolved.ProfileID, Model: model, AI: openrouter.New(endpoint, key)}, nil
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
			return fail(fmt.Errorf("not logged in: %w (run `rune login codex`)", err))
		}
		return providerSelection{Provider: provider, ProfileID: resolved.ProfileID, Model: model, AI: codex.New(endpoint, src)}, nil
	}
}

// startupRecoveryNotice turns a buildProvider failure into a short, actionable
// banner shown when rune drops into the TUI instead of exiting. It always points
// at the two in-TUI recovery paths: /login (re-authenticate) and /providers
// (switch provider).
func startupRecoveryNotice(provider string, err error) string {
	p := strings.TrimSpace(provider)
	msg := err.Error()
	switch {
	case p == "":
		return "⚠ No provider configured. Type /providers to choose one, or /login to sign in to Codex."
	case p == providers.Codex && (strings.Contains(msg, "refresh_token_invalidated") ||
		strings.Contains(msg, "token endpoint 401") || strings.Contains(msg, "no refresh token")):
		// Expired/invalidated session or never logged in — not a transient fault.
		return "⚠ Codex session expired or not signed in. Type /login to re-authenticate, or /providers to switch providers."
	case p == providers.Codex:
		// Network or other unexpected failure — keep the underlying detail.
		return fmt.Sprintf("⚠ Codex sign-in failed: %v. Type /login to re-authenticate, or /providers to switch providers.", err)
	default:
		return fmt.Sprintf("⚠ %s is unavailable: %v. Fix it via /settings, or /providers to switch providers.", providerDisplay(p), err)
	}
}

func providerDisplay(id string) string {
	for _, info := range providers.All() {
		if info.ID == id {
			return info.Display
		}
	}
	return id
}
