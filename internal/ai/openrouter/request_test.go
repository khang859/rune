package openrouter

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestBuildPayloadIncludesMessagesToolsAndNoReasoningEffort(t *testing.T) {
	body, err := buildPayload(ai.Request{
		Model:  "~openai/gpt-latest",
		System: "system prompt",
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hello"}}},
		},
		Tools: []ai.ToolSpec{{Name: "read", Description: "read files", Schema: json.RawMessage(`{"type":"object"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "reasoning_effort") {
		t.Fatalf("payload included reasoning_effort: %s", body)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["model"] != "~openai/gpt-latest" || got["stream"] != true || got["tool_choice"] != "auto" {
		t.Fatalf("payload = %s", body)
	}
	messages := got["messages"].([]any)
	if len(messages) != 2 {
		t.Fatalf("messages len = %d", len(messages))
	}
	tools := got["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("tools len = %d", len(tools))
	}
}

func TestBuildPayloadIncludesProviderRouting(t *testing.T) {
	body, err := buildPayload(ai.Request{
		Model:           "anthropic/claude-sonnet-4.5",
		ProviderRouting: "anthropic",
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	provider, ok := got["provider"].(map[string]any)
	if !ok {
		t.Fatalf("provider missing: %s", body)
	}
	order := provider["order"].([]any)
	if len(order) != 1 || order[0] != "anthropic" {
		t.Fatalf("provider.order = %v, want [anthropic]", order)
	}
}

func TestBuildPayloadOmitsProviderRoutingWhenEmpty(t *testing.T) {
	body, err := buildPayload(ai.Request{Model: "anthropic/claude-sonnet-4.5"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), `"provider"`) {
		t.Fatalf("payload should omit provider: %s", body)
	}
}

func TestBuildPayloadSerializesImageToolCallAndToolResult(t *testing.T) {
	body, err := buildPayload(ai.Request{
		Model: "m",
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.ImageBlock{MimeType: "image/png", Data: []byte{1, 2, 3}}}},
			{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.ToolUseBlock{ID: "call_1", Name: "bash", Args: json.RawMessage(`{"cmd":"pwd"}`)}}},
			{Role: ai.RoleToolResult, ToolCallID: "call_1", Content: []ai.ContentBlock{ai.ToolResultBlock{ToolCallID: "call_1", Output: "/tmp"}}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	for _, want := range []string{"image_url", "data:image/png;base64,AQID", "tool_calls", "call_1", "\"role\":\"tool\""} {
		if !strings.Contains(text, want) {
			t.Fatalf("payload missing %q: %s", want, text)
		}
	}
}
