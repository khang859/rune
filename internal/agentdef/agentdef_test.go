package agentdef

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseMarkdownFrontmatter(t *testing.T) {
	def, err := ParseMarkdown("agent.md", "fallback", `---
name: impl-agent
description: Makes focused changes
model: gpt-5.5
timeout_secs: 1200
tools: full
---

Implement narrow tasks.
`)
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}
	if def.Name != "impl-agent" || def.Description != "Makes focused changes" || def.Model != "gpt-5.5" || def.TimeoutSecs != 1200 || def.Tools != ToolsFull {
		t.Fatalf("definition = %+v", def)
	}
	if def.Instructions != "Implement narrow tasks." {
		t.Fatalf("instructions = %q", def.Instructions)
	}
}

func TestParseMarkdownDefaults(t *testing.T) {
	def, err := ParseMarkdown("security-reviewer.md", "security-reviewer", "Review only.")
	if err != nil {
		t.Fatalf("ParseMarkdown error: %v", err)
	}
	if def.Name != "security-reviewer" || def.Tools != ToolsReadOnly || def.TimeoutSecs != 0 {
		t.Fatalf("definition = %+v", def)
	}
}

func TestLoaderProjectOverridesGlobalAndRejectsReserved(t *testing.T) {
	root := t.TempDir()
	global := filepath.Join(root, "global")
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(global, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(global, "reviewer.md"), []byte("global"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "reviewer.md"), []byte("project"), 0o644); err != nil {
		t.Fatal(err)
	}
	defs, err := (&Loader{Roots: []string{global, project}}).Load()
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	if got := defs["reviewer"].Instructions; got != "project" {
		t.Fatalf("project override instructions = %q", got)
	}

	if err := os.WriteFile(filepath.Join(project, "general.md"), []byte("reserved"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = (&Loader{Roots: []string{project}, Reserved: map[string]bool{"general": true}}).Load()
	if err == nil {
		t.Fatal("expected reserved-name error")
	}
}
