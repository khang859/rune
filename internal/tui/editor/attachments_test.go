package editor

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

var tinyPNG = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, 0x00}

func TestAttachments_AddFromPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "x.png")
	_ = os.WriteFile(p, tinyPNG, 0o644)

	a := NewAttachments()
	if err := a.AddFromPath(p); err != nil {
		t.Fatal(err)
	}
	items := a.Drain()
	if len(items) != 1 {
		t.Fatalf("len = %d", len(items))
	}
	if items[0].MimeType != "image/png" {
		t.Fatalf("mime = %q", items[0].MimeType)
	}
}

func TestAttachments_AddFromPathRejectsFakeImage(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "not-really.png")
	_ = os.WriteFile(p, []byte("not image bytes"), 0o644)

	a := NewAttachments()
	if err := a.AddFromPath(p); err == nil {
		t.Fatal("expected fake .png to be rejected")
	}
	if got := a.Pending(); got != 0 {
		t.Fatalf("pending = %d, want 0", got)
	}
}

func TestAttachments_AddFromDataURI(t *testing.T) {
	raw := []byte{0xff, 0xd8, 0xff} // jpeg magic
	uri := "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(raw)
	a := NewAttachments()
	if err := a.AddFromDataURI(uri); err != nil {
		t.Fatal(err)
	}
	items := a.Drain()
	if len(items) != 1 || items[0].MimeType != "image/jpeg" {
		t.Fatalf("items = %#v", items)
	}
	if string(items[0].Data) != string(raw) {
		t.Fatal("decoded bytes mismatch")
	}
}

func TestAttachments_DrainEmptiesBuffer(t *testing.T) {
	a := NewAttachments()
	_ = a.AddFromDataURI("data:image/png;base64," + base64.StdEncoding.EncodeToString(tinyPNG))
	_ = a.Drain()
	if got := a.Drain(); len(got) != 0 {
		t.Fatalf("expected empty after drain, got %d", len(got))
	}
}
