package main

import (
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/mcp"
	"github.com/khang859/rune/internal/tui/modal"
)

type mcpWizardProgram struct {
	modal modal.Modal
	out   io.Writer
	saved bool
	err   error
}

func newMCPWizardProgram(out io.Writer) *mcpWizardProgram {
	return &mcpWizardProgram{modal: modal.NewMCPWizard(), out: out}
}

func (m *mcpWizardProgram) Init() tea.Cmd { return m.modal.Init() }

func (m *mcpWizardProgram) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case modal.ResultMsg:
		if v.Cancel {
			return m, tea.Quit
		}
		res, ok := v.Payload.(modal.MCPWizardResult)
		if !ok {
			m.err = fmt.Errorf("unexpected wizard result %T", v.Payload)
			return m, tea.Quit
		}
		if err := mcp.AddServer(config.MCPConfig(), res.Name, res.Config, false); err != nil {
			m.err = err
			return m, tea.Quit
		}
		m.saved = true
		_, _ = fmt.Fprintf(m.out, "added MCP server %q; restart rune to load its tools\n", res.Name)
		return m, tea.Quit
	}
	var cmd tea.Cmd
	m.modal, cmd = m.modal.Update(msg)
	return m, cmd
}

func (m *mcpWizardProgram) View() string {
	if m.err != nil {
		return "error: " + m.err.Error() + "\n"
	}
	if m.saved {
		return ""
	}
	return m.modal.View(80, 24)
}

func runMCPWizard(stdout io.Writer) error {
	model := newMCPWizardProgram(stdout)
	finalModel, err := tea.NewProgram(model).Run()
	if err != nil {
		return err
	}
	if m, ok := finalModel.(*mcpWizardProgram); ok && m.err != nil {
		return m.err
	}
	return nil
}
