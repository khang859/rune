// internal/tui/modal/tree_test.go
package modal

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/session"
)

func TestTree_PicksNodeID(t *testing.T) {
	s := session.New("gpt-5")
	n1 := s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "a"}}})
	s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "x"}}})
	s.Fork(n1)
	s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "y"}}})

	tr := NewTree(s)
	// Pick whatever is highlighted; just verify a node id flows through.
	_, cmd := tr.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd().(ResultMsg)
	id := msg.Payload.(string)
	if id == "" {
		t.Fatal("empty id")
	}
}

func TestTree_ViewWindowsLongSessions(t *testing.T) {
	s := session.New("gpt-5")
	for i := 0; i < 30; i++ {
		s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "message"}}})
	}

	tr := NewTree(s)
	out := tr.View(80, 12)
	if got := strings.Count(out, "message"); got >= 30 {
		t.Fatalf("expected long tree to be windowed, rendered %d rows", got)
	}
	if !strings.Contains(out, "…") {
		t.Fatalf("expected clipped indicator in output:\n%s", out)
	}
}

func TestForkTree_HasForkSpecificCopy(t *testing.T) {
	s := session.New("gpt-5")
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "a"}}})

	out := NewForkTree(s).View(80, 24)
	if !strings.Contains(out, "Fork From Message") || !strings.Contains(out, "Enter fork here") {
		t.Fatalf("missing fork-specific copy:\n%s", out)
	}
}
