package tui

import (
	"fmt"
	"strings"
)

type Footer struct {
	Cwd            string
	GitBranch      string
	Session        string
	Model          string
	ThinkingEffort string
	Tokens         int
	ContextPct     int
	Width          int
	Mode           string
}

func (f Footer) Render(s Styles) string {
	sep := s.FooterSep.Render(" | ")
	parts := []string{
		s.FooterApp.Render(iconLabel(s.Icons.App, "rune")),
		s.FooterCwd.Render(iconLabel(s.Icons.Cwd, f.Cwd)),
	}
	if f.GitBranch != "" {
		parts = append(parts, s.FooterSession.Render(iconLabel(s.Icons.GitBranch, f.GitBranch)))
	}
	parts = append(parts,
		s.FooterSession.Render(iconLabel(s.Icons.Session, f.Session)),
		s.FooterModel.Render(f.Model),
	)
	if f.ThinkingEffort != "" {
		parts = append(parts, s.FooterModel.Render(iconLabel(s.Icons.Thinking, f.ThinkingEffort)))
	}
	if f.Mode != "" {
		parts = append(parts, s.FooterModel.Render(f.Mode))
	}
	parts = append(parts,
		s.FooterTokens.Render(iconLabel(s.Icons.Tokens, fmt.Sprintf("%s tok", compactCount(f.Tokens)))),
		s.FooterContext.Render(iconLabel(s.Icons.Context, fmt.Sprintf("%d%% ctx", f.ContextPct))),
	)
	return s.Footer.Width(f.Width).Render(strings.Join(parts, sep))
}

func compactCount(n int) string {
	if n < 0 {
		return fmt.Sprintf("%d", n)
	}
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}

	units := []struct {
		suffix string
		value  int
	}{
		{"b", 1_000_000_000},
		{"m", 1_000_000},
		{"k", 1_000},
	}
	for _, unit := range units {
		if n >= unit.value {
			whole := n / unit.value
			decimal := (n % unit.value) / (unit.value / 10)
			if whole >= 10 || decimal == 0 {
				return fmt.Sprintf("%d%s", whole, unit.suffix)
			}
			return fmt.Sprintf("%d.%d%s", whole, decimal, unit.suffix)
		}
	}
	return fmt.Sprintf("%d", n)
}
