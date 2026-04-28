// internal/tui/modal/tree.go
package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/session"
)

type Tree struct {
	sess *session.Session
	flat []treeRow
	sel  int
}

type treeRow struct {
	Node  *session.Node
	Depth int
}

func NewTree(s *session.Session) Modal {
	t := &Tree{sess: s}
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
	var sb strings.Builder
	sb.WriteString("Pick a node to continue from (↑/↓, Enter, Esc):\n")
	for i, r := range t.flat {
		marker := "  "
		if i == t.sel {
			marker = "> "
		}
		snippet := previewMessage(r.Node.Message)
		sb.WriteString(fmt.Sprintf("%s%s%s %s\n", marker, indent(r.Depth), prefix(r.Node.Message.Role), snippet))
	}
	return sb.String()
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
