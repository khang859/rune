package repomap

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/khang859/rune/internal/codeindex"
)

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectFileGraphEdges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a
import "fmt"
func Caller() { Callee(); fmt.Println("x") }
func Callee() {}
`)
	writeFile(t, dir, "b.go", `package a
func Other() { Callee() }
`)
	idx, err := codeindex.NewBuilder().Build(context.Background(), codeindex.BuildOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	focus := Focus{}
	nodes, edges := ProjectFileGraph(idx, focus)
	if len(nodes) < 2 {
		t.Fatalf("want >=2 file nodes, got %d", len(nodes))
	}
	var found bool
	for _, e := range edges {
		if filepath.Base(e.From) == "b.go" && filepath.Base(e.To) == "a.go" {
			found = true
			if e.Weight <= 0 {
				t.Errorf("edge b.go->a.go has non-positive weight %f", e.Weight)
			}
		}
	}
	if !found {
		t.Fatalf("expected cross-file edge b.go -> a.go (Callee reference), got edges %+v", edges)
	}
}

func TestProjectFileGraphFocusBoost(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a
func Callee() {}
`)
	writeFile(t, dir, "b.go", `package a
func Other() { Callee() }
`)
	idx, err := codeindex.NewBuilder().Build(context.Background(), codeindex.BuildOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	baseFocus := Focus{}
	_, baseEdges := ProjectFileGraph(idx, baseFocus)

	bPath := filepath.Join(dir, "b.go")
	focused := Focus{InFocusFiles: []string{bPath}}
	_, boostedEdges := ProjectFileGraph(idx, focused)

	baseWeight := edgeWeight(baseEdges, bPath, filepath.Join(dir, "a.go"))
	boostedWeight := edgeWeight(boostedEdges, bPath, filepath.Join(dir, "a.go"))
	if boostedWeight <= baseWeight {
		t.Errorf("focus boost should raise edge weight, base=%f boosted=%f", baseWeight, boostedWeight)
	}
}

func edgeWeight(edges []WeightedEdge, from, to string) float64 {
	for _, e := range edges {
		if e.From == from && e.To == to {
			return e.Weight
		}
	}
	return 0
}
