package memory

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendLoadAndSystemBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "memory.md")
	when := time.Date(2026, 6, 19, 8, 30, 0, 0, time.UTC)

	if err := Append(path, "prefer table-driven tests", when); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "prefer table-driven tests") {
		t.Fatalf("memory missing entry: %q", got)
	}
	block, err := SystemBlock(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<user_memory>", "explicitly saved by the user", "prefer table-driven tests", "</user_memory>"} {
		if !strings.Contains(block, want) {
			t.Fatalf("SystemBlock missing %q in %q", want, block)
		}
	}
}

func TestLoadMissingIsEmpty(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "missing.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("Load missing = %q, want empty", got)
	}
}
