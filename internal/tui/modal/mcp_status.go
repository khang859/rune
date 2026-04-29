package modal

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/khang859/rune/internal/mcp"
)

type MCPStatus struct {
	statuses []mcp.Status
}

func NewMCPStatus(statuses []mcp.Status) Modal {
	copied := make([]mcp.Status, len(statuses))
	for i, st := range statuses {
		copied[i] = st
		copied[i].Tools = append([]string(nil), st.Tools...)
	}
	return MCPStatus{statuses: copied}
}

func (MCPStatus) Init() tea.Cmd { return nil }

func (m MCPStatus) Update(msg tea.Msg) (Modal, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		return m, Cancel()
	}
	return m, nil
}

func (m MCPStatus) View(width, height int) string {
	if len(m.statuses) == 0 {
		return "MCP status:\n\n  No MCP servers configured or loaded.\n\n(any key to close)"
	}

	var sb strings.Builder
	sb.WriteString("MCP status:\n")
	for _, st := range m.statuses {
		state := "disconnected"
		mark := "✗"
		if st.Connected {
			state = "connected"
			mark = "✓"
		}
		fmt.Fprintf(&sb, "\n%s %s  %s", mark, st.Name, state)
		if st.ToolCount > 0 {
			fmt.Fprintf(&sb, "  (%d tools)", st.ToolCount)
		}
		if st.Type != "" {
			fmt.Fprintf(&sb, "\n  type: %s", st.Type)
		}
		if st.Description != "" {
			fmt.Fprintf(&sb, "\n  config: %s", st.Description)
		}
		if st.Error != "" {
			fmt.Fprintf(&sb, "\n  error: %s", st.Error)
		}
		if len(st.Tools) > 0 {
			fmt.Fprintf(&sb, "\n  tools: %s", strings.Join(st.Tools, ", "))
		}
		sb.WriteByte('\n')
	}
	sb.WriteString("\n(any key to close)")
	return sb.String()
}
