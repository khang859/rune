package tui

import (
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

func (m *Messages) Render(s Styles, showThinking, showToolResults bool, now time.Time) string {
	var sb strings.Builder
	for i, b := range m.blocks {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		var rendered string
		switch b.kind {
		case bkUser:
			rendered = s.User.Render("user> ") + b.text
		case bkAssistant:
			if i == m.streamingAsstIdx {
				rendered = s.Assistant.Render(b.text)
			} else {
				rendered = s.Markdown.Render(b.text)
			}
		case bkThinking:
			rendered = renderThinking(s, b, showThinking, now)
		case bkToolCall:
			rendered = s.ToolCall.Render(fmt.Sprintf("· %s(%s)", b.meta, b.text))
		case bkToolResult:
			rendered = renderToolResult(s, b, showToolResults)
		case bkError:
			rendered = s.ToolError.Render("error: " + b.text)
		case bkInfo:
			rendered = s.Info.Render(b.text)
		case bkSummary:
			header := fmt.Sprintf("── compacted summary (%d messages) ──", b.count)
			rendered = s.SummaryHeader.Render(header) + "\n" + s.Markdown.Render(b.text)
		}
		if m.width > 0 {
			rendered = ansi.Wrap(rendered, m.width, " \t")
		}
		sb.WriteString(rendered)
	}
	return sb.String()
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
		header = fmt.Sprintf("%s thinking… (%ds)", caret, secs)
	} else {
		secs := int(b.endedAt.Sub(b.startedAt).Seconds())
		if secs < 0 {
			secs = 0
		}
		header = fmt.Sprintf("%s thought for %ds", caret, secs)
	}
	headerLine := s.ThinkingHeader.Render(header)
	if !showThinking {
		return headerLine
	}
	return headerLine + "\n" + s.Thinking.Render(b.text)
}

func renderToolResult(s Styles, b block, show bool) string {
	if show {
		return s.ToolResult.Render(fmt.Sprintf("▾ ← %s\n%s", b.meta, b.text))
	}
	lines := 0
	if b.text != "" {
		lines = strings.Count(b.text, "\n") + 1
	}
	return s.ToolResult.Render(fmt.Sprintf("▸ ← %s (%d lines)", b.meta, lines))
}
