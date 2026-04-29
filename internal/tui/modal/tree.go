// internal/tui/modal/tree.go
package modal

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/session"
)

type Tree struct {
	sess  *session.Session
	flat  []treeRow
	sel   int
	title string
	help  string
}

type treeRow struct {
	Node  *session.Node
	Depth int
}

func NewTree(s *session.Session) Modal {
	return newTree(s, "✦ Conversation Tree ✦", "↑/↓ choose rune · Enter bind · Esc dismiss")
}

func NewForkTree(s *session.Session) Modal {
	return newTree(s, "✦ Fork From Message ✦", "↑/↓ choose source · Enter fork here · Esc cancel")
}

func newTree(s *session.Session, title, help string) Modal {
	t := &Tree{sess: s, title: title, help: help}
	t.flatten(s.Root, 0)
	// Default selection: the node currently active.
	for i, r := range t.flat {
		if r.Node == s.Active {
			t.sel = i
			break
		}
	}
	return t
}

func (t *Tree) flatten(n *session.Node, depth int) {
	if n != nil && n.Parent != nil { // skip root sentinel
		t.flat = append(t.flat, treeRow{Node: n, Depth: depth})
	}
	for _, c := range n.Children {
		t.flatten(c, depth+1)
	}
}

func (t *Tree) Init() tea.Cmd { return nil }

func (t *Tree) Update(msg tea.Msg) (Modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		switch k.Type {
		case tea.KeyUp:
			if t.sel > 0 {
				t.sel--
			}
		case tea.KeyDown:
			if t.sel < len(t.flat)-1 {
				t.sel++
			}
		case tea.KeyEnter:
			if len(t.flat) == 0 {
				return t, Cancel()
			}
			return t, Result(t.flat[t.sel].Node.ID)
		case tea.KeyEsc:
			return t, Cancel()
		}
	}
	return t, nil
}

func (t *Tree) View(width, height int) string {
	rows := make([]choiceRow, len(t.flat))
	for i, r := range t.flat {
		rows[i] = choiceRow{Label: indent(r.Depth) + prefix(r.Node.Message.Role), Value: previewMessage(r.Node.Message)}
	}
	return renderChoiceModal(width, height, t.title, "Branches", t.help, rows, t.sel)
}

func indent(d int) string { return strings.Repeat("  ", d) }

func prefix(role ai.Role) string {
	switch role {
	case ai.RoleUser:
		return "U:"
	case ai.RoleAssistant:
		return "A:"
	case ai.RoleToolResult:
		return "T:"
	}
	return "?:"
}

func previewMessage(m ai.Message) string {
	for _, c := range m.Content {
		if t, ok := c.(ai.TextBlock); ok {
			line := t.Text
			if len(line) > 60 {
				line = line[:60] + "…"
			}
			return strings.ReplaceAll(line, "\n", " ")
		}
		if t, ok := c.(ai.ToolUseBlock); ok {
			return "(tool " + t.Name + ")"
		}
		if t, ok := c.(ai.ToolResultBlock); ok {
			line := t.Output
			if len(line) > 40 {
				line = line[:40] + "…"
			}
			return "(result) " + line
		}
	}
	return ""
}
