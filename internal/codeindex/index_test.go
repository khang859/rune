package codeindex

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildMultiLanguageIndex(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "main.go"), `package main

import "fmt"

type Server struct{}

func main() { helper() }
func helper() { fmt.Println("hi") }
`)
	writeTestFile(t, filepath.Join(dir, "app.ts"), `import { thing } from "./thing";

class App {
  start() { run(); }
}

function run() { thing(); }
`)
	writeTestFile(t, filepath.Join(dir, "mod.py"), `import os

class Worker:
    def start(self):
        run()

def run():
    print(os.getcwd())
`)

	idx, err := NewBuilder().Build(context.Background(), BuildOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	assertSymbol(t, idx, "main", SymbolFunction)
	assertSymbol(t, idx, "helper", SymbolFunction)
	assertSymbol(t, idx, "App", SymbolClass)
	assertSymbol(t, idx, "run", SymbolFunction)
	assertSymbol(t, idx, "Worker", SymbolClass)

	var hasResolvedCall bool
	for _, edge := range idx.Graph.Edges {
		from := idx.Symbols[edge.From]
		to := idx.Symbols[edge.To]
		if edge.Relation == RelCalls && from != nil && to != nil && from.Name == "main" && to.Name == "helper" {
			hasResolvedCall = true
		}
	}
	if !hasResolvedCall {
		t.Fatalf("expected resolved call edge main -> helper; edges: %#v", idx.Graph.Edges)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertSymbol(t *testing.T, idx *Index, name string, kind SymbolKind) {
	t.Helper()
	for _, sym := range idx.Symbols {
		if sym.Name == name && sym.Kind == kind {
			return
		}
	}
	t.Fatalf("missing %s symbol %q in %#v", kind, name, idx.Symbols)
}
