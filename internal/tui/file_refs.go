package tui

import (
	"strconv"
	"strings"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/attachments"
)

func resolveFileReferences(text, cwd, provider, model string) attachments.ResolvedUserInput {
	return attachments.ResolveUserInput(text, attachments.Options{CWD: cwd, Provider: provider, Model: model})
}

// expandFileReferences preserves the historical text-only helper for tests and
// callers that only need inline <file> expansion.
func expandFileReferences(text, cwd string) string {
	return attachments.ResolveUserInput(text, attachments.Options{CWD: cwd}).Text
}

func attachmentSummary(files []attachments.AttachedFile) string {
	if len(files) == 0 {
		return ""
	}
	counts := map[string]int{}
	for _, f := range files {
		switch {
		case strings.HasPrefix(f.MimeType, "image/"):
			counts["image"]++
		case f.MimeType == "application/pdf" && f.Mode == "native":
			counts["pdf-native"]++
		case f.MimeType == "application/pdf" && f.Mode == "extracted-text":
			counts["pdf-text"]++
		case f.Mode == "inlined-text":
			counts["text"]++
		}
	}
	var parts []string
	if counts["image"] > 0 {
		parts = append(parts, plural(counts["image"], "image")+" attached")
	}
	if counts["pdf-native"] > 0 {
		parts = append(parts, plural(counts["pdf-native"], "PDF")+" attached using native provider PDF input")
	}
	if counts["pdf-text"] > 0 {
		parts = append(parts, plural(counts["pdf-text"], "PDF")+" attached as extracted text")
	}
	if counts["text"] > 0 {
		parts = append(parts, plural(counts["text"], "text file")+" inlined")
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, "; ") + ")"
}

func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	if noun == "PDF" {
		return strconv.Itoa(n) + " PDFs"
	}
	return strconv.Itoa(n) + " " + noun + "s"
}

func imageBlocksToContent(images []ai.ImageBlock) []ai.ContentBlock {
	out := make([]ai.ContentBlock, 0, len(images))
	for _, img := range images {
		out = append(out, img)
	}
	return out
}

func countImages(blocks []ai.ContentBlock) int { return len(imagesFromBlocks(blocks)) }

func imagesFromBlocks(blocks []ai.ContentBlock) []ai.ImageBlock {
	var images []ai.ImageBlock
	for _, b := range blocks {
		if img, ok := b.(ai.ImageBlock); ok {
			images = append(images, img)
		}
	}
	return images
}
