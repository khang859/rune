package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
)

type Messages struct {
	width            int
	blocks           []block
	streamingAsstIdx int // -1 when no assistant block is currently streaming
}

type blockKind int

const (
	bkUser blockKind = iota
	bkAssistant
	bkThinking
	bkToolCall
	bkToolResult
	bkError
	bkInfo
	bkSummary
	bkSubagent
)

type block struct {
	kind      blockKind
	text      string
	meta      string
	count     int
	startedAt time.Time
	endedAt   time.Time
}

func NewMessages(width int) *Messages { return &Messages{width: width, streamingAsstIdx: -1} }

func (m *Messages) SetWidth(w int) { m.width = w }

func (m *Messages) IsEmpty() bool { return len(m.blocks) == 0 }

func (m *Messages) AppendUser(text string) {
	m.blocks = append(m.blocks, block{kind: bkUser, text: text})
	m.streamingAsstIdx = -1
}

func (m *Messages) OnAssistantDelta(delta string) {
	m.FinalizeStreamingThinking(time.Now())
	if m.streamingAsstIdx == -1 {
		m.blocks = append(m.blocks, block{kind: bkAssistant})
		m.streamingAsstIdx = len(m.blocks) - 1
	}
	m.blocks[m.streamingAsstIdx].text += delta
}

// OnThinkingDelta appends to (or starts) the active thinking block, using time.Now()
// for the start timestamp. Tests should use OnThinkingDeltaAt for deterministic times.
func (m *Messages) OnThinkingDelta(delta string) {
	m.OnThinkingDeltaAt(delta, time.Now())
}

func (m *Messages) OnThinkingDeltaAt(delta string, now time.Time) {
	last := len(m.blocks)
	if last > 0 && m.blocks[last-1].kind == bkThinking && m.blocks[last-1].endedAt.IsZero() {
		m.blocks[last-1].text += delta
		return
	}
	m.blocks = append(m.blocks, block{kind: bkThinking, text: delta, startedAt: now})
}

// FinalizeStreamingThinking sets endedAt on the most recent in-progress thinking block, if any.
func (m *Messages) FinalizeStreamingThinking(now time.Time) {
	for i := len(m.blocks) - 1; i >= 0; i-- {
		if m.blocks[i].kind != bkThinking {
			continue
		}
		if m.blocks[i].endedAt.IsZero() {
			m.blocks[i].endedAt = now
		}
		return
	}
}

// HasInProgressThinking reports whether at least one thinking block has not yet been finalized.
// Used by RootModel to decide whether to keep its 1-second tick alive.
func (m *Messages) HasInProgressThinking() bool {
	for _, b := range m.blocks {
		if b.kind == bkThinking && b.endedAt.IsZero() {
			return true
		}
	}
	return false
}

func (m *Messages) OnToolStarted(call ai.ToolCall) {
	m.FinalizeStreamingThinking(time.Now())
	m.streamingAsstIdx = -1
	m.blocks = append(m.blocks, block{
		kind: bkToolCall,
		meta: call.Name,
		text: string(call.Args),
	})
}

func (m *Messages) OnToolFinished(f agent.ToolFinished) {
	kind := bkToolResult
	if f.Result.IsError {
		kind = bkError
	}
	m.blocks = append(m.blocks, block{
		kind: kind,
		meta: f.Call.Name,
		text: f.Result.Output,
	})
}

func (m *Messages) OnTurnDone(reason string) {
	m.FinalizeStreamingThinking(time.Now())
	m.streamingAsstIdx = -1
	if reason != "" && reason != "stop" {
		m.blocks = append(m.blocks, block{kind: bkInfo, text: fmt.Sprintf("(turn ended: %s)", reason)})
	}
}

func (m *Messages) OnTurnError(err error) {
	m.FinalizeStreamingThinking(time.Now())
	m.streamingAsstIdx = -1
	m.blocks = append(m.blocks, block{kind: bkError, text: err.Error()})
}

func (m *Messages) OnInfo(text string) {
	m.blocks = append(m.blocks, block{kind: bkInfo, text: text})
}

func (m *Messages) AppendSummary(text string, count int) {
	m.streamingAsstIdx = -1
	m.blocks = append(m.blocks, block{kind: bkSummary, text: text, count: count})
}

func (m *Messages) OnSubagentEvent(ev agent.SubagentEvent) {
	m.FinalizeStreamingThinking(time.Now())
	m.streamingAsstIdx = -1
	m.blocks = append(m.blocks, block{kind: bkSubagent, meta: string(ev.Status), text: renderSubagentEventText(ev)})
}

func (m *Messages) Render(s Styles, showThinking, showToolResults bool, now time.Time) string {
	var sb strings.Builder
	renderedBlocks := 0
	for i := 0; i < len(m.blocks); {
		b := m.blocks[i]
		rendered := ""
		next := i + 1
		if b.kind == bkToolCall {
			interactions, consumed := collectToolRun(m.blocks, i)
			if len(interactions) > 1 {
				rendered = renderToolGroup(s, b.meta, interactions, showToolResults)
				next = i + consumed
			} else {
				rendered = renderToolCall(s, b)
			}
		} else {
			rendered = renderBlock(s, b, showThinking, showToolResults, now, i == m.streamingAsstIdx)
		}
		if m.width > 0 {
			rendered = ansi.Wrap(rendered, m.width, " \t")
		}
		if renderedBlocks > 0 {
			sb.WriteString("\n\n")
		}
		sb.WriteString(rendered)
		renderedBlocks++
		i = next
	}
	return sb.String()
}

func renderBlock(s Styles, b block, showThinking, showToolResults bool, now time.Time, streamingAssistant bool) string {
	switch b.kind {
	case bkUser:
		label := s.User.Render(iconLabel(s.Icons.User, "you>"))
		return label + "\n" + s.Markdown.Render(b.text)
	case bkAssistant:
		label := s.Assistant.Render(iconLabel(s.Icons.Assistant, "rune"))
		if streamingAssistant {
			return label + "\n" + s.Assistant.Render(b.text)
		}
		return label + "\n" + s.Markdown.Render(b.text)
	case bkThinking:
		return renderThinking(s, b, showThinking, now)
	case bkToolCall:
		return renderToolCall(s, b)
	case bkToolResult:
		return renderToolResult(s, b, showToolResults)
	case bkError:
		return s.ToolError.Render(iconLabel(s.Icons.Error, "error") + ": " + b.text)
	case bkInfo:
		return s.Info.Render(b.text)
	case bkSummary:
		header := fmt.Sprintf("── %s (%d messages) ──", iconLabel(s.Icons.Summary, "compacted memory"), b.count)
		return s.SummaryHeader.Render(header) + "\n" + s.Markdown.Render(b.text)
	case bkSubagent:
		if b.meta == string(agent.SubagentCompleted) {
			return s.FamiliarSuccess.Render(b.text)
		}
		if b.meta == string(agent.SubagentFailed) || b.meta == string(agent.SubagentCancelled) {
			return s.ToolError.Render(b.text)
		}
		return s.FamiliarActive.Render(b.text)
	}
	return ""
}

func renderSubagentEventText(ev agent.SubagentEvent) string {
	t := ev.Task
	label := familiarLabel(t.FamiliarName, t.Name)
	if t.ID != "" {
		label += fmt.Sprintf(" (%s)", t.ID)
	}
	switch ev.Status {
	case agent.SubagentBlocked:
		return fmt.Sprintf("◌ %s waits within the summoning circle", label)
	case agent.SubagentPending:
		return fmt.Sprintf("◌ %s is being summoned", label)
	case agent.SubagentRunning:
		return fmt.Sprintf("◐ %s is scrying through the task…", label)
	case agent.SubagentCompleted:
		if strings.TrimSpace(t.Summary) == "" {
			return fmt.Sprintf("✓ %s returned from the veil", label)
		}
		lines := strings.Count(strings.TrimSpace(t.Summary), "\n") + 1
		return fmt.Sprintf("✓ %s returned with %d lines of findings added to context", label, lines)
	case agent.SubagentFailed:
		if strings.TrimSpace(t.Error) == "" {
			return fmt.Sprintf("✗ %s lost the thread", label)
		}
		return fmt.Sprintf("✗ %s lost the thread: %s", label, strings.TrimSpace(t.Error))
	case agent.SubagentCancelled:
		return fmt.Sprintf("⊘ %s was dismissed", label)
	default:
		return fmt.Sprintf("%s %s", label, ev.Status)
	}
}

func familiarLabel(familiar, task string) string {
	familiar = strings.TrimSpace(familiar)
	task = strings.TrimSpace(task)
	if familiar == "" {
		if task == "" {
			return "a familiar"
		}
		return "familiar of " + task
	}
	if task == "" {
		return familiar
	}
	return familiar + ", familiar of " + task
}

func renderThinking(s Styles, b block, showThinking bool, now time.Time) string {
	caret := "▸"
	if showThinking {
		caret = "▾"
	}
	var header string
	if b.endedAt.IsZero() {
		secs := int(now.Sub(b.startedAt).Seconds())
		if secs < 0 {
			secs = 0
		}
		header = fmt.Sprintf("%s %s… (%ds)", caret, iconLabel(s.Icons.Thinking, "thinking"), secs)
	} else {
		secs := int(b.endedAt.Sub(b.startedAt).Seconds())
		if secs < 0 {
			secs = 0
		}
		header = fmt.Sprintf("%s %s for %ds", caret, iconLabel(s.Icons.Thinking, "thought"), secs)
	}
	headerLine := s.ThinkingHeader.Render(header)
	if !showThinking {
		return headerLine
	}
	return headerLine + "\n" + s.Thinking.Render(b.text)
}

func renderToolCall(s Styles, b block) string {
	args := json.RawMessage(b.text)
	switch b.meta {
	case "edit":
		if r, ok := formatEditDiff(s, args); ok {
			return r
		}
	case "bash":
		if r, ok := formatBashCall(s, args); ok {
			return r
		}
	case "write":
		if r, ok := formatWriteCall(s, args); ok {
			return r
		}
	case "read":
		if r, ok := formatReadCall(s, args); ok {
			return r
		}
	case "spawn_subagent":
		if r, ok := formatSpawnSubagentCall(s, args); ok {
			return r
		}
	case "list_subagents":
		return familiarHeader(s, "read the familiar ledger")
	case "get_subagent_result":
		if r, ok := formatGetSubagentCall(s, args); ok {
			return r
		}
	case "cancel_subagent":
		if r, ok := formatCancelSubagentCall(s, args); ok {
			return r
		}
	}
	return s.ToolCall.Render(fmt.Sprintf("%s %s(%s)", iconLabel(s.Icons.Tool, "tool"), b.meta, b.text))
}

type toolInteraction struct {
	call   block
	result *block
}

func collectToolRun(blocks []block, start int) ([]toolInteraction, int) {
	if start < 0 || start >= len(blocks) || blocks[start].kind != bkToolCall {
		return nil, 0
	}
	name := blocks[start].meta
	var interactions []toolInteraction
	for i := start; i < len(blocks); {
		b := blocks[i]
		if b.kind != bkToolCall || b.meta != name {
			break
		}
		interaction := toolInteraction{call: b}
		i++
		if i < len(blocks) && isToolOutcomeFor(blocks[i], name) {
			interaction.result = &blocks[i]
			i++
		}
		interactions = append(interactions, interaction)
	}
	return interactions, toolRunBlockCount(interactions)
}

func toolRunBlockCount(interactions []toolInteraction) int {
	n := 0
	for _, interaction := range interactions {
		n++
		if interaction.result != nil {
			n++
		}
	}
	return n
}

func isToolOutcomeFor(b block, name string) bool {
	return (b.kind == bkToolResult || b.kind == bkError) && b.meta == name
}

func renderToolGroup(s Styles, name string, interactions []toolInteraction, showToolResults bool) string {
	var sb strings.Builder
	if isFamiliarTool(name) {
		sb.WriteString(familiarHeader(s, fmt.Sprintf("%s (%d)", groupedToolTitle(name), len(interactions))))
	} else {
		sb.WriteString(toolHeader(s, fmt.Sprintf("%s (%d)", groupedToolTitle(name), len(interactions))))
	}
	for _, interaction := range interactions {
		sb.WriteString("\n")
		sb.WriteString(groupedToolItemStyle(s, name).Render("  " + toolCallSummary(name, json.RawMessage(interaction.call.text))))
		if interaction.result != nil && (showToolResults || interaction.result.kind == bkError) {
			for _, line := range strings.Split(renderToolResultInline(*interaction.result), "\n") {
				sb.WriteString("\n")
				sb.WriteString(groupedToolResultStyle(s, *interaction.result).Render("    " + line))
			}
		}
	}
	return sb.String()
}

func groupedToolTitle(name string) string {
	switch name {
	case "spawn_subagent":
		return "summon familiar"
	case "list_subagents":
		return "read familiar ledger"
	case "get_subagent_result":
		return "unseal familiar scroll"
	case "cancel_subagent":
		return "dismiss familiar"
	default:
		return name
	}
}

func groupedToolItemStyle(s Styles, name string) lipgloss.Style {
	if isFamiliarTool(name) {
		return s.FamiliarCall
	}
	return s.ToolCall
}

func groupedToolResultStyle(s Styles, b block) lipgloss.Style {
	if b.kind == bkError {
		return s.ToolError
	}
	return s.ToolResult
}

func renderToolResultInline(b block) string {
	if b.kind == bkError {
		return "error: " + b.text
	}
	if strings.TrimSpace(b.text) == "" {
		return "(no output)"
	}
	return b.text
}

func isFamiliarTool(name string) bool {
	switch name {
	case "spawn_subagent", "list_subagents", "get_subagent_result", "cancel_subagent":
		return true
	default:
		return false
	}
}

func toolHeader(s Styles, body string) string {
	return s.ToolCall.Render(iconLabel(s.Icons.Tool, "tool") + " " + body)
}

func familiarHeader(s Styles, body string) string {
	return s.FamiliarCall.Render(iconLabel(s.Icons.Familiar, "familiar") + " " + body)
}

func formatSpawnSubagentCall(s Styles, args json.RawMessage) (string, bool) {
	var a struct {
		Name         string   `json:"name"`
		AgentType    string   `json:"agent_type"`
		Background   *bool    `json:"background"`
		Dependencies []string `json:"dependencies"`
		TimeoutSecs  int      `json:"timeout_secs"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	name := strings.TrimSpace(a.Name)
	if name == "" {
		name = "unnamed task"
	}
	body := "open a summoning circle for " + name
	if a.AgentType != "" && a.AgentType != "general" {
		body += " [" + a.AgentType + "]"
	}
	var details []string
	if len(a.Dependencies) > 0 {
		details = append(details, fmt.Sprintf("after %d omen%s", len(a.Dependencies), pluralS(len(a.Dependencies))))
	}
	if a.Background != nil && !*a.Background {
		details = append(details, "awaiting return")
	}
	if a.TimeoutSecs > 0 {
		details = append(details, fmt.Sprintf("%ds ward", a.TimeoutSecs))
	}
	if len(details) > 0 {
		body += " (" + strings.Join(details, ", ") + ")"
	}
	return familiarHeader(s, body), true
}

func formatGetSubagentCall(s Styles, args json.RawMessage) (string, bool) {
	var a struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	return familiarHeader(s, "unseal familiar scroll "+strings.TrimSpace(a.TaskID)), true
}

func formatCancelSubagentCall(s Styles, args json.RawMessage) (string, bool) {
	var a struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	return familiarHeader(s, "dismiss familiar "+strings.TrimSpace(a.TaskID)), true
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func toolCallSummary(name string, args json.RawMessage) string {
	switch name {
	case "read":
		if s, ok := summarizeReadArgs(args); ok {
			return s
		}
	case "write":
		if s, ok := summarizeWriteArgs(args); ok {
			return s
		}
	case "edit":
		if s, ok := summarizePathArg(args, "path"); ok {
			return s
		}
	case "bash":
		if s, ok := summarizeBashArgs(args); ok {
			return s
		}
	case "list_files":
		if s, ok := summarizeListFilesArgs(args); ok {
			return s
		}
	case "search_files":
		if s, ok := summarizeSearchFilesArgs(args); ok {
			return s
		}
	case "web_search":
		if s, ok := summarizeWebSearchArgs(args); ok {
			return s
		}
	case "web_fetch":
		if s, ok := summarizeWebFetchArgs(args); ok {
			return s
		}
	case "git_status":
		if s, ok := summarizeGitStatusArgs(args); ok {
			return s
		}
	case "git_diff":
		if s, ok := summarizeGitDiffArgs(args); ok {
			return s
		}
	case "spawn_subagent":
		if s, ok := summarizeSpawnSubagentArgs(args); ok {
			return s
		}
	case "list_subagents":
		return "ledger"
	case "get_subagent_result", "cancel_subagent":
		if s, ok := summarizePathArg(args, "task_id"); ok {
			return s
		}
	}
	return compactJSON(args)
}

func formatBashCall(s Styles, args json.RawMessage) (string, bool) {
	summary, ok := summarizeBashArgs(args)
	if !ok {
		return "", false
	}
	header := toolHeader(s, "bash")
	if summary == "" {
		return header, true
	}
	var sb strings.Builder
	sb.WriteString(header)
	for _, line := range strings.Split(summary, "\n") {
		sb.WriteString("\n")
		sb.WriteString(s.ToolCall.Render("  " + line))
	}
	return sb.String(), true
}

func summarizeBashArgs(args json.RawMessage) (string, bool) {
	var a struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	return a.Command, true
}

func formatWriteCall(s Styles, args json.RawMessage) (string, bool) {
	summary, ok := summarizeWriteArgs(args)
	if !ok {
		return "", false
	}
	return toolHeader(s, "write "+summary), true
}

func summarizeWriteArgs(args json.RawMessage) (string, bool) {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	lines := 0
	if a.Content != "" {
		lines = strings.Count(a.Content, "\n")
		if !strings.HasSuffix(a.Content, "\n") {
			lines++
		}
	}
	return fmt.Sprintf("%s (%d lines, %d bytes)", a.Path, lines, len(a.Content)), true
}

func formatReadCall(s Styles, args json.RawMessage) (string, bool) {
	summary, ok := summarizeReadArgs(args)
	if !ok {
		return "", false
	}
	return toolHeader(s, "read "+summary), true
}

func summarizeReadArgs(args json.RawMessage) (string, bool) {
	var a struct {
		Path    string `json:"path"`
		Offset  int    `json:"offset"`
		Limit   int    `json:"limit"`
		ReadAll bool   `json:"read_all"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	body := a.Path
	switch {
	case a.ReadAll:
		body += " (all)"
	case a.Offset > 0 && a.Limit > 0:
		body += fmt.Sprintf(" (lines %d-%d)", a.Offset, a.Offset+a.Limit-1)
	case a.Offset > 0:
		body += fmt.Sprintf(" (from line %d)", a.Offset)
	case a.Limit > 0:
		body += fmt.Sprintf(" (first %d lines)", a.Limit)
	}
	return body, true
}

func summarizePathArg(args json.RawMessage, key string) (string, bool) {
	var m map[string]any
	if err := json.Unmarshal(args, &m); err != nil {
		return "", false
	}
	v, ok := m[key]
	if !ok {
		return "", true
	}
	return strings.TrimSpace(fmt.Sprint(v)), true
}

func summarizeListFilesArgs(args json.RawMessage) (string, bool) {
	var a struct {
		Path       string `json:"path"`
		Glob       string `json:"glob"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	parts := []string{defaultIfBlank(a.Path, ".")}
	if a.Glob != "" {
		parts = append(parts, "glob "+a.Glob)
	}
	if a.MaxResults > 0 {
		parts = append(parts, fmt.Sprintf("max %d", a.MaxResults))
	}
	return strings.Join(parts, " · "), true
}

func summarizeSearchFilesArgs(args json.RawMessage) (string, bool) {
	var a struct {
		Query        string `json:"query"`
		Path         string `json:"path"`
		Glob         string `json:"glob"`
		ContextLines int    `json:"context_lines"`
		MaxResults   int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	parts := []string{fmt.Sprintf("%q in %s", a.Query, defaultIfBlank(a.Path, "."))}
	if a.Glob != "" {
		parts = append(parts, "glob "+a.Glob)
	}
	if a.ContextLines > 0 {
		parts = append(parts, fmt.Sprintf("context %d", a.ContextLines))
	}
	if a.MaxResults > 0 {
		parts = append(parts, fmt.Sprintf("max %d", a.MaxResults))
	}
	return strings.Join(parts, " · "), true
}

func summarizeWebSearchArgs(args json.RawMessage) (string, bool) {
	var a struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	if a.Limit > 0 {
		return fmt.Sprintf("%q (limit %d)", a.Query, a.Limit), true
	}
	return fmt.Sprintf("%q", a.Query), true
}

func summarizeWebFetchArgs(args json.RawMessage) (string, bool) {
	var a struct {
		URL      string `json:"url"`
		MaxBytes int64  `json:"max_bytes"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	if a.MaxBytes > 0 {
		return fmt.Sprintf("%s (max %d bytes)", a.URL, a.MaxBytes), true
	}
	return a.URL, true
}

func summarizeGitStatusArgs(args json.RawMessage) (string, bool) {
	var a struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	return defaultIfBlank(a.Path, "."), true
}

func summarizeGitDiffArgs(args json.RawMessage) (string, bool) {
	var a struct {
		Repo     string `json:"repo"`
		Path     string `json:"path"`
		Staged   bool   `json:"staged"`
		Stat     bool   `json:"stat"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	parts := []string{defaultIfBlank(a.Repo, ".")}
	if a.Path != "" {
		parts = append(parts, a.Path)
	}
	if a.Staged {
		parts = append(parts, "staged")
	}
	if a.Stat {
		parts = append(parts, "stat")
	}
	if a.MaxBytes > 0 {
		parts = append(parts, fmt.Sprintf("max %d bytes", a.MaxBytes))
	}
	return strings.Join(parts, " · "), true
}

func summarizeSpawnSubagentArgs(args json.RawMessage) (string, bool) {
	var a struct {
		Name      string `json:"name"`
		AgentType string `json:"agent_type"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	name := defaultIfBlank(a.Name, "unnamed task")
	if a.AgentType != "" && a.AgentType != "general" {
		return name + " [" + a.AgentType + "]", true
	}
	return name, true
}

func defaultIfBlank(s, def string) string {
	if strings.TrimSpace(s) == "" {
		return def
	}
	return s
}

func compactJSON(args json.RawMessage) string {
	var v any
	if err := json.Unmarshal(args, &v); err != nil {
		return string(args)
	}
	b, err := json.Marshal(v)
	if err != nil {
		return string(args)
	}
	return string(b)
}

// formatEditDiff renders an edit tool call as a header line plus a unified
// diff body: "- " for old_string lines, "+ " for new_string lines. Returns
// false if the args can't be parsed, so the caller can fall back to the
// generic format.
func formatEditDiff(s Styles, args json.RawMessage) (string, bool) {
	var a struct {
		Path      string `json:"path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	var sb strings.Builder
	sb.WriteString(toolHeader(s, "edit "+a.Path))
	for _, line := range strings.Split(a.OldString, "\n") {
		sb.WriteString("\n")
		sb.WriteString(s.DiffDel.Render("- " + line))
	}
	for _, line := range strings.Split(a.NewString, "\n") {
		sb.WriteString("\n")
		sb.WriteString(s.DiffAdd.Render("+ " + line))
	}
	return sb.String(), true
}

func renderToolResult(s Styles, b block, show bool) string {
	if show {
		return s.ToolResult.Render(fmt.Sprintf("▾ %s %s\n%s", s.Icons.Tool, b.meta, b.text))
	}
	lines := 0
	if b.text != "" {
		lines = strings.Count(b.text, "\n") + 1
	}
	return s.ToolResult.Render(fmt.Sprintf("▸ %s %s (%d lines)", s.Icons.Tool, b.meta, lines))
}
