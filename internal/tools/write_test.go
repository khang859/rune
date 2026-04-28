package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestWrite_NewFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.txt")
	args, _ := json.Marshal(map[string]any{"path": p, "content": "hi"})
	res, err := (Write{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("error: %s", res.Output)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "hi" {
		t.Fatalf("content = %q", b)
	}
}

func TestWrite_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(p, []byte("old"), 0o644)
	args, _ := json.Marshal(map[string]any{"path": p, "content": "new"})
	if _, err := (Write{}).Run(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "new" {
		t.Fatalf("content = %q", b)
	}
}

func TestWrite_CreatesParents(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a", "b", "c.txt")
	args, _ := json.Marshal(map[string]any{"path": p, "content": "ok"})
	if _, err := (Write{}).Run(context.Background(), args); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
}
