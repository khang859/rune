package tui

import (
	"context"
	"fmt"
	"math/rand/v2"
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
	"github.com/khang859/rune/internal/mcp"
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

	subagentCtx                 context.Context
	subagentCancel              context.CancelFunc
	subagentCh                  <-chan agent.SubagentEvent
	subagents                   map[string]agent.SubagentEvent
	autoContinueSubagentResults map[string]bool
	pendingSubagentContinuation bool

	currentTokens int

	modal           modal.Modal
	settings        modal.Settings
	lastModal       modal.Modal
	pendingForkMode bool
	clipboardReady  bool
	clipboardErr    error
	copyMode        bool

	showThinking    bool
	showToolResults bool
	pendingTickCmd  tea.Cmd
	activityFrame   int
	activityPhrase  int
	activityTicking bool
	activitySeq     int

	skills           map[string]string
	pendingSkillBody string
	mcpStatuses      []mcp.Status

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
	"/quit", "/model", "/thinking", "/tree", "/resume", "/settings", "/mcp", "/mcp-status",
	"/new", "/name", "/session", "/fork", "/clone", "/copy", "/copy-mode",
	"/compact", "/reload", "/hotkeys",
}

func NewRootModel(a *agent.Agent, sess *session.Session) *RootModel {
	realCwd, _ := os.Getwd()
	iconMode := string(DefaultIconMode())
	settings := modalSettingsFromConfig(config.DefaultSettings(), braveKeyConfigured())
	if settings.Effort != "" {
		a.SetReasoningEffort(settings.Effort)
	}
	settings.Effort = a.ReasoningEffort()
	if settings.IconMode == "" {
		settings.IconMode = iconMode
	}
	displayCwd := realCwd
	if home, _ := os.UserHomeDir(); home != "" && strings.HasPrefix(realCwd, home) {
		displayCwd = "~" + strings.TrimPrefix(realCwd, home)
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
		footer: Footer{
			Cwd:            displayCwd,
			GitBranch:      currentGitBranch(realCwd),
			Session:        sessionLabel(sess),
			Model:          sess.Model,
			ThinkingEffort: footerThinkingEffort(sess.Model, a.ReasoningEffort()),
		},
		queue:                       &Queue{},
		settings:                    settings,
		subagents:                   map[string]agent.SubagentEvent{},
		autoContinueSubagentResults: map[string]bool{},
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

func (m *RootModel) SetMCPStatuses(statuses []mcp.Status) {
	m.mcpStatuses = make([]mcp.Status, len(statuses))
	for i, st := range statuses {
		m.mcpStatuses[i] = st
		m.mcpStatuses[i].Tools = append([]string(nil), st.Tools...)
	}
}

func (m *RootModel) Init() tea.Cmd {
	return tea.Batch(enableKittyKeyboard, m.startSubagentListener())
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
		if !m.showActivity() {
			m.stopActivityTick()
		}
		if !m.copyMode {
			m.editor.Focus()
		}
		m.rebuildMessagesFromSession()
		if v.err != nil {
			m.msgs.OnTurnError(fmt.Errorf("compact failed: %v", v.err))
		} else {
			m.saveSessionIfStarted()
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
		if m.activityFrame%activityPhraseChangeFrames == 0 {
			m.activityPhrase = nextActivityPhraseIndex(m.activityPhrase, len(m.activityPhrases()))
		}
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
		m.saveSessionIfStarted()
		m.streaming = false
		m.eventCh = nil
		m.cancel = nil
		if !m.showActivity() {
			m.stopActivityTick()
		}
		if !m.copyMode {
			m.editor.Focus()
		}
		m.layout()
		if item, ok := m.queue.Pop(); ok {
			m.msgs.AppendUser(item.Text)
			m.refreshViewport()
			return m, m.startTurn(item.Text, item.Images)
		}
		if m.pendingSubagentContinuation {
			m.pendingSubagentContinuation = false
			return m, m.startSubagentContinuationTurn()
		}
		return m, nil

	case SubagentEventMsg:
		if v.Ch != m.subagentCh {
			return m, nil
		}
		m.handleSubagentEvent(v.Event)
		m.refreshViewport()
		cmds := []tea.Cmd{nextSubagentEventCmd(m.subagentCh)}
		if m.pendingTickCmd != nil {
			cmds = append(cmds, m.pendingTickCmd)
			m.pendingTickCmd = nil
		}
		return m, tea.Batch(cmds...)

	case SubagentChannelDoneMsg:
		if v.Ch == m.subagentCh {
			m.subagentCh = nil
		}
		return m, nil

	case tea.MouseMsg:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd

	case modal.ResultMsg:
		cur := m.modal
		if cur == nil {
			cur = m.lastModal
		}
		m.modal = nil
		m.lastModal = nil
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
	m.saveSessionIfStarted()
	m.eventCh = ch
	return tea.Batch(nextEventCmd(ch), m.startActivityTick())
}

const subagentContinuationPrompt = "A subagent has completed. Review the subagent result that was added to your context and continue the user's task. If actions are needed, take them; otherwise briefly report the conclusion."

func (m *RootModel) startSubagentContinuationTurn() tea.Cmd {
	return m.startTurn(subagentContinuationPrompt, nil)
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
	if strings.HasPrefix(cmd, "/subagent-cancel ") {
		m.cancelSubagentsCommand(strings.TrimSpace(strings.TrimPrefix(cmd, "/subagent-cancel ")))
		m.refreshViewport()
		m.layout()
		return nil
	}
	var initCmd tea.Cmd
	switch cmd {
	case "/quit":
		return tea.Quit
	case "/model":
		initCmd = m.openModal(modal.NewModelPicker(codexModelIDs(), m.sess.Model))
	case "/thinking":
		levels := thinkingLevelsForModel(m.sess.Model)
		if len(levels) == 0 {
			m.msgs.OnInfo(fmt.Sprintf("(thinking levels for %s are unknown)", m.sess.Model))
			break
		}
		initCmd = m.openModal(modal.NewThinkingPicker(levels, m.agent.ReasoningEffort()))
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
	case "/mcp":
		initCmd = m.openModal(modal.NewMCPWizard())
	case "/mcp-status":
		initCmd = m.openModal(modal.NewMCPStatus(m.mcpStatuses))
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
	case "/subagents":
		m.showSubagentsInfo()
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

func (m *RootModel) showSubagentsInfo() {
	tasks := m.agent.Subagents().List()
	if len(tasks) == 0 {
		m.msgs.OnInfo("(no subagents)")
		return
	}
	var b strings.Builder
	b.WriteString("(subagents)")
	for _, task := range tasks {
		b.WriteString("\n- ")
		b.WriteString(task.ID)
		if task.Name != "" {
			b.WriteString(" ")
			b.WriteString(task.Name)
		}
		b.WriteString(" ")
		b.WriteString(task.Status)
		if task.Error != "" {
			b.WriteString(": ")
			b.WriteString(task.Error)
		}
	}
	m.msgs.OnInfo(b.String())
}

func (m *RootModel) cancelSubagentsCommand(arg string) {
	if arg == "" {
		m.msgs.OnInfo("(usage: /subagent-cancel <task_id|all>)")
		return
	}
	if arg == "all" {
		cancelled := 0
		for _, task := range m.agent.Subagents().List() {
			if !isActiveSubagentStatus(task.Status) {
				continue
			}
			if err := m.agent.Subagents().Cancel(task.ID); err != nil {
				m.msgs.OnTurnError(err)
				continue
			}
			cancelled++
		}
		m.msgs.OnInfo(fmt.Sprintf("(cancelled %d subagents)", cancelled))
		return
	}
	if err := m.agent.Subagents().Cancel(arg); err != nil {
		m.msgs.OnTurnError(err)
		return
	}
	m.msgs.OnInfo(fmt.Sprintf("(cancelled %s)", arg))
}

// openModal sets the current modal and returns its Init cmd so async
// loaders work. Always go through this rather than assigning m.modal directly.
func (m *RootModel) openModal(md modal.Modal) tea.Cmd {
	m.modal = md
	m.lastModal = md
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

func (m *RootModel) handleSubagentEvent(e agent.SubagentEvent) {
	if m.subagents == nil {
		m.subagents = map[string]agent.SubagentEvent{}
	}
	if m.autoContinueSubagentResults == nil {
		m.autoContinueSubagentResults = map[string]bool{}
	}
	m.subagents[e.Task.ID] = e
	m.msgs.OnSubagentEvent(e)
	if e.Status == agent.SubagentRunning || e.Status == agent.SubagentPending {
		m.pendingTickCmd = m.startActivityTick()
	} else if !m.showActivity() {
		m.stopActivityTick()
	}
	if e.Status == agent.SubagentCompleted && strings.TrimSpace(e.Task.Summary) != "" && !m.autoContinueSubagentResults[e.Task.ID] {
		m.autoContinueSubagentResults[e.Task.ID] = true
		if m.streaming || m.compacting {
			m.pendingSubagentContinuation = true
			return
		}
		m.pendingTickCmd = tea.Batch(m.pendingTickCmd, m.startSubagentContinuationTurn())
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
		m.currentTokens = v.Usage.Input + v.Usage.Output
		m.footer.Tokens = m.currentTokens
		m.footer.ContextPct = ctxPctForModel(m.sess.Model, m.currentTokens)
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

func ctxPctForModel(model string, tokens int) int {
	cap := contextWindowForModel(model)
	if cap <= 0 || tokens <= 0 {
		return 0
	}
	p := tokens * 100 / cap
	if p > 100 {
		return 100
	}
	return p
}

func contextWindowForModel(model string) int {
	switch strings.ToLower(model) {
	case "gpt-5.3-codex-spark":
		return 128_000
	case "gpt-5.1", "gpt-5.1-codex-max", "gpt-5.1-codex-mini",
		"gpt-5.2", "gpt-5.2-codex", "gpt-5.3-codex",
		"gpt-5.4", "gpt-5.4-mini", "gpt-5.5":
		return 272_000
	default:
		return 272_000
	}
}

func codexModelIDs() []string {
	return []string{
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.3-codex",
		"gpt-5.3-codex-spark",
		"gpt-5.2",
		"gpt-5.2-codex",
		"gpt-5.1",
		"gpt-5.1-codex-max",
		"gpt-5.1-codex-mini",
	}
}

func thinkingLevelsForModel(model string) []string {
	switch strings.ToLower(model) {
	case "gpt-5.5", "gpt-5.4", "gpt-5.2":
		return []string{"none", "low", "medium", "high", "xhigh"}
	case "gpt-5.3-codex", "gpt-5.2-codex":
		return []string{"low", "medium", "high", "xhigh"}
	case "gpt-5.1":
		return []string{"none", "low", "medium", "high"}
	default:
		return nil
	}
}

func supportedThinkingEffort(model, effort string) bool {
	for _, level := range thinkingLevelsForModel(model) {
		if level == effort {
			return true
		}
	}
	return false
}

func footerThinkingEffort(model, effort string) string {
	if effort == "" || !supportedThinkingEffort(model, effort) {
		return ""
	}
	return effort
}

func (m *RootModel) refreshFooterThinkingEffort() {
	m.footer.ThinkingEffort = footerThinkingEffort(m.sess.Model, m.agent.ReasoningEffort())
}

func defaultThinkingEffort(levels []string) string {
	for _, level := range levels {
		if level == "medium" {
			return level
		}
	}
	if len(levels) > 0 {
		return levels[0]
	}
	return ""
}

func (m *RootModel) clampThinkingForCurrentModel() {
	levels := thinkingLevelsForModel(m.sess.Model)
	if len(levels) == 0 || supportedThinkingEffort(m.sess.Model, m.agent.ReasoningEffort()) {
		return
	}
	m.applyThinkingEffort(defaultThinkingEffort(levels))
}

func (m *RootModel) applyThinkingEffort(effort string) {
	if !supportedThinkingEffort(m.sess.Model, effort) {
		m.msgs.OnInfo(fmt.Sprintf("(thinking effort %q is not supported by %s)", effort, m.sess.Model))
		m.refreshViewport()
		return
	}
	m.settings.Effort = effort
	m.applySettings(m.settings)
	m.msgs.OnInfo(fmt.Sprintf("(thinking effort: %s)", effort))
	m.refreshViewport()
}

func (m *RootModel) applyModalResult(cur modal.Modal, payload any) tea.Cmd {
	switch cur.(type) {
	case *modal.ModelPicker:
		if id, ok := payload.(string); ok {
			m.sess.Model = id
			m.footer.Model = id
			m.footer.ContextPct = ctxPctForModel(m.sess.Model, m.currentTokens)
			m.clampThinkingForCurrentModel()
			m.refreshFooterThinkingEffort()
			m.saveSessionIfStarted()
		}
	case *modal.ThinkingPicker:
		if effort, ok := payload.(string); ok {
			m.applyThinkingEffort(effort)
		}
	case *modal.SettingsModal:
		switch v := payload.(type) {
		case modal.SettingsAction:
			m.applySettings(v.Settings)
			if v.Action == "brave_api_key" {
				return m.openModal(modal.NewSecretInput("Brave Search API key", "brave_api_key"))
			}
		case modal.Settings:
			m.applySettings(v)
		}
	case *modal.SecretInput:
		if res, ok := payload.(modal.SecretInputResult); ok && res.Action == "brave_api_key" {
			if err := config.NewSecretStore(config.SecretsPath()).SetBraveSearchAPIKey(res.Value); err != nil {
				m.msgs.OnTurnError(err)
			} else {
				m.settings.BraveAPIKeyStatus = "configured — Enter to replace"
				m.reconfigureWebTools()
				m.msgs.OnInfo("(saved Brave Search API key; web_search enabled if settings allow it)")
			}
			m.refreshViewport()
		}
	case *modal.MCPWizard:
		if res, ok := payload.(modal.MCPWizardResult); ok {
			if err := mcp.AddServer(config.MCPConfig(), res.Name, res.Config, false); err != nil {
				m.msgs.OnTurnError(fmt.Errorf("mcp: %v", err))
			} else {
				m.msgs.OnInfo(fmt.Sprintf("(added MCP server %q; restart rune to load its tools)", res.Name))
			}
			m.refreshViewport()
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
				m.saveSessionIfStarted()
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

func (m *RootModel) saveSessionIfStarted() {
	if m == nil || m.sess == nil || len(m.sess.PathToActive()) == 0 {
		return
	}
	_ = m.sess.Save()
}

func sessionLabel(s *session.Session) string {
	if s == nil {
		return ""
	}
	if s.Name != "" {
		return s.Name
	}
	if len(s.ID) > 8 {
		return s.ID[:8]
	}
	return s.ID
}

func (m *RootModel) startSubagentListener() tea.Cmd {
	if m.subagentCancel != nil {
		m.subagentCancel()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.subagentCtx = ctx
	m.subagentCancel = cancel
	m.subagentCh = m.agent.Subagents().Subscribe(ctx)
	return nextSubagentEventCmd(m.subagentCh)
}

func (m *RootModel) swapSession(s *session.Session) {
	// Cancel any in-flight turn on the previous session before replacing
	// pointers — otherwise the old goroutine's events could bleed into
	// the new session's UI, and a queued message could pop into the
	// wrong session after AgentChannelDoneMsg fires.
	m.stopActiveTurn()
	m.sess = s
	m.footer.Session = sessionLabel(s)
	m.footer.Model = s.Model
	m.rebuildMessagesFromSession()
	prev := m.agent.ReasoningEffort()
	m.agent = agent.NewWithSubagentConfig(m.agent.Provider(), m.agent.Tools(), s, m.agent.System(), currentSubagentConfig())
	m.agent.SetReasoningEffort(prev)
	m.agent.RegisterSubagentToolsEnabled(currentSubagentsEnabled())
	m.subagents = map[string]agent.SubagentEvent{}
	m.autoContinueSubagentResults = map[string]bool{}
	m.pendingSubagentContinuation = false
	_ = m.startSubagentListener()
	m.clampThinkingForCurrentModel()
	m.refreshFooterThinkingEffort()
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

const (
	activityTickInterval       = 120 * time.Millisecond
	activityPhraseChangeFrames = 14
)

func activityTickCmd(seq int) tea.Cmd {
	return tea.Tick(activityTickInterval, func(time.Time) tea.Msg { return activityTickMsg{seq: seq} })
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
	return (m.streaming || m.compacting || m.activeSubagentCount() > 0) && m.settings.ActivityMode != "off"
}

func (m *RootModel) activeSubagentCount() int {
	count := 0
	for _, ev := range m.subagents {
		if ev.Status == agent.SubagentPending || ev.Status == agent.SubagentRunning {
			count++
		}
	}
	return count
}

func isActiveSubagentStatus(status string) bool {
	return status == string(agent.SubagentPending) || status == string(agent.SubagentRunning)
}

func (m *RootModel) renderActivityLine() string {
	if !m.showActivity() {
		return ""
	}
	left := ""
	if m.streaming || m.compacting {
		phrases := m.activityPhrases()
		left = activityMotionText(phrases[m.activityPhrase%len(phrases)], m.activityFrame)
	}
	right := m.renderSubagentActivityIndicator()
	line := composeActivityLine(left, right, m.width)
	return m.styles.Activity.Width(m.width).Render(line)
}

func (m *RootModel) renderSubagentActivityIndicator() string {
	n := m.activeSubagentCount()
	if n == 0 {
		return ""
	}
	label := "subagent"
	if n != 1 {
		label = "subagents"
	}
	return subagentSpinnerText(fmt.Sprintf("%d %s working", n, label), m.activityFrame)
}

func composeActivityLine(left, right string, width int) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		return rightAlignActivitySegment(right, width)
	}
	if right == "" {
		return fitActivitySegment(left, width)
	}
	if width <= 0 {
		return left + " " + right
	}
	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	if leftW+1+rightW > width {
		if rightW+1 >= width {
			return rightAlignActivitySegment(right, width)
		}
		left = fitActivitySegment(left, width-rightW-1)
		leftW = lipgloss.Width(left)
	}
	pad := width - leftW - rightW
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

func rightAlignActivitySegment(s string, width int) string {
	if width <= 0 {
		return s
	}
	w := lipgloss.Width(s)
	if w >= width {
		return fitActivitySegment(s, width)
	}
	return strings.Repeat(" ", width-w) + s
}

func fitActivitySegment(s string, width int) string {
	if width <= 0 || lipgloss.Width(s) <= width {
		return s
	}
	return ansi.Truncate(s, width, "…")
}

func (m *RootModel) activityPhrases() []string {
	if m.settings.ActivityMode == "arcane" || m.settings.ActivityMode == "" {
		return arcaneActivityPhrases(m.styles.Icons)
	}
	return simpleActivityPhrases()
}

func nextActivityPhraseIndex(current, phraseCount int) int {
	if phraseCount <= 1 {
		return 0
	}
	next := rand.IntN(phraseCount - 1)
	if next >= current%phraseCount {
		next++
	}
	return next
}

func activityMotionText(text string, frame int) string {
	dots := []string{"", ".", "..", "..."}
	base := strings.TrimRight(strings.TrimSpace(text), ".…")
	return base + dots[frame%len(dots)]
}

// subagentSpinnerText prefixes the subagent indicator with a fixed-width
// braille spinner instead of trailing dots. The constant width keeps the
// right edge of the activity bar from jiggling as frames advance.
func subagentSpinnerText(text string, frame int) string {
	spinner := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	base := strings.TrimRight(strings.TrimSpace(text), ".…")
	return spinner[frame%len(spinner)] + " " + base
}

func simpleActivityPhrases() []string {
	return []string{
		"running...",
		"waiting for response...",
		"working...",
		"thinking...",
		"checking the plan...",
		"reading context...",
		"following the trail...",
		"looking things over...",
		"sorting it out...",
		"making progress...",
		"connecting the dots...",
		"reviewing the details...",
	}
}

func arcaneActivityPhrases(icons IconSet) []string {
	return []string{
		iconLabel(icons.Thinking, "consulting the runes..."),
		iconLabel(icons.Assistant, "scrying through the code..."),
		iconLabel(icons.Invoke, "tracing the invocation..."),
		iconLabel(icons.Summary, "opening the grimoire..."),
		iconLabel(icons.Tool, "binding tools to the circle..."),
		iconLabel(icons.Context, "measuring the leyline pressure..."),
		iconLabel(icons.Thinking, "listening for compiler omens..."),
		iconLabel(icons.Assistant, "deciphering ancient stack traces..."),
		iconLabel(icons.Invoke, "aligning the sigils..."),
		iconLabel(icons.Summary, "dusting off a forgotten scroll..."),
		iconLabel(icons.Tool, "sharpening the ritual dagger..."),
		iconLabel(icons.Context, "charting the ley lines..."),
		iconLabel(icons.Thinking, "stirring the cauldron..."),
		iconLabel(icons.Assistant, "summoning a helpful familiar..."),
		iconLabel(icons.Invoke, "etching glyphs into the terminal..."),
		iconLabel(icons.Summary, "reading tea leaves in the diff..."),
		iconLabel(icons.Tool, "polishing the crystal ball..."),
		iconLabel(icons.Context, "weighing the context crystals..."),
		iconLabel(icons.Thinking, "casting a careful incantation..."),
		iconLabel(icons.Assistant, "wandering the astral syntax tree..."),
		iconLabel(icons.Invoke, "opening a tiny portal..."),
		iconLabel(icons.Summary, "consulting the moonlit changelog..."),
		iconLabel(icons.Tool, "feeding breadcrumbs to the tools..."),
		iconLabel(icons.Context, "counting tokens by candlelight..."),
		iconLabel(icons.Thinking, "asking the oracle for hints..."),
		iconLabel(icons.Assistant, "tracking a spectral bug..."),
		iconLabel(icons.Invoke, "braiding command threads..."),
		iconLabel(icons.Summary, "unrolling the parchment..."),
		iconLabel(icons.Tool, "charging the wand..."),
		iconLabel(icons.Context, "mapping hidden corridors..."),
		iconLabel(icons.Thinking, "brewing a safer answer..."),
		iconLabel(icons.Assistant, "translating whispers from the repo..."),
		iconLabel(icons.Invoke, "nudging the invocation circle..."),
		iconLabel(icons.Summary, "indexing enchanted footnotes..."),
		iconLabel(icons.Tool, "waking the clockwork helpers..."),
		iconLabel(icons.Context, "measuring the spell radius..."),
		iconLabel(icons.Thinking, "testing the wards..."),
		iconLabel(icons.Assistant, "following foxfire through the files..."),
		iconLabel(icons.Invoke, "threading a silver needle..."),
		iconLabel(icons.Summary, "checking the prophecy twice..."),
		iconLabel(icons.Tool, "calibrating the astrolabe..."),
		iconLabel(icons.Context, "gathering stardust from context..."),
		iconLabel(icons.Thinking, "pondering beneath a wizard hat..."),
		iconLabel(icons.Assistant, "hunting gremlins in the margins..."),
		iconLabel(icons.Invoke, "whispering to the shell spirits..."),
		iconLabel(icons.Summary, "illuminating the manuscript..."),
		iconLabel(icons.Tool, "unlocking the tool chest..."),
		iconLabel(icons.Context, "balancing the mana budget..."),
		iconLabel(icons.Thinking, "peering beyond the veil..."),
		iconLabel(icons.Assistant, "arranging constellations of code..."),
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
	m.agent = agent.NewWithSubagentConfig(m.agent.Provider(), m.agent.Tools(), m.sess, sys, currentSubagentConfig())
	m.agent.SetReasoningEffort(prev)
	m.agent.RegisterSubagentToolsEnabled(currentSubagentsEnabled())
	m.subagents = map[string]agent.SubagentEvent{}
	m.autoContinueSubagentResults = map[string]bool{}
	m.pendingSubagentContinuation = false
	_ = m.startSubagentListener()
}
