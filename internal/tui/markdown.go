package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

// Markdown renders markdown to an ANSI string suitable for terminal display.
// The zero value renders without styling.
type Markdown struct {
	r *glamour.TermRenderer
}

// NewMarkdown returns a renderer with WithWordWrap(0) so the surrounding
// ansi.Wrap in Messages.Render stays the single source of width truncation.
// We use WithStandardStyle("dark") instead of WithAutoStyle() to avoid a
// blocking terminal background-color query that leaks into the viewport.
func NewMarkdown() Markdown {
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		return Markdown{}
	}
	return Markdown{r: r}
}

// NewMarkdownWidth returns a renderer configured for the given terminal width.
// Tables and prose are soft-wrapped within the width instead of being hard-wrapped
// later, which preserves table borders. We use WithStandardStyle("dark") instead
// of WithAutoStyle() to avoid a blocking terminal background-color query that
// leaks into the viewport.
func NewMarkdownWidth(width int) Markdown {
	if width <= 0 {
		return NewMarkdown()
	}
	r, err := glamour.NewTermRenderer(
		glamour.WithStandardStyle("dark"),
		glamour.WithWordWrap(width),
		glamour.WithTableWrap(true),
	)
	if err != nil {
		return Markdown{}
	}
	return Markdown{r: r}
}

func (m Markdown) Render(s string) string {
	if m.r == nil {
		return s
	}
	out, err := m.r.Render(s)
	if err != nil {
		return s
	}
	return strings.Trim(out, "\n")
}
