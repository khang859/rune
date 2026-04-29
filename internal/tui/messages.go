package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

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
	for i, b := range m.blocks {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		var rendered string
		switch b.kind {
		case bkUser:
			label := s.User.Render(iconLabel(s.Icons.User, "you>"))
			rendered = label + "\n" + s.Markdown.Render(b.text)
		case bkAssistant:
			label := s.Assistant.Render(iconLabel(s.Icons.Assistant, "rune"))
			if i == m.streamingAsstIdx {
				rendered = label + "\n" + s.Assistant.Render(b.text)
			} else {
				rendered = label + "\n" + s.Markdown.Render(b.text)
			}
		case bkThinking:
			rendered = renderThinking(s, b, showThinking, now)
		case bkToolCall:
			rendered = renderToolCall(s, b)
		case bkToolResult:
			rendered = renderToolResult(s, b, showToolResults)
		case bkError:
			rendered = s.ToolError.Render(iconLabel(s.Icons.Error, "error") + ": " + b.text)
		case bkInfo:
			rendered = s.Info.Render(b.text)
		case bkSummary:
			header := fmt.Sprintf("── %s (%d messages) ──", iconLabel(s.Icons.Summary, "compacted memory"), b.count)
			rendered = s.SummaryHeader.Render(header) + "\n" + s.Markdown.Render(b.text)
		case bkSubagent:
			if b.meta == string(agent.SubagentCompleted) {
				rendered = s.ToolResult.Render(b.text)
			} else if b.meta == string(agent.SubagentFailed) || b.meta == string(agent.SubagentCancelled) {
				rendered = s.ToolError.Render(b.text)
			} else {
				rendered = s.Info.Render(b.text)
			}
		}
		if m.width > 0 {
			rendered = ansi.Wrap(rendered, m.width, " \t")
		}
		sb.WriteString(rendered)
	}
	return sb.String()
}

func renderSubagentEventText(ev agent.SubagentEvent) string {
	t := ev.Task
	label := fmt.Sprintf("subagent %s", t.Name)
	if t.ID != "" {
		label += fmt.Sprintf(" (%s)", t.ID)
	}
	switch ev.Status {
	case agent.SubagentPending:
		return fmt.Sprintf("◌ %s queued", label)
	case agent.SubagentRunning:
		return fmt.Sprintf("◐ %s working…", label)
	case agent.SubagentCompleted:
		if strings.TrimSpace(t.Summary) == "" {
			return fmt.Sprintf("✓ %s completed", label)
		}
		lines := strings.Count(strings.TrimSpace(t.Summary), "\n") + 1
		return fmt.Sprintf("✓ %s completed — %d result lines added to context", label, lines)
	case agent.SubagentFailed:
		if strings.TrimSpace(t.Error) == "" {
			return fmt.Sprintf("✗ %s failed", label)
		}
		return fmt.Sprintf("✗ %s failed: %s", label, strings.TrimSpace(t.Error))
	case agent.SubagentCancelled:
		return fmt.Sprintf("⊘ %s cancelled", label)
	default:
		return fmt.Sprintf("%s %s", label, ev.Status)
	}
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
	}
	return s.ToolCall.Render(fmt.Sprintf("%s %s(%s)", iconLabel(s.Icons.Tool, "tool"), b.meta, b.text))
}

func toolHeader(s Styles, body string) string {
	return s.ToolCall.Render(iconLabel(s.Icons.Tool, "tool") + " " + body)
}

func formatBashCall(s Styles, args json.RawMessage) (string, bool) {
	var a struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	header := toolHeader(s, "bash")
	if a.Command == "" {
		return header, true
	}
	var sb strings.Builder
	sb.WriteString(header)
	for _, line := range strings.Split(a.Command, "\n") {
		sb.WriteString("\n")
		sb.WriteString(s.ToolCall.Render("  " + line))
	}
	return sb.String(), true
}

func formatWriteCall(s Styles, args json.RawMessage) (string, bool) {
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
	return toolHeader(s, fmt.Sprintf("write %s (%d lines, %d bytes)", a.Path, lines, len(a.Content))), true
}

func formatReadCall(s Styles, args json.RawMessage) (string, bool) {
	var a struct {
		Path    string `json:"path"`
		Offset  int    `json:"offset"`
		Limit   int    `json:"limit"`
		ReadAll bool   `json:"read_all"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return "", false
	}
	body := "read " + a.Path
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
	return toolHeader(s, body), true
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
