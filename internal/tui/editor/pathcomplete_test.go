package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestComplete_SinglePrefixUniqueExpands(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "alpha.go"), nil, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "beta.go"), nil, 0o644)

	// Caller passes (current word, cwd). We complete using cwd.
	out, ok := CompletePath("alp", dir)
	if !ok {
		t.Fatal("expected unique completion for 'alp'")
	}
	if out != "alpha.go" {
		t.Fatalf("complete = %q", out)
	}
}

func TestComplete_AmbiguousReturnsFalse(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "alpha.go"), nil, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "almost.go"), nil, 0o644)
	if _, ok := CompletePath("al", dir); ok {
		t.Fatal("expected ambiguity to return ok=false")
	}
}

func TestComplete_DirSlash(t *testing.T) {
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "internal"), 0o755)
	out, ok := CompletePath("inter", dir)
	if !ok {
		t.Fatal("expected unique dir completion")
	}
	if out != "internal/" {
		t.Fatalf("complete = %q", out)
	}
}
