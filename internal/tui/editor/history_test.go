package editor

import (
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestHistory_PushDedupAndCap(t *testing.T) {
	h := NewHistory("")
	h.Push("a")
	h.Push("a") // dedup adjacent
	h.Push("b")
	if got := h.entries; len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("entries = %v", got)
	}
}

func TestHistory_PrevNextRestoresDraft(t *testing.T) {
	h := NewHistory("")
	h.Push("first")
	h.Push("second")

	got, ok := h.Prev("draft")
	if !ok || got != "second" {
		t.Fatalf("Prev1 = (%q, %v)", got, ok)
	}
	got, ok = h.Prev("ignored")
	if !ok || got != "first" {
		t.Fatalf("Prev2 = (%q, %v)", got, ok)
	}
	// Already at oldest — Prev should stay there.
	got, ok = h.Prev("ignored")
	if !ok || got != "first" {
		t.Fatalf("Prev3 = (%q, %v)", got, ok)
	}

	got, ok = h.Next()
	if !ok || got != "second" {
		t.Fatalf("Next1 = (%q, %v)", got, ok)
	}
	got, ok = h.Next()
	if !ok || got != "draft" {
		t.Fatalf("Next2 (draft restore) = (%q, %v)", got, ok)
	}
	if h.Navigating() {
		t.Fatalf("expected nav cleared after draft restore")
	}
}

func TestHistory_PersistAcrossInstances(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history")
	h := NewHistory(path)
	h.Push("one")
	h.Push("two")

	h2 := NewHistory(path)
	if got := h2.entries; len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("reloaded entries = %v", got)
	}
}

func TestEditor_ArrowUpRecallsLastPrompt(t *testing.T) {
	e := New(t.TempDir(), nil)
	e.SetHistory(NewHistory(""))
	for _, r := range "hello" {
		e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !res.Send || res.Text != "hello" {
		t.Fatalf("submit res = %#v", res)
	}
	// Editor was reset; Up should bring "hello" back.
	e.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := e.ta.Value(); got != "hello" {
		t.Fatalf("after Up, value = %q, want %q", got, "hello")
	}
}

// History navigation should not fire when the editor already has typed
// content — otherwise Up would destroy the draft. Once already navigating
// (entered via Up from an empty input), continued Up/Down stays in history.
func TestEditor_ArrowUpPreservesDraftWhenContentPresent(t *testing.T) {
	e := New(t.TempDir(), nil)
	e.SetHistory(NewHistory(""))
	for _, r := range "old" {
		e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	for _, r := range "draft" {
		e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	e.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := e.ta.Value(); got != "draft" {
		t.Fatalf("Up with content present must not navigate history; value = %q", got)
	}
}

func TestEditor_NoHistoryNoNav(t *testing.T) {
	e := New(t.TempDir(), nil)
	// No SetHistory — Up should fall through to textarea (no panic, no value
	// change since we're already on row 0 of an empty buffer).
	e.Update(tea.KeyMsg{Type: tea.KeyUp})
	if got := e.ta.Value(); got != "" {
		t.Fatalf("value = %q, want empty", got)
	}
}
