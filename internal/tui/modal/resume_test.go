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

func TestResume_FirstPageShowsLatestSeven(t *testing.T) {
	items := makeResumeSummaries(12)
	r := NewResume(items)

	view := r.View(100, 40)
	for i := 0; i < 7; i++ {
		if !strings.Contains(view, fmt.Sprintf("session-%02d", i)) {
			t.Fatalf("first page missing session-%02d:\n%s", i, view)
		}
	}
	for i := 7; i < 12; i++ {
		if strings.Contains(view, fmt.Sprintf("session-%02d", i)) {
			t.Fatalf("first page should not contain session-%02d:\n%s", i, view)
		}
	}
	if !strings.Contains(view, "Page 1/2") {
		t.Fatalf("missing page indicator in view:\n%s", view)
	}
}

func TestResume_PageDownShowsNextSevenAndSelectsUnderlyingSession(t *testing.T) {
	items := makeResumeSummaries(23)
	r := NewResume(items)

	_, _ = r.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	view := r.View(100, 40)
	for i := 7; i < 14; i++ {
		if !strings.Contains(view, fmt.Sprintf("session-%02d", i)) {
			t.Fatalf("second page missing session-%02d:\n%s", i, view)
		}
	}
	if strings.Contains(view, "session-06") || strings.Contains(view, "session-14") {
		t.Fatalf("second page contains item from another page:\n%s", view)
	}
	if !strings.Contains(view, "Page 2/4") {
		t.Fatalf("missing second page indicator in view:\n%s", view)
	}

	_, cmd := r.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd().(ResultMsg)
	if got := msg.Payload.(session.Summary); got.ID != "id-07" {
		t.Fatalf("payload id = %q, want id-07", got.ID)
	}
}

func TestResume_ShowsUnnamedLabelPreviewModelAndUpdatedTime(t *testing.T) {
	updated := time.Date(2026, 4, 30, 9, 8, 0, 0, time.UTC)
	items := []session.Summary{{
		ID:           "a",
		Preview:      "fix resume session list",
		Updated:      updated,
		MessageCount: 3,
		Model:        "gpt-5",
	}}
	r := NewResume(items)

	view := r.View(120, 30)
	for _, want := range []string{"(unnamed)", "fix resume session list", "3 msgs", "gpt-5", "updated 2026-04-30 09:08"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestResume_TruncatesLongPreview(t *testing.T) {
	items := []session.Summary{{
		ID:           "a",
		Preview:      "this preview is intentionally very long and should not wrap into multiple lines because wrapping makes the resume modal hard to scan",
		Updated:      time.Date(2026, 4, 30, 9, 8, 0, 0, time.UTC),
		MessageCount: 3,
		Model:        "gpt-5",
	}}
	r := NewResume(items)

	view := r.View(70, 30)
	if strings.Contains(view, "because wrapping") {
		t.Fatalf("long preview was not truncated:\n%s", view)
	}
	if !strings.Contains(view, "…") {
		t.Fatalf("expected ellipsis for truncated preview:\n%s", view)
	}
}

func TestResume_LastPartialPageClampsSelection(t *testing.T) {
	items := makeResumeSummaries(23)
	r := NewResume(items)

	// Move to offset 6 on page 1, then page down three times. The final page has
	// two items, so selection should clamp to its last item rather than pointing
	// outside the visible page.
	for i := 0; i < 6; i++ {
		_, _ = r.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	_, _ = r.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	_, _ = r.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	_, _ = r.Update(tea.KeyMsg{Type: tea.KeyPgDown})

	view := r.View(100, 40)
	if !strings.Contains(view, "Page 4/4") {
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
			Updated: base.Add(-time.Duration(i) * time.Minute),
			Model:   "gpt-5",
		}
	}
	return items
}
