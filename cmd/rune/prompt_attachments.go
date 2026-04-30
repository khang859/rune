package main

import (
	"strconv"
	"strings"

	"github.com/khang859/rune/internal/attachments"
)

func promptAttachmentSummary(files []attachments.AttachedFile) string {
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
		parts = append(parts, promptPlural(counts["image"], "image")+" attached")
	}
	if counts["pdf-native"] > 0 {
		parts = append(parts, promptPlural(counts["pdf-native"], "PDF")+" attached using native provider PDF input")
	}
	if counts["pdf-text"] > 0 {
		parts = append(parts, promptPlural(counts["pdf-text"], "PDF")+" attached as extracted text")
	}
	if counts["text"] > 0 {
		parts = append(parts, promptPlural(counts["text"], "text file")+" inlined")
	}
	if len(parts) == 0 {
		return ""
	}
	return "(" + strings.Join(parts, "; ") + ")"
}

func promptPlural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	if noun == "PDF" {
		return strconv.Itoa(n) + " PDFs"
	}
	return strconv.Itoa(n) + " " + noun + "s"
}
