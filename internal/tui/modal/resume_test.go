// internal/tui/modal/resume_test.go
package modal

import (
	"fmt"
	"strings"
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

func TestResume_FirstPageShowsLatestTen(t *testing.T) {
	items := makeResumeSummaries(12)
	r := NewResume(items)

	view := r.View(100, 40)
	for i := 0; i < 10; i++ {
		if !strings.Contains(view, fmt.Sprintf("session-%02d", i)) {
			t.Fatalf("first page missing session-%02d:\n%s", i, view)
		}
	}
	for i := 10; i < 12; i++ {
		if strings.Contains(view, fmt.Sprintf("session-%02d", i)) {
			t.Fatalf("first page should not contain session-%02d:\n%s", i, view)
		}
	}
	if !strings.Contains(view, "Page 1/2") {
		t.Fatalf("missing page indicator in view:\n%s", view)
	}
}

func TestResume_PageDownShowsNextTenAndSelectsUnderlyingSession(t *testing.T) {
	items := makeResumeSummaries(23)
	r := NewResume(items)

	_, _ = r.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	view := r.View(100, 40)
	for i := 10; i < 20; i++ {
		if !strings.Contains(view, fmt.Sprintf("session-%02d", i)) {
			t.Fatalf("second page missing session-%02d:\n%s", i, view)
		}
	}
	if strings.Contains(view, "session-09") || strings.Contains(view, "session-20") {
		t.Fatalf("second page contains item from another page:\n%s", view)
	}
	if !strings.Contains(view, "Page 2/3") {
		t.Fatalf("missing second page indicator in view:\n%s", view)
	}

	_, cmd := r.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd().(ResultMsg)
	if got := msg.Payload.(session.Summary); got.ID != "id-10" {
		t.Fatalf("payload id = %q, want id-10", got.ID)
	}
}

func TestResume_LastPartialPageClampsSelection(t *testing.T) {
	items := makeResumeSummaries(23)
	r := NewResume(items)

	// Move to offset 9 on page 1, then page down twice. The final page has only
	// three items, so selection should clamp to its last item rather than
	// pointing outside the visible page.
	for i := 0; i < 9; i++ {
		_, _ = r.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	_, _ = r.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	_, _ = r.Update(tea.KeyMsg{Type: tea.KeyPgDown})

	view := r.View(100, 40)
	if !strings.Contains(view, "Page 3/3") {
		t.Fatalf("missing final page indicator in view:\n%s", view)
	}
	_, cmd := r.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd().(ResultMsg)
	if got := msg.Payload.(session.Summary); got.ID != "id-22" {
		t.Fatalf("payload id = %q, want id-22", got.ID)
	}
}

func makeResumeSummaries(n int) []session.Summary {
	base := time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC)
	items := make([]session.Summary, n)
	for i := range items {
		items[i] = session.Summary{
			ID:      fmt.Sprintf("id-%02d", i),
			Name:    fmt.Sprintf("session-%02d", i),
			Created: base.Add(-time.Duration(i) * time.Minute),
		}
	}
	return items
}
