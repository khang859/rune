package codeindex

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
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

func TestCacheBuildSharesInflightBuilds(t *testing.T) {
	dir := t.TempDir()
	cache := &Cache{}
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	cache.build = func(ctx context.Context, opts BuildOptions) (*Index, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return New(dir), nil
	}

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := cache.Build(context.Background(), BuildOptions{Root: dir})
			errs <- err
		}()
	}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for first build to start")
	}
	for calls.Load() != 1 {
		t.Fatalf("concurrent Build started %d builds, want 1", calls.Load())
	}
	close(release)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("build calls = %d, want 1", got)
	}
}

func TestBuildRespectsGitignoreAndRuneignore(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, ".gitignore"), "ignored.go\ngenerated/\n")
	writeTestFile(t, filepath.Join(dir, ".runeignore"), "local_only.go\n")
	writeTestFile(t, filepath.Join(dir, "kept.go"), `package main

func Kept() {}
`)
	writeTestFile(t, filepath.Join(dir, "ignored.go"), `package main

func IgnoredFile() {}
`)
	writeTestFile(t, filepath.Join(dir, "generated", "gen.go"), `package main

func Generated() {}
`)
	writeTestFile(t, filepath.Join(dir, "local_only.go"), `package main

func RuneIgnored() {}
`)

	idx, err := NewBuilder().Build(context.Background(), BuildOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	assertSymbol(t, idx, "Kept", SymbolFunction)
	assertNoSymbol(t, idx, "IgnoredFile")
	assertNoSymbol(t, idx, "Generated")
	assertNoSymbol(t, idx, "RuneIgnored")
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

func assertNoSymbol(t *testing.T, idx *Index, name string) {
	t.Helper()
	for _, sym := range idx.Symbols {
		if sym.Name == name {
			t.Fatalf("unexpected symbol %q in ignored file: %#v", name, sym)
		}
	}
}
