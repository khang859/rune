package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/khang859/rune/internal/config"
)

func (m *RootModel) maybeStartMemoryUpdate() tea.Cmd {
	if m.memoryWriting || m.streaming || m.compacting || m.agent == nil || !m.agent.MemoryEnabled() || m.agent.MemoryStore() == nil {
		return nil
	}
	nodes := m.sess.PathToActiveNodes()
	if len(nodes) < 2 {
		return nil
	}
	nodeID := nodes[len(nodes)-1].ID
	if nodeID == "" || nodeID == m.memoryLastNode {
		return nil
	}
	history := m.sess.PathToActive()
	m.memoryWriting = true
	m.memoryLastNode = nodeID
	sess := m.sess
	agent := m.agent
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		updated, changed, err := agent.ExtractMemoryUpdate(ctx, history)
		return memoryUpdateDoneMsg{sess: sess, nodeID: nodeID, content: updated, changed: changed, err: err}
	}
}

func (m *RootModel) handleMemoryCommand(arg string) {
	arg = strings.TrimSpace(arg)
	store := m.agent.MemoryStore()
	if store == nil {
		m.msgs.OnInfo("(memory unavailable)")
		return
	}
	switch arg {
	case "", "status":
		state := "off"
		if m.agent.MemoryEnabled() {
			state = "on"
		}
		m.msgs.OnInfo(fmt.Sprintf("(memory: %s, path=%s)", state, store.Path()))
	case "show":
		content, err := store.Load()
		if err != nil {
			m.msgs.OnTurnError(fmt.Errorf("memory: %v", err))
			return
		}
		if strings.TrimSpace(content) == "" {
			m.msgs.OnInfo("(memory is empty)")
			return
		}
		m.msgs.OnInfo("project memory:\n" + content)
	case "path":
		m.msgs.OnInfo(store.Path())
	case "on":
		m.agent.SetMemoryEnabled(true)
		m.setAutoMemorySetting(true)
		m.msgs.OnInfo("(memory enabled)")
	case "off":
		m.agent.SetMemoryEnabled(false)
		m.setAutoMemorySetting(false)
		m.msgs.OnInfo("(memory disabled)")
	default:
		m.msgs.OnInfo("(usage: /memory [status|show|path|on|off])")
	}
}

func (m *RootModel) setAutoMemorySetting(enabled bool) {
	s, err := config.LoadSettings(config.SettingsPath())
	if err != nil {
		return
	}
	s.AutoMemory.Enabled = boolPtr(enabled)
	_ = config.SaveSettings(config.SettingsPath(), s)
}
