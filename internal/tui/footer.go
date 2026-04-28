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
	line := fmt.Sprintf(" %s | %s | %s | %d tok | %d%% ctx ",
		f.Cwd, f.Session, f.Model, f.Tokens, f.ContextPct)
	return s.Footer.Width(f.Width).Render(line)
}
