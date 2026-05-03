package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

func TestRuneIndexingTextIsThemed(t *testing.T) {
	out := runeIndexingText(0)
	if !strings.Contains(out, "ᚱ") || !strings.Contains(out, "scrying the codebase") {
		t.Fatalf("unexpected indexing text: %q", out)
	}
	if strings.Contains(out, ".") {
		t.Fatalf("indexing text should not include dot animation: %q", out)
	}
}

func TestRoot_IndexingAllowsTypingWhilePrewarming(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.indexing = true

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if got := m.editor.Value(); got != "h" {
		t.Fatalf("editor value while indexing = %q, want h", got)
	}
}

func TestRoot_IndexingKeepsEditorVisible(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.indexing = true
	m.editor.SetValue("hello")

	view := m.View()
	if strings.Contains(view, "hold your incantation") || strings.Contains(view, "indexing AST/graph") {
		t.Fatalf("indexing should not replace the editor while prewarming:\n%s", view)
	}
	if !strings.Contains(view, "hello") {
		t.Fatalf("editor content should remain visible while indexing:\n%s", view)
	}
}

func TestRoot_SplashNoticeShowsIndexing(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.indexing = true
	if got := m.splashNotice(); !strings.Contains(got, "scrying") {
		t.Fatalf("splash notice = %q, want indexing text", got)
	}
}
