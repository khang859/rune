package runpod

import (
	"encoding/base64"
	"encoding/json"

	"github.com/khang859/rune/internal/ai"
)

type payload struct {
	Model         string        `json:"model"`
	Messages      []messageWire `json:"messages"`
	Stream        bool          `json:"stream"`
	StreamOptions streamOptions `json:"stream_options"`
	Tools         []toolWire    `json:"tools,omitempty"`
	ToolChoice    string        `json:"tool_choice,omitempty"`
}

type streamOptions struct {
	IncludeUsage bool `json:"include_usage"`
}

type messageWire struct {
	Role       string         `json:"role"`
	Content    any            `json:"content,omitempty"`
	ToolCalls  []toolCallWire `json:"tool_calls,omitempty"`
	ToolCallID string         `json:"tool_call_id,omitempty"`
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type toolCallWire struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function toolCallFunction `json:"function"`
}

type toolCallFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type toolWire struct {
	Type     string       `json:"type"`
	Function functionWire `json:"function"`
}

type functionWire struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

func buildPayload(req ai.Request) ([]byte, error) {
	p := payload{
		Model:         req.Model,
		Stream:        true,
		StreamOptions: streamOptions{IncludeUsage: true},
	}
	if req.System != "" {
		p.Messages = append(p.Messages, messageWire{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		items, err := messageToMessages(m)
		if err != nil {
			return nil, err
		}
		p.Messages = append(p.Messages, items...)
	}
	for _, t := range req.Tools {
		p.Tools = append(p.Tools, toolWire{Type: "function", Function: functionWire{
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Schema,
		}})
	}
	if len(p.Tools) > 0 {
		p.ToolChoice = "auto"
	}
	return json.Marshal(p)
}

func messageToMessages(m ai.Message) ([]messageWire, error) {
	switch m.Role {
	case ai.RoleUser:
		parts := contentParts(m.Content)
		if len(parts) == 0 {
			return nil, nil
		}
		if len(parts) == 1 && parts[0].Type == "text" {
			return []messageWire{{Role: "user", Content: parts[0].Text}}, nil
		}
		return []messageWire{{Role: "user", Content: parts}}, nil
	case ai.RoleAssistant:
		msg := messageWire{Role: "assistant"}
		var text string
		for _, c := range m.Content {
			switch v := c.(type) {
			case ai.TextBlock:
				text += v.Text
			case ai.ToolUseBlock:
				args := string(v.Args)
				if args == "" {
					args = "{}"
				}
				msg.ToolCalls = append(msg.ToolCalls, toolCallWire{
					ID:   v.ID,
					Type: "function",
					Function: toolCallFunction{
						Name:      v.Name,
						Arguments: args,
					},
				})
			}
		}
		if text != "" {
			msg.Content = text
		} else if len(msg.ToolCalls) == 0 {
			return nil, nil
		}
		return []messageWire{msg}, nil
	case ai.RoleToolResult:
		var out []messageWire
		for _, c := range m.Content {
			if v, ok := c.(ai.ToolResultBlock); ok {
				text := v.Output
				if text == "" {
					text = "(no output)"
				}
				id := v.ToolCallID
				if id == "" {
					id = m.ToolCallID
				}
				out = append(out, messageWire{Role: "tool", ToolCallID: id, Content: text})
			}
		}
		return out, nil
	}
	return nil, nil
}

func contentParts(blocks []ai.ContentBlock) []contentPart {
	var parts []contentPart
	for _, c := range blocks {
		switch v := c.(type) {
		case ai.TextBlock:
			if v.Text != "" {
				parts = append(parts, contentPart{Type: "text", Text: v.Text})
			}
		case ai.ImageBlock:
			if len(v.Data) > 0 && v.MimeType != "" {
				parts = append(parts, contentPart{Type: "image_url", ImageURL: &imageURL{URL: "data:" + v.MimeType + ";base64," + base64.StdEncoding.EncodeToString(v.Data)}})
			}
		case ai.DocumentBlock:
			if v.Text != "" {
				parts = append(parts, contentPart{Type: "text", Text: v.Text})
			}
		}
	}
	return parts
}
