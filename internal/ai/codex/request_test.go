package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestBuildPayload_PreservesParallelToolCallsInOneAssistantMessage(t *testing.T) {
	req := ai.Request{
		Model: "gpt-5",
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hi"}}},
			{Role: ai.RoleAssistant, Content: []ai.ContentBlock{
				ai.TextBlock{Text: "ok"},
				ai.ToolUseBlock{ID: "fc_AAA", Name: "read", Args: json.RawMessage(`{"path":"/x"}`)},
				ai.ToolUseBlock{ID: "fc_BBB", Name: "bash", Args: json.RawMessage(`{"command":"ls"}`)},
			}},
			{Role: ai.RoleToolResult, ToolCallID: "fc_AAA", Content: []ai.ContentBlock{
				ai.ToolResultBlock{ToolCallID: "fc_AAA", Output: "err"},
			}},
			{Role: ai.RoleToolResult, ToolCallID: "fc_BBB", Content: []ai.ContentBlock{
				ai.ToolResultBlock{ToolCallID: "fc_BBB", Output: "listing"},
			}},
		},
	}
	b, err := buildPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"call_id":"fc_AAA"`,
		`"call_id":"fc_BBB"`,
		`"name":"read"`,
		`"name":"bash"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("payload missing %q — both parallel tool calls must appear so their outputs bind:\n%s", want, s)
		}
	}
}

func TestBuildPayload_IncludesMessagesAndTools(t *testing.T) {
	req := ai.Request{
		Model:  "gpt-5",
		System: "you are helpful",
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hi"}}},
		},
		Tools: []ai.ToolSpec{
			{Name: "read", Description: "Read file", Schema: json.RawMessage(`{"type":"object"}`)},
		},
		Reasoning: ai.ReasoningConfig{Effort: "medium"},
	}
	b, err := buildPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"model":"gpt-5"`,
		`"instructions":"you are helpful"`,
		`"input"`,
		`"tools"`,
		`"reasoning":{"effort":"medium"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("payload missing %q:\n%s", want, s)
		}
	}
}
