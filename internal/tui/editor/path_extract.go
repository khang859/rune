package editor

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/khang859/rune/internal/attachments"
)

type quotedPathSpan struct {
	text       string
	start, end int // byte offsets; end is exclusive
}

// extractImagePathCandidates returns normalized local image path candidates found
// anywhere in text. Quoted paths may contain spaces. Unquoted paths are parsed as
// shell-ish tokens and therefore stop at whitespace.
func extractImagePathCandidates(text, cwd string) []string {
	seen := make(map[string]bool)
	var out []string
	add := func(raw string) {
		p, ok := normalizeImagePathCandidate(raw, cwd)
		if !ok || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}

	quoted := quotedSpans(text)
	for _, q := range quoted {
		add(q.text)
	}

	unquotedText := blankQuotedSpans(text, quoted)
	for _, tok := range unquotedTokens(unquotedText) {
		add(tok)
	}

	return out
}

func normalizeImagePathCandidate(raw, cwd string) (string, bool) {
	raw = strings.TrimSpace(raw)
	raw = trimMatchingQuotes(raw)
	raw = trimPathPunctuation(raw)
	if raw == "" {
		return "", false
	}

	if strings.HasPrefix(raw, "~/") || strings.HasPrefix(raw, "~"+string(filepath.Separator)) {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", false
		}
		raw = filepath.Join(home, raw[2:])
	} else if strings.HasPrefix(raw, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", false
		}
		raw = filepath.Join(home, raw[2:])
	}

	if attachments.ImageMimeFromExt(filepath.Ext(raw)) == "" {
		return "", false
	}

	if !filepath.IsAbs(raw) && !isWindowsAbsPath(raw) {
		if cwd == "" {
			if wd, err := os.Getwd(); err == nil {
				cwd = wd
			}
		}
		if cwd != "" {
			raw = filepath.Join(cwd, raw)
		}
	}

	return filepath.Clean(raw), true
}

func quotedSpans(s string) []quotedPathSpan {
	var spans []quotedPathSpan
	for i := 0; i < len(s); i++ {
		quote := s[i]
		if quote != '\'' && quote != '"' && quote != '`' {
			continue
		}
		contentStart := i + 1
		for j := contentStart; j < len(s); j++ {
			if s[j] == '\\' {
				j++
				continue
			}
			if s[j] == quote {
				spans = append(spans, quotedPathSpan{text: s[contentStart:j], start: i, end: j + 1})
				i = j
				break
			}
		}
	}
	return spans
}

func blankQuotedSpans(s string, spans []quotedPathSpan) string {
	if len(spans) == 0 {
		return s
	}
	b := []byte(s)
	for _, span := range spans {
		for i := span.start; i < span.end && i < len(b); i++ {
			b[i] = ' '
		}
	}
	return string(b)
}

func unquotedTokens(s string) []string {
	var toks []string
	for i := 0; i < len(s); {
		for i < len(s) && isTokenSeparator(s[i]) {
			i++
		}
		start := i
		for i < len(s) && !isTokenSeparator(s[i]) {
			i++
		}
		if start < i {
			toks = append(toks, s[start:i])
		}
	}
	return toks
}

func isTokenSeparator(c byte) bool {
	switch c {
	case ' ', '\n', '\r', '\t', '\'', '"', '`', '(', '[', '{', '<':
		return true
	default:
		return false
	}
}

func trimMatchingQuotes(s string) string {
	if len(s) < 2 {
		return s
	}
	first, last := s[0], s[len(s)-1]
	if (first == '\'' && last == '\'') || (first == '"' && last == '"') || (first == '`' && last == '`') {
		return strings.TrimSpace(s[1 : len(s)-1])
	}
	return s
}

func trimPathPunctuation(s string) string {
	s = strings.TrimLeft(s, "([{<")
	s = strings.TrimRight(s, "\"'`,;:!?)>]}.")
	return strings.TrimSpace(s)
}

func isWindowsDriveStart(s string, i int) bool {
	if i+2 >= len(s) {
		return false
	}
	c := s[i]
	return ((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) && s[i+1] == ':' && (s[i+2] == '\\' || s[i+2] == '/')
}

func isWindowsAbsPath(s string) bool {
	return isWindowsDriveStart(s, 0)
}
