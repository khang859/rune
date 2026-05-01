package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeBraveAPIKeyInput(t *testing.T) {
	cases := map[string]string{
		" key\n":                               "key",
		"\"key\"":                              "key",
		"'key'":                                "key",
		"export RUNE_BRAVE_SEARCH_API_KEY=key": "key",
		"RUNE_BRAVE_SEARCH_API_KEY='key'":      "key",
	}
	for in, want := range cases {
		if got := NormalizeBraveAPIKeyInput(in); got != want {
			t.Fatalf("NormalizeBraveAPIKeyInput(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestValidateBraveAPIKeyRejectsObviousMistakes(t *testing.T) {
	for _, key := range []string{"", "short", strings.Repeat("x", 513), "abc defghijklmnopqrstuvwxyz", "abc\ndefghijklmnopqrstuvwxyz", "abc{defghijklmnopqrstuvwxyz"} {
		if err := ValidateBraveAPIKey(key); err == nil {
			t.Fatalf("ValidateBraveAPIKey(%q) = nil", key)
		}
	}
}

func TestSecretStoreSaveLoadAndEnvPrecedence(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rune")
	store := NewSecretStore(filepath.Join(dir, "secrets.json"))
	stored := strings.Repeat("a", 24)
	env := strings.Repeat("b", 24)
	if err := store.SetBraveSearchAPIKey(stored); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "secrets.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("permissions = %v, want 0600", info.Mode().Perm())
	}
	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permissions = %o, want 700", got)
	}
	key, err := store.BraveSearchAPIKey()
	if err != nil || key != stored {
		t.Fatalf("stored key = %q, %v", key, err)
	}
	t.Setenv("RUNE_BRAVE_SEARCH_API_KEY", env)
	key, err = store.BraveSearchAPIKey()
	if err != nil || key != env {
		t.Fatalf("env key = %q, %v", key, err)
	}
}

func TestSecretStore_MigratesExistingDirPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "rune")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	store := NewSecretStore(filepath.Join(dir, "secrets.json"))
	if err := store.SetBraveSearchAPIKey(strings.Repeat("a", 24)); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permissions = %o, want 700", got)
	}
}

func TestSecretStoreRejectsInvalidWithoutOverwriting(t *testing.T) {
	store := NewSecretStore(filepath.Join(t.TempDir(), "secrets.json"))
	valid := strings.Repeat("a", 24)
	if err := store.SetBraveSearchAPIKey(valid); err != nil {
		t.Fatal(err)
	}
	if err := store.SetBraveSearchAPIKey("bad key"); err == nil {
		t.Fatal("expected invalid key error")
	}
	sec, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if sec.BraveSearchAPIKey != valid {
		t.Fatalf("key overwritten: %q", sec.BraveSearchAPIKey)
	}
}

func TestTavilyAPIKeySaveLoadAndEnvPrecedence(t *testing.T) {
	store := NewSecretStore(filepath.Join(t.TempDir(), "secrets.json"))
	stored := "tvly-" + strings.Repeat("a", 24)
	env := "tvly-" + strings.Repeat("b", 24)
	if err := store.SetTavilyAPIKey("export RUNE_TAVILY_API_KEY='" + stored + "'"); err != nil {
		t.Fatal(err)
	}
	key, err := store.TavilyAPIKey()
	if err != nil || key != stored {
		t.Fatalf("stored tavily key = %q, %v", key, err)
	}
	t.Setenv("RUNE_TAVILY_API_KEY", env)
	key, err = store.TavilyAPIKey()
	if err != nil || key != env {
		t.Fatalf("env tavily key = %q, %v", key, err)
	}
}

func TestGroqAPIKeySaveLoadAndEnvPrecedence(t *testing.T) {
	store := NewSecretStore(filepath.Join(t.TempDir(), "secrets.json"))
	stored := strings.Repeat("g", 24)
	env := strings.Repeat("h", 24)
	if err := store.SetGroqAPIKey("export GROQ_API_KEY='" + stored + "'"); err != nil {
		t.Fatal(err)
	}
	key, err := store.GroqAPIKey()
	if err != nil || key != stored {
		t.Fatalf("stored groq key = %q, %v", key, err)
	}
	t.Setenv("RUNE_GROQ_API_KEY", env)
	key, err = store.GroqAPIKey()
	if err != nil || key != env {
		t.Fatalf("env groq key = %q, %v", key, err)
	}
}

func TestOllamaAPIKeySaveLoadAndEnvPrecedence(t *testing.T) {
	store := NewSecretStore(filepath.Join(t.TempDir(), "secrets.json"))
	stored := "ollama-local-token"
	env := "ollama-env-token"
	if err := store.SetOllamaAPIKey("export OLLAMA_API_KEY='" + stored + "'"); err != nil {
		t.Fatal(err)
	}
	key, err := store.OllamaAPIKey()
	if err != nil || key != stored {
		t.Fatalf("stored ollama key = %q, %v", key, err)
	}
	t.Setenv("RUNE_OLLAMA_API_KEY", env)
	key, err = store.OllamaAPIKey()
	if err != nil || key != env {
		t.Fatalf("env ollama key = %q, %v", key, err)
	}
}

func TestProfileAPIKeySaveLoad(t *testing.T) {
	store := NewSecretStore(filepath.Join(t.TempDir(), "secrets.json"))
	if err := store.SetProfileAPIKey("ollama-gpu", "profile-token"); err != nil {
		t.Fatal(err)
	}
	key, err := store.ProfileAPIKey("ollama-gpu")
	if err != nil || key != "profile-token" {
		t.Fatalf("profile key = %q, %v", key, err)
	}
}

func TestRunpodAPIKeySaveLoadAndEnvPrecedence(t *testing.T) {
	store := NewSecretStore(filepath.Join(t.TempDir(), "secrets.json"))
	stored := strings.Repeat("r", 24)
	env := strings.Repeat("p", 24)
	if err := store.SetRunpodAPIKey("export RUNPOD_API_KEY='" + stored + "'"); err != nil {
		t.Fatal(err)
	}
	key, err := store.RunpodAPIKey()
	if err != nil || key != stored {
		t.Fatalf("stored runpod key = %q, %v", key, err)
	}
	t.Setenv("RUNE_RUNPOD_API_KEY", env)
	key, err = store.RunpodAPIKey()
	if err != nil || key != env {
		t.Fatalf("env runpod key = %q, %v", key, err)
	}
}

func TestNormalizeOllamaAPIKeyInput(t *testing.T) {
	if got := NormalizeOllamaAPIKeyInput("OLLAMA_API_KEY='key'"); got != "key" {
		t.Fatalf("NormalizeOllamaAPIKeyInput = %q, want key", got)
	}
}

func TestNormalizeRunpodAPIKeyInput(t *testing.T) {
	if got := NormalizeRunpodAPIKeyInput("RUNPOD_API_KEY='key'"); got != "key" {
		t.Fatalf("NormalizeRunpodAPIKeyInput = %q, want key", got)
	}
}
