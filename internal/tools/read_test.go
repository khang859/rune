package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRead_File(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "foo.txt")
	if err := os.WriteFile(p, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"path": p})
	res, err := (Read{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if res.Output != "hello\nworld\n" {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestRead_Missing(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"path": "/does/not/exist"})
	res, err := (Read{}).Run(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError=true, got %#v", res)
	}
	if !strings.Contains(res.Output, "/does/not/exist") || !strings.Contains(res.Output, "ls") {
		t.Fatalf("output should guide recovery: %q", res.Output)
	}
}

func TestRead_BadArgs(t *testing.T) {
	res, err := (Read{}).Run(context.Background(), json.RawMessage(`not-json`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true")
	}
	if !strings.Contains(res.Output, `"path"`) {
		t.Fatalf("output should echo expected schema: %q", res.Output)
	}
}
