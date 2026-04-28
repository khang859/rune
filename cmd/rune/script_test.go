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
