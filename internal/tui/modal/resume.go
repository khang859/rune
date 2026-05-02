// internal/tui/modal/resume.go
package modal

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/khang859/rune/internal/session"
)

const resumePageSize = 7

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
		case tea.KeyPgUp, tea.KeyLeft:
			r.prevPage()
		case tea.KeyPgDown, tea.KeyRight:
			r.nextPage()
		case tea.KeyHome:
			r.sel = 0
		case tea.KeyEnd:
			if len(r.items) > 0 {
				r.sel = len(r.items) - 1
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
	start, end := r.pageBounds()
	rows := make([]choiceRow, end-start)
	for i, it := range r.items[start:end] {
		rows[i] = choiceRow{Label: resumeLabel(it), Value: resumeValue(it), Detail: resumeDetail(it)}
	}
	page, pages := r.pageInfo()
	help := fmt.Sprintf("Page %d/%d · ↑/↓ choose rune · PgUp/PgDown page · Enter bind · Esc dismiss", page, pages)
	return renderChoiceModal(width, height, "✦ Resume Session ✦", "Saved Sessions", help, rows, r.sel-start)
}

func resumeLabel(it session.Summary) string {
	if it.Name != "" {
		return it.Name
	}
	return "(unnamed)"
}

func resumeValue(it session.Summary) string {
	ts := it.Updated
	label := "updated"
	if ts.IsZero() {
		ts = it.Created
		label = "created"
	}
	model := it.Model
	if model == "" {
		model = "unknown model"
	}
	return fmt.Sprintf("%d msgs — %s — %s %s", it.MessageCount, model, label, ts.Format("2006-01-02 15:04"))
}

func resumeDetail(it session.Summary) string {
	return it.Preview
}

func (r *Resume) pageBounds() (int, int) {
	if len(r.items) == 0 {
		return 0, 0
	}
	if r.sel < 0 {
		r.sel = 0
	}
	if r.sel >= len(r.items) {
		r.sel = len(r.items) - 1
	}
	start := (r.sel / resumePageSize) * resumePageSize
	end := start + resumePageSize
	if end > len(r.items) {
		end = len(r.items)
	}
	return start, end
}

func (r *Resume) pageInfo() (int, int) {
	if len(r.items) == 0 {
		return 0, 0
	}
	pages := (len(r.items) + resumePageSize - 1) / resumePageSize
	page := r.sel/resumePageSize + 1
	return page, pages
}

func (r *Resume) prevPage() {
	if r.sel <= 0 {
		return
	}
	offset := r.sel % resumePageSize
	r.sel -= resumePageSize
	if r.sel < 0 {
		r.sel = 0
		return
	}
	pageStart := (r.sel / resumePageSize) * resumePageSize
	pageEnd := pageStart + resumePageSize
	if pageEnd > len(r.items) {
		pageEnd = len(r.items)
	}
	if pageStart+offset >= pageEnd {
		r.sel = pageEnd - 1
	}
}

func (r *Resume) nextPage() {
	if len(r.items) == 0 {
		return
	}
	offset := r.sel % resumePageSize
	currentStart := (r.sel / resumePageSize) * resumePageSize
	pageStart := currentStart + resumePageSize
	if pageStart >= len(r.items) {
		return
	}
	pageEnd := pageStart + resumePageSize
	if pageEnd > len(r.items) {
		pageEnd = len(r.items)
	}
	r.sel = pageStart + offset
	if r.sel >= pageEnd {
		r.sel = pageEnd - 1
	}
}
