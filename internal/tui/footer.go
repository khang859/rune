package tui

import "fmt"

type Footer struct {
	Cwd        string
	Session    string
	Model      string
	Tokens     int
	ContextPct int
	Width      int
}

func (f Footer) Render(s Styles) string {
	line := fmt.Sprintf(" %s | %s | %s | %s | %s | %s ",
		iconLabel(s.Icons.App, "rune"),
		iconLabel(s.Icons.Cwd, f.Cwd),
		iconLabel(s.Icons.Session, f.Session),
		f.Model,
		iconLabel(s.Icons.Tokens, fmt.Sprintf("%d tok", f.Tokens)),
		iconLabel(s.Icons.Context, fmt.Sprintf("%d%% ctx", f.ContextPct)))
	return s.Footer.Width(f.Width).Render(line)
}
