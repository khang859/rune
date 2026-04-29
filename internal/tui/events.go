package tui

import (
	tea "github.com/charmbracelet/bubbletea"

	"github.com/khang859/rune/internal/agent"
)

// AgentEventMsg carries one streamed event plus the channel it came from,
// so the root model can drop messages from a swapped-out session.
type AgentEventMsg struct {
	Event agent.Event
	Ch    <-chan agent.Event
}

// AgentChannelDoneMsg fires when the agent's event channel closes. Ch
// identifies which channel finished, so a stale "done" from a previously
// swapped-out session does not pop the queue on the new session.
type AgentChannelDoneMsg struct {
	Ch <-chan agent.Event
}

type SubagentEventMsg struct {
	Event agent.SubagentEvent
	Ch    <-chan agent.SubagentEvent
}

type SubagentChannelDoneMsg struct {
	Ch <-chan agent.SubagentEvent
}

// nextEventCmd returns a tea.Cmd that reads ONE event from ch.
// If the channel closes, it sends AgentChannelDoneMsg.
func nextEventCmd(ch <-chan agent.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return AgentChannelDoneMsg{Ch: ch}
		}
		return AgentEventMsg{Event: e, Ch: ch}
	}
}

func nextSubagentEventCmd(ch <-chan agent.SubagentEvent) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return SubagentChannelDoneMsg{Ch: ch}
		}
		return SubagentEventMsg{Event: e, Ch: ch}
	}
}
