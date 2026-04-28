// internal/tui/modal/resume_test.go
package modal

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/khang859/rune/internal/session"
)

func TestResume_PicksHighlighted(t *testing.T) {
	items := []session.Summary{
		{ID: "a", Name: "old", Created: time.Now().Add(-time.Hour)},
		{ID: "b", Name: "newer", Created: time.Now()},
	}
	r := NewResume(items)
	_, cmd := r.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd().(ResultMsg)
	if got := msg.Payload.(session.Summary); got.ID != "a" {
		t.Fatalf("payload id = %q", got.ID)
	}
}
