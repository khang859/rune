package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Settings struct {
	ReasoningEffort string      `json:"reasoning_effort,omitempty"`
	IconMode        string      `json:"icon_mode,omitempty"`
	ActivityMode    string      `json:"activity_mode,omitempty"`
	Web             WebSettings `json:"web,omitempty"`
}

type WebSettings struct {
	FetchEnabled      bool   `json:"fetch_enabled"`
	FetchAllowPrivate bool   `json:"fetch_allow_private"`
	SearchEnabled     string `json:"search_enabled,omitempty"`
	SearchProvider    string `json:"search_provider,omitempty"`
}

func DefaultSettings() Settings {
	return Settings{ReasoningEffort: "medium", IconMode: "unicode", ActivityMode: "arcane", Web: WebSettings{FetchEnabled: true, SearchEnabled: "auto", SearchProvider: "auto"}}
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
	if s.ReasoningEffort == "" {
		s.ReasoningEffort = d.ReasoningEffort
	}
	if s.IconMode == "" {
		s.IconMode = d.IconMode
	}
	if s.ActivityMode == "" {
		s.ActivityMode = d.ActivityMode
	}
	if s.Web.SearchEnabled == "" {
		s.Web.SearchEnabled = d.Web.SearchEnabled
	}
	if s.Web.SearchProvider == "" {
		s.Web.SearchProvider = d.Web.SearchProvider
	}
	return s
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
