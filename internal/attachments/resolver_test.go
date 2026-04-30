package attachments

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/providers"
)

var tinyPNG = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00}

func TestResolveUserInput_AttachesMultipleImagesAndPDFs(t *testing.T) {
	dir := t.TempDir()
	png := filepath.Join(dir, "a.png")
	pdf1 := filepath.Join(dir, "one.pdf")
	pdf2 := filepath.Join(dir, "two.pdf")
	mustWrite(t, png, tinyPNG)
	mustWrite(t, pdf1, simplePDF("Hello one"))
	mustWrite(t, pdf2, simplePDF("Hello two"))

	res := ResolveUserInput("compare a.png one.pdf ./two.pdf", Options{CWD: dir, Provider: providers.Codex, Model: "gpt-5"})
	if res.Text != "compare a.png one.pdf ./two.pdf" {
		t.Fatalf("text changed: %q", res.Text)
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("warnings = %#v", res.Warnings)
	}
	if len(res.Attachments) != 3 {
		t.Fatalf("attachments = %d, want 3", len(res.Attachments))
	}
	if _, ok := res.Attachments[0].(ai.ImageBlock); !ok {
		t.Fatalf("first attachment = %#v", res.Attachments[0])
	}
	if _, ok := res.Attachments[1].(ai.DocumentBlock); !ok {
		t.Fatalf("second attachment = %#v", res.Attachments[1])
	}
	if _, ok := res.Attachments[2].(ai.DocumentBlock); !ok {
		t.Fatalf("third attachment = %#v", res.Attachments[2])
	}
}

func TestResolveUserInput_PDFTextFallbackForUnsupportedProvider(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "paper.pdf")
	mustWrite(t, pdf, simplePDF("Extract me"))

	res := ResolveUserInput("summarize paper.pdf", Options{CWD: dir, Provider: providers.Groq, Model: "llama-3.3-70b-versatile"})
	if len(res.Attachments) != 1 {
		t.Fatalf("attachments = %d", len(res.Attachments))
	}
	tb, ok := res.Attachments[0].(ai.TextBlock)
	if !ok {
		t.Fatalf("attachment = %#v", res.Attachments[0])
	}
	if !strings.Contains(tb.Text, `<document name="`) || !strings.Contains(tb.Text, "Extract me") || !strings.Contains(tb.Text, `source="extracted-text"`) {
		t.Fatalf("unexpected fallback text:\n%s", tb.Text)
	}
}

func TestResolveUserInput_InlinesTextMentions(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "notes with spaces.md")
	mustWrite(t, p, []byte("# Notes"))

	res := ResolveUserInput("read @\"notes with spaces.md\"", Options{CWD: dir})
	if !strings.Contains(res.Text, "<file name=\"") || !strings.Contains(res.Text, "# Notes") {
		t.Fatalf("text was not inlined:\n%s", res.Text)
	}
	if strings.Contains(res.Text, "@\"notes") {
		t.Fatalf("reference was not replaced:\n%s", res.Text)
	}
}

func TestResolveUserInput_DedupesAndWarnsMissing(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "a.pdf")
	mustWrite(t, pdf, simplePDF("Hi"))

	res := ResolveUserInput("compare a.pdf with @a.pdf and missing.pdf", Options{CWD: dir, Provider: providers.Codex, Model: "gpt-5"})
	if len(res.Attachments) != 1 {
		t.Fatalf("attachments = %d, want 1", len(res.Attachments))
	}
	if len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0], "missing.pdf") {
		t.Fatalf("warnings = %#v", res.Warnings)
	}
}

func TestExtractPDFText_Truncates(t *testing.T) {
	text, truncated, err := ExtractPDFText(simplePDF("abcdef"), 3)
	if err != nil {
		t.Fatal(err)
	}
	if text != "abc" || !truncated {
		t.Fatalf("text=%q truncated=%v", text, truncated)
	}
}

func mustWrite(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func simplePDF(text string) []byte {
	return []byte("%PDF-1.4\n1 0 obj<<>>stream\nBT (" + text + ") Tj ET\nendstream\n%%EOF")
}
