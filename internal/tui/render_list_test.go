package tui

import (
	"strings"
	"testing"
)

func TestRenderList_MarksSelectedRow(t *testing.T) {
	out := renderList("commands", []string{"/a", "/b", "/c"}, 1)
	if !strings.Contains(out, "> /b") {
		t.Fatalf("expected '> /b' marker for selected row, got:\n%s", out)
	}
	if strings.Contains(out, "> /a") || strings.Contains(out, "> /c") {
		t.Fatalf("only selected row should be marked, got:\n%s", out)
	}
}

func TestRenderList_ScrollsWindowToKeepSelectionVisible(t *testing.T) {
	items := make([]string, 12)
	for i := range items {
		items[i] = string(rune('a' + i))
	}
	out := renderList("files", items, 10)
	// First 3 entries should be hidden (window of 8 ending at sel=10).
	for _, hidden := range []string{"  a\n", "  b\n", "  c\n"} {
		if strings.Contains(out, hidden) {
			t.Fatalf("expected %q to be scrolled out of view, got:\n%s", hidden, out)
		}
	}
	if !strings.Contains(out, "> k") {
		t.Fatalf("expected '> k' (sel=10) in window, got:\n%s", out)
	}
	if !strings.HasPrefix(strings.TrimPrefix(out, "files:\n"), "  …") {
		t.Fatalf("expected leading … indicator when scrolled, got:\n%s", out)
	}
}

func TestRenderList_EmptyShowsPlaceholder(t *testing.T) {
	if got := renderList("files", nil, 0); got != "(no files)" {
		t.Fatalf("unexpected empty render: %q", got)
	}
}
