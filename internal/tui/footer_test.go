package tui

import (
	"strings"
	"testing"
)

func TestFooter_RendersAllFields(t *testing.T) {
	f := Footer{
		Cwd:        "/home/x/proj",
		Session:    "demo",
		Model:      "gpt-5",
		Tokens:     1234,
		ContextPct: 42,
		Width:      120,
	}
	out := f.Render(DefaultStylesWithIconMode("nerd"))
	for _, want := range []string{"ᚱ rune", " /home/x/proj", " demo", "gpt-5", "󰆙 1234 tok", "󰊚 42% ctx"} {
		if !strings.Contains(out, want) {
			t.Fatalf("footer missing %q:\n%s", want, out)
		}
	}
}
