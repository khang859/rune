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
	"github.com/khang859/rune/internal/ai/codex"
	"github.com/khang859/rune/internal/ai/groq"
	"github.com/khang859/rune/internal/ai/oauth"
	"github.com/khang859/rune/internal/ai/ollama"
	"github.com/khang859/rune/internal/ai/runpod"
	"github.com/khang859/rune/internal/ai/unavailable"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/mcp"
	"github.com/khang859/rune/internal/providers"
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
	activeProfile   string
	lastModal       modal.Modal
	pendingForkMode bool
	clipboardReady  bool
	clipboardErr    error
	copyMode        bool
	planPending     bool

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
	version          string

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
	"/quit", "/providers", "/model", "/thinking", "/tree", "/resume", "/settings", "/mcp", "/mcp-status",
	"/git-status", "/new", "/clear", "/name", "/session", "/fork", "/clone", "/copy", "/copy-mode",
	"/plan", "/act", "/approve", "/cancel-plan",
	"/compact", "/reload", "/hotkeys", "/skill-creator", "/feature-dev",
}

func NewRootModel(a *agent.Agent, sess *session.Session) *RootModel {
	realCwd, _ := os.Getwd()
	loadedSettings, err := config.LoadSettings(config.SettingsPath())
	if err != nil {
		loadedSettings = config.DefaultSettings()
	}
	settings := modalSettingsFromConfig(loadedSettings, braveKeyConfigured(), tavilyKeyConfigured())
	iconMode := settings.IconMode
	if iconMode == "" || iconMode == "auto" {
		iconMode = string(DefaultIconMode())
	}
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
	footer := Footer{
		Cwd:            displayCwd,
		GitBranch:      currentGitBranch(realCwd),
		Model:          sess.Model,
		ThinkingEffort: footerThinkingEffort(sess.Model, a.ReasoningEffort()),
		Mode:           footerMode(a.Mode()),
	}
	if providerUnavailable(a.Provider()) {
		footer.Model = "no provider"
		footer.ThinkingEffort = ""
	}
	return &RootModel{
		agent:                       a,
		sess:                        sess,
		styles:                      DefaultStylesWithIconMode(iconMode),
		msgs:                        NewMessages(80),
		viewport:                    viewport.New(80, 20),
		editor:                      ed,
		footer:                      footer,
		queue:                       &Queue{},
		settings:                    settings,
		activeProfile:               loadedSettings.ActiveProfile,
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

func (m *RootModel) SetActiveProfile(profileID string) {
	m.activeProfile = strings.TrimSpace(profileID)
}

func (m *RootModel) SetVersion(version string) {
	m.version = version
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
			m.msgs.AppendUser(formatUserMessageForDisplay(item.displayText(), countImages(item.Attachments)))
			m.refreshViewport()
			return m, m.startTurn(item.Text, item.Attachments)
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
		compactCmd := m.maybeStartAutoCompact()
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
		if compactCmd != nil {
			return m, compactCmd
		}
		if item, ok := m.queue.Pop(); ok {
			m.msgs.AppendUser(formatUserMessageForDisplay(item.displayText(), countImages(item.Attachments)))
			m.refreshViewport()
			return m, m.startTurn(item.Text, item.Attachments)
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
			return m, m.cycleInteractionMode()
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
	if res.ShellCommand != "" {
		return m.handleShellShortcut(res, cmd)
	}
	if res.Send {
		if !m.hasActiveProvider() {
			m.msgs.OnInfo("(no active provider configured; use /providers to choose one, or /settings to add API keys)")
			m.refreshViewport()
			m.layout()
			return m, cmd
		}
		resolved := resolveFileReferences(res.Text, m.editor.Cwd(), m.sess.Provider, m.sess.Model)
		displayText := res.Text
		text := resolved.Text
		attachments := imageBlocksToContent(res.Images)
		attachments = append(attachments, resolved.Attachments...)
		if summary := attachmentSummary(resolved.Attached); summary != "" {
			m.msgs.OnInfo(summary)
		}
		for _, warning := range resolved.Warnings {
			m.msgs.OnInfo("(" + warning + ")")
		}
		if m.pendingSkillBody != "" {
			text = m.pendingSkillBody + "\n\n" + text
			displayText = m.pendingSkillBody + "\n\n" + displayText
			m.pendingSkillBody = ""
		}
		imageCount := countImages(attachments)
		if m.streaming || m.compacting {
			m.queue.Push(QueueItem{Text: text, DisplayText: displayText, Attachments: attachments})
			m.msgs.OnInfo(queueMessage(m.queue.Len(), imageCount))
			m.refreshViewport()
			return m, cmd
		}
		m.msgs.AppendUser(formatUserMessageForDisplay(displayText, imageCount))
		m.refreshViewport()
		return m, m.startTurn(text, attachments)
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

func (m *RootModel) handleShellShortcut(res editor.Result, cmd tea.Cmd) (tea.Model, tea.Cmd) {
	if m.agent.Mode() == agent.ModePlan {
		m.msgs.OnInfo("shell shortcuts are disabled in Plan Mode; use /act to run commands")
		m.refreshViewport()
		m.layout()
		return m, cmd
	}
	out, _ := editor.RunShell(context.Background(), res.ShellCommand)
	if res.ShellMode == editor.ShellModeInsert {
		label := fmt.Sprintf("(ran: %s)", res.ShellCommand)
		body := out
		if body == "" {
			body = "(no output)"
		}
		m.msgs.OnInfo(label + "\n" + body)
		m.refreshViewport()
		return m, cmd
	}
	text := "I ran `" + res.ShellCommand + "` and it produced:\n```\n" + out + "\n```"
	if m.pendingSkillBody != "" {
		text = m.pendingSkillBody + "\n\n" + text
		m.pendingSkillBody = ""
	}
	attachments := imageBlocksToContent(res.Images)
	imageCount := countImages(attachments)
	if m.streaming || m.compacting {
		m.queue.Push(QueueItem{Text: text, Attachments: attachments})
		m.msgs.OnInfo(queueMessage(m.queue.Len(), imageCount))
		m.refreshViewport()
		return m, cmd
	}
	m.msgs.AppendUser(formatUserMessageForDisplay(text, imageCount))
	m.refreshViewport()
	return m, m.startTurn(text, attachments)
}

func (m *RootModel) warnImageSupport(images []ai.ImageBlock) {
	if len(images) == 0 {
		return
	}
	switch providers.ImageInputSupport(m.sess.Provider, m.sess.Model) {
	case providers.ImageUnsupported:
		m.msgs.OnInfo(fmt.Sprintf("(%d image(s) attached; %s/%s is not documented as image-capable, so the provider may reject or ignore them)", len(images), m.sess.Provider, m.sess.Model))
	case providers.ImageUnknown:
		m.msgs.OnInfo(fmt.Sprintf("(%d image(s) attached; image support for %s/%s is unknown, sending anyway)", len(images), m.sess.Provider, m.sess.Model))
	}
}

func queueMessage(n, images int) string {
	if images > 0 {
		return fmt.Sprintf("queued (%d in queue, %d image(s) attached)", n, images)
	}
	return fmt.Sprintf("queued (%d in queue)", n)
}

func (m *RootModel) maybeAutoNameSession(text string) {
	if m == nil || m.sess == nil || strings.TrimSpace(m.sess.Name) != "" {
		return
	}
	name := autoSessionName(text)
	if name == "" {
		return
	}
	m.sess.Name = name
}

func autoSessionName(text string) string {
	text = stripFileBlocks(text)
	if _, after, ok := strings.Cut(text, "Feature request:"); ok {
		text = after
	}
	text = strings.Join(strings.Fields(text), " ")
	text = strings.Trim(text, " \t\n\r\"'`")
	if text == "" || strings.HasPrefix(text, "I ran `") || strings.HasPrefix(text, "You are running Rune's") {
		return ""
	}
	return truncateRunes(text, 60)
}

func stripFileBlocks(text string) string {
	for {
		start := strings.Index(text, "<file ")
		if start < 0 {
			return text
		}
		end := strings.Index(text[start:], "</file>")
		if end < 0 {
			return text[:start]
		}
		end += start + len("</file>")
		text = text[:start] + " " + text[end:]
	}
}

func truncateRunes(s string, limit int) string {
	if limit <= 0 || len([]rune(s)) <= limit {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}

func formatUserMessageForDisplay(text string, imageCount int) string {
	if imageCount == 0 {
		return text
	}
	label := fmt.Sprintf("[%d image(s) attached]", imageCount)
	if strings.TrimSpace(text) == "" {
		return label
	}
	return text + "\n" + label
}

func (m *RootModel) startTurn(text string, attachments []ai.ContentBlock) tea.Cmd {
	if !m.hasActiveProvider() {
		m.msgs.OnInfo("(no active provider configured; use /providers to choose one, or /settings to add API keys)")
		m.refreshViewport()
		m.layout()
		return nil
	}
	m.warnImageSupport(imagesFromBlocks(attachments))
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.streaming = true
	m.editor.Blur()
	m.layout()
	m.viewport.GotoBottom()
	content := []ai.ContentBlock{ai.TextBlock{Text: text}}
	content = append(content, attachments...)
	msg := ai.Message{Role: ai.RoleUser, Content: content}
	ch := m.agent.Run(ctx, msg)
	m.maybeAutoNameSession(text)
	m.saveSessionIfStarted()
	m.eventCh = ch
	return tea.Batch(nextEventCmd(ch), m.startActivityTick())
}

const subagentContinuationPrompt = "A subagent has completed. Review the subagent result that was added to your context and continue the user's task. If actions are needed, take them; otherwise briefly report the conclusion."

const featureDevPrompt = `You are running Rune's /feature-dev workflow for a non-trivial feature request.

Follow this workflow:

1. Discovery
   - Understand the requested feature and summarize the goal, constraints, and success criteria.
   - If the request is unclear, ask exactly one clarifying question at a time and wait.
   - Prefer repository evidence over assumptions.

2. Codebase exploration
   - Spawn 2-3 code-explorer subagents with distinct focuses such as entry points, data flow, similar features, tests, UI/CLI/API integration, persistence, or configuration.
   - Ask each code-explorer to return file:line evidence and essential files for the parent agent to read.
   - Let subagents run; do not immediately duplicate delegated work unless a small amount of context is needed to synthesize findings.
   - After subagents complete, read the key files yourself before designing.

3. Clarifying questions
   - Identify blocking ambiguity around scope, edge cases, integration points, compatibility, validation, data migration, permissions, or user experience.
   - Ask specific questions before architecture design when decisions cannot be resolved from the repository.

4. Architecture design
   - Use code-architect subagents for architecture options or focused design analysis when useful.
   - Present approaches and tradeoffs when they matter, make a clear recommendation, and provide an implementation plan.
   - Ask the user to approve the implementation plan before editing files or running mutating commands.

5. Implementation
   - Respect Plan Mode / Act Mode semantics. Do not use mutating tools before explicit approval.
   - Keep changes surgical, preserve user work, and follow existing project style.
   - Add or update tests when practical.
   - Do not rely on TodoWrite; maintain progress with concise status updates.

6. Quality review
   - After implementation, spawn code-reviewer subagents for focused reviews.
   - Ask reviewers to report only high-confidence, actionable issues with confidence >= 80.
   - Consolidate findings and fix approved issues.

7. Summary
   - Summarize what changed, files modified, validation run, and remaining risks.`

const skillCreatorPrompt = `You are helping the user create or improve a rune skill.

Rune skills are single Markdown files placed in ~/.rune/skills/ or ./.rune/skills/. The filename without .md becomes the slash command slug, for example refactor-step.md becomes /skill:refactor-step. Rune prepends the entire skill body to the user's next submitted message. Rune skills currently have no schema, no front matter, no skill folders, no bundled scripts, and no automatic progressive-disclosure references.

Follow this workflow:

1. Understand the intended reusable workflow.
   - Ask at most one clarifying question at a time when required.
   - Prefer concrete source material: a real successful task, user corrections, project conventions, runbooks, examples, edge cases, or desired input/output formats.
   - If the user only has a rough idea, help narrow it to one coherent skill.

2. Decide whether a skill is warranted.
   - Create a skill for repeatable procedures, project-specific conventions, domain-specific gotchas, or tool/API usage the agent would otherwise get wrong.
   - Push back if the request is too broad, too generic, or already handled well without a skill.

3. Draft a rune-compatible Markdown skill.
   - Write only the skill body, unless the user asks for explanation too.
   - Keep it concise and directly actionable.
   - Include what the agent would not know without the skill; omit generic advice.
   - Favor procedures over declarations: steps, defaults, gotchas, validation loops, and output templates.
   - Provide clear defaults instead of presenting many equal options.
   - Match specificity to task fragility: be prescriptive for fragile sequences, flexible where judgment is useful.
   - Include a short "When to use" section only if it helps users choose the skill.
   - Include examples or success criteria when they clarify expected behavior.
   - Do not mention unsupported rune skill features such as front matter, SKILL.md folders, references directories, or bundled scripts as requirements.

4. Suggest installation.
   - Recommend a kebab-case filename such as ~/.rune/skills/<slug>.md or ./.rune/skills/<slug>.md.
   - Tell the user to run /reload if rune is already open.

5. Iterate.
   - Encourage testing on a real task and revising based on where the agent wastes effort, misses context, follows irrelevant instructions, or needs more concrete defaults.`

func (m *RootModel) startSubagentContinuationTurn() tea.Cmd {
	return m.startTurn(subagentContinuationPrompt, nil)
}

func providerUnavailable(p ai.Provider) bool {
	_, ok := p.(*unavailable.Provider)
	return ok
}

func (m *RootModel) hasActiveProvider() bool {
	return !providerUnavailable(m.agent.Provider())
}

func (m *RootModel) noProviderNotice() string {
	if m.hasActiveProvider() {
		return ""
	}
	return "No active provider configured.\nUse /providers to choose one, or /settings to add API keys.\nCodex requires: rune login codex"
}

func (m *RootModel) handleSlashCommand(cmd string) tea.Cmd {
	cmd = strings.TrimSpace(cmd)
	name, arg, hasArg := strings.Cut(cmd, " ")
	arg = strings.TrimSpace(arg)
	if strings.HasPrefix(name, "/skill:") && !hasArg {
		slug := strings.TrimPrefix(name, "/skill:")
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
	if name == "/subagent-cancel" && hasArg {
		m.cancelSubagentsCommand(arg)
		m.refreshViewport()
		m.layout()
		return nil
	}
	var initCmd tea.Cmd
	switch name {
	case "/quit":
		return tea.Quit
	case "/providers":
		initCmd = m.openProviderPicker()
	case "/model":
		initCmd = m.openModelPicker()
	case "/thinking":
		if !m.hasActiveProvider() {
			m.msgs.OnInfo("(no active provider configured; use /providers first)")
			break
		}
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
	case "/git-status":
		data, err := loadGitStatusData(m.editor.Cwd())
		if err != nil {
			m.msgs.OnInfo(fmt.Sprintf("(git status unavailable: %v)", err))
			break
		}
		initCmd = m.openModal(modal.NewGitStatus(data))
	case "/hotkeys":
		initCmd = m.openModal(modal.NewHotkeys())
	case "/skill-creator":
		m.pendingSkillBody = skillCreatorPrompt
		m.msgs.OnInfo("(skill-creator armed; describe the skill you want to create or improve)")
	case "/feature-dev":
		if arg == "" {
			m.pendingSkillBody = featureDevPrompt
			m.msgs.OnInfo("(feature-dev armed; describe the feature you want to build)")
			break
		}
		if !m.hasActiveProvider() {
			m.msgs.OnInfo("(no active provider configured; use /providers to choose one, or /settings to add API keys)")
			break
		}
		text := featureDevPrompt + "\n\nFeature request:\n" + arg
		if m.streaming || m.compacting {
			m.queue.Push(QueueItem{Text: text})
			m.msgs.OnInfo(queueMessage(m.queue.Len(), 0))
			break
		}
		m.msgs.AppendUser(text)
		m.refreshViewport()
		initCmd = m.startTurn(text, nil)
	case "/new", "/clear":
		m.startNewSession()
	case "/name":
		if strings.TrimSpace(arg) == "" {
			m.msgs.OnInfo("(usage: /name <session name>)")
			m.refreshViewport()
			break
		}
		m.sess.Name = strings.TrimSpace(arg)
		m.saveSessionIfStarted()
		m.msgs.OnInfo(fmt.Sprintf("(session name: %s)", m.sess.Name))
		m.refreshViewport()
	case "/session":
		m.msgs.OnInfo(fmt.Sprintf("session id=%s name=%q provider=%s model=%s", m.sess.ID, m.sess.Name, m.sess.Provider, m.sess.Model))
	case "/fork":
		if m.streaming {
			m.msgs.OnInfo("(busy — wait for current turn to finish)")
			break
		}
		if len(m.sess.Root.Children) == 0 {
			m.msgs.OnInfo("(nothing to fork yet)")
			m.refreshViewport()
			break
		}
		m.pendingForkMode = true
		initCmd = m.openModal(modal.NewForkTree(m.sess))
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
	case "/plan":
		if !m.canChangeAgentMode() {
			m.msgs.OnInfo("(busy — wait for current turn to finish)")
			break
		}
		initCmd = m.enterPlanMode("plan mode: edits and bash disabled; read-only gh available; MCP tools require read-only allowlist")
	case "/act":
		if !m.canChangeAgentMode() {
			m.msgs.OnInfo("(busy — wait for current turn to finish)")
			break
		}
		initCmd = m.enterActMode("act mode: implementation tools enabled")
	case "/approve":
		if !m.canChangeAgentMode() {
			m.msgs.OnInfo("(busy — wait for current turn to finish)")
			break
		}
		initCmd = m.enterActMode("plan approved; act mode enabled — send your next message to implement")
	case "/cancel-plan":
		if m.streaming || m.compacting {
			m.msgs.OnInfo("(busy — wait for current turn to finish)")
			break
		}
		m.planPending = false
		m.msgs.OnInfo("plan cancelled; still in plan mode")
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

func footerMode(mode agent.Mode) string {
	if mode == agent.ModePlan {
		return "plan"
	}
	return ""
}

func (m *RootModel) openProviderPicker() tea.Cmd {
	settings, _ := config.LoadSettings(config.SettingsPath())
	settings = config.NormalizeSettings(settings)
	choices := []modal.ProviderChoice{
		{ID: providers.Codex, Label: "Codex", Value: settings.CodexModel},
		{ID: providers.Groq, Label: "Groq", Value: settings.GroqModel},
	}
	choices = append(choices, modal.ProviderChoice{ID: providers.Ollama, Label: "Ollama", Value: settings.OllamaModel})
	for _, p := range settings.Profiles {
		if p.Provider == providers.Ollama {
			choices = append(choices, providerChoiceFromProfile(p))
		}
	}
	choices = append(choices, modal.ProviderChoice{ID: providers.Runpod, Label: "Runpod", Value: settings.RunpodModel})
	for _, p := range settings.Profiles {
		if p.Provider == providers.Runpod {
			choices = append(choices, providerChoiceFromProfile(p))
		}
	}
	return m.openModal(modal.NewProviderProfilePicker(choices, m.sess.Provider, m.activeProfile))
}

func providerChoiceFromProfile(p config.ProviderProfile) modal.ProviderChoice {
	label := strings.ToUpper(p.Provider[:1]) + p.Provider[1:] + ": " + config.ProfileDisplayName(p)
	value := p.Model
	if p.Endpoint != "" {
		if value != "" {
			value += " · "
		}
		value += p.Endpoint
	}
	return modal.ProviderChoice{ID: p.Provider, ProfileID: p.ID, Label: label, Value: value}
}

func (m *RootModel) openModelPicker() tea.Cmd {
	if !m.hasActiveProvider() {
		m.msgs.OnInfo("(no active provider configured; use /providers first)")
		m.refreshViewport()
		return nil
	}
	if providers.Normalize(m.sess.Provider) != providers.Ollama {
		return m.openModal(modal.NewModelPicker(providers.Models(m.sess.Provider), m.sess.Model))
	}
	items := providers.Models(providers.Ollama)
	settings, _ := config.LoadSettings(config.SettingsPath())
	resolved := providers.Resolve(settings, providers.ResolveOptions{ProviderOverride: providers.Ollama, ProfileOverride: providers.Profile(m.activeProfile)})
	endpoint := resolved.Endpoint
	store := config.NewSecretStore(config.SecretsPath())
	key, keyErr := config.OllamaEnvAPIKey()
	if keyErr == nil && key == "" && resolved.ProfileID != "" {
		key, keyErr = store.ProfileAPIKey(resolved.ProfileID)
	}
	if keyErr == nil && key == "" {
		key, keyErr = store.OllamaAPIKey()
	}
	if keyErr != nil {
		m.msgs.OnInfo(fmt.Sprintf("(could not load Ollama API key: %v; choose custom or run `ollama pull <model>`)", keyErr))
		m.refreshViewport()
		return m.openModal(modal.NewOllamaModelPicker(items, m.sess.Model))
	}
	if models, err := ollama.ListModels(context.Background(), endpoint, key); err == nil && len(models) > 0 {
		items = models
	} else if err != nil {
		m.msgs.OnInfo(fmt.Sprintf("(could not list Ollama models: %v; choose custom or run `ollama pull <model>`)", err))
		m.refreshViewport()
	}
	return m.openModal(modal.NewOllamaModelPicker(items, m.sess.Model))
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
		if task.FamiliarName != "" {
			b.WriteString(" ")
			b.WriteString(task.FamiliarName)
		}
		if task.Name != "" {
			b.WriteString(" — ")
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
	} else if m.agent.Mode() == agent.ModePlan {
		banner = m.styles.PlanModeBanner.Render("Plan Mode: edits and bash disabled · read-only gh available · MCP tools require read-only allowlist · /approve or /act to implement")
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
	if m.copyMode || m.agent.Mode() == agent.ModePlan {
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
		m.viewport.SetContent(renderSplashWithNotice(m.width, m.viewport.Height, m.styles, m.version, m.noProviderNotice()))
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
	wasShowingActivity := m.showActivity()
	m.subagents[e.Task.ID] = e
	m.msgs.OnSubagentEvent(e)
	isActive := isActiveSubagentStatus(string(e.Status))
	if isActive {
		m.pendingTickCmd = m.startActivityTick()
	} else if !m.showActivity() {
		m.stopActivityTick()
	}
	if wasShowingActivity != m.showActivity() {
		m.layout()
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
		m.msgs.OnInfo("context overflow — auto-compacting and retrying…")
	case agent.InvalidToolCallRecovered:
		names := strings.Join(v.Names, ", ")
		m.msgs.OnInfo(fmt.Sprintf("model emitted invalid tool call(s): %s — recovering with a nudge", names))
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
	case "llama3.2", "qwen3:4b", "qwen3:8b", "qwen2.5-coder:7b", "qwen2.5-coder:14b", "deepseek-r1:8b", "gpt-oss:20b":
		return 128_000
	case "gpt-5.3-codex-spark":
		return 128_000
	case "llama-3.3-70b-versatile", "llama-3.1-8b-instant", "openai/gpt-oss-120b", "groq/compound", "groq/compound-mini", "deepseek-r1-distill-llama-70b", "meta-llama/llama-4-maverick-17b-128e-instruct", "meta-llama/llama-4-scout-17b-16e-instruct":
		return 131_072
	case "qwen/qwen3-32b":
		return 131_000
	case "openai/gpt-oss-20b":
		return 131_072
	case "gpt-5.1", "gpt-5.1-codex-max", "gpt-5.1-codex-mini",
		"gpt-5.2", "gpt-5.2-codex", "gpt-5.3-codex",
		"gpt-5.4", "gpt-5.4-mini", "gpt-5.5":
		return 272_000
	default:
		return 272_000
	}
}

func codexModelIDs() []string { return providers.Models(providers.Codex) }

func thinkingLevelsForModel(model string) []string {
	switch strings.ToLower(model) {
	case "gpt-5.5", "gpt-5.4", "gpt-5.2":
		return []string{"none", "low", "medium", "high", "xhigh"}
	case "gpt-5.3-codex", "gpt-5.2-codex":
		return []string{"low", "medium", "high", "xhigh"}
	case "gpt-5.1":
		return []string{"none", "low", "medium", "high"}
	}
	if levels := providers.GroqThinkingLevels(strings.ToLower(model)); levels != nil {
		return levels
	}
	return nil
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
	case *modal.ProviderPicker:
		if choice, ok := payload.(modal.ProviderChoice); ok {
			return m.switchProviderChoice(choice)
		}
		if id, ok := payload.(string); ok {
			return m.switchProvider(id)
		}
	case *modal.ModelPicker:
		if id, ok := payload.(string); ok {
			if id == modal.ModelPickerCustom {
				return m.openModal(modal.NewTextInput("Ollama model id", "ollama_model", m.sess.Model))
			}
			m.sess.Model = id
			m.footer.Model = id
			m.footer.ContextPct = ctxPctForModel(m.sess.Model, m.currentTokens)
			m.clampThinkingForCurrentModel()
			m.refreshFooterThinkingEffort()
			m.saveProviderSettings()
			m.saveSessionIfStarted()
		}
	case *modal.ThinkingPicker:
		if effort, ok := payload.(string); ok {
			m.applyThinkingEffort(effort)
		}
	case *modal.SettingsModal:
		switch v := payload.(type) {
		case modal.SettingsAction:
			if v.Settings.Provider != m.sess.Provider {
				return m.switchProvider(v.Settings.Provider)
			}
			m.applySettings(v.Settings)
			if v.Action == "brave_api_key" {
				return m.openModal(modal.NewSecretInput("Brave Search API key", "brave_api_key"))
			}
			if v.Action == "tavily_api_key" {
				return m.openModal(modal.NewSecretInput("Tavily API key", "tavily_api_key"))
			}
			if v.Action == "groq_api_key" {
				return m.openModal(modal.NewSecretInput("Groq API key", "groq_api_key"))
			}
			if v.Action == "ollama_api_key" {
				return m.openModal(modal.NewSecretInput("Ollama API key", "ollama_api_key"))
			}
			if v.Action == "add_ollama_profile" {
				return m.openModal(modal.NewTextInput("Ollama profile name", "add_ollama_profile", ""))
			}
			if v.Action == "edit_active_profile" {
				return m.openEditActiveProfile()
			}
			if v.Action == "runpod_api_key" {
				return m.openModal(modal.NewSecretInput("Runpod API key", "runpod_api_key"))
			}
			if v.Action == "ollama_endpoint" {
				settings, _ := config.LoadSettings(config.SettingsPath())
				return m.openModal(modal.NewTextInput("Ollama endpoint", "ollama_endpoint", settings.OllamaEndpoint))
			}
			if v.Action == "runpod_endpoint" {
				settings, _ := config.LoadSettings(config.SettingsPath())
				return m.openModal(modal.NewTextInput("Runpod endpoint", "runpod_endpoint", settings.RunpodEndpoint))
			}
		case modal.Settings:
			if v.Provider != m.sess.Provider {
				return m.switchProvider(v.Provider)
			}
			m.applySettings(v)
		}
	case *modal.TextInput:
		if res, ok := payload.(modal.TextInputResult); ok {
			switch res.Action {
			case "add_ollama_profile":
				return m.addOllamaProfile(res.Value)
			case "edit_profile_endpoint":
				return m.saveActiveProfileEndpoint(res.Value)
			case "ollama_model":
				if strings.TrimSpace(res.Value) != "" {
					id := strings.TrimSpace(res.Value)
					m.sess.Model = id
					m.footer.Model = id
					m.footer.ContextPct = ctxPctForModel(m.sess.Model, m.currentTokens)
					m.clampThinkingForCurrentModel()
					m.refreshFooterThinkingEffort()
					m.saveProviderSettings()
					m.saveSessionIfStarted()
					m.msgs.OnInfo(fmt.Sprintf("(ollama model: %s)", id))
					m.refreshViewport()
				}
			case "ollama_endpoint":
				m.saveEndpointSetting(providers.Ollama, res.Value)
			case "runpod_endpoint":
				m.saveEndpointSetting(providers.Runpod, res.Value)
			}
		}
	case *modal.SecretInput:
		if res, ok := payload.(modal.SecretInputResult); ok {
			switch res.Action {
			case "brave_api_key":
				if err := config.NewSecretStore(config.SecretsPath()).SetBraveSearchAPIKey(res.Value); err != nil {
					m.msgs.OnTurnError(err)
				} else {
					m.settings.BraveAPIKeyStatus = "configured — Enter to replace"
					m.reconfigureWebTools()
					m.msgs.OnInfo("(saved Brave Search API key; web_search enabled if settings allow it)")
				}
			case "tavily_api_key":
				if err := config.NewSecretStore(config.SecretsPath()).SetTavilyAPIKey(res.Value); err != nil {
					m.msgs.OnTurnError(err)
				} else {
					m.settings.TavilyAPIKeyStatus = "configured — Enter to replace"
					m.reconfigureWebTools()
					m.msgs.OnInfo("(saved Tavily API key; web_search enabled if settings allow it)")
				}
			case "groq_api_key":
				if err := config.NewSecretStore(config.SecretsPath()).SetGroqAPIKey(res.Value); err != nil {
					m.msgs.OnTurnError(err)
				} else {
					m.settings.GroqAPIKeyStatus = "configured — Enter to replace"
					m.msgs.OnInfo("(saved Groq API key)")
				}
			case "ollama_api_key":
				store := config.NewSecretStore(config.SecretsPath())
				var err error
				if m.activeProfile != "" && m.sess.Provider == providers.Ollama {
					err = store.SetProfileAPIKey(m.activeProfile, res.Value)
				} else {
					err = store.SetOllamaAPIKey(res.Value)
				}
				if err != nil {
					m.msgs.OnTurnError(err)
				} else {
					m.settings.OllamaAPIKeyStatus = "configured — Enter to replace"
					if m.sess.Provider == providers.Ollama {
						settings, _ := config.LoadSettings(config.SettingsPath())
						resolved := providers.Resolve(settings, providers.ResolveOptions{ProviderOverride: providers.Ollama, ProfileOverride: providers.Profile(m.activeProfile)})
						if p, err := buildTUIProviderResolved(resolved); err == nil {
							m.replaceActiveProvider(p)
						} else {
							m.msgs.OnTurnError(err)
						}
					}
					if m.activeProfile != "" && m.sess.Provider == providers.Ollama {
						m.msgs.OnInfo("(saved Ollama profile API key)")
					} else {
						m.msgs.OnInfo("(saved Ollama API key)")
					}
				}
			case "runpod_api_key":
				if err := config.NewSecretStore(config.SecretsPath()).SetRunpodAPIKey(res.Value); err != nil {
					m.msgs.OnTurnError(err)
				} else {
					m.settings.RunpodAPIKeyStatus = "configured — Enter to replace"
					m.msgs.OnInfo("(saved Runpod API key)")
				}
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
	nc.Provider = m.sess.Provider
	nc.SetPath(filepath.Join(config.SessionsDir(), nc.ID+".json"))
	m.swapSession(nc)
}

func (m *RootModel) saveSessionIfStarted() {
	if m == nil || m.sess == nil || len(m.sess.PathToActive()) == 0 {
		return
	}
	_ = m.sess.Save()
}

func (m *RootModel) switchProvider(provider string) tea.Cmd {
	return m.switchProviderChoice(modal.ProviderChoice{ID: provider})
}

func (m *RootModel) switchProviderChoice(choice modal.ProviderChoice) tea.Cmd {
	provider := strings.TrimSpace(choice.ID)
	if provider == "none" {
		provider = ""
	}
	if provider != "" {
		provider = providers.Normalize(provider)
	}
	profileID := strings.TrimSpace(choice.ProfileID)
	if provider == m.sess.Provider && profileID == m.activeProfile {
		return nil
	}
	loadedSettings, _ := config.LoadSettings(config.SettingsPath())
	loadedSettings = config.NormalizeSettings(loadedSettings)
	resolved := providers.Resolve(loadedSettings, providers.ResolveOptions{ProviderOverride: provider, ProfileOverride: providers.Profile(profileID)})
	model := resolved.Model
	if provider == "" {
		model = ""
	}
	p, err := buildTUIProviderResolved(resolved)
	if err != nil {
		m.msgs.OnTurnError(err)
		m.refreshViewport()
		return nil
	}
	m.stopActiveTurn()
	m.sess.Provider = provider
	m.sess.Model = model
	m.activeProfile = resolved.ProfileID
	m.footer.Model = model
	if provider == "" {
		m.footer.Model = "no provider"
	}
	m.footer.ContextPct = ctxPctForModel(m.sess.Model, m.currentTokens)
	prev := m.agent.ReasoningEffort()
	prevMode := m.agent.Mode()
	m.agent = agent.NewWithSubagentConfig(p, m.agent.Tools(), m.sess, m.agent.System(), currentSubagentConfig())
	m.agent.SetReasoningEffort(prev)
	m.agent.SetMode(prevMode)
	m.agent.RegisterSubagentToolsEnabled(currentSubagentsEnabled())
	m.subagents = map[string]agent.SubagentEvent{}
	m.autoContinueSubagentResults = map[string]bool{}
	m.pendingSubagentContinuation = false
	_ = m.startSubagentListener()
	m.settings.Provider = provider
	loadedSettings, _ = config.LoadSettings(config.SettingsPath())
	loadedSettings.Provider = provider
	loadedSettings.ActiveProfile = m.activeProfile
	m.settings = modalSettingsFromConfig(loadedSettings, braveKeyConfigured(), tavilyKeyConfigured())
	m.clampThinkingForCurrentModel()
	m.refreshFooterThinkingEffort()
	if provider == "" {
		m.footer.ThinkingEffort = ""
	}
	m.saveProviderSettings()
	m.saveSessionIfStarted()
	if provider == "" {
		m.msgs.OnInfo("(no active provider configured)")
	} else {
		m.msgs.OnInfo(fmt.Sprintf("(provider: %s, model: %s)", provider, model))
	}
	m.refreshViewport()
	return nil
}

func buildTUIProvider(provider string) (ai.Provider, error) {
	settings, _ := config.LoadSettings(config.SettingsPath())
	resolved := providers.Resolve(settings, providers.ResolveOptions{ProviderOverride: provider})
	return buildTUIProviderResolved(resolved)
}

func buildTUIProviderResolved(resolved providers.ResolvedProvider) (ai.Provider, error) {
	if strings.TrimSpace(resolved.Provider) == "" {
		return unavailable.New("no active provider configured"), nil
	}
	switch providers.Normalize(resolved.Provider) {
	case providers.Groq:
		endpoint := resolved.Endpoint
		key, err := config.NewSecretStore(config.SecretsPath()).GroqAPIKey()
		if err != nil {
			return nil, err
		}
		return groq.New(endpoint, key), nil
	case providers.Ollama:
		endpoint := resolved.Endpoint
		store := config.NewSecretStore(config.SecretsPath())
		key, err := config.OllamaEnvAPIKey()
		if err != nil {
			return nil, err
		}
		if key == "" && resolved.ProfileID != "" {
			key, err = store.ProfileAPIKey(resolved.ProfileID)
			if err != nil {
				return nil, err
			}
		}
		if key == "" {
			key, err = store.OllamaAPIKey()
			if err != nil {
				return nil, err
			}
		}
		return ollama.New(endpoint, key), nil
	case providers.Runpod:
		endpoint := resolved.Endpoint
		key, err := config.NewSecretStore(config.SecretsPath()).RunpodAPIKey()
		if err != nil {
			return nil, err
		}
		return runpod.New(endpoint, key), nil
	default:
		endpoint := oauth.CodexResponsesBaseURL + oauth.CodexResponsesPath
		if v := os.Getenv("RUNE_CODEX_ENDPOINT"); v != "" {
			endpoint = v
		}
		tokenURL := oauth.CodexTokenURL
		if v := os.Getenv("RUNE_OAUTH_TOKEN_URL"); v != "" {
			tokenURL = v
		}
		src := &oauth.CodexSource{Store: oauth.NewStore(config.AuthPath()), TokenURL: tokenURL}
		if _, err := src.Token(context.Background()); err != nil {
			return nil, fmt.Errorf("not logged in: %w (run `rune login codex`)", err)
		}
		return codex.New(endpoint, src), nil
	}
}

func (m *RootModel) replaceActiveProvider(p ai.Provider) {
	prev := m.agent.ReasoningEffort()
	prevMode := m.agent.Mode()
	m.agent = agent.NewWithSubagentConfig(p, m.agent.Tools(), m.sess, m.agent.System(), currentSubagentConfig())
	m.agent.SetReasoningEffort(prev)
	m.agent.SetMode(prevMode)
	m.agent.RegisterSubagentToolsEnabled(currentSubagentsEnabled())
	m.subagents = map[string]agent.SubagentEvent{}
	m.autoContinueSubagentResults = map[string]bool{}
	m.pendingSubagentContinuation = false
	_ = m.startSubagentListener()
}

func (m *RootModel) saveEndpointSetting(provider, endpoint string) {
	s, err := config.LoadSettings(config.SettingsPath())
	if err != nil {
		s = config.DefaultSettings()
	}
	endpoint = strings.TrimSpace(endpoint)
	switch provider {
	case providers.Ollama:
		s.OllamaEndpoint = endpoint
	case providers.Runpod:
		s.RunpodEndpoint = endpoint
	}
	if err := config.SaveSettings(config.SettingsPath(), s); err != nil {
		m.msgs.OnTurnError(fmt.Errorf("settings: %v", err))
		return
	}
	updated, _ := config.LoadSettings(config.SettingsPath())
	m.settings = modalSettingsFromConfig(updated, braveKeyConfigured(), tavilyKeyConfigured())
	if m.sess.Provider == provider {
		if p, err := buildTUIProvider(provider); err == nil {
			m.replaceActiveProvider(p)
		} else {
			m.msgs.OnTurnError(err)
		}
	}
	if provider == providers.Runpod && endpoint == "" {
		m.msgs.OnInfo("(runpod endpoint reset to model default)")
	} else if provider == providers.Ollama && endpoint == "" {
		m.msgs.OnInfo("(ollama endpoint reset to default local)")
	} else {
		m.msgs.OnInfo(fmt.Sprintf("(%s endpoint saved)", provider))
	}
	m.refreshViewport()
}

func (m *RootModel) saveProviderSettings() {
	s := configFromModalSettings(m.settings)
	resolved := providers.Resolve(s, providers.ResolveOptions{ProviderOverride: m.sess.Provider, ModelOverride: m.sess.Model, ProfileOverride: providers.Profile(m.activeProfile)})
	if err := providers.SaveResolvedSelection(config.SettingsPath(), s, resolved); err != nil {
		m.msgs.OnTurnError(fmt.Errorf("settings: %v", err))
	}
}

func (m *RootModel) openEditActiveProfile() tea.Cmd {
	settings, _ := config.LoadSettings(config.SettingsPath())
	if p := config.FindProviderProfile(settings.Profiles, m.activeProfile); p != nil {
		return m.openModal(modal.NewTextInput("Profile endpoint", "edit_profile_endpoint", p.Endpoint))
	}
	m.msgs.OnInfo("(no active profile to edit; use add ollama profile first)")
	m.refreshViewport()
	return nil
}

func (m *RootModel) addOllamaProfile(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "Ollama"
	}
	settings, err := config.LoadSettings(config.SettingsPath())
	if err != nil {
		settings = config.DefaultSettings()
	}
	settings = config.NormalizeSettings(settings)
	id := uniqueProfileID(settings.Profiles, "ollama-"+slug(name))
	settings.Profiles = append(settings.Profiles, config.ProviderProfile{ID: id, Name: name, Provider: providers.Ollama, Endpoint: settings.OllamaEndpoint, Model: settings.OllamaModel})
	settings.Provider = providers.Ollama
	settings.ActiveProfile = id
	if err := config.SaveSettings(config.SettingsPath(), settings); err != nil {
		m.msgs.OnTurnError(fmt.Errorf("settings: %v", err))
		return nil
	}
	m.msgs.OnInfo(fmt.Sprintf("(added Ollama profile %s; edit active profile to set endpoint)", name))
	return m.switchProviderChoice(modal.ProviderChoice{ID: providers.Ollama, ProfileID: id})
}

func (m *RootModel) saveActiveProfileEndpoint(endpoint string) tea.Cmd {
	settings, err := config.LoadSettings(config.SettingsPath())
	if err != nil {
		settings = config.DefaultSettings()
	}
	settings = config.NormalizeSettings(settings)
	p := config.FindProviderProfile(settings.Profiles, m.activeProfile)
	if p == nil {
		m.msgs.OnInfo("(no active profile to edit)")
		m.refreshViewport()
		return nil
	}
	for i := range settings.Profiles {
		if settings.Profiles[i].ID == m.activeProfile {
			settings.Profiles[i].Endpoint = strings.TrimSpace(endpoint)
		}
	}
	if err := config.SaveSettings(config.SettingsPath(), settings); err != nil {
		m.msgs.OnTurnError(fmt.Errorf("settings: %v", err))
		return nil
	}
	updated, _ := config.LoadSettings(config.SettingsPath())
	m.settings = modalSettingsFromConfig(updated, braveKeyConfigured(), tavilyKeyConfigured())
	if m.sess.Provider == p.Provider {
		resolved := providers.Resolve(updated, providers.ResolveOptions{ProviderOverride: p.Provider, ProfileOverride: providers.Profile(m.activeProfile)})
		if provider, err := buildTUIProviderResolved(resolved); err == nil {
			m.replaceActiveProvider(provider)
		} else {
			m.msgs.OnTurnError(err)
		}
	}
	m.msgs.OnInfo("(profile endpoint saved)")
	m.refreshViewport()
	return nil
}

func uniqueProfileID(profiles []config.ProviderProfile, base string) string {
	base = strings.Trim(base, "-")
	if base == "" {
		base = "ollama"
	}
	id := base
	for n := 2; config.FindProviderProfile(profiles, id) != nil; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	lastDash := false
	for _, r := range s {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	return strings.Trim(b.String(), "-")
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
	oldProvider := m.sess.Provider
	m.sess = s
	m.footer.Model = s.Model
	if strings.TrimSpace(s.Provider) == "" {
		m.footer.Model = "no provider"
	}
	m.rebuildMessagesFromSession()
	prev := m.agent.ReasoningEffort()
	prevMode := m.agent.Mode()
	p := m.agent.Provider()
	if s.Provider != oldProvider {
		m.activeProfile = ""
		if np, err := buildTUIProvider(s.Provider); err == nil {
			p = np
		} else {
			m.msgs.OnTurnError(err)
		}
	}
	m.agent = agent.NewWithSubagentConfig(p, m.agent.Tools(), s, m.agent.System(), currentSubagentConfig())
	m.agent.SetReasoningEffort(prev)
	m.agent.SetMode(prevMode)
	m.footer.Mode = footerMode(m.agent.Mode())
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
			var text string
			images := 0
			for _, c := range msg.Content {
				switch v := c.(type) {
				case ai.TextBlock:
					text += v.Text
				case ai.ImageBlock:
					images++
				}
			}
			m.msgs.AppendUser(formatUserMessageForDisplay(text, images))
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

func (m *RootModel) maybeStartAutoCompact() tea.Cmd {
	if m.compacting || m.streaming || !m.settingsAutoCompactEnabled() || m.currentTokens <= 0 {
		return nil
	}
	threshold := m.settingsAutoCompactThreshold()
	if threshold <= 0 {
		return nil
	}
	if ctxPctForModel(m.sess.Model, m.currentTokens) < threshold {
		return nil
	}
	if !sessionCanCompact(m.sess) {
		return nil
	}
	m.compacting = true
	m.editor.Blur()
	m.msgs.OnInfo(fmt.Sprintf("(auto-compacting at %d%% context…)", threshold))
	m.refreshViewport()
	m.layout()
	return tea.Batch(m.startCompact(), m.startActivityTick())
}

func (m *RootModel) settingsAutoCompactEnabled() bool {
	return m.settings.AutoCompact != "off"
}

func (m *RootModel) settingsAutoCompactThreshold() int {
	return parsePercentDefault(m.settings.AutoCompactThreshold, 80)
}

func sessionCanCompact(s *session.Session) bool {
	path := s.PathToActive()
	return lastUserIndex(path) > 0
}

func lastUserIndex(path []ai.Message) int {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i].Role == ai.RoleUser {
			return i
		}
	}
	return -1
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
		if isActiveSubagentStatus(string(ev.Status)) {
			count++
		}
	}
	return count
}

func isActiveSubagentStatus(status string) bool {
	return status == string(agent.SubagentBlocked) || status == string(agent.SubagentPending) || status == string(agent.SubagentRunning)
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
	label := "familiar"
	if n != 1 {
		label = "familiars"
	}
	return subagentSpinnerText(fmt.Sprintf("%d %s scrying", n, label), m.activityFrame)
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

func (m *RootModel) canChangeAgentMode() bool {
	return !m.streaming && !m.compacting
}

func (m *RootModel) enterPlanMode(info string) tea.Cmd {
	cmd := m.exitCopyMode()
	m.agent.SetMode(agent.ModePlan)
	m.footer.Mode = footerMode(m.agent.Mode())
	m.planPending = true
	if info != "" {
		m.msgs.OnInfo(info)
	}
	m.layout()
	return cmd
}

func (m *RootModel) enterActMode(info string) tea.Cmd {
	cmd := m.exitCopyMode()
	m.agent.SetMode(agent.ModeAct)
	m.footer.Mode = footerMode(m.agent.Mode())
	m.planPending = false
	if info != "" {
		m.msgs.OnInfo(info)
	}
	m.layout()
	return cmd
}

// cycleInteractionMode advances through normal/act -> plan -> copy ->
// normal/act. Copy mode is terminal-native, so entering it leaves plan mode and
// surrenders mouse capture; leaving it restores mouse capture.
func (m *RootModel) cycleInteractionMode() tea.Cmd {
	if m.copyMode {
		return m.enterActMode("normal mode")
	}
	if m.agent.Mode() == agent.ModePlan {
		if !m.canChangeAgentMode() {
			m.msgs.OnInfo("(busy — wait for current turn to finish)")
			m.refreshViewport()
			return nil
		}
		m.agent.SetMode(agent.ModeAct)
		m.footer.Mode = footerMode(m.agent.Mode())
		m.planPending = false
		return m.enterCopyMode()
	}
	if !m.canChangeAgentMode() {
		m.msgs.OnInfo("(busy — wait for current turn to finish)")
		m.refreshViewport()
		return nil
	}
	return m.enterPlanMode("plan mode: edits and bash disabled; read-only gh available; MCP tools require read-only allowlist")
}

// toggleCopyMode flips terminal-native copy mode. When entering, we surrender
// mouse capture so the terminal handles click-drag selection itself; the editor
// is blurred and the box dims to signal it's not accepting input. When exiting,
// we re-enable wheel-scroll and restore editor focus (unless a turn is
// streaming or compacting, which independently keep the editor blurred).
func (m *RootModel) toggleCopyMode() tea.Cmd {
	if m.copyMode {
		return m.exitCopyMode()
	}
	return m.enterCopyMode()
}

func (m *RootModel) enterCopyMode() tea.Cmd {
	if m.copyMode {
		return nil
	}
	m.copyMode = true
	m.editor.Blur()
	m.layout()
	return tea.DisableMouse
}

func (m *RootModel) exitCopyMode() tea.Cmd {
	if !m.copyMode {
		return nil
	}
	m.copyMode = false
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
