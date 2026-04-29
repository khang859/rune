package tui

import (
	"fmt"
	"strings"
)

type Footer struct {
	Cwd        string
	Session    string
	Model      string
	Tokens     int
	ContextPct int
	Width      int
}

func (f Footer) Render(s Styles) string {
	sep := s.FooterSep.Render(" | ")
	parts := []string{
		s.FooterApp.Render(iconLabel(s.Icons.App, "rune")),
		s.FooterCwd.Render(iconLabel(s.Icons.Cwd, f.Cwd)),
		s.FooterSession.Render(iconLabel(s.Icons.Session, f.Session)),
		s.FooterModel.Render(f.Model),
		s.FooterTokens.Render(iconLabel(s.Icons.Tokens, fmt.Sprintf("%d tok", f.Tokens))),
		s.FooterContext.Render(iconLabel(s.Icons.Context, fmt.Sprintf("%d%% ctx", f.ContextPct))),
	}
	return s.Footer.Width(f.Width).Render(strings.Join(parts, sep))
}
