package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Settings struct {
	Provider        string            `json:"provider,omitempty"`
	ActiveProfile   string            `json:"active_profile,omitempty"`
	Profiles        []ProviderProfile `json:"profiles,omitempty"`
	CodexModel      string            `json:"codex_model,omitempty"`
	GroqModel       string            `json:"groq_model,omitempty"`
	OllamaModel     string            `json:"ollama_model,omitempty"`
	RunpodModel     string            `json:"runpod_model,omitempty"`
	OllamaEndpoint  string            `json:"ollama_endpoint,omitempty"`
	RunpodEndpoint  string            `json:"runpod_endpoint,omitempty"`
	ReasoningEffort string            `json:"reasoning_effort,omitempty"`
	IconMode        string            `json:"icon_mode,omitempty"`
	ActivityMode    string            `json:"activity_mode,omitempty"`
	AutoCompact     AutoCompact       `json:"auto_compact,omitempty"`
	Web             WebSettings       `json:"web,omitempty"`
	Subagents       SubagentSettings  `json:"subagents,omitempty"`
}

type ProviderProfile struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Provider string `json:"provider"`
	Endpoint string `json:"endpoint,omitempty"`
	Model    string `json:"model,omitempty"`
}

type WebSettings struct {
	FetchEnabled      bool   `json:"fetch_enabled"`
	FetchAllowPrivate bool   `json:"fetch_allow_private"`
	SearchEnabled     string `json:"search_enabled,omitempty"`
	SearchProvider    string `json:"search_provider,omitempty"`
}

type SubagentSettings struct {
	Enabled            *bool `json:"enabled,omitempty"`
	MaxConcurrent      int   `json:"max_concurrent,omitempty"`
	DefaultTimeoutSecs int   `json:"default_timeout_secs,omitempty"`
	MaxCompletedRetain int   `json:"max_completed_retain,omitempty"`
}

type AutoCompact struct {
	Enabled      *bool `json:"enabled,omitempty"`
	ThresholdPct int   `json:"threshold_pct,omitempty"`
}

func boolPtr(v bool) *bool { return &v }

func (s SubagentSettings) EnabledValue() bool {
	if s.Enabled == nil {
		return true
	}
	return *s.Enabled
}

func (a AutoCompact) EnabledValue() bool {
	if a.Enabled == nil {
		return true
	}
	return *a.Enabled
}

func DefaultSettings() Settings {
	return Settings{
		CodexModel:      "gpt-5.5",
		GroqModel:       "llama-3.3-70b-versatile",
		OllamaModel:     "llama3.2",
		RunpodModel:     "openai/gpt-oss-120b",
		OllamaEndpoint:  "http://localhost:11434/v1/chat/completions",
		ReasoningEffort: "medium",
		IconMode:        "unicode",
		ActivityMode:    "arcane",
		AutoCompact:     AutoCompact{Enabled: boolPtr(true), ThresholdPct: 80},
		Web:             WebSettings{FetchEnabled: true, SearchEnabled: "auto", SearchProvider: "auto"},
		Subagents:       SubagentSettings{Enabled: boolPtr(true), MaxConcurrent: 4, DefaultTimeoutSecs: 600, MaxCompletedRetain: 100},
	}
}

func LoadSettings(path string) (Settings, error) {
	s := DefaultSettings()
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return s, nil
		}
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	return NormalizeSettings(s), nil
}

func NormalizeSettings(s Settings) Settings {
	d := DefaultSettings()
	if s.Provider != "" && s.Provider != "codex" && s.Provider != "groq" && s.Provider != "ollama" && s.Provider != "runpod" {
		s.Provider = ""
	}
	s.Profiles = NormalizeProviderProfiles(s.Profiles)
	if s.ActiveProfile != "" && FindProviderProfile(s.Profiles, s.ActiveProfile) == nil {
		s.ActiveProfile = ""
	}
	if s.CodexModel == "" {
		s.CodexModel = d.CodexModel
	}
	if s.GroqModel == "" {
		s.GroqModel = d.GroqModel
	}
	if s.OllamaModel == "" {
		s.OllamaModel = d.OllamaModel
	}
	if s.RunpodModel == "" {
		s.RunpodModel = d.RunpodModel
	}
	if s.OllamaEndpoint == "" {
		s.OllamaEndpoint = d.OllamaEndpoint
	}
	if s.ReasoningEffort == "" {
		s.ReasoningEffort = d.ReasoningEffort
	}
	if s.IconMode == "" {
		s.IconMode = d.IconMode
	}
	if s.ActivityMode == "" {
		s.ActivityMode = d.ActivityMode
	}
	if s.AutoCompact.Enabled == nil {
		s.AutoCompact.Enabled = d.AutoCompact.Enabled
	}
	if s.AutoCompact.ThresholdPct <= 0 || s.AutoCompact.ThresholdPct >= 100 {
		s.AutoCompact.ThresholdPct = d.AutoCompact.ThresholdPct
	}
	if s.Web.SearchEnabled == "" {
		s.Web.SearchEnabled = d.Web.SearchEnabled
	}
	if s.Web.SearchProvider == "" {
		s.Web.SearchProvider = d.Web.SearchProvider
	}
	if s.Subagents.Enabled == nil {
		s.Subagents.Enabled = d.Subagents.Enabled
	}
	if s.Subagents.MaxConcurrent <= 0 {
		s.Subagents.MaxConcurrent = d.Subagents.MaxConcurrent
	}
	if s.Subagents.DefaultTimeoutSecs <= 0 {
		s.Subagents.DefaultTimeoutSecs = d.Subagents.DefaultTimeoutSecs
	}
	if s.Subagents.MaxCompletedRetain <= 0 {
		s.Subagents.MaxCompletedRetain = d.Subagents.MaxCompletedRetain
	}
	return s
}

func NormalizeProviderProfiles(profiles []ProviderProfile) []ProviderProfile {
	out := make([]ProviderProfile, 0, len(profiles))
	seen := map[string]bool{}
	for _, p := range profiles {
		p.ID = strings.TrimSpace(p.ID)
		p.Name = strings.TrimSpace(p.Name)
		p.Provider = normalizeProviderID(p.Provider)
		p.Endpoint = strings.TrimSpace(p.Endpoint)
		p.Model = strings.TrimSpace(p.Model)
		if p.ID == "" || p.Provider == "" || seen[p.ID] {
			continue
		}
		seen[p.ID] = true
		out = append(out, p)
	}
	return out
}

func FindProviderProfile(profiles []ProviderProfile, id string) *ProviderProfile {
	id = strings.TrimSpace(id)
	for i := range profiles {
		if profiles[i].ID == id {
			return &profiles[i]
		}
	}
	return nil
}

func DefaultProfileForProvider(profiles []ProviderProfile, provider string) *ProviderProfile {
	provider = normalizeProviderID(provider)
	for i := range profiles {
		if profiles[i].Provider == provider {
			return &profiles[i]
		}
	}
	return nil
}

func ProfileDisplayName(p ProviderProfile) string {
	if strings.TrimSpace(p.Name) != "" {
		return strings.TrimSpace(p.Name)
	}
	if strings.TrimSpace(p.ID) != "" {
		return strings.TrimSpace(p.ID)
	}
	return strings.TrimSpace(p.Provider)
}

func normalizeProviderID(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "codex", "groq", "ollama", "runpod":
		return strings.ToLower(strings.TrimSpace(provider))
	default:
		return ""
	}
}

func SaveSettings(path string, s Settings) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(NormalizeSettings(s), "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("save settings: %w", err)
	}
	return nil
}
