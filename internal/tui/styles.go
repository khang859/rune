package tui

import "github.com/charmbracelet/lipgloss"

type Styles struct {
	User           lipgloss.Style
	Assistant      lipgloss.Style
	Thinking       lipgloss.Style
	ToolCall       lipgloss.Style
	ToolResult     lipgloss.Style
	ToolError      lipgloss.Style
	Info           lipgloss.Style
	SummaryHeader  lipgloss.Style
	ThinkingHeader lipgloss.Style
	Footer         lipgloss.Style
	EditorBox      lipgloss.Style
}

func DefaultStyles() Styles {
	return Styles{
		User:           lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		Assistant:      lipgloss.NewStyle().Foreground(lipgloss.Color("15")),
		Thinking:       lipgloss.NewStyle().Faint(true).Italic(true),
		ToolCall:       lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		ToolResult:     lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		ToolError:      lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		Info:           lipgloss.NewStyle().Faint(true).Italic(true).Foreground(lipgloss.Color("8")),
		SummaryHeader:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")),
		ThinkingHeader: lipgloss.NewStyle().Faint(true),
		Footer:         lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Reverse(true).Padding(0, 1),
		EditorBox:      lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
	}
}
