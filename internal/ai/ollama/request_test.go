package ollama

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestBuildPayload_IncludesMessagesAndToolsButOmitsToolChoice(t *testing.T) {
	req := ai.Request{
		Model:  "qwen3:4b",
		System: "you are helpful",
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hi"}}},
		},
		Tools: []ai.ToolSpec{{Name: "read", Description: "Read file", Schema: json.RawMessage(`{"type":"object"}`)}},
	}
	b, err := buildPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"model":"qwen3:4b"`,
		`"stream":true`,
		`"stream_options":{"include_usage":true}`,
		`"role":"system","content":"you are helpful"`,
		`"role":"user","content":"hi"`,
		`"tools"`,
		`"name":"read"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("payload missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "tool_choice") {
		t.Fatalf("ollama payload should omit unsupported tool_choice:\n%s", s)
	}
}

func TestBuildPayload_IncludesUserImages(t *testing.T) {
	req := ai.Request{
		Model: "qwen3-vl:8b",
		Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{
			ai.TextBlock{Text: "what is this?"},
			ai.ImageBlock{Data: []byte("gif"), MimeType: "image/gif"},
		}}},
	}
	b, err := buildPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"type":"image_url"`,
		`"url":"data:image/gif;base64,Z2lm"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("payload missing %q:\n%s", want, b)
		}
	}
}

func TestBuildPayload_ToolCallsAndToolResults(t *testing.T) {
	req := ai.Request{Model: "m", Messages: []ai.Message{
		{Role: ai.RoleAssistant, Content: []ai.ContentBlock{
			ai.TextBlock{Text: "checking"},
			ai.ToolUseBlock{ID: "call_1", Name: "bash", Args: json.RawMessage(`{"command":"ls"}`)},
		}},
		{Role: ai.RoleToolResult, Content: []ai.ContentBlock{
			ai.ToolResultBlock{ToolCallID: "call_1", Output: ""},
		}},
	}}
	b, err := buildPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"tool_calls"`,
		`"id":"call_1"`,
		`"name":"bash"`,
		`"arguments":"{\"command\":\"ls\"}"`,
		`"role":"tool"`,
		`"tool_call_id":"call_1"`,
		`"content":"(no output)"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("payload missing %q:\n%s", want, s)
		}
	}
}
