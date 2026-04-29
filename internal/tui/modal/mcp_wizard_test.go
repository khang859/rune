package modal

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMCPWizardHTTPFlow(t *testing.T) {
	md := NewMCPWizard()
	w := md.(*MCPWizard)

	// Type defaults to stdio; toggle to HTTP and select it.
	_, _ = w.Update(tea.KeyMsg{Type: tea.KeyTab})
	if w.serverType != "http" {
		t.Fatalf("serverType = %q, want http", w.serverType)
	}
	_, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if w.step != mcpWizardName {
		t.Fatalf("step = %v, want name", w.step)
	}

	w.name.SetValue("context7")
	_, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if w.step != mcpWizardURL {
		t.Fatalf("step = %v, want url", w.step)
	}
	w.url.SetValue("https://mcp.context7.com/mcp")
	_, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if w.step != mcpWizardHeaders {
		t.Fatalf("step = %v, want headers", w.step)
	}
	w.headers.SetValue("CONTEXT7_API_KEY=secret")
	_, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if w.step != mcpWizardReview {
		t.Fatalf("step = %v, want review", w.step)
	}

	_, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	msg := cmd()
	res, ok := msg.(ResultMsg)
	if !ok || res.Cancel {
		t.Fatalf("msg = %#v, want result", msg)
	}
	payload := res.Payload.(MCPWizardResult)
	if payload.Name != "context7" || payload.Config.Type != "http" || payload.Config.URL != "https://mcp.context7.com/mcp" {
		t.Fatalf("payload = %#v", payload)
	}
	if payload.Config.Headers["CONTEXT7_API_KEY"] != "secret" {
		t.Fatalf("headers = %#v", payload.Config.Headers)
	}
}

func TestMCPWizardStdioFlow(t *testing.T) {
	md := NewMCPWizard()
	w := md.(*MCPWizard)

	_, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w.name.SetValue("filesystem")
	_, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if w.step != mcpWizardCommand {
		t.Fatalf("step = %v, want command", w.step)
	}
	w.command.SetValue("npx")
	_, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	w.args.SetValue("-y @modelcontextprotocol/server-filesystem '/tmp/my work'")
	_, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})

	_, cmd := w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	res := cmd().(ResultMsg).Payload.(MCPWizardResult)
	if res.Name != "filesystem" || res.Config.Command != "npx" {
		t.Fatalf("result = %#v", res)
	}
	want := []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp/my work"}
	if len(res.Config.Args) != len(want) {
		t.Fatalf("args = %#v, want %#v", res.Config.Args, want)
	}
	for i := range want {
		if res.Config.Args[i] != want[i] {
			t.Fatalf("args = %#v, want %#v", res.Config.Args, want)
		}
	}
}

func TestMCPWizardInvalidHeaderStaysOnHeaders(t *testing.T) {
	w := NewMCPWizard().(*MCPWizard)
	w.serverType = "http"
	w.step = mcpWizardHeaders
	w.headers.SetValue("not-a-header")

	_, _ = w.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if w.step != mcpWizardHeaders {
		t.Fatalf("step = %v, want headers", w.step)
	}
	if w.err == "" {
		t.Fatal("expected error")
	}
}
