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

func TestEnsureRuneDir_CreatesPrivateDirs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", filepath.Join(dir, "nested"))
	if err := EnsureRuneDir(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{RuneDir(), SessionsDir()} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", path)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("%s permissions = %o, want 700", path, got)
		}
	}
}

func TestEnsureRuneDir_MigratesExistingDirPermissions(t *testing.T) {
	dir := t.TempDir()
	runeDir := filepath.Join(dir, "nested")
	sessionsDir := filepath.Join(runeDir, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sessionsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("RUNE_DIR", runeDir)
	if err := EnsureRuneDir(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{runeDir, sessionsDir} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != 0o700 {
			t.Fatalf("%s permissions = %o, want 700", path, got)
		}
	}
}
