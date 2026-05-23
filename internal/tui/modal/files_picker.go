package modal

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
)

const (
	filesPickerLimit       = 200
	filesPickerMaxScan     = 10000
	filesPickerPreviewSize = 64 * 1024
)

type FilesPickerResult struct {
	Path string
}

type FilesPicker struct {
	root    string
	files   []string
	query   string
	matches []string
	sel     int
	scanned int
}

func NewFilesPicker(root string) Modal {
	p := &FilesPicker{root: root}
	p.scan()
	p.filter()
	return p
}

func (p *FilesPicker) Init() tea.Cmd { return nil }

func (p *FilesPicker) Update(msg tea.Msg) (Modal, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return p, nil
	}

	switch k.Type {
	case tea.KeyEsc:
		return p, Cancel()
	case tea.KeyEnter:
		if sel := p.selected(); sel != "" {
			return p, Result(FilesPickerResult{Path: sel})
		}
		return p, Cancel()
	case tea.KeyUp:
		if p.sel > 0 {
			p.sel--
		}
	case tea.KeyDown:
		if p.sel < len(p.matches)-1 {
			p.sel++
		}
	case tea.KeyPgUp:
		p.sel -= 10
		if p.sel < 0 {
			p.sel = 0
		}
	case tea.KeyPgDown:
		p.sel += 10
		if p.sel >= len(p.matches) {
			p.sel = len(p.matches) - 1
		}
		if p.sel < 0 {
			p.sel = 0
		}
	case tea.KeyHome:
		p.sel = 0
	case tea.KeyEnd:
		if len(p.matches) > 0 {
			p.sel = len(p.matches) - 1
		}
	case tea.KeyBackspace, tea.KeyCtrlH:
		if p.query != "" {
			_, size := utf8.DecodeLastRuneInString(p.query)
			p.query = p.query[:len(p.query)-size]
			p.filter()
		}
	case tea.KeyCtrlU:
		if p.query != "" {
			p.query = ""
			p.filter()
		}
	case tea.KeySpace:
		p.query += " "
		p.filter()
	case tea.KeyRunes:
		p.query += string(k.Runes)
		p.filter()
	}
	return p, nil
}

func (p *FilesPicker) View(width, height int) string {
	styles := settingsStyles()
	contentWidth := width - 4
	if contentWidth < 60 {
		contentWidth = 60
	}
	if contentWidth > 140 {
		contentWidth = 140
	}
	contentHeight := height - 4
	if contentHeight < 14 {
		contentHeight = 14
	}

	headerLines := 5
	footerLines := 2
	bodyHeight := contentHeight - headerLines - footerLines
	if bodyHeight < 4 {
		bodyHeight = 4
	}
	listWidth := contentWidth * 43 / 100
	if listWidth < 28 {
		listWidth = 28
	}
	previewWidth := contentWidth - listWidth - 3
	if previewWidth < 24 {
		previewWidth = 24
	}

	var sb strings.Builder
	sb.WriteString(styles.Title.Render("✦ Files ✦"))
	sb.WriteByte('\n')
	sb.WriteString(styles.Divider.Width(contentWidth).Render(""))
	sb.WriteString("\n")
	placeholder := "type to fuzzy search"
	query := p.query
	if query == "" {
		query = styles.Gutter.Render(placeholder)
	}
	fmt.Fprintf(&sb, "%s %s\n", styles.Section.Render("query:"), query)
	status := fmt.Sprintf("%d/%d files", len(p.matches), p.scanned)
	if p.scanned >= filesPickerMaxScan {
		status += " (scan limit reached)"
	}
	fmt.Fprintf(&sb, "%s %s\n\n", styles.Section.Render("status:"), styles.Value.Render(status))

	list := p.renderList(listWidth, bodyHeight)
	preview := p.renderPreview(previewWidth, bodyHeight)
	sb.WriteString(lipgloss.JoinHorizontal(lipgloss.Top, list, " │ ", preview))
	sb.WriteString("\n\n")
	sb.WriteString(styles.Help.Render("↑/↓ choose · type filter · Enter insert @path · Ctrl+U clear · Esc dismiss"))

	content := lipgloss.NewStyle().Width(contentWidth).MaxWidth(contentWidth).Height(contentHeight).MaxHeight(contentHeight).Render(sb.String())
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Height(height).MaxHeight(height).Align(lipgloss.Center).AlignVertical(lipgloss.Center).Render(content)
}

func (p *FilesPicker) scan() {
	root := p.root
	if root == "" {
		if wd, err := os.Getwd(); err == nil {
			root = wd
		}
	}
	p.root = root
	var paths []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == "." {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if shouldSkipPickerPath(rel, d) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		paths = append(paths, rel)
		if len(paths) >= filesPickerMaxScan {
			return filepath.SkipAll
		}
		return nil
	})
	p.files = paths
	p.scanned = len(paths)
}

func shouldSkipPickerPath(rel string, d fs.DirEntry) bool {
	parts := strings.Split(rel, "/")
	for _, part := range parts {
		switch part {
		case ".git", "node_modules", "vendor", "dist", "build", "target", ".cache":
			return true
		}
		if strings.HasPrefix(part, ".") {
			return true
		}
	}
	return false
}

func (p *FilesPicker) filter() {
	p.sel = 0
	if p.query == "" {
		p.matches = limitStrings(p.files, filesPickerLimit)
		return
	}
	matches := fuzzy.Find(p.query, p.files)
	out := make([]string, 0, min(len(matches), filesPickerLimit))
	for i, m := range matches {
		if i >= filesPickerLimit {
			break
		}
		out = append(out, m.Str)
	}
	p.matches = out
}

func limitStrings(in []string, limit int) []string {
	if len(in) <= limit {
		return append([]string(nil), in...)
	}
	return append([]string(nil), in[:limit]...)
}

func (p *FilesPicker) selected() string {
	if p.sel < 0 || p.sel >= len(p.matches) {
		return ""
	}
	return p.matches[p.sel]
}

func (p *FilesPicker) renderList(width, height int) string {
	styles := settingsStyles()
	lines := make([]string, 0, height)
	if len(p.matches) == 0 {
		lines = append(lines, styles.Value.Render("no matches"))
		return boxLines(lines, width, height)
	}
	start := p.sel - height/2
	if start < 0 {
		start = 0
	}
	if start+height > len(p.matches) {
		start = len(p.matches) - height
		if start < 0 {
			start = 0
		}
	}
	end := start + height
	if end > len(p.matches) {
		end = len(p.matches)
	}
	for i := start; i < end; i++ {
		selector := styles.Gutter.Render("  ")
		style := styles.Value
		if i == p.sel {
			selector = styles.Selector.Render("➤ ")
			style = styles.SelectedValue
		}
		label := clipLine(p.matches[i], width-2)
		lines = append(lines, selector+style.Render(label))
	}
	return boxLines(lines, width, height)
}

func (p *FilesPicker) renderPreview(width, height int) string {
	styles := settingsStyles()
	selected := p.selected()
	if selected == "" {
		return boxLines([]string{styles.Value.Render("select a file")}, width, height)
	}
	path := filepath.Join(p.root, filepath.FromSlash(selected))
	lines := []string{styles.Section.Render(clipLine(selected, width))}
	info, err := os.Stat(path)
	if err != nil {
		lines = append(lines, styles.Value.Render(clipLine(err.Error(), width)))
		return boxLines(lines, width, height)
	}
	lines = append(lines, styles.Gutter.Render(clipLine(fmt.Sprintf("%d bytes", info.Size()), width)))
	if info.Size() > filesPickerPreviewSize {
		lines = append(lines, styles.Value.Render("preview skipped: file is large"))
		return boxLines(lines, width, height)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		lines = append(lines, styles.Value.Render(clipLine(err.Error(), width)))
		return boxLines(lines, width, height)
	}
	if looksBinary(b) {
		lines = append(lines, styles.Value.Render("binary file"))
		return boxLines(lines, width, height)
	}
	text := strings.ReplaceAll(string(b), "\r\n", "\n")
	for _, line := range strings.Split(text, "\n") {
		if len(lines) >= height {
			break
		}
		lines = append(lines, clipLine(line, width))
	}
	return boxLines(lines, width, height)
}

func boxLines(lines []string, width, height int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	out := make([]string, 0, height)
	for i := 0; i < height; i++ {
		line := ""
		if i < len(lines) {
			line = ansi.Truncate(lines[i], width, "…")
		}
		out = append(out, lipgloss.NewStyle().Width(width).MaxWidth(width).Inline(true).Render(line))
	}
	return lipgloss.NewStyle().Width(width).MaxWidth(width).Height(height).MaxHeight(height).Render(strings.Join(out, "\n"))
}

func clipLine(s string, width int) string {
	if width < 1 {
		return ""
	}
	return ansi.Truncate(sanitizePickerLine(s), width, "…")
}

func sanitizePickerLine(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\t':
			b.WriteString("    ")
		case r < 0x20 || r == 0x7f:
			b.WriteRune('·')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func looksBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	for i, c := range b {
		if i >= 8000 {
			break
		}
		if c == 0 {
			return true
		}
	}
	return false
}
