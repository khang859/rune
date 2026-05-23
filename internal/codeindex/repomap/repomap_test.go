package repomap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/codeindex"
)

func TestBuildSurfacesMentionedSymbol(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib.go"), []byte(`package lib
func ParseConfig() error { return nil }
func unused() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package lib
func Run() error { return ParseConfig() }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := codeindex.NewBuilder().Build(context.Background(), codeindex.BuildOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	focus := Focus{
		InFocusFiles:    []string{filepath.Join(dir, "main.go")},
		MentionedIdents: map[string]bool{"ParseConfig": true},
	}
	out, err := Build(context.Background(), idx, focus, Options{MaxTokens: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ParseConfig") {
		t.Errorf("expected ParseConfig in output, got:\n%s", out)
	}
	if !strings.Contains(out, "lib.go") {
		t.Errorf("expected lib.go file header, got:\n%s", out)
	}
}

func TestBuildReturnsEmptyForNilIndex(t *testing.T) {
	out, err := Build(context.Background(), nil, Focus{}, Options{MaxTokens: 1000})
	if err != nil {
		t.Errorf("nil index should not error, got %v", err)
	}
	if out != "" {
		t.Errorf("nil index should return empty string, got %q", out)
	}
}

func TestBuildRespectsZeroBudget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\nfunc F() {}\n"), 0o644)
	idx, _ := codeindex.NewBuilder().Build(context.Background(), codeindex.BuildOptions{Root: dir})
	out, _ := Build(context.Background(), idx, Focus{}, Options{MaxTokens: 0})
	if out != "" {
		t.Errorf("zero budget should return empty string, got %q", out)
	}
}
