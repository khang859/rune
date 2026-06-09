package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/session"
)

func TestInitialInteractiveSession_ResumeLoadsFromRuneDir(t *testing.T) {
	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)

	s := session.New("gpt-5")
	s.Provider = "ollama"
	s.Cwd = filepath.Join(t.TempDir(), "project")
	s.SetPath(filepath.Join(config.SessionsDir(), s.ID+".json"))
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "saved"}}})
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := initialInteractiveSession(s.ID, "ignored")
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != s.ID {
		t.Fatalf("loaded ID = %q, want %q", loaded.ID, s.ID)
	}
	if loaded.Path() != s.Path() {
		t.Fatalf("path = %q, want %q", loaded.Path(), s.Path())
	}
	if got := len(loaded.PathToActive()); got != 1 {
		t.Fatalf("active path len = %d, want 1", got)
	}
}

func TestInitialInteractiveSession_ResumeRejectsBadID(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	if _, err := initialInteractiveSession("../escape", "ignored"); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildProvider_EmptyOverrideUsesConfiguredProvider(t *testing.T) {
	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	if err := config.SaveSettings(config.SettingsPath(), config.Settings{Provider: "ollama", OllamaModel: "llama3.2"}); err != nil {
		t.Fatal(err)
	}

	selection, err := buildProvider(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if selection.Provider != "ollama" || selection.Model != "llama3.2" {
		t.Fatalf("selection = %q/%q, want ollama/llama3.2", selection.Provider, selection.Model)
	}
}
