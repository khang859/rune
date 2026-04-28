package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"golang.design/x/clipboard"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tui/editor"
	"github.com/khang859/rune/internal/tui/modal"
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

	modal           modal.Modal
	settings        modal.Settings
	pendingForkMode bool
	clipboardReady  bool
	clipboardErr    error
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
	cmds := []string{
		"/quit", "/model", "/tree", "/resume", "/settings",
		"/new", "/name", "/session", "/fork", "/clone", "/copy",
		"/compact", "/reload", "/hotkeys",
	}
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
	case compactDoneMsg:
		m.rebuildMessagesFromSession()
		return m, nil

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

	case modal.ResultMsg:
		cur := m.modal
		m.modal = nil
		if v.Cancel {
			m.layout()
			return m, nil
		}
		cmd := m.applyModalResult(cur, v.Payload)
		m.layout()
		return m, cmd
	}

	if m.modal != nil {
		// Quit shortcut still works while a modal is open.
		if k, ok := msg.(tea.KeyMsg); ok && k.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}
		next, cmd := m.modal.Update(msg)
		m.modal = next
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
	case "/model":
		m.modal = modal.NewModelPicker([]string{"gpt-5", "gpt-5-codex", "gpt-5.1-codex-mini"}, m.sess.Model)
	case "/tree":
		m.modal = modal.NewTree(m.sess)
	case "/resume":
		items, _ := session.ListSessions(config.SessionsDir())
		m.modal = modal.NewResume(items)
	case "/settings":
		m.modal = modal.NewSettings(m.settings)
	case "/hotkeys":
		m.modal = modal.NewHotkeys()
	case "/new":
		m.startNewSession()
	case "/name":
		m.msgs.OnTurnError(fmt.Errorf("(use /settings or future inline prompt)"))
	case "/session":
		m.msgs.OnTurnError(fmt.Errorf("session id=%s name=%q model=%s", m.sess.ID, m.sess.Name, m.sess.Model))
	case "/fork":
		m.modal = modal.NewTree(m.sess)
		m.pendingForkMode = true
	case "/clone":
		nc := m.sess.Clone()
		nc.SetPath(filepath.Join(config.SessionsDir(), nc.ID+".json"))
		_ = nc.Save()
		m.swapSession(nc)
	case "/copy":
		last := lastAssistantText(m.sess)
		if last != "" {
			m.copyToClipboard(last)
		}
	case "/compact":
		return m.startCompact()
	case "/reload":
		m.refreshSystemPrompt()
	}
	m.layout()
	return nil
}

func (m *RootModel) View() string {
	if m.modal != nil {
		return m.modal.View(m.width, m.height)
	}
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
	// EditorBox renders with rounded border (2 cols) + horizontal padding (2 cols);
	// reserve those so the editor never overflows the terminal width.
	m.editor.SetWidth(m.width - 4)
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

func (m *RootModel) applyModalResult(cur modal.Modal, payload any) tea.Cmd {
	switch cur.(type) {
	case *modal.ModelPicker:
		if id, ok := payload.(string); ok {
			m.sess.Model = id
			m.footer.Model = id
		}
	case *modal.SettingsModal:
		if s, ok := payload.(modal.Settings); ok {
			m.settings = s
		}
	case *modal.Resume:
		if sum, ok := payload.(session.Summary); ok {
			ns, err := session.Load(sum.Path)
			if err == nil {
				m.swapSession(ns)
			}
		}
	case *modal.Tree:
		if id, ok := payload.(string); ok {
			target := findNode(m.sess.Root, id)
			if target != nil {
				if m.pendingForkMode {
					m.sess.Fork(target)
					m.pendingForkMode = false
				} else {
					m.sess.Active = target
				}
				m.rebuildMessagesFromSession()
			}
		}
	}
	return nil
}

func findNode(n *session.Node, id string) *session.Node {
	if n == nil {
		return nil
	}
	if n.ID == id {
		return n
	}
	for _, c := range n.Children {
		if r := findNode(c, id); r != nil {
			return r
		}
	}
	return nil
}

func lastAssistantText(s *session.Session) string {
	for n := s.Active; n != nil && n.Parent != nil; n = n.Parent {
		if n.Message.Role == ai.RoleAssistant {
			for _, c := range n.Message.Content {
				if t, ok := c.(ai.TextBlock); ok {
					return t.Text
				}
			}
		}
	}
	return ""
}

func (m *RootModel) startNewSession() {
	nc := session.New(m.sess.Model)
	nc.SetPath(filepath.Join(config.SessionsDir(), nc.ID+".json"))
	m.swapSession(nc)
}

func (m *RootModel) swapSession(s *session.Session) {
	m.sess = s
	m.footer.Session = s.Name
	m.footer.Model = s.Model
	m.rebuildMessagesFromSession()
	m.agent = agent.New(m.agent.Provider(), m.agent.Tools(), s, m.agent.System())
}

func (m *RootModel) rebuildMessagesFromSession() {
	m.msgs = NewMessages(m.width)
	for _, msg := range m.sess.PathToActive() {
		switch msg.Role {
		case ai.RoleUser:
			for _, c := range msg.Content {
				if t, ok := c.(ai.TextBlock); ok {
					m.msgs.AppendUser(t.Text)
				}
			}
		case ai.RoleAssistant:
			for _, c := range msg.Content {
				if t, ok := c.(ai.TextBlock); ok {
					m.msgs.OnAssistantDelta(t.Text)
				}
			}
			m.msgs.OnTurnDone("stop")
		}
	}
	m.refreshViewport()
}

func (m *RootModel) startCompact() tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		_ = m.agent.Compact(ctx, "")
		return compactDoneMsg{}
	}
}

type compactDoneMsg struct{}

func (m *RootModel) copyToClipboard(text string) {
	if !m.clipboardReady && m.clipboardErr == nil {
		if err := clipboard.Init(); err != nil {
			m.clipboardErr = err
			m.msgs.OnTurnError(fmt.Errorf("clipboard unavailable: %v", err))
			return
		}
		m.clipboardReady = true
	}
	if m.clipboardErr != nil {
		m.msgs.OnTurnError(fmt.Errorf("clipboard unavailable: %v", m.clipboardErr))
		return
	}
	clipboard.Write(clipboard.FmtText, []byte(text))
}

func (m *RootModel) refreshSystemPrompt() {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	sys := defaultSystemPromptForRoot() + "\n\n" + agent.LoadAgentsMD(cwd, home)
	m.agent = agent.New(m.agent.Provider(), m.agent.Tools(), m.sess, sys)
}

func defaultSystemPromptForRoot() string {
	return "You are rune, a coding agent. Use the available tools."
}
