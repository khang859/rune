package providers

import (
	"fmt"
	"os"
	"strings"

	"github.com/khang859/rune/internal/config"
)

const (
	DefaultGroqEndpoint      = "https://api.groq.com/openai/v1/chat/completions"
	DefaultOllamaEndpoint    = "http://localhost:11434/v1/chat/completions"
	DefaultRunpodEndpoint    = "https://api.runpod.ai/v2/gpt-oss-120b/openai/v1/chat/completions"
	QwenRunpodPublicEndpoint = "https://api.runpod.ai/v2/qwen3-32b-awq/openai/v1/chat/completions"
)

type ResolvedProvider struct {
	Provider  string
	ProfileID string
	Model     string
	Endpoint  string
}

type ResolveOptions struct {
	ProviderOverride string
	ModelOverride    string
	ProfileOverride  *string
}

func NoProfile() *string        { return stringPtr("") }
func Profile(id string) *string { return stringPtr(id) }

func stringPtr(s string) *string { return &s }

func Resolve(settings config.Settings, opts ResolveOptions) ResolvedProvider {
	settings = config.NormalizeSettings(settings)
	profileID := ""
	profileExplicit := opts.ProfileOverride != nil
	if opts.ProfileOverride != nil {
		profileID = strings.TrimSpace(*opts.ProfileOverride)
	} else {
		profileID = strings.TrimSpace(os.Getenv("RUNE_PROVIDER_PROFILE"))
		if profileID == "" {
			profileID = settings.ActiveProfile
		}
	}

	profile := config.FindProviderProfile(settings.Profiles, profileID)
	provider := settings.Provider
	if profile != nil {
		provider = profile.Provider
	}
	if v := strings.TrimSpace(os.Getenv("RUNE_PROVIDER")); v != "" {
		provider = v
	}
	if strings.TrimSpace(opts.ProviderOverride) != "" {
		provider = opts.ProviderOverride
	}
	provider = strings.TrimSpace(provider)
	if provider != "" {
		provider = Normalize(provider)
	}
	if provider != "" && (profile == nil || profile.Provider != provider) {
		if profileExplicit {
			profile = nil
			profileID = ""
		}
	}

	model := strings.TrimSpace(opts.ModelOverride)
	if model == "" {
		switch provider {
		case Groq:
			model = os.Getenv("RUNE_GROQ_MODEL")
		case Ollama:
			model = os.Getenv("RUNE_OLLAMA_MODEL")
		case Runpod:
			model = os.Getenv("RUNE_RUNPOD_MODEL")
		default:
			model = os.Getenv("RUNE_CODEX_MODEL")
		}
	}
	if model == "" && profile != nil {
		model = profile.Model
	}
	if model == "" {
		switch provider {
		case Groq:
			model = settings.GroqModel
		case Ollama:
			model = settings.OllamaModel
		case Runpod:
			model = settings.RunpodModel
		default:
			model = settings.CodexModel
		}
	}
	if model == "" && provider != "" {
		model = DefaultModel(provider)
	}

	endpoint := ""
	if profile != nil {
		endpoint = profile.Endpoint
	}
	if provider == Ollama && endpoint == "" {
		endpoint = settings.OllamaEndpoint
	}
	if provider == Runpod && endpoint == "" {
		endpoint = settings.RunpodEndpoint
	}
	if provider == Runpod && endpoint == "" {
		endpoint = EndpointForRunpodModel(model)
	}
	if provider == Ollama && endpoint == "" {
		endpoint = DefaultOllamaEndpoint
	}
	if provider == Groq && endpoint == "" {
		endpoint = DefaultGroqEndpoint
	}
	switch provider {
	case Groq:
		if v := os.Getenv("RUNE_GROQ_ENDPOINT"); v != "" {
			endpoint = v
		}
	case Ollama:
		if v := os.Getenv("RUNE_OLLAMA_ENDPOINT"); v != "" {
			endpoint = v
		}
	case Runpod:
		if v := os.Getenv("RUNE_RUNPOD_ENDPOINT"); v != "" {
			endpoint = v
		}
	}

	return ResolvedProvider{Provider: provider, ProfileID: profileID, Model: model, Endpoint: endpoint}
}

func EndpointForRunpodModel(model string) string {
	if strings.TrimSpace(model) == "Qwen/Qwen3-32B-AWQ" {
		return QwenRunpodPublicEndpoint
	}
	return DefaultRunpodEndpoint
}

func SaveResolvedSelection(path string, s config.Settings, r ResolvedProvider) error {
	s = config.NormalizeSettings(s)
	s.Provider = r.Provider
	s.ActiveProfile = r.ProfileID
	if r.ProfileID != "" {
		for i := range s.Profiles {
			if s.Profiles[i].ID == r.ProfileID {
				s.Profiles[i].Model = r.Model
			}
		}
	}
	switch r.Provider {
	case Codex:
		s.CodexModel = r.Model
	case Groq:
		s.GroqModel = r.Model
	case Ollama:
		s.OllamaModel = r.Model
	case Runpod:
		s.RunpodModel = r.Model
	case "":
		// Save provider reset without changing model defaults.
	default:
		return fmt.Errorf("unknown provider %q", r.Provider)
	}
	return config.SaveSettings(path, s)
}
