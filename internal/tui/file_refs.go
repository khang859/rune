package tui

import (
	"bytes"
	"html"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type fileReferenceSpan struct {
	start int
	end   int
	path  string
}

// expandFileReferences replaces @path references in user-submitted text with
// inline <file> blocks for readable UTF-8 text files. References that cannot be
// resolved, are not regular files, or point at binary/document/image files are
// left unchanged.
func expandFileReferences(text, cwd string) string {
	refs := findFileReferences(text)
	if len(refs) == 0 {
		return text
	}

	var b strings.Builder
	last := 0
	changed := false
	for _, ref := range refs {
		if ref.start < last {
			continue
		}
		replacement, ok := inlineFileReference(ref.path, cwd)
		if !ok {
			continue
		}
		b.WriteString(text[last:ref.start])
		b.WriteString(replacement)
		last = ref.end
		changed = true
	}
	if !changed {
		return text
	}
	b.WriteString(text[last:])
	return b.String()
}

func findFileReferences(text string) []fileReferenceSpan {
	var refs []fileReferenceSpan
	for i := 0; i < len(text); i++ {
		if text[i] != '@' {
			continue
		}
		if i > 0 && !isFileReferenceBoundary(text[i-1]) {
			continue
		}
		if i+1 >= len(text) {
			continue
		}

		next := text[i+1]
		if next == '\'' || next == '"' || next == '`' {
			quote := next
			contentStart := i + 2
			for j := contentStart; j < len(text); j++ {
				if text[j] == '\\' {
					j++
					continue
				}
				if text[j] == quote {
					path := strings.TrimSpace(text[contentStart:j])
					if path != "" {
						refs = append(refs, fileReferenceSpan{start: i, end: j + 1, path: path})
					}
					i = j
					break
				}
			}
			continue
		}

		if isFileReferenceSeparator(next) {
			continue
		}
		j := i + 1
		for j < len(text) && !isFileReferenceSeparator(text[j]) {
			j++
		}
		path := strings.TrimSpace(text[i+1 : j])
		path = trimReferencePathPunctuation(path)
		if path != "" {
			refs = append(refs, fileReferenceSpan{start: i, end: j, path: path})
		}
		i = j - 1
	}
	return refs
}

func inlineFileReference(rawPath, cwd string) (string, bool) {
	path, ok := resolveFileReferencePath(rawPath, cwd)
	if !ok {
		return "", false
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || !info.Mode().IsRegular() {
		return "", false
	}
	data, err := os.ReadFile(path)
	if err != nil || !isInlineTextFile(path, data) {
		return "", false
	}
	return "<file name=\"" + html.EscapeString(path) + "\">\n" + string(data) + "\n</file>", true
}

func resolveFileReferencePath(rawPath, cwd string) (string, bool) {
	rawPath = strings.TrimSpace(rawPath)
	rawPath = trimReferencePathPunctuation(rawPath)
	if rawPath == "" {
		return "", false
	}
	if strings.HasPrefix(rawPath, "~/") || strings.HasPrefix(rawPath, "~"+string(filepath.Separator)) || strings.HasPrefix(rawPath, "~\\") {
		home, err := os.UserHomeDir()
		if err != nil || home == "" {
			return "", false
		}
		rawPath = filepath.Join(home, rawPath[2:])
	}
	if !filepath.IsAbs(rawPath) && !isWindowsAbsPath(rawPath) {
		if cwd == "" {
			if wd, err := os.Getwd(); err == nil {
				cwd = wd
			}
		}
		if cwd != "" {
			rawPath = filepath.Join(cwd, rawPath)
		}
	}
	return filepath.Clean(rawPath), true
}

func isInlineTextFile(path string, data []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if isImageExt(ext) || isKnownBinaryDocumentExt(ext) {
		return false
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return utf8.Valid(data)
}

func isImageExt(ext string) bool {
	switch ext {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".tiff", ".tif", ".ico", ".heic", ".heif":
		return true
	default:
		return false
	}
}

func isKnownBinaryDocumentExt(ext string) bool {
	switch ext {
	case ".pdf", ".doc", ".docx", ".ppt", ".pptx", ".xls", ".xlsx", ".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar", ".dmg", ".iso", ".exe", ".dll", ".so", ".dylib", ".bin", ".wasm", ".class", ".jar", ".pyc":
		return true
	default:
		return false
	}
}

func isFileReferenceBoundary(c byte) bool {
	return isFileReferenceSeparator(c) || c == '(' || c == '[' || c == '{' || c == '<'
}

func isFileReferenceSeparator(c byte) bool {
	switch c {
	case ' ', '\n', '\r', '\t', '\'', '"', '`':
		return true
	default:
		return false
	}
}

func trimReferencePathPunctuation(s string) string {
	s = strings.TrimLeft(s, "([{<")
	s = strings.TrimRight(s, "\"'`,;!?)>]}")
	// Strip sentence-ending dots, but keep relative path components like ".." intact.
	for strings.HasSuffix(s, ".") && s != "." && s != ".." && !strings.HasSuffix(s, string(filepath.Separator)+"..") {
		s = strings.TrimSuffix(s, ".")
	}
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
