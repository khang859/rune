package ollama

import (
	"encoding/base64"
	"encoding/json"

	"github.com/khang859/rune/internal/ai"
)

// payload is the body for Ollama's native /api/chat endpoint. Unlike the
// OpenAI-compatible /v1/chat/completions endpoint, this one honors:
//   - top-level `think` (Qwen3/DeepSeek thinking toggle)
//   - `options.num_ctx` (KV cache size; thinking models stall without this)
//   - `images` on the user message (no OpenAI content-parts wrapper)
//   - tool result messages keyed by `tool_name` instead of `tool_call_id`
type payload struct {
	Model    string         `json:"model"`
	Messages []messageWire  `json:"messages"`
	Stream   bool           `json:"stream"`
	Think    bool           `json:"think"`
	Tools    []toolWire     `json:"tools,omitempty"`
	Options  payloadOptsRaw `json:"options,omitempty"`
}

type payloadOptsRaw struct {
	NumCtx int `json:"num_ctx,omitempty"`
}

type payloadOptions struct {
	NumCtx int
	Think  bool
}

type messageWire struct {
	Role      string         `json:"role"`
	Content   string         `json:"content,omitempty"`
	Images    []string       `json:"images,omitempty"`
	ToolCalls []toolCallWire `json:"tool_calls,omitempty"`
	ToolName  string         `json:"tool_name,omitempty"`
}

type toolCallWire struct {
	Function toolCallFunction `json:"function"`
}

// toolCallFunction.Arguments is a JSON object on the native endpoint (not a
// JSON-encoded string like OpenAI). Use RawMessage so we preserve whatever the
// model or our own assistant turn produced.
type toolCallFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
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

func buildPayload(req ai.Request, opts payloadOptions) ([]byte, error) {
	p := payload{
		Model:  req.Model,
		Stream: true,
		Think:  opts.Think,
	}
	if opts.NumCtx > 0 {
		p.Options.NumCtx = opts.NumCtx
	}
	if req.System != "" {
		p.Messages = append(p.Messages, messageWire{Role: "system", Content: req.System})
	}
	// Native /api/chat tool results are keyed by tool_name, but our shared
	// ToolResultBlock only carries the call id. Build a call_id -> tool_name
	// map from prior assistant turns so we can recover the name.
	callNames := map[string]string{}
	for _, m := range req.Messages {
		if m.Role == ai.RoleAssistant {
			for _, c := range m.Content {
				if tu, ok := c.(ai.ToolUseBlock); ok && tu.ID != "" {
					callNames[tu.ID] = tu.Name
				}
			}
		}
		items, err := messageToMessages(m, callNames)
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
	return json.Marshal(p)
}

func messageToMessages(m ai.Message, callNames map[string]string) ([]messageWire, error) {
	switch m.Role {
	case ai.RoleUser:
		text, images := collectUserContent(m.Content)
		if text == "" && len(images) == 0 {
			return nil, nil
		}
		return []messageWire{{Role: "user", Content: text, Images: images}}, nil
	case ai.RoleAssistant:
		msg := messageWire{Role: "assistant"}
		var text string
		for _, c := range m.Content {
			switch v := c.(type) {
			case ai.TextBlock:
				text += v.Text
			case ai.ToolUseBlock:
				args := v.Args
				if len(args) == 0 {
					args = json.RawMessage(`{}`)
				}
				msg.ToolCalls = append(msg.ToolCalls, toolCallWire{
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
				name := callNames[id]
				if name == "" {
					// Fall back to the call id so the message still typechecks
					// on the server; tool routing may be imperfect but the
					// content reaches the model.
					name = id
				}
				out = append(out, messageWire{Role: "tool", ToolName: name, Content: text})
			}
		}
		return out, nil
	}
	return nil, nil
}

func collectUserContent(blocks []ai.ContentBlock) (string, []string) {
	var text string
	var images []string
	for _, c := range blocks {
		switch v := c.(type) {
		case ai.TextBlock:
			if v.Text != "" {
				if text != "" {
					text += "\n"
				}
				text += v.Text
			}
		case ai.ImageBlock:
			if len(v.Data) > 0 {
				images = append(images, base64.StdEncoding.EncodeToString(v.Data))
			}
		case ai.DocumentBlock:
			if v.Text != "" {
				if text != "" {
					text += "\n"
				}
				text += v.Text
			}
		}
	}
	return text, images
}
