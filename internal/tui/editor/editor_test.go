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

func TestEditor_ShiftEnterInsertsNewline(t *testing.T) {
	e := New(t.TempDir(), nil)
	for _, r := range "hello" {
		e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	res, _ := e.Update(tea.KeyMsg{Type: tea.KeyCtrlJ})
	if res.Send {
		t.Fatalf("Shift+Enter/Ctrl+J must insert newline, not submit: %#v", res)
	}
	if got := e.ta.Value(); got != "hello\n" {
		t.Fatalf("Shift+Enter/Ctrl+J must preserve existing text and append newline, got %q", got)
	}
	if got := e.Rows(); got != 2 {
		t.Fatalf("Shift+Enter/Ctrl+J should pre-grow editor to show both lines, got %d rows", got)
	}
	for _, r := range "world" {
		e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	res, _ = e.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !res.Send || res.Text != "hello\nworld" {
		t.Fatalf("unexpected res after multiline submit: %#v", res)
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

func TestEditor_ShellModeReflectsPrefix(t *testing.T) {
	cases := []struct {
		typed string
		want  ShellMode
	}{
		{"", ShellModeNone},
		{"hi", ShellModeNone},
		{"!", ShellModeSend},
		{"!ls", ShellModeSend},
		{" !ls", ShellModeSend},
		{"!!", ShellModeInsert},
		{"!!ls", ShellModeInsert},
		{" !!ls", ShellModeInsert},
	}
	for _, c := range cases {
		e := New(t.TempDir(), nil)
		for _, r := range c.typed {
			e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		}
		if got := e.ShellMode(); got != c.want {
			t.Errorf("ShellMode after %q = %v, want %v", c.typed, got, c.want)
		}
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

func TestEditor_BackspacingPrefixClosesOverlay(t *testing.T) {
	e := New(t.TempDir(), []string{"/quit", "/new"})
	e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	if e.Mode() != ModeSlashMenu {
		t.Fatalf("expected ModeSlashMenu after '/', got %v", e.Mode())
	}
	e.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if e.Mode() != ModeNormal {
		t.Fatalf("expected ModeNormal after backspacing '/', got %v", e.Mode())
	}

	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "foo.go"), nil, 0o644)
	e2 := New(dir, nil)
	e2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'@'}})
	if e2.Mode() != ModeFilePicker {
		t.Fatalf("expected ModeFilePicker after '@', got %v", e2.Mode())
	}
	e2.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	if e2.Mode() != ModeNormal {
		t.Fatalf("expected ModeNormal after backspacing '@', got %v", e2.Mode())
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
	const w = 12 // wrapWidth = 12 - promptWidth(2) = 10
	cases := []struct {
		in    string
		width int
		want  int
	}{
		{"", w, 1},
		{"hi", w, 1},
		{"a\nb", w, 2},
		{"a\nb\nc", w, 3},
		{strings.Repeat("x\n", 50), w, 51}, // 50 lines + trailing empty
		{strings.Repeat("x", 10), w, 1},    // exactly fits
		{strings.Repeat("x", 11), w, 2},    // soft-wrap to 2
		{strings.Repeat("x", 30), w, 3},    // 30 / 10 = 3
		{strings.Repeat("x", 100), w, 10},  // 100 / 10 = 10, uncapped
	}
	for _, c := range cases {
		if got := rowsFor(c.in, c.width); got != c.want {
			t.Errorf("rowsFor(%q, %d) = %d, want %d", c.in, c.width, got, c.want)
		}
	}
}

// Regression: typing runes that soft-wrap to a new visual row used to leave
// the textarea's internal viewport scrolled past the top — even after the
// editor grew to fit the content. Pre-growing to maxRows before each KeyMsg
// keeps the textarea from scrolling, so all rows stay visible while the
// content fits within the cap.
func TestEditor_TypingThatWrapsKeepsTopVisible(t *testing.T) {
	e := New(t.TempDir(), nil)
	e.SetWidth(12) // wrapWidth = 10
	e.SetMaxRows(8)
	for _, r := range strings.Repeat("x", 30) { // wraps to 3 rows, fits cap
		e.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if got, want := e.Rows(), 3; got != want {
		t.Fatalf("Rows() = %d, want %d", got, want)
	}
	above, below := e.ScrollState()
	if above != 0 || below != 0 {
		t.Fatalf("expected all content visible, got above=%d below=%d", above, below)
	}
	view := e.View(12)
	if !strings.Contains(view, "xxxxxxxxxx") {
		t.Fatalf("expected first wrapped row visible in view, got:\n%s", view)
	}
}

func TestEditor_RowsCappedByMaxRows(t *testing.T) {
	e := New(t.TempDir(), nil)
	e.SetWidth(12) // wrapWidth = 10
	e.SetMaxRows(8)
	e.ta.SetValue(strings.Repeat("x", 100)) // 10 raw rows
	if got, want := e.Rows(), 8; got != want {
		t.Fatalf("Rows() = %d, want %d (capped)", got, want)
	}
	if got, want := e.RawRows(), 10; got != want {
		t.Fatalf("RawRows() = %d, want %d", got, want)
	}
	above, below := e.ScrollState()
	if above+below == 0 {
		t.Fatalf("expected scroll state with %d hidden rows, got above=%d below=%d", 10-8, above, below)
	}
	if above+below != 2 {
		t.Fatalf("expected total hidden = 2, got above=%d below=%d", above, below)
	}
}
