package editor

import (
	"os"
	"path/filepath"
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

func TestEditor_EmptyEnterDoesNotSend(t *testing.T) {
	e := New(t.TempDir(), nil)
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if res.Send {
		t.Fatalf("empty Enter must not send: %#v", res)
	}
	if res.Text != "" {
		t.Fatalf("expected empty Text, got %q", res.Text)
	}
}

func TestEditor_DoubleBangDoesNotSend(t *testing.T) {
	e := New(t.TempDir(), nil)
	for _, r := range "!!echo rune-bang-test" {
		e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if res.Send {
		t.Fatalf("!!cmd must not Send: %#v", res)
	}
	if res.RanCommand != "echo rune-bang-test" {
		t.Fatalf("expected RanCommand=echo rune-bang-test, got %q", res.RanCommand)
	}
	if !strings.Contains(res.InsertText, "rune-bang-test") {
		t.Fatalf("expected InsertText to contain marker, got %q", res.InsertText)
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

func TestEditor_BangCommandRunsAndSends(t *testing.T) {
	e := New(t.TempDir(), nil)
	for _, r := range "!echo rune-test-marker" {
		e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !res.Send {
		t.Fatalf("expected Send=true, got %#v", res)
	}
	if !strings.Contains(res.Text, "rune-test-marker") {
		t.Fatalf("expected output to contain marker, got %q", res.Text)
	}
}

func TestEditor_AtOverlayOpensAndEscCloses(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "foo.go"), nil, 0o644)
	e := New(dir, nil)
	e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
	if e.Mode() != ModeFilePicker {
		t.Fatalf("expected ModeFilePicker after @, got %v", e.Mode())
	}
	e.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if e.Mode() != ModeNormal {
		t.Fatalf("expected ModeNormal after Esc, got %v", e.Mode())
	}
}

func TestEditor_TabCompletesUniquePath(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "alpha.go"), nil, 0o644)
	e := New(dir, nil)
	for _, r := range "alp" {
		e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	e.Update(tea.KeyMsg{Type: tea.KeyTab})
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if res.Text != "alpha.go" {
		t.Fatalf("expected textarea to expand 'alp' -> 'alpha.go'; submit text = %q", res.Text)
	}
}

func TestEditor_SlashNoMatchSubmitsAsText(t *testing.T) {
	e := New(t.TempDir(), []string{"/quit", "/new"})
	for _, r := range "/zzzzz" {
		e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !res.Send {
		t.Fatalf("expected Send=true (slash menu had no match, should fall through to submit), got %#v", res)
	}
	if res.Text != "/zzzzz" {
		t.Fatalf("expected text /zzzzz, got %q", res.Text)
	}
	if res.SlashCommand != "" {
		t.Fatalf("expected empty SlashCommand, got %q", res.SlashCommand)
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
