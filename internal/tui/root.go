package tui

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tui/editor"
)

type RootModel struct {
	agent    *agent.Agent
	sess     *session.Session
	styles   Styles
	msgs     *Messages
	viewport viewport.Model
	editor   *editor.Editor
	footer   Footer
	queue    *Queue

	width  int
	height int

	streaming bool
	eventCh   <-chan agent.Event
	cancel    context.CancelFunc

	totalTokens int
}

func NewRootModel(a *agent.Agent, sess *session.Session) *RootModel {
	realCwd, _ := os.Getwd()
	displayCwd := realCwd
	if home, _ := os.UserHomeDir(); home != "" && strings.HasPrefix(realCwd, home) {
		displayCwd = "~" + strings.TrimPrefix(realCwd, home)
	}
	sessLabel := sess.Name
	if sessLabel == "" {
		sessLabel = sess.ID
		if len(sessLabel) > 8 {
			sessLabel = sessLabel[:8]
		}
	}
	cmds := []string{"/quit", "/new", "/copy", "/hotkeys"}
	return &RootModel{
		agent:    a,
		sess:     sess,
		styles:   DefaultStyles(),
		msgs:     NewMessages(80),
		viewport: viewport.New(80, 20),
		editor:   editor.New(realCwd, cmds),
		footer:   Footer{Cwd: displayCwd, Session: sessLabel, Model: sess.Model},
		queue:    &Queue{},
	}
}

func (m *RootModel) Init() tea.Cmd { return nil }

func (m *RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch v := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
		m.layout()
		m.refreshViewport()
		return m, nil

	case AgentEventMsg:
		m.handleEvent(v.Event)
		m.refreshViewport()
		return m, nextEventCmd(m.eventCh)

	case AgentChannelDoneMsg:
		m.streaming = false
		m.eventCh = nil
		m.cancel = nil
		m.editor.Focus()
		if item, ok := m.queue.Pop(); ok {
			m.msgs.AppendUser(item.Text)
			m.refreshViewport()
			return m, m.startTurn(item.Text, item.Images)
		}
		return m, nil

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}

	if k, ok := msg.(tea.KeyMsg); ok {
		if k.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		if m.streaming && k.Type == tea.KeyEsc && m.cancel != nil {
			m.cancel()
			return m, nil
		}
		if !m.streaming {
			switch k.Type {
			case tea.KeyPgUp, tea.KeyPgDown, tea.KeyHome, tea.KeyEnd:
				var cmd tea.Cmd
				m.viewport, cmd = m.viewport.Update(msg)
				return m, cmd
			}
		}
	}

	res, cmd := m.editor.Update(msg)
	if res.Send {
		text := res.Text
		if m.streaming {
			m.queue.Push(QueueItem{Text: text, Images: res.Images})
			m.msgs.OnInfo(fmt.Sprintf("queued (%d in queue)", m.queue.Len()))
			m.refreshViewport()
			return m, cmd
		}
		m.msgs.AppendUser(text)
		m.refreshViewport()
		return m, m.startTurn(text, res.Images)
	}
	if res.RanCommand != "" {
		label := fmt.Sprintf("(ran: %s)", res.RanCommand)
		body := res.InsertText
		if body == "" {
			body = "(no output)"
		}
		m.msgs.OnInfo(label + "\n" + body)
		m.refreshViewport()
	}
	if res.SlashCommand != "" {
		if c := m.handleSlashCommand(res.SlashCommand); c != nil {
			return m, c
		}
	}
	if _, ok := msg.(tea.KeyMsg); ok {
		m.layout()
	}
	return m, cmd
}

func (m *RootModel) startTurn(text string, images []ai.ImageBlock) tea.Cmd {
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.streaming = true
	m.editor.Blur()
	content := []ai.ContentBlock{ai.TextBlock{Text: text}}
	for _, im := range images {
		content = append(content, im)
	}
	msg := ai.Message{Role: ai.RoleUser, Content: content}
	ch := m.agent.Run(ctx, msg)
	m.eventCh = ch
	return nextEventCmd(ch)
}

func (m *RootModel) handleSlashCommand(cmd string) tea.Cmd {
	switch cmd {
	case "/quit":
		return tea.Quit
	case "/new":
		m.msgs = NewMessages(m.width)
		m.refreshViewport()
	case "/hotkeys":
		m.msgs.OnTurnError(fmt.Errorf("(hotkeys list lands in Plan 05)"))
		m.refreshViewport()
	}
	return nil
}

func (m *RootModel) View() string {
	msgArea := m.viewport.View()
	edArea := m.styles.EditorBox.Render(m.editor.View(m.width))
	overlay := ""
	switch m.editor.Mode() {
	case editor.ModeFilePicker:
		overlay = renderList("files", m.editor.FilePicker().Items())
	case editor.ModeSlashMenu:
		overlay = renderList("commands", m.editor.SlashMenu().Items())
	}
	foot := m.footer.Render(m.styles)
	if overlay != "" {
		return msgArea + "\n" + edArea + "\n" + overlay + "\n" + foot
	}
	return msgArea + "\n" + edArea + "\n" + foot
}

func renderList(title string, items []string) string {
	if len(items) == 0 {
		return "(no " + title + ")"
	}
	out := title + ":"
	for i, it := range items {
		if i >= 8 {
			out += "\n  …"
			break
		}
		out += "\n  " + it
	}
	return out
}

func (m *RootModel) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	footerH := 1
	editorRows := m.editor.Rows()
	editorH := editorRows + 2 // border top + bottom
	overlayRows := 0
	if m.editor.Mode() != editor.ModeNormal {
		overlayRows = 9
	}
	msgH := m.height - footerH - editorH - overlayRows
	if msgH < 3 {
		msgH = 3
	}
	m.viewport.Width = m.width
	m.viewport.Height = msgH
	m.editor.SetWidth(m.width - 2)
	m.footer.Width = m.width
	m.msgs.SetWidth(m.width)
}

func (m *RootModel) refreshViewport() {
	atBottom := m.viewport.AtBottom()
	m.viewport.SetContent(m.msgs.Render(m.styles))
	if atBottom {
		m.viewport.GotoBottom()
	}
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
