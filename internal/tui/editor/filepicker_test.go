package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFilePicker_FiltersOnQuery(t *testing.T) {
	dir := t.TempDir()
	paths := []string{"foo.go", "bar.go", "internal/baz.go", "internal/foo.txt"}
	for _, p := range paths {
		full := filepath.Join(dir, p)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte("x"), 0o644)
	}
	fp := NewFilePicker(dir)
	fp.SetQuery("foo")
	got := fp.Items()
	if len(got) == 0 {
		t.Fatal("expected matches for foo")
	}
	// Both foo.go and internal/foo.txt should match.
	seen := map[string]bool{}
	for _, it := range got {
		seen[it] = true
	}
	if !seen["foo.go"] || !seen["internal/foo.txt"] {
		t.Fatalf("unexpected items: %v", got)
	}
}

func TestFilePicker_HiddenFilesExcludedUnlessDotQuery(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, ".hidden"), []byte("x"), 0o644)
	fp := NewFilePicker(dir)
	fp.SetQuery("hid")
	if items := fp.Items(); len(items) != 0 {
		t.Fatalf("hidden leaked: %v", items)
	}
	fp.SetQuery(".hid")
	if items := fp.Items(); len(items) != 1 {
		t.Fatalf("expected 1 with .hid query, got %v", items)
	}
}

func TestFilePicker_Selection(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a"), nil, 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b"), nil, 0o644)
	fp := NewFilePicker(dir)
	fp.SetQuery("")
	fp.Down()
	if fp.Selected() == "" {
		t.Fatal("expected selection")
	}
}
