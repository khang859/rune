package modal

import (
	"strings"
	"testing"

	"github.com/khang859/rune/internal/mcp"
)

func TestMCPStatusView(t *testing.T) {
	md := NewMCPStatus([]mcp.Status{
		{Name: "context7", Type: "http", Description: "http https://mcp.context7.com/mcp", Connected: true, ToolCount: 2, Tools: []string{"resolve", "docs"}},
		{Name: "sqlite", Type: "stdio", Description: "uvx mcp-server-sqlite", Error: "executable not found"},
	})
	out := md.View(80, 24)
	for _, want := range []string{"MCP status:", "context7", "connected", "2 tools", "resolve", "docs", "sqlite", "disconnected", "executable not found"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view missing %q:\n%s", want, out)
		}
	}
}
