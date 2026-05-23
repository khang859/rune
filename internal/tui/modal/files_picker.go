package modal

import (
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/sahilm/fuzzy"
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/tiff"
	_ "golang.org/x/image/webp"

	"github.com/khang859/rune/internal/attachments"
)

const (
	filesPickerLimit       = 200
	filesPickerMaxScan     = 10000
	filesPickerPreviewSize = 64 * 1024
)

type FilesPickerAction string

const (
	FilesPickerInsert FilesPickerAction = "insert"
	FilesPickerAttach FilesPickerAction = "attach"
	FilesPickerOpen   FilesPickerAction = "open"
)

type FilesPickerResult struct {
	Path   string
	Action FilesPickerAction
}

type FilesPickerOpenMsg struct {
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
			return p, Result(FilesPickerResult{Path: sel, Action: FilesPickerInsert})
		}
		return p, Cancel()
	case tea.KeyCtrlA:
		if sel := p.selected(); sel != "" {
			return p, Result(FilesPickerResult{Path: sel, Action: FilesPickerAttach})
		}
	case tea.KeySpace:
		if sel := p.selected(); sel != "" && p.isImage(sel) {
			return p, func() tea.Msg { return FilesPickerOpenMsg{Path: sel} }
		}
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
	footerLines := 4
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
	sb.WriteString(styles.Section.Render("Shortcuts"))
	sb.WriteString("  ")
	sb.WriteString(styles.SelectedValue.Render("Enter"))
	sb.WriteString(styles.Value.Render(" insert @path  "))
	sb.WriteString(styles.SelectedValue.Render("Ctrl+A"))
	sb.WriteString(styles.Value.Render(" attach image  "))
	sb.WriteString(styles.SelectedValue.Render("Space"))
	sb.WriteString(styles.Value.Render(" open image  "))
	sb.WriteString(styles.SelectedValue.Render("Ctrl+U"))
	sb.WriteString(styles.Value.Render(" clear  "))
	sb.WriteString(styles.SelectedValue.Render("Esc"))
	sb.WriteString(styles.Value.Render(" close"))

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

func (p *FilesPicker) isImage(rel string) bool {
	path := filepath.Join(p.root, filepath.FromSlash(rel))
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, filesPickerPreviewSize))
	if err != nil {
		return false
	}
	return attachments.SniffImageMime(b) != ""
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
	if attachments.ImageMimeFromExt(filepath.Ext(path)) != "" {
		return p.renderImageThumbnail(lines, path, width, height)
	}
	if info.Size() > filesPickerPreviewSize {
		lines = append(lines, styles.Value.Render("preview skipped: file is large"))
		return boxLines(lines, width, height)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		lines = append(lines, styles.Value.Render(clipLine(err.Error(), width)))
		return boxLines(lines, width, height)
	}
	if mime := attachments.SniffImageMime(b); mime != "" {
		_ = mime
		return p.renderImageThumbnail(lines, path, width, height)
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

func (p *FilesPicker) renderImageThumbnail(lines []string, path string, width, height int) string {
	styles := settingsStyles()
	mime := attachments.ImageMimeFromExt(filepath.Ext(path))
	if mime == "" {
		mime = "image"
	}
	img, format, err := decodeImageFile(path)
	if err != nil {
		lines = append(lines, styles.Value.Render(clipLine("image: "+mime, width)))
		lines = append(lines, styles.Value.Render(clipLine("preview unavailable: "+err.Error(), width)))
		lines = append(lines, "")
		lines = append(lines, styles.Section.Render("Actions"))
		lines = append(lines, styles.Value.Render(clipLine("Enter  insert @path into prompt", width)))
		lines = append(lines, styles.Value.Render(clipLine("Ctrl+A attach image now", width)))
		lines = append(lines, styles.Value.Render(clipLine("Space   open image viewer", width)))
		return boxLines(lines, width, height)
	}
	bounds := img.Bounds()
	lines = append(lines, styles.Value.Render(clipLine(fmt.Sprintf("image: %s · %d×%d", mime, bounds.Dx(), bounds.Dy()), width)))
	if format != "" {
		lines = append(lines, styles.Value.Render(clipLine("format: "+format, width)))
	}
	lines = append(lines, styles.Value.Render(clipLine("Space: open image viewer", width)))
	available := height - len(lines)
	if available < 1 {
		return boxLines(lines, width, height)
	}
	preview := renderANSIImage(img, width, available)
	if preview != "" {
		lines = append(lines, strings.Split(preview, "\n")...)
	}
	return boxLines(lines, width, height)
}

func decodeImageFile(path string) (image.Image, string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, "", err
	}
	defer f.Close()
	img, format, err := image.Decode(f)
	if err != nil {
		return nil, "", err
	}
	return img, format, nil
}

func renderANSIImage(img image.Image, width, height int) string {
	if img == nil || width <= 0 || height <= 0 {
		return ""
	}
	b := img.Bounds()
	srcW, srcH := b.Dx(), b.Dy()
	if srcW <= 0 || srcH <= 0 {
		return ""
	}
	// One terminal cell rendered with a half-block can represent two vertical
	// image samples. A typical terminal cell is roughly twice as tall as it is
	// wide, so the two half-block samples are close to square pixels.
	scaleX := float64(width) / float64(srcW)
	scaleY := float64(height*2) / float64(srcH)
	scale := math.Min(scaleX, scaleY)
	if scale <= 0 {
		return ""
	}
	outW := int(math.Floor(float64(srcW) * scale))
	outHCells := int(math.Ceil(float64(srcH) * scale / 2))
	if outW < 1 {
		outW = 1
	}
	if outW > width {
		outW = width
	}
	if outHCells < 1 {
		outHCells = 1
	}
	if outHCells > height {
		outHCells = height
	}

	var sb strings.Builder
	for yCell := 0; yCell < outHCells; yCell++ {
		if yCell > 0 {
			sb.WriteByte('\n')
		}
		for xCell := 0; xCell < outW; xCell++ {
			top := averageImageColor(img, b, xCell, yCell*2, outW, outHCells*2)
			bottom := averageImageColor(img, b, xCell, yCell*2+1, outW, outHCells*2)
			r1, g1, b1 := colorToRGB(top)
			r2, g2, b2 := colorToRGB(bottom)
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀", r1, g1, b1, r2, g2, b2)
		}
		sb.WriteString("\x1b[0m")
	}
	return sb.String()
}

func averageImageColor(img image.Image, b image.Rectangle, x, y, outW, outH int) color.Color {
	if outW < 1 {
		outW = 1
	}
	if outH < 1 {
		outH = 1
	}
	x0 := b.Min.X + int(math.Floor(float64(x)*float64(b.Dx())/float64(outW)))
	x1 := b.Min.X + int(math.Ceil(float64(x+1)*float64(b.Dx())/float64(outW)))
	y0 := b.Min.Y + int(math.Floor(float64(y)*float64(b.Dy())/float64(outH)))
	y1 := b.Min.Y + int(math.Ceil(float64(y+1)*float64(b.Dy())/float64(outH)))
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}
	var rsum, gsum, bsum, asum uint64
	var n uint64
	stepX := samplingStep(x1 - x0)
	stepY := samplingStep(y1 - y0)
	for sy := y0; sy < y1; sy += stepY {
		for sx := x0; sx < x1; sx += stepX {
			r, g, bb, a := img.At(sx, sy).RGBA()
			rsum += uint64(r)
			gsum += uint64(g)
			bsum += uint64(bb)
			asum += uint64(a)
			n++
		}
	}
	if n == 0 {
		return color.RGBA{}
	}
	return color.RGBA64{R: uint16(rsum / n), G: uint16(gsum / n), B: uint16(bsum / n), A: uint16(asum / n)}
}

func samplingStep(span int) int {
	if span <= 8 {
		return 1
	}
	step := span / 8
	if step < 1 {
		return 1
	}
	return step
}

func colorToRGB(c color.Color) (uint8, uint8, uint8) {
	r, g, b, a := c.RGBA()
	if a == 0 {
		return 0, 0, 0
	}
	if a < 0xffff {
		r = r * 0xffff / a
		g = g * 0xffff / a
		b = b * 0xffff / a
	}
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
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
