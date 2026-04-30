package editor

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

type Attachments struct {
	items []ai.ImageBlock
}

func NewAttachments() *Attachments { return &Attachments{} }

func (a *Attachments) AddFromPath(p string) error {
	info, err := os.Stat(p)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return fmt.Errorf("not a file: %s", p)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", p)
	}
	if mimeFromExt(filepath.Ext(p)) == "" {
		return fmt.Errorf("not an image: %s", p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	mime := sniffImageMime(b)
	if mime == "" {
		return fmt.Errorf("not a valid image: %s", p)
	}
	a.items = append(a.items, ai.ImageBlock{Data: b, MimeType: mime})
	return nil
}

func (a *Attachments) AddFromDataURI(s string) error {
	if !strings.HasPrefix(s, "data:image/") {
		return fmt.Errorf("not a data: URI")
	}
	semi := strings.Index(s, ";base64,")
	if semi < 0 {
		return fmt.Errorf("only base64 supported")
	}
	declaredMime := s[5:semi]
	enc := s[semi+len(";base64,"):]
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return err
	}
	mime := sniffImageMime(raw)
	if mime == "" {
		return fmt.Errorf("not a valid image")
	}
	if declaredMime != "" && mime != declaredMime {
		return fmt.Errorf("declared MIME %s does not match image bytes %s", declaredMime, mime)
	}
	a.items = append(a.items, ai.ImageBlock{Data: raw, MimeType: mime})
	return nil
}

func (a *Attachments) Pending() int { return len(a.items) }

func (a *Attachments) Drain() []ai.ImageBlock {
	out := a.items
	a.items = nil
	return out
}

func mimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}
	return ""
}

func sniffImageMime(b []byte) string {
	if bytes.HasPrefix(b, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return "image/png"
	}
	if bytes.HasPrefix(b, []byte{0xff, 0xd8, 0xff}) {
		return "image/jpeg"
	}
	if bytes.HasPrefix(b, []byte("GIF87a")) || bytes.HasPrefix(b, []byte("GIF89a")) {
		return "image/gif"
	}
	if len(b) >= 12 && bytes.Equal(b[0:4], []byte("RIFF")) && bytes.Equal(b[8:12], []byte("WEBP")) {
		return "image/webp"
	}
	mime := http.DetectContentType(b)
	if strings.HasPrefix(mime, "image/") {
		switch mime {
		case "image/png", "image/jpeg", "image/gif", "image/webp":
			return mime
		}
	}
	return ""
}
