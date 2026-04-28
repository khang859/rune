package editor

import (
	"encoding/base64"
	"fmt"
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
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	mime := mimeFromExt(filepath.Ext(p))
	if mime == "" {
		return fmt.Errorf("not an image: %s", p)
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
	mime := s[5:semi]
	enc := s[semi+len(";base64,"):]
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return err
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
