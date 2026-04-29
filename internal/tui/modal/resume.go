// internal/tui/modal/resume.go
package modal

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/khang859/rune/internal/session"
)

type Resume struct {
	items []session.Summary
	sel   int
}

func NewResume(items []session.Summary) Modal {
	return &Resume{items: items}
}

func (r *Resume) Init() tea.Cmd { return nil }

func (r *Resume) Update(msg tea.Msg) (Modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyUp:
			if r.sel > 0 {
				r.sel--
			}
		case tea.KeyDown:
			if r.sel < len(r.items)-1 {
				r.sel++
			}
		case tea.KeyEnter:
			if len(r.items) == 0 {
				return r, Cancel()
			}
			return r, Result(r.items[r.sel])
		case tea.KeyEsc:
			return r, Cancel()
		}
	}
	return r, nil
}

func (r *Resume) View(width, height int) string {
	if len(r.items) == 0 {
		return renderChoiceModal(width, height, "✦ Resume Session ✦", "Saved Sessions", "Esc dismiss", []choiceRow{{Label: "(no saved sessions)"}}, -1)
	}
	rows := make([]choiceRow, len(r.items))
	for i, it := range r.items {
		name := it.Name
		if name == "" {
			name = "(unnamed)"
		}
		rows[i] = choiceRow{Label: name, Value: fmt.Sprintf("%d msgs — %s", it.MessageCount, it.Created.Format("2006-01-02 15:04"))}
	}
	return renderChoiceModal(width, height, "✦ Resume Session ✦", "Saved Sessions", "↑/↓ choose rune · Enter bind · Esc dismiss", rows, r.sel)
}
