package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRuneDir_DefaultsToHomeRune(t *testing.T) {
	t.Setenv("HOME", "/tmp/fakehome")
	t.Setenv("RUNE_DIR", "")
	got := RuneDir()
	want := filepath.Join("/tmp/fakehome", ".rune")
	if got != want {
		t.Fatalf("RuneDir() = %q, want %q", got, want)
	}
}

func TestRuneDir_RespectsRUNE_DIR(t *testing.T) {
	t.Setenv("RUNE_DIR", "/custom/path")
	if got := RuneDir(); got != "/custom/path" {
		t.Fatalf("RuneDir() = %q, want %q", got, "/custom/path")
	}
}

func TestSessionsDir_IsUnderRuneDir(t *testing.T) {
	t.Setenv("RUNE_DIR", "/r")
	if got := SessionsDir(); !strings.HasSuffix(got, "/sessions") || !strings.HasPrefix(got, "/r") {
		t.Fatalf("SessionsDir() = %q, want under /r ending /sessions", got)
	}
}

func TestEnsureRuneDir_CreatesDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", filepath.Join(dir, "nested"))
	if err := EnsureRuneDir(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(RuneDir()); err != nil {
		t.Fatal(err)
	}
}
