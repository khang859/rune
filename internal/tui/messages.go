package tui

import (
	"fmt"
	"strings"

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
)

type block struct {
	kind blockKind
	text string
	meta string
}

func NewMessages(width int) *Messages { return &Messages{width: width, streamingAsstIdx: -1} }

func (m *Messages) SetWidth(w int) { m.width = w }

func (m *Messages) AppendUser(text string) {
	m.blocks = append(m.blocks, block{kind: bkUser, text: text})
	m.streamingAsstIdx = -1
}

func (m *Messages) OnAssistantDelta(delta string) {
	if m.streamingAsstIdx == -1 {
		m.blocks = append(m.blocks, block{kind: bkAssistant})
		m.streamingAsstIdx = len(m.blocks) - 1
	}
	m.blocks[m.streamingAsstIdx].text += delta
}

func (m *Messages) OnThinkingDelta(delta string) {
	last := len(m.blocks)
	if last > 0 && m.blocks[last-1].kind == bkThinking {
		m.blocks[last-1].text += delta
		return
	}
	m.blocks = append(m.blocks, block{kind: bkThinking, text: delta})
}

func (m *Messages) OnToolStarted(call ai.ToolCall) {
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
	m.streamingAsstIdx = -1
	if reason != "" && reason != "stop" {
		m.blocks = append(m.blocks, block{kind: bkThinking, text: fmt.Sprintf("(turn ended: %s)", reason)})
	}
}

func (m *Messages) OnTurnError(err error) {
	m.streamingAsstIdx = -1
	m.blocks = append(m.blocks, block{kind: bkError, text: err.Error()})
}

func (m *Messages) Render(s Styles) string {
	var sb strings.Builder
	for i, b := range m.blocks {
		if i > 0 {
			sb.WriteString("\n\n")
		}
		switch b.kind {
		case bkUser:
			sb.WriteString(s.User.Render("user> ") + b.text)
		case bkAssistant:
			sb.WriteString(s.Assistant.Render(b.text))
		case bkThinking:
			sb.WriteString(s.Thinking.Render(b.text))
		case bkToolCall:
			sb.WriteString(s.ToolCall.Render(fmt.Sprintf("· %s(%s)", b.meta, b.text)))
		case bkToolResult:
			sb.WriteString(s.ToolResult.Render(fmt.Sprintf("← %s\n%s", b.meta, b.text)))
		case bkError:
			sb.WriteString(s.ToolError.Render("error: " + b.text))
		}
	}
	return sb.String()
}
