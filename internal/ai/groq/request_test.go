package groq

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestBuildPayload_IncludesMessagesAndTools(t *testing.T) {
	req := ai.Request{
		Model:  "openai/gpt-oss-120b",
		System: "you are helpful",
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hi"}}},
		},
		Tools:     []ai.ToolSpec{{Name: "read", Description: "Read file", Schema: json.RawMessage(`{"type":"object"}`)}},
		Reasoning: ai.ReasoningConfig{Effort: "medium"},
	}
	b, err := buildPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"model":"openai/gpt-oss-120b"`,
		`"stream":true`,
		`"stream_options":{"include_usage":true}`,
		`"role":"system","content":"you are helpful"`,
		`"role":"user","content":"hi"`,
		`"tools"`,
		`"tool_choice":"auto"`,
		`"reasoning_effort":"medium"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("payload missing %q:\n%s", want, s)
		}
	}
}

func TestBuildPayload_IncludesUserImages(t *testing.T) {
	req := ai.Request{
		Model: "meta-llama/llama-4-scout-17b-16e-instruct",
		Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{
			ai.TextBlock{Text: "what is this?"},
			ai.ImageBlock{Data: []byte("jpg"), MimeType: "image/jpeg"},
		}}},
	}
	b, err := buildPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`"type":"image_url"`,
		`"url":"data:image/jpeg;base64,anBn"`,
	} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("payload missing %q:\n%s", want, b)
		}
	}
}

func TestBuildPayload_IncludesDocumentTextFallback(t *testing.T) {
	req := ai.Request{
		Model: "llama-3.3-70b-versatile",
		Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{
			ai.TextBlock{Text: "summarize"},
			ai.DocumentBlock{Text: "<document>pdf text</document>", MimeType: "application/pdf", Name: "paper.pdf"},
		}}},
	}
	b, err := buildPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "pdf text") {
		t.Fatalf("payload missing document fallback text:\n%s", b)
	}
}

func TestBuildPayload_OmitsReasoningEffortForUnsupportedModel(t *testing.T) {
	for _, model := range []string{
		"llama-3.3-70b-versatile",
		"llama-3.1-8b-instant",
		"meta-llama/llama-4-scout-17b-16e-instruct",
		"deepseek-r1-distill-llama-70b",
	} {
		b, err := buildPayload(ai.Request{Model: model, Reasoning: ai.ReasoningConfig{Effort: "medium"}})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "reasoning_effort") {
			t.Fatalf("%s: reasoning_effort should be omitted:\n%s", model, b)
		}
	}
}

func TestBuildPayload_MapsReasoningEffortForQwen3(t *testing.T) {
	b, err := buildPayload(ai.Request{Model: "qwen/qwen3-32b", Reasoning: ai.ReasoningConfig{Effort: "medium"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"reasoning_effort":"default"`) {
		t.Fatalf("qwen3 should remap medium → default:\n%s", b)
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

func TestBuildPayload_OmitsReasoningNone(t *testing.T) {
	b, err := buildPayload(ai.Request{Model: "m", Reasoning: ai.ReasoningConfig{Effort: "none"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "reasoning_effort") {
		t.Fatalf("reasoning_effort should be omitted for none:\n%s", string(b))
	}
}
