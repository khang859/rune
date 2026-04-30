package attachments

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/providers"
)

const (
	DefaultMaxFiles          = 20
	DefaultMaxTotalBytes     = int64(50_000_000)
	DefaultMaxPDFBytes       = int64(25_000_000)
	DefaultMaxExtractedChars = 200_000
)

type Options struct {
	CWD               string
	Provider          string
	Model             string
	MaxFiles          int
	MaxTotalBytes     int64
	MaxPDFBytes       int64
	MaxExtractedChars int
}

type ResolvedUserInput struct {
	Text        string
	Attachments []ai.ContentBlock
	Warnings    []string
	Attached    []AttachedFile
}

type AttachedFile struct {
	Path     string
	MimeType string
	Mode     string // native, extracted-text, inlined-text
}

type fileRef struct {
	start    int
	end      int
	path     string
	explicit bool
}

type replacement struct {
	start int
	end   int
	text  string
}

// ResolveUserInput finds local file references in user text, inlines readable
// text files, and returns image/PDF content blocks plus warnings for anything
// that looked like a reference but could not be used.
func ResolveUserInput(text string, opts Options) ResolvedUserInput {
	opts = opts.withDefaults()
	refs := findReferences(text)
	if len(refs) == 0 {
		return ResolvedUserInput{Text: text}
	}

	var warnings []string
	var attached []AttachedFile
	var blocks []ai.ContentBlock
	seen := map[string]bool{}
	var totalBytes int64
	filesAdded := 0

	var replacements []replacement

	for _, ref := range refs {
		path, ok := resolvePath(ref.path, opts.CWD)
		if !ok {
			if ref.explicit || looksLikeAttachablePath(ref.path) {
				warnings = append(warnings, fmt.Sprintf("could not attach %s: invalid path", ref.path))
			}
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			if ref.explicit || looksLikeAttachablePath(ref.path) {
				warnings = append(warnings, fmt.Sprintf("could not attach %s: file not found", ref.path))
			}
			continue
		}
		if info.IsDir() || !info.Mode().IsRegular() {
			if ref.explicit || looksLikeAttachablePath(ref.path) {
				warnings = append(warnings, fmt.Sprintf("could not attach %s: not a regular file", ref.path))
			}
			continue
		}

		canon := canonicalPath(path)
		if seen[canon] {
			continue
		}

		kind := fileKind(path)
		if kind == "" {
			if !ref.explicit && !pathExistsAsTypedFile(ref.path, opts.CWD) {
				continue
			}
			kind = "text"
		}

		if filesAdded >= opts.MaxFiles {
			warnings = append(warnings, fmt.Sprintf("could not attach %s: attachment count exceeds %d file limit", ref.path, opts.MaxFiles))
			continue
		}
		if totalBytes+info.Size() > opts.MaxTotalBytes {
			warnings = append(warnings, fmt.Sprintf("could not attach %s: total attachment size exceeds %d byte limit", ref.path, opts.MaxTotalBytes))
			continue
		}

		switch kind {
		case "image":
			data, err := os.ReadFile(path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("could not attach %s: %v", ref.path, err))
				continue
			}
			mime := SniffImageMime(data)
			if mime == "" {
				warnings = append(warnings, fmt.Sprintf("could not attach %s: not a valid supported image", ref.path))
				continue
			}
			seen[canon] = true
			filesAdded++
			totalBytes += info.Size()
			blocks = append(blocks, ai.ImageBlock{Data: data, MimeType: mime})
			attached = append(attached, AttachedFile{Path: path, MimeType: mime, Mode: "native"})

		case "pdf":
			if info.Size() > opts.MaxPDFBytes {
				warnings = append(warnings, fmt.Sprintf("could not attach %s: file size exceeds %d byte PDF limit", ref.path, opts.MaxPDFBytes))
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("could not attach %s: %v", ref.path, err))
				continue
			}
			if !bytes.HasPrefix(data, []byte("%PDF-")) {
				warnings = append(warnings, fmt.Sprintf("could not attach %s: not a valid PDF", ref.path))
				continue
			}
			seen[canon] = true
			filesAdded++
			totalBytes += info.Size()
			if providers.PDFInputSupport(opts.Provider, opts.Model) == providers.DocumentSupported {
				blocks = append(blocks, ai.DocumentBlock{Data: data, MimeType: "application/pdf", Name: filepath.Base(path), Path: path})
				attached = append(attached, AttachedFile{Path: path, MimeType: "application/pdf", Mode: "native"})
				continue
			}
			extracted, truncated, err := ExtractPDFText(data, opts.MaxExtractedChars)
			if err != nil || strings.TrimSpace(extracted) == "" {
				if err == nil {
					err = fmt.Errorf("no extractable text found")
				}
				warnings = append(warnings, fmt.Sprintf("could not extract text from %s: %v", ref.path, err))
				continue
			}
			blocks = append(blocks, ai.TextBlock{Text: documentTextBlock(path, "application/pdf", extracted, truncated)})
			attached = append(attached, AttachedFile{Path: path, MimeType: "application/pdf", Mode: "extracted-text"})

		case "text":
			data, err := os.ReadFile(path)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("could not attach %s: %v", ref.path, err))
				continue
			}
			if !IsInlineTextFile(path, data) {
				if ref.explicit || looksLikeAttachablePath(ref.path) {
					warnings = append(warnings, fmt.Sprintf("could not attach %s: unsupported file type", ref.path))
				}
				continue
			}
			seen[canon] = true
			filesAdded++
			totalBytes += info.Size()
			replacements = append(replacements, replacement{start: ref.start, end: ref.end, text: inlineTextBlock(path, data)})
			attached = append(attached, AttachedFile{Path: path, MimeType: "text/plain", Mode: "inlined-text"})
		}
	}

	return ResolvedUserInput{
		Text:        applyReplacements(text, replacements),
		Attachments: blocks,
		Warnings:    warnings,
		Attached:    attached,
	}
}

func (o Options) withDefaults() Options {
	if o.MaxFiles <= 0 {
		o.MaxFiles = DefaultMaxFiles
	}
	if o.MaxTotalBytes <= 0 {
		o.MaxTotalBytes = DefaultMaxTotalBytes
	}
	if o.MaxPDFBytes <= 0 {
		o.MaxPDFBytes = DefaultMaxPDFBytes
	}
	if o.MaxExtractedChars <= 0 {
		o.MaxExtractedChars = DefaultMaxExtractedChars
	}
	return o
}

func findReferences(text string) []fileRef {
	explicit := findAtReferences(text)
	refs := append([]fileRef(nil), explicit...)
	blanked := blankSpans(text, explicit)

	quoted := quotedSpans(blanked)
	for _, q := range quoted {
		if candidateRef(q.text, false) {
			refs = append(refs, fileRef{start: q.start, end: q.end, path: q.text})
		}
	}
	blanked = blankQuotedSpans(blanked, quoted)
	for _, tok := range unquotedTokenSpans(blanked) {
		p := trimPathPunctuation(tok.text)
		if candidateRef(p, false) {
			refs = append(refs, fileRef{start: tok.start, end: tok.end, path: p})
		}
	}
	sort.SliceStable(refs, func(i, j int) bool { return refs[i].start < refs[j].start })
	return refs
}

func findAtReferences(text string) []fileRef {
	var refs []fileRef
	for i := 0; i < len(text); i++ {
		if text[i] != '@' {
			continue
		}
		if i > 0 && !isReferenceBoundary(text[i-1]) {
			continue
		}
		if i+1 >= len(text) {
			continue
		}
		next := text[i+1]
		if next == '\'' || next == '"' || next == '`' {
			quote := next
			start := i + 2
			for j := start; j < len(text); j++ {
				if text[j] == '\\' {
					j++
					continue
				}
				if text[j] == quote {
					p := strings.TrimSpace(text[start:j])
					if p != "" {
						refs = append(refs, fileRef{start: i, end: j + 1, path: p, explicit: true})
					}
					i = j
					break
				}
			}
			continue
		}
		if isReferenceSeparator(next) {
			continue
		}
		j := i + 1
		for j < len(text) && !isReferenceSeparator(text[j]) {
			j++
		}
		p := trimPathPunctuation(text[i+1 : j])
		if p != "" {
			refs = append(refs, fileRef{start: i, end: j, path: p, explicit: true})
		}
		i = j - 1
	}
	return refs
}

type spanText struct {
	text       string
	start, end int
}

func quotedSpans(s string) []spanText {
	var spans []spanText
	for i := 0; i < len(s); i++ {
		quote := s[i]
		if quote != '\'' && quote != '"' && quote != '`' {
			continue
		}
		start := i + 1
		for j := start; j < len(s); j++ {
			if s[j] == '\\' {
				j++
				continue
			}
			if s[j] == quote {
				spans = append(spans, spanText{text: s[start:j], start: i, end: j + 1})
				i = j
				break
			}
		}
	}
	return spans
}

func blankSpans(s string, refs []fileRef) string {
	if len(refs) == 0 {
		return s
	}
	b := []byte(s)
	for _, r := range refs {
		for i := r.start; i < r.end && i < len(b); i++ {
			b[i] = ' '
		}
	}
	return string(b)
}

func blankQuotedSpans(s string, spans []spanText) string {
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

func unquotedTokenSpans(s string) []spanText {
	var toks []spanText
	for i := 0; i < len(s); {
		for i < len(s) && isTokenSeparator(s[i]) {
			i++
		}
		start := i
		for i < len(s) && !isTokenSeparator(s[i]) {
			i++
		}
		if start < i {
			toks = append(toks, spanText{text: s[start:i], start: start, end: i})
		}
	}
	return toks
}

func candidateRef(raw string, explicit bool) bool {
	raw = strings.TrimSpace(trimPathPunctuation(raw))
	if raw == "" {
		return false
	}
	if explicit {
		return true
	}
	if looksLikeAttachablePath(raw) {
		return true
	}
	return strings.HasPrefix(raw, "./") || strings.HasPrefix(raw, "../") || strings.HasPrefix(raw, "~/") || filepath.IsAbs(raw) || isWindowsAbsPath(raw)
}

func looksLikeAttachablePath(raw string) bool {
	ext := strings.ToLower(filepath.Ext(trimPathPunctuation(raw)))
	if ext == "" {
		return false
	}
	return ImageMimeFromExt(ext) != "" || ext == ".pdf" || isLikelyTextExt(ext)
}

func fileKind(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch {
	case ImageMimeFromExt(ext) != "":
		return "image"
	case ext == ".pdf":
		return "pdf"
	case isLikelyTextExt(ext):
		return "text"
	default:
		return ""
	}
}

func isLikelyTextExt(ext string) bool {
	switch ext {
	case ".txt", ".md", ".markdown", ".json", ".yaml", ".yml", ".toml", ".xml", ".html", ".css", ".js", ".jsx", ".ts", ".tsx", ".go", ".py", ".rb", ".rs", ".java", ".c", ".h", ".cpp", ".hpp", ".cs", ".sh", ".bash", ".zsh", ".fish", ".sql", ".csv", ".log", ".ini", ".cfg", ".conf", ".env", ".gitignore", ".dockerignore":
		return true
	default:
		return false
	}
}

func resolvePath(rawPath, cwd string) (string, bool) {
	rawPath = strings.TrimSpace(rawPath)
	rawPath = trimPathPunctuation(rawPath)
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

func pathExistsAsTypedFile(raw, cwd string) bool {
	p, ok := resolvePath(raw, cwd)
	if !ok {
		return false
	}
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular()
}

func canonicalPath(path string) string {
	if p, err := filepath.EvalSymlinks(path); err == nil {
		path = p
	}
	if p, err := filepath.Abs(path); err == nil {
		path = p
	}
	return filepath.Clean(path)
}

func applyReplacements(text string, reps []replacement) string {
	if len(reps) == 0 {
		return text
	}
	sort.SliceStable(reps, func(i, j int) bool { return reps[i].start < reps[j].start })
	var b strings.Builder
	last := 0
	for _, r := range reps {
		if r.start < last || r.start > len(text) || r.end > len(text) {
			continue
		}
		b.WriteString(text[last:r.start])
		b.WriteString(r.text)
		last = r.end
	}
	b.WriteString(text[last:])
	return b.String()
}

func inlineTextBlock(path string, data []byte) string {
	return "<file name=\"" + html.EscapeString(path) + "\">\n" + string(data) + "\n</file>"
}

func documentTextBlock(path, mime, text string, truncated bool) string {
	trunc := ""
	if truncated {
		trunc = " truncated=\"true\""
	}
	return "<document name=\"" + html.EscapeString(path) + "\" type=\"" + html.EscapeString(mime) + "\" source=\"extracted-text\"" + trunc + ">\n" + text + "\n</document>"
}

func IsInlineTextFile(path string, data []byte) bool {
	ext := strings.ToLower(filepath.Ext(path))
	if ImageMimeFromExt(ext) != "" || ext == ".pdf" || isKnownBinaryDocumentExt(ext) {
		return false
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return false
	}
	return utf8.Valid(data)
}

func isKnownBinaryDocumentExt(ext string) bool {
	switch ext {
	case ".doc", ".docx", ".ppt", ".pptx", ".xls", ".xlsx", ".zip", ".tar", ".gz", ".tgz", ".bz2", ".xz", ".7z", ".rar", ".dmg", ".iso", ".exe", ".dll", ".so", ".dylib", ".bin", ".wasm", ".class", ".jar", ".pyc":
		return true
	default:
		return false
	}
}

func ImageMimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".tiff", ".tif":
		return "image/tiff"
	case ".ico":
		return "image/x-icon"
	case ".heic":
		return "image/heic"
	case ".heif":
		return "image/heif"
	}
	return ""
}

func SniffImageMime(b []byte) string {
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
	if bytes.HasPrefix(b, []byte("BM")) {
		return "image/bmp"
	}
	if bytes.HasPrefix(b, []byte{0x49, 0x49, 0x2a, 0x00}) || bytes.HasPrefix(b, []byte{0x4d, 0x4d, 0x00, 0x2a}) {
		return "image/tiff"
	}
	if bytes.HasPrefix(b, []byte{0x00, 0x00, 0x01, 0x00}) {
		return "image/x-icon"
	}
	if len(b) >= 12 && string(b[4:8]) == "ftyp" {
		brand := string(b[8:12])
		if strings.HasPrefix(brand, "heic") || strings.HasPrefix(brand, "heix") || strings.HasPrefix(brand, "hevc") || strings.HasPrefix(brand, "hevx") {
			return "image/heic"
		}
		if strings.HasPrefix(brand, "heif") || strings.HasPrefix(brand, "mif1") || strings.HasPrefix(brand, "msf1") {
			return "image/heif"
		}
	}
	mime := http.DetectContentType(b)
	if strings.HasPrefix(mime, "image/") {
		switch mime {
		case "image/png", "image/jpeg", "image/gif", "image/webp", "image/bmp", "image/tiff", "image/x-icon":
			return mime
		}
	}
	return ""
}

// ExtractPDFText performs a best-effort text extraction for simple PDFs without
// external binaries. It scans raw and Flate-compressed streams for PDF string
// drawing operators. Complex/encrypted/scanned PDFs may still require native PDF
// model input or OCR.
func ExtractPDFText(data []byte, maxChars int) (string, bool, error) {
	if maxChars <= 0 {
		maxChars = DefaultMaxExtractedChars
	}
	if !bytes.HasPrefix(data, []byte("%PDF-")) {
		return "", false, fmt.Errorf("not a PDF")
	}
	var chunks [][]byte
	chunks = append(chunks, data)
	chunks = append(chunks, flateStreams(data)...)
	var texts []string
	for _, c := range chunks {
		texts = append(texts, extractPDFStrings(c)...)
	}
	joined := normalizeExtractedText(strings.Join(texts, "\n"))
	if joined == "" {
		return "", false, nil
	}
	truncated := false
	if len([]rune(joined)) > maxChars {
		r := []rune(joined)
		joined = string(r[:maxChars])
		truncated = true
	}
	return joined, truncated, nil
}

func flateStreams(data []byte) [][]byte {
	var out [][]byte
	idx := 0
	for {
		stream := bytes.Index(data[idx:], []byte("stream"))
		if stream < 0 {
			break
		}
		start := idx + stream + len("stream")
		if start < len(data) && data[start] == '\r' {
			start++
		}
		if start < len(data) && data[start] == '\n' {
			start++
		}
		endRel := bytes.Index(data[start:], []byte("endstream"))
		if endRel < 0 {
			break
		}
		end := start + endRel
		r, err := zlib.NewReader(bytes.NewReader(bytes.TrimSpace(data[start:end])))
		if err == nil {
			b, _ := io.ReadAll(io.LimitReader(r, 10_000_000))
			_ = r.Close()
			if len(b) > 0 {
				out = append(out, b)
			}
		}
		idx = end + len("endstream")
	}
	return out
}

var pdfLiteralRe = regexp.MustCompile(`\((?:\\.|[^\\()])*\)\s*(?:Tj|'|")`)
var pdfArrayRe = regexp.MustCompile(`\[((?:\s*(?:\((?:\\.|[^\\()])*\)|<[0-9A-Fa-f\s]+>|[-+]?\d+(?:\.\d+)?))*\s*)\]\s*TJ`)
var pdfArrayItemRe = regexp.MustCompile(`\((?:\\.|[^\\()])*\)|<[0-9A-Fa-f\s]+>`)

func extractPDFStrings(data []byte) []string {
	s := string(data)
	var out []string
	for _, m := range pdfLiteralRe.FindAllString(s, -1) {
		start := strings.IndexByte(m, '(')
		end := strings.LastIndexByte(m, ')')
		if start >= 0 && end > start {
			out = append(out, decodePDFLiteral(m[start+1:end]))
		}
	}
	for _, m := range pdfArrayRe.FindAllString(s, -1) {
		close := strings.LastIndexByte(m, ']')
		if close < 0 {
			continue
		}
		var b strings.Builder
		for _, item := range pdfArrayItemRe.FindAllString(m[:close+1], -1) {
			if strings.HasPrefix(item, "(") {
				b.WriteString(decodePDFLiteral(item[1 : len(item)-1]))
			} else if strings.HasPrefix(item, "<") {
				b.WriteString(decodePDFHex(item[1 : len(item)-1]))
			}
		}
		if b.Len() > 0 {
			out = append(out, b.String())
		}
	}
	return out
}

func decodePDFLiteral(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			b.WriteByte(s[i])
			continue
		}
		i++
		if i >= len(s) {
			break
		}
		switch s[i] {
		case 'n':
			b.WriteByte('\n')
		case 'r':
			b.WriteByte('\r')
		case 't':
			b.WriteByte('\t')
		case 'b':
			b.WriteByte('\b')
		case 'f':
			b.WriteByte('\f')
		case '(', ')', '\\':
			b.WriteByte(s[i])
		case '\n':
			// line continuation
		case '\r':
			if i+1 < len(s) && s[i+1] == '\n' {
				i++
			}
		default:
			if s[i] >= '0' && s[i] <= '7' {
				j := i
				for j < len(s) && j < i+3 && s[j] >= '0' && s[j] <= '7' {
					j++
				}
				var val byte
				for _, c := range s[i:j] {
					val = val*8 + byte(c-'0')
				}
				b.WriteByte(val)
				i = j - 1
			} else {
				b.WriteByte(s[i])
			}
		}
	}
	return b.String()
}

func decodePDFHex(s string) string {
	s = strings.Join(strings.Fields(s), "")
	if len(s)%2 == 1 {
		s += "0"
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return ""
	}
	// UTF-16BE with BOM is common in PDFs.
	if len(b) >= 2 && b[0] == 0xfe && b[1] == 0xff {
		var r []rune
		for i := 2; i+1 < len(b); i += 2 {
			r = append(r, rune(b[i])<<8|rune(b[i+1]))
		}
		return string(r)
	}
	return string(b)
}

func normalizeExtractedText(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	lastBlank := false
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			if !lastBlank && len(out) > 0 {
				out = append(out, "")
			}
			lastBlank = true
			continue
		}
		out = append(out, line)
		lastBlank = false
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func isReferenceBoundary(c byte) bool {
	return isReferenceSeparator(c) || c == '(' || c == '[' || c == '{' || c == '<'
}

func isReferenceSeparator(c byte) bool {
	switch c {
	case ' ', '\n', '\r', '\t', '\'', '"', '`':
		return true
	default:
		return false
	}
}

func isTokenSeparator(c byte) bool {
	switch c {
	case ' ', '\n', '\r', '\t', '\'', '"', '`', '(', '[', '{', '<':
		return true
	default:
		return false
	}
}

func trimPathPunctuation(s string) string {
	s = strings.TrimLeft(s, "([{<")
	s = strings.TrimRight(s, "\"'`,;:!?)>]}")
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

func isWindowsAbsPath(s string) bool { return isWindowsDriveStart(s, 0) }
