package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/session"

	"github.com/charmbracelet/bubbles/viewport"
)

type RootModel struct {
	agent    *agent.Agent
	sess     *session.Session
	styles   Styles
	msgs     *Messages
	viewport viewport.Model
	editor   Editor
	footer   Footer

	width  int
	height int

	streaming bool
	eventCh   <-chan agent.Event
	cancel    context.CancelFunc

	totalTokens int
}

func NewRootModel(a *agent.Agent, sess *session.Session) *RootModel {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	if strings.HasPrefix(cwd, home) {
		cwd = "~" + strings.TrimPrefix(cwd, home)
	}
	return &RootModel{
		agent:    a,
		sess:     sess,
		styles:   DefaultStyles(),
		msgs:     NewMessages(80),
		viewport: viewport.New(80, 20),
		editor:   NewEditor(),
		footer:   Footer{Cwd: cwd, Session: sess.Name, Model: sess.Model},
	}
}

func (m *RootModel) Init() tea.Cmd { return nil }

func (m *RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
		m.layout()
		m.refreshViewport()
		return m, nil

	case tea.KeyMsg:
		// Ctrl+C must always quit, even mid-turn.
		if v.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.streaming {
			if v.Type == tea.KeyEsc && m.cancel != nil {
				m.cancel()
			}
			return m, nil
		}
		switch v.Type {
		case tea.KeyEnter:
			if !v.Alt && v.Type == tea.KeyEnter && !isShiftEnter(v) {
				text := strings.TrimSpace(m.editor.Value())
				if text == "" {
					return m, nil
				}
				m.editor.Reset()
				m.msgs.AppendUser(text)
				m.refreshViewport()
				return m, m.startTurn(text)
			}
		}
		if cmd := m.editor.Update(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
		return m, tea.Batch(cmds...)

	case AgentEventMsg:
		m.handleEvent(v.Event)
		m.refreshViewport()
		return m, nextEventCmd(m.eventCh)

	case AgentChannelDoneMsg:
		m.streaming = false
		m.eventCh = nil
		m.cancel = nil
		m.editor.Focus()
		return m, nil
	}

	if cmd := m.editor.Update(msg); cmd != nil {
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m *RootModel) View() string {
	msgArea := m.viewport.View()
	edArea := m.editor.View(m.styles)
	foot := m.footer.Render(m.styles)
	return msgArea + "\n" + edArea + "\n" + foot
}

func (m *RootModel) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	footerH := 1
	editorH := 5
	msgH := m.height - footerH - editorH
	if msgH < 3 {
		msgH = 3
	}
	m.viewport.Width = m.width
	m.viewport.Height = msgH
	m.editor.SetWidth(m.width - 2)
	m.editor.SetHeight(editorH - 2)
	m.footer.Width = m.width
	m.msgs.SetWidth(m.width)
}

func (m *RootModel) refreshViewport() {
	m.viewport.SetContent(m.msgs.Render(m.styles))
	m.viewport.GotoBottom()
}

func (m *RootModel) startTurn(text string) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.streaming = true
	m.editor.Blur()
	msg := ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}}
	ch := m.agent.Run(ctx, msg)
	m.eventCh = ch
	return nextEventCmd(ch)
}

func (m *RootModel) handleEvent(e agent.Event) {
	switch v := e.(type) {
	case agent.AssistantText:
		m.msgs.OnAssistantDelta(v.Delta)
	case agent.ThinkingText:
		m.msgs.OnThinkingDelta(v.Delta)
	case agent.ToolStarted:
		m.msgs.OnToolStarted(v.Call)
	case agent.ToolFinished:
		m.msgs.OnToolFinished(v)
	case agent.TurnUsage:
		m.totalTokens += v.Usage.Input + v.Usage.Output
		m.footer.Tokens = m.totalTokens
		m.footer.ContextPct = ctxPct(m.totalTokens)
	case agent.ContextOverflow:
		m.msgs.OnTurnError(fmt.Errorf("context overflow — manual /compact recommended"))
	case agent.TurnError:
		m.msgs.OnTurnError(v.Err)
	case agent.TurnAborted:
		m.msgs.OnTurnError(fmt.Errorf("(aborted)"))
	case agent.TurnDone:
		m.msgs.OnTurnDone(v.Reason)
	}
}

func ctxPct(tokens int) int {
	const cap = 200000
	p := tokens * 100 / cap
	if p > 100 {
		return 100
	}
	return p
}

func isShiftEnter(k tea.KeyMsg) bool {
	s := k.String()
	return s == "shift+enter"
}
