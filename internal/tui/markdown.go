package tui

import (
	"strings"

	"github.com/charmbracelet/glamour"
)

type Markdown struct {
	r *glamour.TermRenderer
}

// NewMarkdown returns a renderer with WithWordWrap(0) so the surrounding
// ansi.Wrap in Messages.Render stays the single source of width truncation.
func NewMarkdown() Markdown {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0),
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
