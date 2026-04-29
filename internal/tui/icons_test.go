package tui

import "testing"

func TestIconSetForModeSelectsNerdGlyphs(t *testing.T) {
	icons := IconSetForMode("nerd")
	if icons.Assistant != "󰬯" {
		t.Fatalf("assistant icon = %q", icons.Assistant)
	}
	if icons.Tool != "" {
		t.Fatalf("tool icon = %q", icons.Tool)
	}
}

func TestIconSetForModeFallsBackToUnicode(t *testing.T) {
	for _, mode := range []string{"", "auto", "bogus"} {
		icons := IconSetForMode(mode)
		if icons.Thinking != "✦" {
			t.Fatalf("mode %q thinking icon = %q", mode, icons.Thinking)
		}
	}
}

func TestDefaultIconModeReadsEnvironment(t *testing.T) {
	t.Setenv("RUNE_ICONS", "ascii")
	if got := DefaultIconMode(); got != "ascii" {
		t.Fatalf("DefaultIconMode() = %q", got)
	}
}
