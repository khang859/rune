package tui

import (
	"strings"
	"testing"
)

func TestFooter_RendersAllFields(t *testing.T) {
	f := Footer{
		Cwd:            "/home/x/proj",
		GitBranch:      "main",
		Session:        "demo",
		Model:          "gpt-5",
		ThinkingEffort: "medium",
		Tokens:         1234,
		ContextPct:     42,
		Width:          120,
	}
	out := f.Render(DefaultStylesWithIconMode("nerd"))
	for _, want := range []string{"ᚱ rune", " /home/x/proj", " main", " demo", "gpt-5", " medium", "󰆙 1.2k tok", "󰊚 42% ctx"} {
		if !strings.Contains(out, want) {
			t.Fatalf("footer missing %q:\n%s", want, out)
		}
	}
}

func TestFooter_RendersPlanMode(t *testing.T) {
	f := Footer{Cwd: "/p", Session: "s", Model: "gpt-5", Mode: "plan", Width: 120}
	out := f.Render(DefaultStylesWithIconMode("unicode"))
	if !strings.Contains(out, "plan") {
		t.Fatalf("footer missing plan mode:\n%s", out)
	}
}

func TestFooter_OmitsThinkingWhenEmpty(t *testing.T) {
	f := Footer{
		Cwd:        "/home/x/proj",
		Session:    "demo",
		Model:      "gpt-5.4-mini",
		Tokens:     1234,
		ContextPct: 42,
		Width:      120,
	}
	out := f.Render(DefaultStylesWithIconMode("unicode"))
	if strings.Contains(out, "medium") || strings.Contains(out, "thinking") {
		t.Fatalf("footer rendered empty thinking effort:\n%s", out)
	}
}

func TestCompactCount(t *testing.T) {
	tests := map[int]string{
		0:             "0",
		999:           "999",
		1000:          "1k",
		1234:          "1.2k",
		9999:          "9.9k",
		10000:         "10k",
		12500:         "12k",
		1_234_567:     "1.2m",
		1_000_000_000: "1b",
	}

	for in, want := range tests {
		if got := compactCount(in); got != want {
			t.Fatalf("compactCount(%d) = %q, want %q", in, got, want)
		}
	}
}
