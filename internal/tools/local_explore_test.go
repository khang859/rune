package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestListFilesListsFilesAndSkipsNoisyDirs(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "main.go", "package main\n")
	writeTestFile(t, dir, "internal/app.go", "package internal\n")
	writeTestFile(t, dir, ".git/config", "secret\n")
	writeTestFile(t, dir, "node_modules/pkg/index.js", "module\n")

	res, err := (ListFiles{}).Run(context.Background(), json.RawMessage(`{"path":`+quoteJSON(dir)+`}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("result error: %s", res.Output)
	}
	for _, want := range []string{"main.go", "internal/app.go"} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("output missing %q:\n%s", want, res.Output)
		}
	}
	for _, unwanted := range []string{".git/config", "node_modules"} {
		if strings.Contains(res.Output, unwanted) {
			t.Fatalf("output included ignored path %q:\n%s", unwanted, res.Output)
		}
	}
}

func TestListFilesGlobAndMaxResults(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.go", "package a\n")
	writeTestFile(t, dir, "b.go", "package b\n")
	writeTestFile(t, dir, "c.txt", "c\n")

	res, err := (ListFiles{}).Run(context.Background(), json.RawMessage(`{"path":`+quoteJSON(dir)+`,"glob":"*.go","max_results":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("result error: %s", res.Output)
	}
	if strings.Contains(res.Output, "c.txt") {
		t.Fatalf("glob output included txt file:\n%s", res.Output)
	}
	if !strings.Contains(res.Output, "showing first 1 files") {
		t.Fatalf("missing truncation footer:\n%s", res.Output)
	}
}

func TestSearchFilesFindsLiteralMatchesWithLineNumbers(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.txt", "alpha\nbeta target\ngamma\n")
	writeTestFile(t, dir, "nested/b.txt", "target again\n")
	writeTestFile(t, dir, "nested/c.go", "nope\n")

	res, err := (SearchFiles{}).Run(context.Background(), json.RawMessage(`{"path":`+quoteJSON(dir)+`,"query":"target"}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("result error: %s", res.Output)
	}
	for _, want := range []string{"a.txt:2: beta target", "nested/b.txt:1: target again"} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("output missing %q:\n%s", want, res.Output)
		}
	}
}

func TestSearchFilesContextLinesAndGlob(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.go", "before\nneedle\nafter\n")
	writeTestFile(t, dir, "a.txt", "needle in txt\n")

	res, err := (SearchFiles{}).Run(context.Background(), json.RawMessage(`{"path":`+quoteJSON(dir)+`,"query":"needle","glob":"*.go","context_lines":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("result error: %s", res.Output)
	}
	for _, want := range []string{"a.go:1:-before", "a.go:2: needle", "a.go:3:-after"} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("output missing %q:\n%s", want, res.Output)
		}
	}
	if strings.Contains(res.Output, "a.txt") {
		t.Fatalf("glob output included txt file:\n%s", res.Output)
	}
}

func TestSearchFilesMaxResults(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, dir, "a.txt", "x\nx\nx\n")

	res, err := (SearchFiles{}).Run(context.Background(), json.RawMessage(`{"path":`+quoteJSON(dir)+`,"query":"x","max_results":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("result error: %s", res.Output)
	}
	if strings.Count(res.Output, "a.txt:") != 2 || !strings.Contains(res.Output, "showing first 2 matching lines") {
		t.Fatalf("unexpected max result output:\n%s", res.Output)
	}
}

func TestPlanModeAllowsLocalExploreToolsButDeniesBash(t *testing.T) {
	r := NewRegistry()
	RegisterBuiltins(r, BuiltinOptions{})
	r.SetPermissionMode(PermissionModePlan)

	var names []string
	for _, spec := range r.Specs() {
		names = append(names, spec.Name)
	}
	for _, want := range []string{"read", "list_files", "search_files"} {
		if !containsString(names, want) {
			t.Fatalf("plan specs missing %q in %v", want, names)
		}
	}
	if containsString(names, "bash") {
		t.Fatalf("plan specs exposed bash: %v", names)
	}

	res, err := r.Run(context.Background(), ai.ToolCall{Name: "bash", Args: json.RawMessage(`{"command":"ls"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Output, "disabled in Plan Mode") {
		t.Fatalf("bash should be denied in plan mode: %#v", res)
	}
}

func writeTestFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func containsString(xs []string, x string) bool {
	for _, s := range xs {
		if s == x {
			return true
		}
	}
	return false
}
