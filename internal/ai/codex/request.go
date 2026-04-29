package codex

import (
	"encoding/json"

	"github.com/khang859/rune/internal/ai"
)

type payload struct {
	Model         string         `json:"model"`
	Stream        bool           `json:"stream"`
	Instructions  string         `json:"instructions,omitempty"`
	Input         []inputItem    `json:"input"`
	Tools         []payloadTool  `json:"tools,omitempty"`
	ToolChoice    string         `json:"tool_choice,omitempty"`
	ParallelTools bool           `json:"parallel_tool_calls,omitempty"`
	Reasoning     map[string]any `json:"reasoning,omitempty"`
	Store         bool           `json:"store"`
}

type inputItem struct {
	Type      string         `json:"type"`
	Role      string         `json:"role,omitempty"`
	Content   []inputContent `json:"content,omitempty"`
	CallID    string         `json:"call_id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Arguments string         `json:"arguments,omitempty"`
	Output    string         `json:"output,omitempty"`
}

type inputContent struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type payloadTool struct {
	Type        string          `json:"type"`
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

func buildPayload(req ai.Request) ([]byte, error) {
	p := payload{
		Model:        req.Model,
		Stream:       true,
		Instructions: req.System,
		ToolChoice:   "auto",
		Store:        false,
	}
	if req.Reasoning.Effort != "" {
		p.Reasoning = map[string]any{"effort": req.Reasoning.Effort, "summary": "auto"}
	}
	for _, t := range req.Tools {
		p.Tools = append(p.Tools, payloadTool{
			Type:        "function",
			Name:        t.Name,
			Description: t.Description,
			Parameters:  t.Schema,
		})
	}
	for _, m := range req.Messages {
		items, err := messageToInputItems(m)
		if err != nil {
			return nil, err
		}
		p.Input = append(p.Input, items...)
	}
	return json.Marshal(p)
}

func messageToInputItems(m ai.Message) ([]inputItem, error) {
	switch m.Role {
	case ai.RoleUser, ai.RoleAssistant:
		msg := inputItem{
			Type: "message",
			Role: string(m.Role),
		}
		var items []inputItem
		for _, c := range m.Content {
			if v, ok := c.(ai.TextBlock); ok {
				msg.Content = append(msg.Content, inputContent{Type: textTypeFor(m.Role), Text: v.Text})
			}
		}
		if len(msg.Content) > 0 {
			items = append(items, msg)
		}
		for _, c := range m.Content {
			v, ok := c.(ai.ToolUseBlock)
			if !ok {
				continue
			}
			args := string(v.Args)
			if args == "" {
				args = "{}"
			}
			items = append(items, inputItem{
				Type:      "function_call",
				CallID:    v.ID,
				Name:      v.Name,
				Arguments: args,
			})
		}
		if len(items) == 0 {
			items = []inputItem{msg}
		}
		return items, nil

	case ai.RoleToolResult:
		for _, c := range m.Content {
			if v, ok := c.(ai.ToolResultBlock); ok {
				out := v.Output
				if out == "" {
					// The Responses API rejects function_call_output items
					// without an "output" field, and our struct tag is
					// omitempty. Substitute a placeholder so the field is
					// always serialized.
					out = "(no output)"
				}
				return []inputItem{{
					Type:   "function_call_output",
					CallID: v.ToolCallID,
					Output: out,
				}}, nil
			}
		}
	}
	return nil, nil
}

func textTypeFor(role ai.Role) string {
	if role == ai.RoleAssistant {
		return "output_text"
	}
	return "input_text"
}
