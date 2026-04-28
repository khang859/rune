package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/khang859/rune/internal/agent"
)

type AgentEventMsg struct{ Event agent.Event }
type AgentChannelDoneMsg struct{}

// nextEventCmd returns a tea.Cmd that reads ONE event from ch.
// If the channel closes, it sends AgentChannelDoneMsg.
func nextEventCmd(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return AgentChannelDoneMsg{}
		}
		return AgentEventMsg{Event: e}
	}
}
