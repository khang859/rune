package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai/faux"
)

func TestRunScript_FauxTextTurn(t *testing.T) {
	dir := t.TempDir()
	sessPath := filepath.Join(dir, "s.json")

	sc := scriptFile{
		Provider: "faux",
		Session:  sessPath,
		Model:    "gpt-5",
		Faux: []fauxStep{
			{Reply: "hello back"},
			{Done: true},
		},
		UserMessage: "hi",
	}
	b, _ := json.Marshal(sc)
	scriptPath := filepath.Join(dir, "in.json")
	_ = os.WriteFile(scriptPath, b, 0o644)

	var out bytes.Buffer
	if err := runScript(context.Background(), scriptPath, &out, faux.New()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "hello back") {
		t.Fatalf("output missing assistant text: %q", out.String())
	}
	if _, err := os.Stat(sessPath); err != nil {
		t.Fatalf("session file not written: %v", err)
	}
}

func TestRunScript_TurnErrorReturnsError(t *testing.T) {
	dir := t.TempDir()
	sessPath := filepath.Join(dir, "s.json")

	// A turn with a reply but no Done step: the faux stream ends without an
	// ai.Done event, so the agent loop emits TurnError("stream ended unexpectedly").
	sc := scriptFile{
		Provider: "faux",
		Session:  sessPath,
		Model:    "gpt-5",
		Faux: []fauxStep{
			{Reply: "partial"},
		},
		UserMessage: "hi",
	}
	b, _ := json.Marshal(sc)
	scriptPath := filepath.Join(dir, "in.json")
	_ = os.WriteFile(scriptPath, b, 0o644)

	var out bytes.Buffer
	err := runScript(context.Background(), scriptPath, &out, faux.New())
	if err == nil || !strings.Contains(err.Error(), "stream ended unexpectedly") {
		t.Fatalf("err = %v, want stream-ended error", err)
	}
	if !strings.Contains(out.String(), "[error:") {
		t.Fatalf("transcript missing [error: ...] line: %q", out.String())
	}
	if _, statErr := os.Stat(sessPath); statErr != nil {
		t.Fatalf("session file not written before error return: %v", statErr)
	}
}

func TestRunScript_SaveErrorTakesPrecedenceOverTurnError(t *testing.T) {
	dir := t.TempDir()
	// Point the session path at an existing directory: session.Save writes a
	// temp file then os.Rename's it onto the path, and renaming a file onto a
	// directory fails — forcing a save error.
	sessDir := filepath.Join(dir, "session-as-dir")
	if err := os.Mkdir(sessDir, 0o755); err != nil {
		t.Fatal(err)
	}

	sc := scriptFile{
		Provider: "faux",
		Session:  sessDir,
		Model:    "gpt-5",
		Faux: []fauxStep{
			{Reply: "partial"}, // no Done step → also triggers a TurnError
		},
		UserMessage: "hi",
	}
	b, _ := json.Marshal(sc)
	scriptPath := filepath.Join(dir, "in.json")
	_ = os.WriteFile(scriptPath, b, 0o644)

	var out bytes.Buffer
	err := runScript(context.Background(), scriptPath, &out, faux.New())
	if err == nil || !strings.Contains(err.Error(), "save session") {
		t.Fatalf("err = %v, want save session error to win over TurnError", err)
	}
}
