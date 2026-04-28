package session

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestSave_AndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New("gpt-5")
	s.Name = "demo"
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	s.Append(userMsg("hi"))
	s.Append(asstMsg("hello"))

	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != s.ID {
		t.Fatalf("id mismatch")
	}
	if loaded.Name != "demo" {
		t.Fatalf("name = %q", loaded.Name)
	}
	if got := len(loaded.PathToActive()); got != 2 {
		t.Fatalf("path len = %d", got)
	}
	// Parent pointers must be reconstructed.
	if loaded.Active.Parent == nil || loaded.Active.Parent.Parent == nil {
		t.Fatal("parent pointers not reconstructed")
	}
	if loaded.Active.Parent.Parent != loaded.Root {
		t.Fatal("parent pointers do not chain to root")
	}
}

func TestSave_AndLoad_PreservesCompactedCount(t *testing.T) {
	dir := t.TempDir()
	s := New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	s.Append(userMsg("hi"))
	n := s.Append(asstMsg("compact summary"))
	n.CompactedCount = 7

	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	nodes := loaded.PathToActiveNodes()
	if len(nodes) != 2 {
		t.Fatalf("nodes len = %d", len(nodes))
	}
	if got := nodes[1].CompactedCount; got != 7 {
		t.Fatalf("CompactedCount = %d after round-trip, want 7", got)
	}
}

func TestSave_IsAtomic(t *testing.T) {
	// After Save, the temp file must be gone — only the final file exists.
	dir := t.TempDir()
	s := New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	s.Append(userMsg("hi"))
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
}

// keep ai import alive for test compilation
var _ = ai.RoleUser
