package tui

import "github.com/charmbracelet/lipgloss"

type Styles struct {
	User                 lipgloss.Style
	Assistant            lipgloss.Style
	Thinking             lipgloss.Style
	ToolCall             lipgloss.Style
	ToolResult           lipgloss.Style
	ToolError            lipgloss.Style
	DiffAdd              lipgloss.Style
	DiffDel              lipgloss.Style
	Info                 lipgloss.Style
	SummaryHeader        lipgloss.Style
	ThinkingHeader       lipgloss.Style
	Footer               lipgloss.Style
	FooterApp            lipgloss.Style
	FooterCwd            lipgloss.Style
	FooterSession        lipgloss.Style
	FooterModel          lipgloss.Style
	FooterTokens         lipgloss.Style
	FooterContext        lipgloss.Style
	FooterSep            lipgloss.Style
	EditorBox            lipgloss.Style
	EditorBoxShellSend   lipgloss.Style // "!cmd": run shell, send output to AI
	EditorBoxShellInsert lipgloss.Style // "!!cmd": run shell, insert output locally
	EditorBoxDim         lipgloss.Style // dimmed border while in copy mode
	Activity             lipgloss.Style
	CopyModeBanner       lipgloss.Style
	QuitPrimedBanner     lipgloss.Style
	Icons                IconSet
	Markdown             Markdown
}

func DefaultStyles() Styles {
	return DefaultStylesWithIconMode(string(DefaultIconMode()))
}

func DefaultStylesWithIconMode(mode string) Styles {
	return Styles{
		User:                 lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12")),
		Assistant:            lipgloss.NewStyle().Foreground(lipgloss.Color("15")),
		Thinking:             lipgloss.NewStyle().Faint(true).Italic(true),
		ToolCall:             lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		ToolResult:           lipgloss.NewStyle().Foreground(lipgloss.Color("8")),
		ToolError:            lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		DiffAdd:              lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		DiffDel:              lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		Info:                 lipgloss.NewStyle().Faint(true).Italic(true).Foreground(lipgloss.Color("8")),
		SummaryHeader:        lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("13")),
		ThinkingHeader:       lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("8")),
		Footer:               lipgloss.NewStyle().Padding(0, 1),
		FooterApp:            lipgloss.NewStyle().Foreground(lipgloss.Color("13")),
		FooterCwd:            lipgloss.NewStyle().Foreground(lipgloss.Color("12")),
		FooterSession:        lipgloss.NewStyle().Foreground(lipgloss.Color("6")),
		FooterModel:          lipgloss.NewStyle().Foreground(lipgloss.Color("15")),
		FooterTokens:         lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		FooterContext:        lipgloss.NewStyle().Foreground(lipgloss.Color("5")),
		FooterSep:            lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("8")),
		EditorBox:            lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).Padding(0, 1),
		EditorBoxShellSend:   lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("11")).Padding(0, 1),
		EditorBoxShellInsert: lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("13")).Padding(0, 1),
		EditorBoxDim:         lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("8")).Faint(true).Padding(0, 1),
		Activity:             lipgloss.NewStyle().Foreground(lipgloss.Color("13")).Italic(true),
		CopyModeBanner:       lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color("14")),
		QuitPrimedBanner:     lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")),
		Icons:                IconSetForMode(mode),
		Markdown:             NewMarkdown(),
	}
}
