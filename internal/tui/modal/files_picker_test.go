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
	if got.Action != FilesPickerInsert {
		t.Fatalf("action = %q, want %q", got.Action, FilesPickerInsert)
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

func TestFilesPickerPreviewShowsImageMetadata(t *testing.T) {
	dir := t.TempDir()
	mustWriteBytes(t, filepath.Join(dir, "shot.png"), tinyPickerPNG)
	p := NewFilesPicker(dir).(*FilesPicker)
	view := p.renderPreview(48, 10)
	for _, want := range []string{"shot.png", "image: image/png", "1×1"} {
		if !strings.Contains(view, want) {
			t.Fatalf("preview missing %q:\n%s", want, view)
		}
	}
}

func TestFilesPickerSpaceOpensImageWithoutClosing(t *testing.T) {
	dir := t.TempDir()
	mustWriteBytes(t, filepath.Join(dir, "shot.png"), tinyPickerPNG)
	md := NewFilesPicker(dir)
	next, cmd := md.Update(tea.KeyMsg{Type: tea.KeySpace})
	if cmd == nil {
		t.Fatal("space on an image should return an open command")
	}
	if next == nil {
		t.Fatal("space on an image should keep picker open")
	}
	msg := cmd()
	got, ok := msg.(FilesPickerOpenMsg)
	if !ok {
		t.Fatalf("message = %T, want FilesPickerOpenMsg", msg)
	}
	if got.Path != "shot.png" {
		t.Fatalf("path = %q, want shot.png", got.Path)
	}
}

func TestFilesPickerCtrlAAttaches(t *testing.T) {
	dir := t.TempDir()
	mustWriteBytes(t, filepath.Join(dir, "shot.png"), tinyPickerPNG)
	md := NewFilesPicker(dir)
	_, cmd := md.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if cmd == nil {
		t.Fatal("ctrl+a should return a result command")
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
	if got.Path != "shot.png" || got.Action != FilesPickerAttach {
		t.Fatalf("result = %#v", got)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustWriteBytes(t, path, []byte(content))
}

func mustWriteBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

var tinyPickerPNG = []byte{
	0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a,
	0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52,
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
	0x08, 0x06, 0x00, 0x00, 0x00, 0x1f, 0x15, 0xc4,
	0x89, 0x00, 0x00, 0x00, 0x0a, 0x49, 0x44, 0x41,
	0x54, 0x78, 0x9c, 0x63, 0x00, 0x01, 0x00, 0x00,
	0x05, 0x00, 0x01, 0x0d, 0x0a, 0x2d, 0xb4, 0x00,
	0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, 0x44, 0xae,
	0x42, 0x60, 0x82,
}

func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
