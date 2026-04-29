package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"golang.design/x/clipboard"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/x/ansi"
	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/skill"
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

	streaming  bool
	compacting bool
	eventCh    <-chan agent.Event
	cancel     context.CancelFunc

	totalTokens int

	modal           modal.Modal
	settings        modal.Settings
	pendingForkMode bool
	clipboardReady  bool
	clipboardErr    error
	copyMode        bool

	showThinking    bool
	showToolResults bool
	pendingTickCmd  tea.Cmd
	activityFrame   int
	activityTicking bool
	activitySeq     int

	skills           map[string]string
	pendingSkillBody string

	// Ctrl+C requires a double press to exit. The first press clears the
	// editor (and closes a modal / cancels a streaming turn), primes the
	// "press again to exit" indicator, and schedules a quitPrimeExpiredMsg
	// after quitPrimeWindow. The seq counter invalidates stale ticks after
	// dis-arm (any other key disarms early).
	quitPrimed    bool
	quitPrimedSeq int
}

const quitPrimeWindow = 2 * time.Second

var baseSlashCmds = []string{
	"/quit", "/model", "/tree", "/resume", "/settings",
	"/new", "/name", "/session", "/fork", "/clone", "/copy", "/copy-mode",
	"/compact", "/reload", "/hotkeys",
}

func NewRootModel(a *agent.Agent, sess *session.Session) *RootModel {
	realCwd, _ := os.Getwd()
	iconMode := string(DefaultIconMode())
	settings := modal.Settings{Effort: a.ReasoningEffort(), IconMode: iconMode, ActivityMode: "arcane"}
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
	cmds := append([]string{}, baseSlashCmds...)
	ed := editor.New(realCwd, cmds)
	ed.SetHistory(editor.NewHistory(config.HistoryPath()))
	return &RootModel{
		agent:    a,
		sess:     sess,
		styles:   DefaultStylesWithIconMode(iconMode),
		msgs:     NewMessages(80),
		viewport: viewport.New(80, 20),
		editor:   ed,
		footer:   Footer{Cwd: displayCwd, GitBranch: currentGitBranch(realCwd), Session: sessLabel, Model: sess.Model},
		queue:    &Queue{},
		settings: settings,
	}
}

func (m *RootModel) SetSkills(skills []skill.Skill) {
	m.skills = make(map[string]string, len(skills))
	cmds := append([]string{}, baseSlashCmds...)
	for _, s := range skills {
		m.skills[s.Slug] = s.Body
		cmds = append(cmds, "/skill:"+s.Slug)
	}
	m.editor.SetSlashCmds(cmds)
}

func (m *RootModel) Init() tea.Cmd {
	return enableKittyKeyboard
}

func enableKittyKeyboard() tea.Msg {
	// Ask terminals that support the Kitty keyboard protocol to report
	// modified keys distinctly, including Shift+Enter as CSI 13;2u.
	// Unsupported terminals safely ignore this escape sequence.
	fmt.Print(ansi.PushKittyKeyboard(ansi.KittyDisambiguateEscapeCodes))
	return nil
}

func normalizeShiftEnterMsg(msg tea.Msg) tea.Msg {
	// Bubble Tea v1.3 treats Kitty CSI-u modified Enter sequences as unknown
	// CSI messages. Normalize Shift+Enter to a synthetic message while leaving
	// plain Enter untouched.
	if fmt.Sprint(msg) == "?CSI[49 51 59 50 117]?" {
		return tea.KeyMsg{Type: tea.KeyCtrlJ}
	}
	return msg
}

func (m *RootModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	msg = normalizeShiftEnterMsg(msg)

	// Single point of Ctrl+C handling. First press primes/clears, second
	// quits. Any other key while primed dis-arms the indicator. This runs
	// before the type switch so it covers both the modal-open and main
	// paths uniformly.
	if k, ok := msg.(tea.KeyMsg); ok {
		if k.Type == tea.KeyCtrlC {
			return m.handleCtrlC()
		}
		if m.quitPrimed {
			m.quitPrimed = false
			m.quitPrimedSeq++
			m.layout()
		}
	}

	switch v := msg.(type) {
	case quitPrimeExpiredMsg:
		if v.seq != m.quitPrimedSeq {
			return m, nil
		}
		m.quitPrimed = false
		m.layout()
		return m, nil
	case compactDoneMsg:
		// Drop late completions from a session that was swapped out mid-compact.
		if v.sess != m.sess {
			return m, nil
		}
		m.compacting = false
		m.stopActivityTick()
		if !m.copyMode {
			m.editor.Focus()
		}
		m.rebuildMessagesFromSession()
		if v.err != nil {
			m.msgs.OnTurnError(fmt.Errorf("compact failed: %v", v.err))
		} else {
			m.msgs.OnInfo(fmt.Sprintf("(compacted %d messages)", v.count))
		}
		m.layout()
		m.refreshViewport()
		if item, ok := m.queue.Pop(); ok {
			m.msgs.AppendUser(item.Text)
			m.refreshViewport()
			return m, m.startTurn(item.Text, item.Images)
		}
		return m, nil

	case tea.WindowSizeMsg:
		m.width, m.height = v.Width, v.Height
		m.layout()
		m.refreshViewport()
		return m, nil

	case thinkingTickMsg:
		m.refreshViewport()
		if m.msgs.HasInProgressThinking() {
			return m, thinkingTickCmd()
		}
		return m, nil

	case activityTickMsg:
		if v.seq != m.activitySeq {
			return m, nil
		}
		m.activityTicking = false
		if !m.showActivity() {
			return m, nil
		}
		m.activityFrame++
		return m, m.startActivityTick()

	case AgentEventMsg:
		if v.Ch != m.eventCh {
			// Stale event from a swapped-out session; drop it.
			return m, nil
		}
		m.handleEvent(v.Event)
		m.refreshViewport()
		cmds := []tea.Cmd{nextEventCmd(m.eventCh)}
		if m.pendingTickCmd != nil {
			cmds = append(cmds, m.pendingTickCmd)
			m.pendingTickCmd = nil
		}
		return m, tea.Batch(cmds...)

	case AgentChannelDoneMsg:
		if v.Ch != m.eventCh {
			// Stale done from a swapped-out session; ignore so we don't
			// pop the queue on the wrong session.
			return m, nil
		}
		m.streaming = false
		m.eventCh = nil
		m.cancel = nil
		m.stopActivityTick()
		if !m.copyMode {
			m.editor.Focus()
		}
		m.layout()
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
			if _, isTree := cur.(*modal.Tree); isTree {
				// Cancelling /fork's tree must not leak fork mode into the next /tree.
				m.pendingForkMode = false
			}
			m.layout()
			return m, nil
		}
		cmd := m.applyModalResult(cur, v.Payload)
		m.layout()
		return m, cmd
	}

	if m.modal != nil {
		next, cmd := m.modal.Update(msg)
		m.modal = next
		return m, cmd
	}

	if k, ok := msg.(tea.KeyMsg); ok {
		if k.Type == tea.KeyShiftTab {
			return m, m.toggleCopyMode()
		}
		if m.copyMode && k.Type == tea.KeyEsc {
			return m, m.toggleCopyMode()
		}
		if m.streaming && k.Type == tea.KeyEsc && m.cancel != nil {
			m.cancel()
			return m, nil
		}
		if m.editor.Mode() == editor.ModeNormal {
			if k.Type == tea.KeyCtrlT {
				m.showThinking = !m.showThinking
				m.refreshViewport()
				return m, nil
			}
			if k.Type == tea.KeyCtrlR {
				m.showToolResults = !m.showToolResults
				m.refreshViewport()
				return m, nil
			}
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
		if m.pendingSkillBody != "" {
			text = m.pendingSkillBody + "\n\n" + text
			m.pendingSkillBody = ""
		}
		if m.streaming || m.compacting {
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
	m.layout()
	m.viewport.GotoBottom()
	content := []ai.ContentBlock{ai.TextBlock{Text: text}}
	for _, im := range images {
		content = append(content, im)
	}
	msg := ai.Message{Role: ai.RoleUser, Content: content}
	ch := m.agent.Run(ctx, msg)
	m.eventCh = ch
	return tea.Batch(nextEventCmd(ch), m.startActivityTick())
}

func (m *RootModel) handleSlashCommand(cmd string) tea.Cmd {
	if strings.HasPrefix(cmd, "/skill:") {
		slug := strings.TrimPrefix(cmd, "/skill:")
		if body, ok := m.skills[slug]; ok {
			m.pendingSkillBody = body
			m.msgs.OnInfo(fmt.Sprintf("(skill %q armed; will be prepended to your next message)", slug))
		} else {
			m.msgs.OnTurnError(fmt.Errorf("unknown skill: %s", slug))
		}
		m.refreshViewport()
		m.layout()
		return nil
	}
	var initCmd tea.Cmd
	switch cmd {
	case "/quit":
		return tea.Quit
	case "/model":
		initCmd = m.openModal(modal.NewModelPicker([]string{"gpt-5", "gpt-5-codex", "gpt-5.1-codex-mini"}, m.sess.Model))
	case "/tree":
		if m.streaming {
			m.msgs.OnInfo("(busy — wait for current turn to finish)")
			break
		}
		initCmd = m.openModal(modal.NewTree(m.sess))
	case "/resume":
		items, _ := session.ListSessions(config.SessionsDir())
		initCmd = m.openModal(modal.NewResume(items))
	case "/settings":
		initCmd = m.openModal(modal.NewSettings(m.settings))
	case "/hotkeys":
		initCmd = m.openModal(modal.NewHotkeys())
	case "/new":
		m.startNewSession()
	case "/name":
		m.msgs.OnTurnError(fmt.Errorf("(use /settings or future inline prompt)"))
	case "/session":
		m.msgs.OnTurnError(fmt.Errorf("session id=%s name=%q model=%s", m.sess.ID, m.sess.Name, m.sess.Model))
	case "/fork":
		if m.streaming {
			m.msgs.OnInfo("(busy — wait for current turn to finish)")
			break
		}
		m.pendingForkMode = true
		initCmd = m.openModal(modal.NewTree(m.sess))
	case "/clone":
		nc := m.sess.Clone()
		nc.SetPath(filepath.Join(config.SessionsDir(), nc.ID+".json"))
		_ = nc.Save()
		m.swapSession(nc)
	case "/copy":
		last := lastAssistantText(m.sess)
		if last == "" {
			m.msgs.OnInfo("(no assistant message to copy)")
		} else {
			m.copyToClipboard(last)
		}
	case "/copy-mode":
		initCmd = m.toggleCopyMode()
	case "/compact":
		if m.streaming || m.compacting {
			m.msgs.OnInfo("(busy — wait for current turn to finish)")
			break
		}
		m.compacting = true
		m.editor.Blur()
		m.msgs.OnInfo("(compacting…)")
		m.refreshViewport()
		m.layout()
		return tea.Batch(m.startCompact(), m.startActivityTick())
	case "/reload":
		if m.streaming {
			m.msgs.OnInfo("(busy — wait for current turn to finish)")
			break
		}
		home, _ := os.UserHomeDir()
		cwd, _ := os.Getwd()
		sks, _ := (&skill.Loader{Roots: []string{
			filepath.Join(home, ".rune", "skills"),
			filepath.Join(cwd, ".rune", "skills"),
		}}).Load()
		m.SetSkills(sks)
		m.msgs.OnInfo(fmt.Sprintf("(reloaded %d skills)", len(sks)))
		m.refreshSystemPrompt()
		m.refreshViewport()
	}
	m.refreshViewport()
	m.layout()
	return initCmd
}

// openModal sets the current modal and returns its Init cmd so async
// loaders work. Always go through this rather than assigning m.modal directly.
func (m *RootModel) openModal(md modal.Modal) tea.Cmd {
	m.modal = md
	return md.Init()
}

func (m *RootModel) View() string {
	if m.modal != nil {
		return m.modal.View(m.width, m.height)
	}
	msgArea := m.viewport.View()
	box := m.styles.EditorBox
	switch m.editor.ShellMode() {
	case editor.ShellModeInsert:
		box = m.styles.EditorBoxShellInsert
	case editor.ShellModeSend:
		box = m.styles.EditorBoxShellSend
	}
	if m.copyMode {
		box = m.styles.EditorBoxDim
	}
	edArea := box.Render(m.editor.View(m.width))
	hint := m.editorScrollHint()
	overlay := ""
	switch m.editor.Mode() {
	case editor.ModeFilePicker:
		fp := m.editor.FilePicker()
		overlay = renderList("files", fp.Items(), fp.Sel())
	case editor.ModeSlashMenu:
		sm := m.editor.SlashMenu()
		overlay = renderList("commands", sm.Items(), sm.Sel())
	}
	foot := m.footer.Render(m.styles)
	activity := m.renderActivityLine()
	banner := ""
	if m.copyMode {
		banner = m.styles.CopyModeBanner.Render("[copy mode] drag to highlight, copy with your terminal shortcut · Shift+Tab/Esc to exit")
	}
	quitNotice := ""
	if m.quitPrimed {
		quitNotice = m.styles.QuitPrimedBanner.Render("Press Ctrl+C again to exit")
	}

	out := msgArea
	if activity != "" {
		out += "\n\n" + activity
	}
	if banner != "" {
		out += "\n" + banner
	}
	if hint != "" {
		out += "\n" + hint
	}
	if quitNotice != "" {
		out += "\n" + quitNotice
	}
	out += "\n" + edArea
	if overlay != "" {
		out += "\n" + overlay
	}
	out += "\n" + foot
	return out
}

func (m *RootModel) editorScrollHint() string {
	above, below := m.editor.ScrollState()
	if above == 0 && below == 0 {
		return ""
	}
	rowWord := func(n int) string {
		if n == 1 {
			return "row"
		}
		return "rows"
	}
	var s string
	switch {
	case above > 0 && below > 0:
		s = fmt.Sprintf("↑ %d above · ↓ %d below", above, below)
	case above > 0:
		s = fmt.Sprintf("↑ %d %s above (use ↑ to scroll)", above, rowWord(above))
	default:
		s = fmt.Sprintf("↓ %d %s below (use ↓ to scroll)", below, rowWord(below))
	}
	return m.styles.Info.Render(s)
}

func renderList(title string, items []string, sel int) string {
	if len(items) == 0 {
		return "(no " + title + ")"
	}
	const window = 8
	start := 0
	if sel >= window {
		start = sel - window + 1
	}
	end := start + window
	if end > len(items) {
		end = len(items)
	}
	out := title + ":"
	if start > 0 {
		out += "\n  …"
	}
	for i := start; i < end; i++ {
		marker := "  "
		line := items[i]
		if i == sel {
			marker = "> "
			line = selectedRowStyle.Render(line)
		}
		out += "\n" + marker + line
	}
	if end < len(items) {
		out += "\n  …"
	}
	return out
}

var selectedRowStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))

func (m *RootModel) layout() {
	if m.width == 0 || m.height == 0 {
		return
	}
	// EditorBox renders with rounded border (2 cols) + horizontal padding (2 cols);
	// reserve those so the editor never overflows the terminal width.
	m.editor.SetWidth(m.width - 4)
	// Cap the editor at ~25% of the terminal so a long wrapped sentence can grow
	// without swallowing the message viewport.
	maxRows := m.height / 4
	if maxRows < 3 {
		maxRows = 3
	}
	m.editor.SetMaxRows(maxRows)

	footerH := 1
	editorRows := m.editor.Rows()
	editorH := editorRows + 2 // border top + bottom
	overlayRows := 0
	if m.editor.Mode() != editor.ModeNormal {
		overlayRows = 9
	}
	activityRows := 0
	if m.showActivity() {
		activityRows = 2
	}
	hintRows := 0
	if m.editor.RawRows() > editorRows {
		hintRows = 1
	}
	bannerRows := 0
	if m.copyMode {
		bannerRows = 1
	}
	quitPrimedRows := 0
	if m.quitPrimed {
		quitPrimedRows = 1
	}
	msgH := m.height - footerH - editorH - overlayRows - activityRows - hintRows - bannerRows - quitPrimedRows
	if msgH < 3 {
		msgH = 3
	}
	m.viewport.Width = m.width
	m.viewport.Height = msgH
	m.footer.Width = m.width
	m.msgs.SetWidth(m.width)
}

func (m *RootModel) refreshViewport() {
	atBottom := m.viewport.AtBottom()
	if m.msgs.IsEmpty() {
		m.viewport.SetContent(renderSplash(m.width, m.viewport.Height, m.styles))
	} else {
		m.viewport.SetContent(m.msgs.Render(m.styles, m.showThinking, m.showToolResults, time.Now()))
	}
	if atBottom {
		m.viewport.GotoBottom()
	}
}

func (m *RootModel) handleEvent(e agent.Event) {
	switch v := e.(type) {
	case agent.AssistantText:
		m.msgs.OnAssistantDelta(v.Delta)
	case agent.ThinkingText:
		wasIdle := !m.msgs.HasInProgressThinking()
		m.msgs.OnThinkingDelta(v.Delta)
		if wasIdle {
			m.pendingTickCmd = thinkingTickCmd()
		}
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
			m.styles.Icons = IconSetForMode(s.IconMode)
			if s.Effort != "" {
				m.agent.SetReasoningEffort(s.Effort)
			}
			if m.showActivity() {
				return m.startActivityTick()
			}
			m.stopActivityTick()
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
	// Cancel any in-flight turn on the previous session before replacing
	// pointers — otherwise the old goroutine's events could bleed into
	// the new session's UI, and a queued message could pop into the
	// wrong session after AgentChannelDoneMsg fires.
	m.stopActiveTurn()
	m.sess = s
	m.footer.Session = s.Name
	m.footer.Model = s.Model
	m.rebuildMessagesFromSession()
	prev := m.agent.ReasoningEffort()
	m.agent = agent.New(m.agent.Provider(), m.agent.Tools(), s, m.agent.System())
	m.agent.SetReasoningEffort(prev)
}

// stopActiveTurn cancels any in-flight agent turn, clears streaming state,
// and drops queued items. Pending events from the old channel are ignored
// downstream because nextEventCmd tags messages with their channel and the
// AgentEventMsg handler drops mismatches.
func (m *RootModel) stopActiveTurn() {
	if m.cancel != nil {
		m.cancel()
	}
	m.cancel = nil
	m.eventCh = nil
	m.streaming = false
	m.compacting = false
	m.pendingTickCmd = nil
	m.stopActivityTick()
	m.queue = &Queue{}
	m.editor.Focus()
}

func (m *RootModel) rebuildMessagesFromSession() {
	m.msgs = NewMessages(m.width)
	for _, n := range m.sess.PathToActiveNodes() {
		msg := n.Message
		switch msg.Role {
		case ai.RoleUser:
			for _, c := range msg.Content {
				if t, ok := c.(ai.TextBlock); ok {
					m.msgs.AppendUser(t.Text)
				}
			}
		case ai.RoleAssistant:
			if n.CompactedCount > 0 {
				var text string
				for _, c := range msg.Content {
					if t, ok := c.(ai.TextBlock); ok {
						text += t.Text
					}
				}
				m.msgs.AppendSummary(text, n.CompactedCount)
				continue
			}
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
	sess := m.sess
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		// Capture pre-compact path length so we can report N. Compact
		// replaces everything before the most recent user message; we'll
		// recover the count from the new summary node below.
		err := m.agent.Compact(ctx, "")
		count := 0
		if err == nil {
			for _, n := range sess.PathToActiveNodes() {
				if n.CompactedCount > 0 {
					count = n.CompactedCount
					break
				}
			}
		}
		return compactDoneMsg{sess: sess, err: err, count: count}
	}
}

type compactDoneMsg struct {
	sess  *session.Session
	err   error
	count int
}

type thinkingTickMsg struct{}

func thinkingTickCmd() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return thinkingTickMsg{} })
}

type quitPrimeExpiredMsg struct{ seq int }

func quitPrimeExpiredCmd(seq int) tea.Cmd {
	return tea.Tick(quitPrimeWindow, func(time.Time) tea.Msg { return quitPrimeExpiredMsg{seq: seq} })
}

// handleCtrlC implements double-press-to-exit. The first press clears the
// editor (and closes a modal / cancels a streaming turn if applicable), then
// arms the "press Ctrl+C again to exit" indicator for quitPrimeWindow.
// A second press within that window quits.
func (m *RootModel) handleCtrlC() (tea.Model, tea.Cmd) {
	if m.quitPrimed {
		return m, tea.Quit
	}
	if m.modal != nil {
		m.modal = nil
	}
	if m.streaming && m.cancel != nil {
		m.cancel()
	}
	m.editor.Reset()
	m.quitPrimed = true
	m.quitPrimedSeq++
	m.layout()
	return m, quitPrimeExpiredCmd(m.quitPrimedSeq)
}

type activityTickMsg struct{ seq int }

func activityTickCmd(seq int) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return activityTickMsg{seq: seq} })
}

func (m *RootModel) startActivityTick() tea.Cmd {
	if !m.showActivity() || m.activityTicking {
		return nil
	}
	m.activityTicking = true
	m.activitySeq++
	return activityTickCmd(m.activitySeq)
}

func (m *RootModel) stopActivityTick() {
	if m.activityTicking {
		m.activitySeq++
	}
	m.activityTicking = false
}

func (m *RootModel) showActivity() bool {
	return (m.streaming || m.compacting) && m.settings.ActivityMode != "off"
}

func (m *RootModel) renderActivityLine() string {
	if !m.showActivity() {
		return ""
	}
	phrases := simpleActivityPhrases()
	if m.settings.ActivityMode == "arcane" || m.settings.ActivityMode == "" {
		phrases = arcaneActivityPhrases(m.styles.Icons)
	}
	line := phrases[m.activityFrame%len(phrases)]
	return m.styles.Activity.Width(m.width).Render(line)
}

func simpleActivityPhrases() []string {
	return []string{"running...", "waiting for response...", "working..."}
}

func arcaneActivityPhrases(icons IconSet) []string {
	return []string{
		iconLabel(icons.Thinking, "consulting the runes..."),
		iconLabel(icons.Assistant, "scrying through the code..."),
		iconLabel(icons.Invoke, "tracing the invocation..."),
		iconLabel(icons.Summary, "opening the grimoire..."),
		iconLabel(icons.Tool, "binding tools to the circle..."),
		iconLabel(icons.Context, "measuring the leyline pressure..."),
	}
}

// toggleCopyMode flips terminal-native copy mode. When entering, we surrender
// mouse capture so the terminal handles click-drag selection itself; the editor
// is blurred and the box dims to signal it's not accepting input. When exiting,
// we re-enable wheel-scroll and restore editor focus (unless a turn is
// streaming or compacting, which independently keep the editor blurred).
func (m *RootModel) toggleCopyMode() tea.Cmd {
	m.copyMode = !m.copyMode
	if m.copyMode {
		m.editor.Blur()
		m.layout()
		return tea.DisableMouse
	}
	if !m.streaming && !m.compacting {
		m.editor.Focus()
	}
	m.layout()
	return tea.EnableMouseCellMotion
}

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
	m.msgs.OnInfo("(copied)")
}

func (m *RootModel) refreshSystemPrompt() {
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	sys := agent.BasePrompt() + "\n\n" + agent.LoadAgentsMD(cwd, home)
	prev := m.agent.ReasoningEffort()
	m.agent = agent.New(m.agent.Provider(), m.agent.Tools(), m.sess, sys)
	m.agent.SetReasoningEffort(prev)
}
