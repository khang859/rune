package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Secrets struct {
	BraveSearchAPIKey string `json:"brave_search_api_key,omitempty"`
	TavilyAPIKey      string `json:"tavily_api_key,omitempty"`
	GroqAPIKey        string `json:"groq_api_key,omitempty"`
	RunpodAPIKey      string `json:"runpod_api_key,omitempty"`
}

type SecretStore struct{ path string }

func NewSecretStore(path string) *SecretStore { return &SecretStore{path: path} }

func (s *SecretStore) Load() (Secrets, error) {
	var sec Secrets
	b, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return sec, nil
		}
		return sec, err
	}
	if err := json.Unmarshal(b, &sec); err != nil {
		return sec, err
	}
	sec.BraveSearchAPIKey = NormalizeBraveAPIKeyInput(sec.BraveSearchAPIKey)
	sec.TavilyAPIKey = NormalizeTavilyAPIKeyInput(sec.TavilyAPIKey)
	sec.GroqAPIKey = NormalizeGroqAPIKeyInput(sec.GroqAPIKey)
	sec.RunpodAPIKey = NormalizeRunpodAPIKeyInput(sec.RunpodAPIKey)
	return sec, nil
}
func (s *SecretStore) Save(sec Secrets) error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(sec, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("save secrets: %w", err)
	}
	_ = os.Chmod(s.path, 0o600)
	return nil
}
func (s *SecretStore) BraveSearchAPIKey() (string, error) {
	if v := NormalizeBraveAPIKeyInput(os.Getenv("RUNE_BRAVE_SEARCH_API_KEY")); v != "" {
		return v, ValidateBraveAPIKey(v)
	}
	sec, err := s.Load()
	if err != nil {
		return "", err
	}
	if sec.BraveSearchAPIKey == "" {
		return "", nil
	}
	return sec.BraveSearchAPIKey, ValidateBraveAPIKey(sec.BraveSearchAPIKey)
}
func (s *SecretStore) SetBraveSearchAPIKey(key string) error {
	key = NormalizeBraveAPIKeyInput(key)
	if err := ValidateBraveAPIKey(key); err != nil {
		return fmt.Errorf("invalid Brave Search API key: %w", err)
	}
	sec, err := s.Load()
	if err != nil {
		return err
	}
	sec.BraveSearchAPIKey = key
	return s.Save(sec)
}
func (s *SecretStore) DeleteBraveSearchAPIKey() error {
	sec, err := s.Load()
	if err != nil {
		return err
	}
	sec.BraveSearchAPIKey = ""
	return s.Save(sec)
}

func (s *SecretStore) TavilyAPIKey() (string, error) {
	if v := NormalizeTavilyAPIKeyInput(os.Getenv("RUNE_TAVILY_API_KEY")); v != "" {
		return v, ValidateTavilyAPIKey(v)
	}
	sec, err := s.Load()
	if err != nil {
		return "", err
	}
	if sec.TavilyAPIKey == "" {
		return "", nil
	}
	return sec.TavilyAPIKey, ValidateTavilyAPIKey(sec.TavilyAPIKey)
}
func (s *SecretStore) SetTavilyAPIKey(key string) error {
	key = NormalizeTavilyAPIKeyInput(key)
	if err := ValidateTavilyAPIKey(key); err != nil {
		return fmt.Errorf("invalid Tavily API key: %w", err)
	}
	sec, err := s.Load()
	if err != nil {
		return err
	}
	sec.TavilyAPIKey = key
	return s.Save(sec)
}
func (s *SecretStore) DeleteTavilyAPIKey() error {
	sec, err := s.Load()
	if err != nil {
		return err
	}
	sec.TavilyAPIKey = ""
	return s.Save(sec)
}

func (s *SecretStore) GroqAPIKey() (string, error) {
	for _, env := range []string{"RUNE_GROQ_API_KEY", "GROQ_API_KEY"} {
		if v := NormalizeGroqAPIKeyInput(os.Getenv(env)); v != "" {
			return v, ValidateGroqAPIKey(v)
		}
	}
	sec, err := s.Load()
	if err != nil {
		return "", err
	}
	if sec.GroqAPIKey == "" {
		return "", nil
	}
	return sec.GroqAPIKey, ValidateGroqAPIKey(sec.GroqAPIKey)
}
func (s *SecretStore) SetGroqAPIKey(key string) error {
	key = NormalizeGroqAPIKeyInput(key)
	if err := ValidateGroqAPIKey(key); err != nil {
		return fmt.Errorf("invalid Groq API key: %w", err)
	}
	sec, err := s.Load()
	if err != nil {
		return err
	}
	sec.GroqAPIKey = key
	return s.Save(sec)
}
func (s *SecretStore) DeleteGroqAPIKey() error {
	sec, err := s.Load()
	if err != nil {
		return err
	}
	sec.GroqAPIKey = ""
	return s.Save(sec)
}

func (s *SecretStore) RunpodAPIKey() (string, error) {
	for _, env := range []string{"RUNE_RUNPOD_API_KEY", "RUNPOD_API_KEY"} {
		if v := NormalizeRunpodAPIKeyInput(os.Getenv(env)); v != "" {
			return v, ValidateRunpodAPIKey(v)
		}
	}
	sec, err := s.Load()
	if err != nil {
		return "", err
	}
	if sec.RunpodAPIKey == "" {
		return "", nil
	}
	return sec.RunpodAPIKey, ValidateRunpodAPIKey(sec.RunpodAPIKey)
}
func (s *SecretStore) SetRunpodAPIKey(key string) error {
	key = NormalizeRunpodAPIKeyInput(key)
	if err := ValidateRunpodAPIKey(key); err != nil {
		return fmt.Errorf("invalid Runpod API key: %w", err)
	}
	sec, err := s.Load()
	if err != nil {
		return err
	}
	sec.RunpodAPIKey = key
	return s.Save(sec)
}
func (s *SecretStore) DeleteRunpodAPIKey() error {
	sec, err := s.Load()
	if err != nil {
		return err
	}
	sec.RunpodAPIKey = ""
	return s.Save(sec)
}

func NormalizeBraveAPIKeyInput(raw string) string {
	s := strings.TrimSpace(raw)
	for _, p := range []string{"export RUNE_BRAVE_SEARCH_API_KEY=", "RUNE_BRAVE_SEARCH_API_KEY="} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimSpace(strings.TrimPrefix(s, p))
			break
		}
	}
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			s = s[1 : len(s)-1]
		}
	}
	return strings.TrimSpace(s)
}
func ValidateBraveAPIKey(key string) error {
	switch {
	case key == "":
		return errors.New("empty")
	case len(key) < 20:
		return errors.New("too short")
	case len(key) > 512:
		return errors.New("too long")
	case strings.ContainsAny(key, " \t\r\n"):
		return errors.New("contains whitespace")
	case strings.ContainsAny(key, "<>{}[]()"):
		return errors.New("contains unexpected characters")
	}
	return nil
}

func NormalizeTavilyAPIKeyInput(raw string) string {
	s := strings.TrimSpace(raw)
	for _, p := range []string{"export RUNE_TAVILY_API_KEY=", "RUNE_TAVILY_API_KEY="} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimSpace(strings.TrimPrefix(s, p))
			break
		}
	}
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			s = s[1 : len(s)-1]
		}
	}
	return strings.TrimSpace(s)
}

func ValidateTavilyAPIKey(key string) error {
	switch {
	case key == "":
		return errors.New("empty")
	case len(key) < 20:
		return errors.New("too short")
	case len(key) > 512:
		return errors.New("too long")
	case strings.ContainsAny(key, " \t\r\n"):
		return errors.New("contains whitespace")
	case strings.ContainsAny(key, "<>{}[]()"):
		return errors.New("contains unexpected characters")
	}
	return nil
}

func NormalizeGroqAPIKeyInput(raw string) string {
	s := strings.TrimSpace(raw)
	for _, p := range []string{"export RUNE_GROQ_API_KEY=", "RUNE_GROQ_API_KEY=", "export GROQ_API_KEY=", "GROQ_API_KEY="} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimSpace(strings.TrimPrefix(s, p))
			break
		}
	}
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			s = s[1 : len(s)-1]
		}
	}
	return strings.TrimSpace(s)
}

func ValidateGroqAPIKey(key string) error {
	switch {
	case key == "":
		return errors.New("empty")
	case len(key) < 20:
		return errors.New("too short")
	case len(key) > 512:
		return errors.New("too long")
	case strings.ContainsAny(key, " \t\r\n"):
		return errors.New("contains whitespace")
	case strings.ContainsAny(key, "<>{}[]()"):
		return errors.New("contains unexpected characters")
	}
	return nil
}

func NormalizeRunpodAPIKeyInput(raw string) string {
	s := strings.TrimSpace(raw)
	for _, p := range []string{"export RUNE_RUNPOD_API_KEY=", "RUNE_RUNPOD_API_KEY=", "export RUNPOD_API_KEY=", "RUNPOD_API_KEY="} {
		if strings.HasPrefix(s, p) {
			s = strings.TrimSpace(strings.TrimPrefix(s, p))
			break
		}
	}
	if len(s) >= 2 {
		first, last := s[0], s[len(s)-1]
		if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
			s = s[1 : len(s)-1]
		}
	}
	return strings.TrimSpace(s)
}

func ValidateRunpodAPIKey(key string) error {
	switch {
	case key == "":
		return errors.New("empty")
	case len(key) < 20:
		return errors.New("too short")
	case len(key) > 512:
		return errors.New("too long")
	case strings.ContainsAny(key, " \t\r\n"):
		return errors.New("contains whitespace")
	case strings.ContainsAny(key, "<>{}[]()"):
		return errors.New("contains unexpected characters")
	}
	return nil
}
