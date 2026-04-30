package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandFileReferences_InlinesMarkdownTextAndJSON(t *testing.T) {
	dir := t.TempDir()
	md := filepath.Join(dir, "README.md")
	js := filepath.Join(dir, "package.json")
	if err := os.WriteFile(md, []byte("# Hello\nworld"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(js, []byte(`{"name":"rune"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	got := expandFileReferences("review @README.md and @package.json please", dir)
	if !strings.Contains(got, "<file name=\""+md+"\">\n# Hello\nworld\n</file>") {
		t.Fatalf("markdown was not inlined:\n%s", got)
	}
	if !strings.Contains(got, "<file name=\""+js+"\">\n{\"name\":\"rune\"}\n</file>") {
		t.Fatalf("json was not inlined:\n%s", got)
	}
	if strings.Contains(got, "@README.md") || strings.Contains(got, "@package.json") {
		t.Fatalf("references should be replaced:\n%s", got)
	}
}

func TestExpandFileReferences_QuotedPathWithSpaces(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "notes with spaces.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := expandFileReferences("read @\"notes with spaces.txt\"", dir)
	if !strings.Contains(got, "<file name=\""+p+"\">\nhello\n</file>") {
		t.Fatalf("quoted path was not inlined:\n%s", got)
	}
}

func TestExpandFileReferences_LeavesMissingBinaryAndImagesUnchanged(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "doc.pdf")
	png := filepath.Join(dir, "image.png")
	bin := filepath.Join(dir, "data.txt")
	if err := os.WriteFile(pdf, []byte("%PDF-1.7"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(png, []byte{0x89, 'P', 'N', 'G'}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bin, []byte{'a', 0, 'b'}, 0o644); err != nil {
		t.Fatal(err)
	}

	input := "see @doc.pdf @image.png @data.txt @missing.md"
	got := expandFileReferences(input, dir)
	if got != input {
		t.Fatalf("binary/image/missing refs should remain unchanged:\ngot  %q\nwant %q", got, input)
	}
}

func TestExpandFileReferences_EscapesFileName(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "a&b.md")
	if err := os.WriteFile(p, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := expandFileReferences("read @a&b.md", dir)
	if !strings.Contains(got, `a&amp;b.md`) {
		t.Fatalf("filename was not escaped:\n%s", got)
	}
}
