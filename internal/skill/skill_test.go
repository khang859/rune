package skill

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoader_FindsAcrossRoots(t *testing.T) {
	home := t.TempDir()
	proj := t.TempDir()
	_ = os.WriteFile(filepath.Join(home, "alpha.md"), []byte("ALPHA-HOME"), 0o644)
	_ = os.WriteFile(filepath.Join(home, "shared.md"), []byte("SHARED-HOME"), 0o644)
	_ = os.WriteFile(filepath.Join(proj, "shared.md"), []byte("SHARED-PROJ"), 0o644)
	_ = os.WriteFile(filepath.Join(proj, "beta.md"), []byte("BETA-PROJ"), 0o644)

	sks, err := (&Loader{Roots: []string{home, proj}}).Load()
	if err != nil {
		t.Fatal(err)
	}

	by := map[string]string{}
	for _, s := range sks {
		by[s.Slug] = s.Body
	}
	if by["shared"] != "SHARED-PROJ" {
		t.Fatalf("override failed: shared=%q", by["shared"])
	}
	if by["alpha"] != "ALPHA-HOME" {
		t.Fatalf("home alpha = %q", by["alpha"])
	}
	if by["beta"] != "BETA-PROJ" {
		t.Fatalf("proj beta = %q", by["beta"])
	}
	if len(by) != 3 {
		t.Fatalf("expected 3 unique slugs, got %d: %v", len(by), by)
	}
}

func TestLoader_IgnoresNonMarkdown(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "x.txt"), []byte("nope"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "y.md"), []byte("yes"), 0o644)
	sks, _ := (&Loader{Roots: []string{dir}}).Load()
	if len(sks) != 1 {
		t.Fatalf("len = %d", len(sks))
	}
}

func TestLoader_MissingRootIsNoOp(t *testing.T) {
	sks, err := (&Loader{Roots: []string{"/does/not/exist"}}).Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(sks) != 0 {
		t.Fatalf("len = %d", len(sks))
	}
}
