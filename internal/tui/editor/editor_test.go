package editor

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestEditor_EnterSendsText(t *testing.T) {
	e := New(t.TempDir(), nil)
	for _, r := range "hi" {
		e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !res.Send || res.Text != "hi" {
		t.Fatalf("unexpected res: %#v", res)
	}
}

func TestEditor_SlashMenuOpensAndCommitsCommand(t *testing.T) {
	e := New(t.TempDir(), []string{"/model", "/tree"})
	e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if res.SlashCommand == "" {
		t.Fatal("expected SlashCommand result")
	}
	if res.SlashCommand != "/model" && res.SlashCommand != "/tree" {
		t.Fatalf("slash = %q", res.SlashCommand)
	}
}

func TestRowsFor(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"", 1},
		{"hi", 1},
		{"a\nb", 2},
		{"a\nb\nc", 3},
		{strings.Repeat("x\n", 50), maxEditorRows},
	}
	for _, c := range cases {
		if got := rowsFor(c.in); got != c.want {
			t.Errorf("rowsFor(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}
