package editor

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractImagePathCandidates_QuotedPathInProse(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Screenshot 2026-04-30 at 2.16.42 PM.png")
	text := "can you see this image? '" + p + "'"

	got := extractImagePathCandidates(text, dir)
	if len(got) != 1 || got[0] != filepath.Clean(p) {
		t.Fatalf("candidates = %#v, want %q", got, filepath.Clean(p))
	}
}

func TestExtractImagePathCandidates_RelativePath(t *testing.T) {
	dir := t.TempDir()
	text := "please inspect ./screenshots/ui.webp"

	got := extractImagePathCandidates(text, dir)
	want := filepath.Join(dir, "screenshots", "ui.webp")
	if len(got) != 1 || got[0] != filepath.Clean(want) {
		t.Fatalf("candidates = %#v, want %q", got, filepath.Clean(want))
	}
}

func TestExtractImagePathCandidates_DedupesSamePath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.png")
	text := p + " and again \"" + p + "\""

	got := extractImagePathCandidates(text, dir)
	if len(got) != 1 || got[0] != filepath.Clean(p) {
		t.Fatalf("candidates = %#v, want one %q", got, filepath.Clean(p))
	}
}

func TestEditor_AttachesQuotedImagePathInProseAndPreservesText(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "Screenshot 2026-04-30 at 2.16.42 PM.png")
	if err := os.WriteFile(p, tinyPNG, 0o644); err != nil {
		t.Fatal(err)
	}

	e := New(dir, nil)
	text := "can you see this image? '" + p + "'"
	for _, r := range text {
		e.ta.SetValue(e.ta.Value() + string(r))
	}
	res := e.submit()

	if !res.Send {
		t.Fatalf("expected send result: %#v", res)
	}
	if res.Text != text {
		t.Fatalf("text changed: got %q want %q", res.Text, text)
	}
	if len(res.Images) != 1 {
		t.Fatalf("images = %d, want 1", len(res.Images))
	}
	if res.Images[0].MimeType != "image/png" {
		t.Fatalf("mime = %q", res.Images[0].MimeType)
	}
}

func TestEditor_DoesNotAttachInvalidImageBytes(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "fake.png")
	if err := os.WriteFile(p, []byte("not image bytes"), 0o644); err != nil {
		t.Fatal(err)
	}

	e := New(dir, nil)
	e.ta.SetValue("look at " + p)
	res := e.submit()

	if !res.Send {
		t.Fatalf("expected send result: %#v", res)
	}
	if len(res.Images) != 0 {
		t.Fatalf("images = %d, want 0", len(res.Images))
	}
}
