package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

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

func TestRoot_IndexingEditorBoxStaysFullWidth(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.indexing = true

	lines := strings.Split(m.View(), "\n")
	var found bool
	for _, line := range lines {
		if strings.Contains(line, "indexing AST/graph") {
			found = true
			if got, want := lipgloss.Width(line), 80; got != want {
				t.Fatalf("indexing editor width = %d, want %d; line %q", got, want, line)
			}
		}
	}
	if !found {
		t.Fatalf("missing indexing editor line:\n%s", m.View())
	}
}

func TestRoot_IndexingBlocksTypingUntilDone(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.indexing = true
	m.editor.Blur()

	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if got := m.editor.Value(); got != "" {
		t.Fatalf("editor accepted input while indexing: %q", got)
	}

	m.Update(codeIndexDoneMsg{})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	if got := m.editor.Value(); got != "h" {
		t.Fatalf("editor value after indexing = %q, want h", got)
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
