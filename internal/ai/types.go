package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

type Provider interface {
	Stream(ctx context.Context, req Request) (<-chan Event, error)
}

type Request struct {
	Model     string          `json:"model"`
	System    string          `json:"system,omitempty"`
	Messages  []Message       `json:"messages"`
	Tools     []ToolSpec      `json:"tools,omitempty"`
	Reasoning ReasoningConfig `json:"reasoning,omitempty"`
}

type Role string

const (
	RoleUser       Role = "user"
	RoleAssistant  Role = "assistant"
	RoleToolResult Role = "tool_result"
)

type Message struct {
	Role       Role           `json:"role"`
	Content    []ContentBlock `json:"content"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type ContentBlock interface{ isContentBlock() }

type TextBlock struct {
	Text string `json:"text"`
}

func (TextBlock) isContentBlock() {}

type ImageBlock struct {
	Data     []byte `json:"data"`
	MimeType string `json:"mime_type"`
}

func (ImageBlock) isContentBlock() {}

type ToolUseBlock struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

func (ToolUseBlock) isContentBlock() {}

type ToolResultBlock struct {
	ToolCallID string `json:"tool_call_id"`
	Output     string `json:"output"`
	IsError    bool   `json:"is_error,omitempty"`
}

func (ToolResultBlock) isContentBlock() {}

type ToolSpec struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Schema      json.RawMessage `json:"schema"`
}

type ReasoningConfig struct {
	Effort string `json:"effort,omitempty"`
}

// Custom JSON for Message so we can deserialize the polymorphic Content slice.
type messageWire struct {
	Role       Role          `json:"role"`
	Content    []contentWire `json:"content"`
	ToolCallID string        `json:"tool_call_id,omitempty"`
}

type contentWire struct {
	Type       string          `json:"type"`
	Text       string          `json:"text,omitempty"`
	Data       []byte          `json:"data,omitempty"`
	MimeType   string          `json:"mime_type,omitempty"`
	ID         string          `json:"id,omitempty"`
	Name       string          `json:"name,omitempty"`
	Args       json.RawMessage `json:"args,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
	Output     string          `json:"output,omitempty"`
	IsError    bool            `json:"is_error,omitempty"`
}

func (m Message) MarshalJSON() ([]byte, error) {
	w := messageWire{Role: m.Role, ToolCallID: m.ToolCallID}
	for _, c := range m.Content {
		switch v := c.(type) {
		case TextBlock:
			w.Content = append(w.Content, contentWire{Type: "text", Text: v.Text})
		case ImageBlock:
			w.Content = append(w.Content, contentWire{Type: "image", Data: v.Data, MimeType: v.MimeType})
		case ToolUseBlock:
			w.Content = append(w.Content, contentWire{Type: "tool_use", ID: v.ID, Name: v.Name, Args: v.Args})
		case ToolResultBlock:
			w.Content = append(w.Content, contentWire{Type: "tool_result", ToolCallID: v.ToolCallID, Output: v.Output, IsError: v.IsError})
		default:
			return nil, fmt.Errorf("unknown content block: %T", c)
		}
	}
	return json.Marshal(w)
}

func (m *Message) UnmarshalJSON(b []byte) error {
	var w messageWire
	if err := json.Unmarshal(b, &w); err != nil {
		return err
	}
	m.Role = w.Role
	m.ToolCallID = w.ToolCallID
	m.Content = nil
	for _, c := range w.Content {
		switch c.Type {
		case "text":
			m.Content = append(m.Content, TextBlock{Text: c.Text})
		case "image":
			m.Content = append(m.Content, ImageBlock{Data: c.Data, MimeType: c.MimeType})
		case "tool_use":
			m.Content = append(m.Content, ToolUseBlock{ID: c.ID, Name: c.Name, Args: c.Args})
		case "tool_result":
			m.Content = append(m.Content, ToolResultBlock{ToolCallID: c.ToolCallID, Output: c.Output, IsError: c.IsError})
		default:
			return fmt.Errorf("unknown content type: %q", c.Type)
		}
	}
	return nil
}

type Event interface{ isEvent() }

type TextDelta struct{ Text string }

func (TextDelta) isEvent() {}

type Thinking struct{ Text string }

func (Thinking) isEvent() {}

type ToolCall struct {
	ID   string
	Name string
	Args json.RawMessage
}

func (ToolCall) isEvent() {}

type Usage struct {
	Input     int
	Output    int
	CacheRead int
}

func (Usage) isEvent() {}

// ErrorClass describes the nature of a stream-level failure so the agent
// loop can decide whether to retry, heal session state, or surface to the user.
// Each provider classifies its own errors at the boundary it understands;
// the agent treats all providers uniformly via Class.
type ErrorClass int

const (
	// ErrFatal is an unrecoverable error. Surface to the user.
	ErrFatal ErrorClass = iota
	// ErrTransient is a network blip / partial stream. Retry with short backoff.
	ErrTransient
	// ErrRateLimit is a 429-equivalent. Retry with backoff.
	ErrRateLimit
	// ErrServer is a 5xx-equivalent. Retry with backoff.
	ErrServer
	// ErrOrphanOutput is a provider rejection caused by a missing tool_result
	// for a previously-emitted tool_use (e.g., OpenAI Responses 400 on
	// "missing required parameter: 'input[N].output'"). The agent heals
	// session state, then retries.
	ErrOrphanOutput
	// ErrToolGenerationFailed is a provider rejection caused by the model
	// producing output that resembles a tool call but cannot be parsed as
	// one (e.g., Groq's "Failed to call a function. ... failed_generation").
	// The agent appends a corrective nudge so the model sees its mistake,
	// then retries the same turn.
	ErrToolGenerationFailed
)

type StreamError struct {
	Err   error
	Class ErrorClass
}

func (StreamError) isEvent() {}

type Done struct{ Reason string }

func (Done) isEvent() {}
