// internal/tui/modal/tree_test.go
package modal

import (
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
