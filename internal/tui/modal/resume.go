// internal/tui/modal/resume.go
package modal

import (
	"fmt"
	"strings"

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
		return "(no saved sessions)"
	}
	var sb strings.Builder
	sb.WriteString("Resume session (↑/↓, Enter, Esc):\n")
	for i, it := range r.items {
		marker := "  "
		if i == r.sel {
			marker = "> "
		}
		name := it.Name
		if name == "" {
			name = "(unnamed)"
		}
		sb.WriteString(fmt.Sprintf("%s%s — %d msgs — %s\n", marker, name, it.MessageCount, it.Created.Format("2006-01-02 15:04")))
	}
	return sb.String()
}
