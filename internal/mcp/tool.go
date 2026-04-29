package mcp

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/tools"
)

type MCPTool struct {
	client *Client
	tool   Tool
}

func NewTool(c *Client, t Tool) *MCPTool {
	return &MCPTool{client: c, tool: t}
}

func (m *MCPTool) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        m.client.Name() + "_" + m.tool.Name,
		Description: m.tool.Description,
		Schema:      m.tool.InputSchema,
	}
}

func (m *MCPTool) Run(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	res, err := m.client.CallTool(ctx, m.tool.Name, args)
	if err != nil {
		return tools.Result{Output: err.Error(), IsError: true}, nil
	}
	var sb strings.Builder
	for i, c := range res.Content {
		if i > 0 {
			sb.WriteString("\n")
		}
		switch c.Type {
		case "text":
			sb.WriteString(c.Text)
		case "image":
			sb.WriteString("[image: ")
			sb.WriteString(c.MIME)
			sb.WriteString(" base64=")
			n := len(c.Data)
			if n > 60 {
				n = 60
			}
			sb.WriteString(c.Data[:n])
			sb.WriteString("…]")
		default:
			sb.WriteString("[")
			sb.WriteString(c.Type)
			sb.WriteString("]")
		}
	}
	return tools.Result{Output: sb.String(), IsError: res.IsError}, nil
}
