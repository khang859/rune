package modal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestFilesPickerFiltersAndSelects(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "README.md"), "# hello\n")
	mustWrite(t, filepath.Join(dir, "internal", "tui", "editor.go"), "package tui\n")
	mustWrite(t, filepath.Join(dir, ".hidden.go"), "package hidden\n")

	p := NewFilesPicker(dir).(*FilesPicker)
	if containsString(p.files, ".hidden.go") {
		t.Fatal("hidden files should be skipped by default")
	}

	var md Modal = p
	for _, r := range "edit" {
		var cmd tea.Cmd
		md, cmd = md.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		if cmd != nil {
			t.Fatal("typing should not close picker")
		}
	}
	p = md.(*FilesPicker)
	if got := p.selected(); got != "internal/tui/editor.go" {
		t.Fatalf("selected = %q, want internal/tui/editor.go; matches=%v", got, p.matches)
	}

	_, cmd := md.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("enter should return a result command")
	}
	msg := cmd()
	res, ok := msg.(ResultMsg)
	if !ok {
		t.Fatalf("message = %T, want ResultMsg", msg)
	}
	got, ok := res.Payload.(FilesPickerResult)
	if !ok {
		t.Fatalf("payload = %T, want FilesPickerResult", res.Payload)
	}
	if got.Path != "internal/tui/editor.go" {
		t.Fatalf("path = %q", got.Path)
	}
}

func TestFilesPickerPreviewShowsText(t *testing.T) {
	dir := t.TempDir()
	mustWrite(t, filepath.Join(dir, "a.txt"), "first\nsecond\n")
	p := NewFilesPicker(dir).(*FilesPicker)
	view := p.renderPreview(40, 6)
	for _, want := range []string{"a.txt", "first", "second"} {
		if !strings.Contains(view, want) {
			t.Fatalf("preview missing %q:\n%s", want, view)
		}
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
