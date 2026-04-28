package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEdit_ReplacesUniqueString(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(p, []byte("alpha BETA gamma"), 0o644)
	args, _ := json.Marshal(map[string]string{"path": p, "old_string": "BETA", "new_string": "delta"})
	res, err := (Edit{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatal(res.Output)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "alpha delta gamma" {
		t.Fatalf("content = %q", b)
	}
}

func TestEdit_FailsOnAmbiguous(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(p, []byte("foo foo"), 0o644)
	args, _ := json.Marshal(map[string]string{"path": p, "old_string": "foo", "new_string": "bar"})
	res, _ := (Edit{}).Run(context.Background(), args)
	if !res.IsError {
		t.Fatal("expected IsError=true on ambiguous match")
	}
	if !strings.Contains(res.Output, "disambiguate") {
		t.Fatalf("output should guide on disambiguation: %q", res.Output)
	}
}

func TestEdit_FailsWhenNotFound(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.txt")
	_ = os.WriteFile(p, []byte("hello"), 0o644)
	args, _ := json.Marshal(map[string]string{"path": p, "old_string": "missing", "new_string": "nope"})
	res, _ := (Edit{}).Run(context.Background(), args)
	if !res.IsError {
		t.Fatal("expected IsError=true on no match")
	}
	if !strings.Contains(res.Output, "whitespace") {
		t.Fatalf("output should guide on exact-match expectations: %q", res.Output)
	}
}
