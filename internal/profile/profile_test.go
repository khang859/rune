package profile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseMarkdown(t *testing.T) {
	content := "---\nname: researcher\nmodel: gpt-4\nskills: [web-research, summarize]\n---\nYou are a meticulous researcher.\n"
	p, err := ParseMarkdown("researcher.md", "researcher", content)
	if err != nil {
		t.Fatalf("ParseMarkdown: %v", err)
	}
	if p.Name != "researcher" {
		t.Errorf("name = %q, want researcher", p.Name)
	}
	if p.Model != "gpt-4" {
		t.Errorf("model = %q, want gpt-4", p.Model)
	}
	if want := []string{"web-research", "summarize"}; !reflect.DeepEqual(p.Skills, want) {
		t.Errorf("skills = %v, want %v", p.Skills, want)
	}
	if p.Instructions != "You are a meticulous researcher." {
		t.Errorf("instructions = %q", p.Instructions)
	}
}

func TestParseMarkdownStripsQuotedScalar(t *testing.T) {
	p, err := ParseMarkdown("r.md", "r", "---\nname: r\nmodel: \"gpt-4o\"\n---\nbody")
	if err != nil {
		t.Fatalf("ParseMarkdown: %v", err)
	}
	if p.Model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o (quotes stripped)", p.Model)
	}
}

func TestParseMarkdownNameFromFilename(t *testing.T) {
	p, err := ParseMarkdown("/x/writer.md", "writer", "no frontmatter body")
	if err != nil {
		t.Fatalf("ParseMarkdown: %v", err)
	}
	if p.Name != "writer" {
		t.Errorf("name = %q, want writer", p.Name)
	}
	if p.Instructions != "no frontmatter body" {
		t.Errorf("instructions = %q", p.Instructions)
	}
	if p.Skills != nil {
		t.Errorf("skills = %v, want nil", p.Skills)
	}
}

func TestProjectOverridesGlobal(t *testing.T) {
	global := t.TempDir()
	project := t.TempDir()
	write(t, global, "reviewer.md", "---\nname: reviewer\nmodel: global-model\n---\nglobal")
	write(t, project, "reviewer.md", "---\nname: reviewer\nmodel: project-model\n---\nproject")

	profiles, err := (&Loader{Roots: []string{global, project}}).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	got := profiles["reviewer"]
	if got.Model != "project-model" || got.Instructions != "project" {
		t.Errorf("project did not override global: %+v", got)
	}
}

func TestResolveUnknown(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "researcher.md", "---\nname: researcher\n---\nbody")
	l := &Loader{Roots: []string{dir}}

	if _, err := l.Resolve("researcher"); err != nil {
		t.Fatalf("Resolve known: %v", err)
	}
	if _, err := l.Resolve("nope"); err == nil {
		t.Fatal("Resolve unknown: expected error, got nil")
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}
