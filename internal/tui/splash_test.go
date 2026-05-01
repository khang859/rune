package tui

import (
	"strings"
	"testing"
)

func TestRenderSplashShowsWordmarkAndTagline(t *testing.T) {
	out := renderSplash(80, 20, DefaultStyles(), "1.2.3")
	if !strings.Contains(out, "██████") {
		t.Fatalf("wordmark missing:\n%s", out)
	}
	if !strings.Contains(out, "speak your incantation") {
		t.Fatalf("tagline missing:\n%s", out)
	}
	if !strings.Contains(out, "rune 1.2.3") {
		t.Fatalf("version missing:\n%s", out)
	}
}

func TestRenderSplashShowsNotice(t *testing.T) {
	out := renderSplashWithNotice(80, 20, DefaultStyles(), "1.2.3", "No active provider configured.")
	if !strings.Contains(out, "No active provider configured") {
		t.Fatalf("notice missing:\n%s", out)
	}
}

func TestRenderSplashHidesWhenTooSmall(t *testing.T) {
	cases := []struct {
		w, h int
	}{
		{20, 20}, // too narrow
		{80, 4},  // too short
		{0, 0},   // pre-layout
	}
	for _, c := range cases {
		if got := renderSplash(c.w, c.h, DefaultStyles(), "1.2.3"); got != "" {
			t.Fatalf("expected empty splash for %dx%d, got:\n%s", c.w, c.h, got)
		}
	}
}

func TestMessagesIsEmpty(t *testing.T) {
	m := NewMessages(80)
	if !m.IsEmpty() {
		t.Fatal("new Messages should be empty")
	}
	m.AppendUser("hi")
	if m.IsEmpty() {
		t.Fatal("Messages with a block should not be empty")
	}
}
