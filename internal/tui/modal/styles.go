package modal

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type settingsViewStyles struct {
	Title         lipgloss.Style
	Divider       lipgloss.Style
	Section       lipgloss.Style
	Gutter        lipgloss.Style
	Selector      lipgloss.Style
	Label         lipgloss.Style
	SelectedLabel lipgloss.Style
	Value         lipgloss.Style
	SelectedValue lipgloss.Style
	Help          lipgloss.Style
}

func settingsStyles() settingsViewStyles {
	return settingsViewStyles{
		Title:         lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")),
		Divider:       lipgloss.NewStyle().Foreground(lipgloss.Color("5")).SetString("─"),
		Section:       lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		Gutter:        lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		Selector:      lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")),
		Label:         lipgloss.NewStyle().Width(24).Foreground(lipgloss.Color("15")),
		SelectedLabel: lipgloss.NewStyle().Width(24).Bold(true).Foreground(lipgloss.Color("14")),
		Value:         lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		SelectedValue: lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")),
		Help:          lipgloss.NewStyle().Faint(true).Italic(true).Foreground(lipgloss.Color("8")),
	}
}

func modalContentWidth(width int) int {
	contentWidth := width - 4
	if contentWidth < 44 {
		contentWidth = 44
	}
	if contentWidth > 76 {
		contentWidth = 76
	}
	return contentWidth
}

func centeredModal(width, height int, contentWidth int, body string) string {
	content := lipgloss.NewStyle().
		Width(contentWidth).
		Align(lipgloss.Left).
		Render(body)

	return lipgloss.NewStyle().
		Width(width).
		Height(height).
		Align(lipgloss.Center).
		AlignVertical(lipgloss.Center).
		Render(content)
}

type choiceRow struct {
	Label string
	Value string
}

func renderChoiceModal(width, height int, title, section, help string, rows []choiceRow, selected int) string {
	styles := settingsStyles()
	contentWidth := modalContentWidth(width)

	start, end, clippedTop, clippedBottom := visibleChoiceWindow(height, len(rows), selected)

	var sb strings.Builder
	sb.WriteString(styles.Title.Render(title))
	sb.WriteByte('\n')
	sb.WriteString(styles.Divider.Width(contentWidth).Render(""))
	sb.WriteString("\n\n")
	if section != "" {
		sb.WriteString(styles.Section.Render("✧ " + section))
		sb.WriteByte('\n')
	}
	if clippedTop {
		sb.WriteString(styles.Gutter.Render("  …"))
		sb.WriteByte('\n')
	}
	for i := start; i < end; i++ {
		row := rows[i]
		selectedRow := i == selected
		selector := styles.Gutter.Render("  ")
		labelStyle := styles.Label
		valueStyle := styles.Value
		if selectedRow {
			selector = styles.Selector.Render("➤ ")
			labelStyle = styles.SelectedLabel
			valueStyle = styles.SelectedValue
		}
		if row.Value != "" {
			fmt.Fprintf(&sb, "%s%s %s\n", selector, labelStyle.Render(row.Label), valueStyle.Render(row.Value))
			continue
		}
		fmt.Fprintf(&sb, "%s%s\n", selector, labelStyle.Render(row.Label))
	}
	if clippedBottom {
		sb.WriteString(styles.Gutter.Render("  …"))
		sb.WriteByte('\n')
	}
	if help != "" {
		sb.WriteByte('\n')
		sb.WriteString(styles.Help.Render(help))
	}

	return centeredModal(width, height, contentWidth, sb.String())
}

func visibleChoiceWindow(height, rowCount, selected int) (start, end int, clippedTop, clippedBottom bool) {
	if rowCount <= 0 {
		return 0, 0, false, false
	}
	visible := height - 8 // title, divider, section, help, spacer, and margins.
	if visible < 1 {
		visible = 1
	}
	if visible > rowCount {
		visible = rowCount
	}
	if selected < 0 {
		selected = 0
	}
	if selected >= rowCount {
		selected = rowCount - 1
	}
	start = selected - visible/2
	if start < 0 {
		start = 0
	}
	if start+visible > rowCount {
		start = rowCount - visible
	}
	end = start + visible
	return start, end, start > 0, end < rowCount
}
