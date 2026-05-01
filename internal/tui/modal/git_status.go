package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type GitStatusData struct {
	Repo     string
	Branch   string
	Files    []GitFileChange
	Diffstat string
}

type GitFileChange struct {
	Path         string
	OriginalPath string
	IndexStatus  byte
	WorkStatus   byte
	StagedDiff   string
	WorkDiff     string
}

type GitStatus struct {
	data   GitStatusData
	sel    int
	detail bool
	scroll int
}

func NewGitStatus(data GitStatusData) Modal {
	return &GitStatus{data: data}
}

func (g *GitStatus) Init() tea.Cmd { return nil }

func (g *GitStatus) Update(msg tea.Msg) (Modal, tea.Cmd) {
	k, ok := msg.(tea.KeyMsg)
	if !ok {
		return g, nil
	}

	if g.detail {
		switch k.Type {
		case tea.KeyUp:
			if g.scroll > 0 {
				g.scroll--
			}
		case tea.KeyDown:
			g.scroll++
		case tea.KeyPgUp:
			g.scroll -= 10
			if g.scroll < 0 {
				g.scroll = 0
			}
		case tea.KeyPgDown:
			g.scroll += 10
		case tea.KeyHome:
			g.scroll = 0
		case tea.KeyEnter, tea.KeyLeft:
			g.detail = false
			g.scroll = 0
		case tea.KeyEsc:
			return g, Cancel()
		case tea.KeyRunes:
			if string(k.Runes) == "q" {
				return g, Cancel()
			}
		}
		return g, nil
	}

	switch k.Type {
	case tea.KeyUp:
		if g.sel > 0 {
			g.sel--
		}
	case tea.KeyDown:
		if g.sel < len(g.data.Files)-1 {
			g.sel++
		}
	case tea.KeyHome:
		g.sel = 0
	case tea.KeyEnd:
		if len(g.data.Files) > 0 {
			g.sel = len(g.data.Files) - 1
		}
	case tea.KeyEnter:
		if len(g.data.Files) > 0 {
			g.detail = true
			g.scroll = 0
		}
	case tea.KeyEsc:
		return g, Cancel()
	case tea.KeyRunes:
		if string(k.Runes) == "q" {
			return g, Cancel()
		}
	}
	return g, nil
}

func (g *GitStatus) View(width, height int) string {
	if g.detail {
		return g.detailView(width, height)
	}
	return g.listView(width, height)
}

func (g *GitStatus) listView(width, height int) string {
	styles := settingsStyles()
	contentWidth := modalContentWidth(width)
	if g.sel >= len(g.data.Files) && len(g.data.Files) > 0 {
		g.sel = len(g.data.Files) - 1
	}
	if g.sel < 0 {
		g.sel = 0
	}

	var sb strings.Builder
	sb.WriteString(styles.Title.Render("✦ Git Status ✦"))
	sb.WriteByte('\n')
	sb.WriteString(styles.Divider.Width(contentWidth).Render(""))
	sb.WriteString("\n\n")
	branch := g.data.Branch
	if branch == "" {
		branch = "unknown branch"
	}
	fmt.Fprintf(&sb, "%s %s\n", styles.Section.Render("Branch:"), styles.Value.Render(branch))
	if g.data.Repo != "" {
		fmt.Fprintf(&sb, "%s %s\n", styles.Section.Render("Repo:"), styles.Value.Render(truncateDisplay(g.data.Repo, contentWidth-8)))
	}
	sb.WriteByte('\n')
	sb.WriteString(styles.Section.Render("✧ Changed files"))
	sb.WriteByte('\n')

	if len(g.data.Files) == 0 {
		sb.WriteString(styles.Value.Render("  Working tree clean"))
		sb.WriteByte('\n')
	} else {
		start, end, clippedTop, clippedBottom := visibleChoiceWindow(height-6, len(g.data.Files), g.sel)
		if clippedTop {
			sb.WriteString(styles.Gutter.Render("  …"))
			sb.WriteByte('\n')
		}
		for i := start; i < end; i++ {
			file := g.data.Files[i]
			selector := styles.Gutter.Render("  ")
			lineStyle := styles.Value
			if i == g.sel {
				selector = styles.Selector.Render("➤ ")
				lineStyle = styles.SelectedValue
			}
			line := fmt.Sprintf("%-2s %s", gitStatusBadge(file), gitDisplayPath(file))
			fmt.Fprintf(&sb, "%s%s\n", selector, lineStyle.Render(truncateDisplay(line, contentWidth-2)))
		}
		if clippedBottom {
			sb.WriteString(styles.Gutter.Render("  …"))
			sb.WriteByte('\n')
		}
	}

	if strings.TrimSpace(g.data.Diffstat) != "" {
		sb.WriteByte('\n')
		sb.WriteString(styles.Section.Render("✧ Diffstat"))
		sb.WriteByte('\n')
		for _, line := range firstNonEmptyLines(g.data.Diffstat, 6) {
			sb.WriteString(styles.Value.Render("  " + truncateDisplay(line, contentWidth-2)))
			sb.WriteByte('\n')
		}
	}

	sb.WriteByte('\n')
	if len(g.data.Files) == 0 {
		sb.WriteString(styles.Help.Render("Esc dismiss"))
	} else {
		sb.WriteString(styles.Help.Render("↑/↓ choose file · Enter view diff · Esc dismiss"))
	}
	return centeredModal(width, height, contentWidth, sb.String())
}

func (g *GitStatus) detailView(width, height int) string {
	styles := settingsStyles()
	contentWidth := modalContentWidth(width)
	file := g.data.Files[g.sel]
	lines := strings.Split(strings.TrimRight(gitDiffText(file), "\n"), "\n")
	visible := height - 8
	if visible < 1 {
		visible = 1
	}
	if g.scroll < 0 {
		g.scroll = 0
	}
	if g.scroll > len(lines)-visible {
		g.scroll = len(lines) - visible
	}
	if g.scroll < 0 {
		g.scroll = 0
	}
	end := g.scroll + visible
	if end > len(lines) {
		end = len(lines)
	}

	var sb strings.Builder
	sb.WriteString(styles.Title.Render("✦ Git Diff ✦"))
	sb.WriteByte('\n')
	sb.WriteString(styles.Divider.Width(contentWidth).Render(""))
	sb.WriteString("\n\n")
	fmt.Fprintf(&sb, "%s %s\n", styles.Section.Render(gitStatusBadge(file)), styles.SelectedValue.Render(truncateDisplay(gitDisplayPath(file), contentWidth-4)))
	if g.scroll > 0 {
		sb.WriteString(styles.Gutter.Render("  …"))
		sb.WriteByte('\n')
	}
	for _, line := range lines[g.scroll:end] {
		sb.WriteString(renderGitDiffLine(line, contentWidth))
		sb.WriteByte('\n')
	}
	if end < len(lines) {
		sb.WriteString(styles.Gutter.Render("  …"))
		sb.WriteByte('\n')
	}
	sb.WriteByte('\n')
	sb.WriteString(styles.Help.Render("↑/↓ scroll · PgUp/PgDown page · Enter back · Esc dismiss"))
	return centeredModal(width, height, contentWidth, sb.String())
}

func gitStatusBadge(file GitFileChange) string {
	if file.IndexStatus == '?' && file.WorkStatus == '?' {
		return "??"
	}
	x := file.IndexStatus
	y := file.WorkStatus
	if x == 0 || x == ' ' {
		x = '·'
	}
	if y == 0 || y == ' ' {
		y = '·'
	}
	return string([]rune{rune(x), rune(y)})
}

func gitDisplayPath(file GitFileChange) string {
	if file.OriginalPath != "" {
		return file.OriginalPath + " → " + file.Path
	}
	return file.Path
}

func gitDiffText(file GitFileChange) string {
	var parts []string
	if strings.TrimSpace(file.StagedDiff) != "" {
		parts = append(parts, "Staged diff:\n"+strings.TrimRight(file.StagedDiff, "\n"))
	}
	if strings.TrimSpace(file.WorkDiff) != "" {
		parts = append(parts, "Unstaged diff:\n"+strings.TrimRight(file.WorkDiff, "\n"))
	}
	if len(parts) == 0 {
		if file.IndexStatus == '?' && file.WorkStatus == '?' {
			return "Untracked file — no diff is available until it is added."
		}
		return "No diff available for this file."
	}
	return strings.Join(parts, "\n\n")
}

func renderGitDiffLine(line string, width int) string {
	line = truncateDisplay(line, width)
	switch {
	case strings.HasPrefix(line, "Staged diff:"), strings.HasPrefix(line, "Unstaged diff:"):
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")).Render(line)
	case strings.HasPrefix(line, "@@"):
		return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")).Render(line)
	case strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render(line)
	case strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Render(line)
	case strings.HasPrefix(line, "diff --git"), strings.HasPrefix(line, "index "), strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"):
		return lipgloss.NewStyle().Foreground(lipgloss.Color("12")).Faint(true).Render(line)
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Render(line)
	}
}

func firstNonEmptyLines(s string, max int) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= max {
			break
		}
	}
	return out
}

func truncateDisplay(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max == 1 {
		return "…"
	}
	return s[:max-1] + "…"
}
