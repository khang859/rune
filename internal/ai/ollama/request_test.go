package ollama

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestBuildPayload_NativeShapeWithThinkAndOptions(t *testing.T) {
	req := ai.Request{
		Model:  "qwen3:4b",
		System: "you are helpful",
		Messages: []ai.Message{
			{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hi"}}},
		},
		Tools: []ai.ToolSpec{{Name: "read", Description: "Read file", Schema: json.RawMessage(`{"type":"object"}`)}},
	}
	b, err := buildPayload(req, payloadOptions{NumCtx: 16384, Think: false})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"model":"qwen3:4b"`,
		`"stream":true`,
		`"think":false`,
		`"options":{"num_ctx":16384}`,
		`"role":"system","content":"you are helpful"`,
		`"role":"user","content":"hi"`,
		`"tools"`,
		`"name":"read"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("payload missing %q:\n%s", want, s)
		}
	}
	// Native endpoint has no tool_choice and no stream_options.
	for _, unwanted := range []string{"tool_choice", "stream_options", "include_usage"} {
		if strings.Contains(s, unwanted) {
			t.Fatalf("payload should not contain %q:\n%s", unwanted, s)
		}
	}
}

func TestBuildPayload_OmitsNumCtxWhenZero(t *testing.T) {
	req := ai.Request{Model: "m", Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hi"}}}}}
	b, err := buildPayload(req, payloadOptions{NumCtx: 0, Think: false})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "num_ctx") {
		t.Fatalf("payload should omit num_ctx when 0:\n%s", b)
	}
}

func TestBuildPayload_UserImagesAsBase64Array(t *testing.T) {
	req := ai.Request{
		Model: "qwen3-vl:8b",
		Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{
			ai.TextBlock{Text: "what is this?"},
			ai.ImageBlock{Data: []byte("gif"), MimeType: "image/gif"},
		}}},
	}
	b, err := buildPayload(req, payloadOptions{NumCtx: 16384})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"images":["Z2lm"]`) {
		t.Fatalf("payload missing native images array:\n%s", s)
	}
	// Should NOT use OpenAI's content-parts wrapper.
	if strings.Contains(s, "image_url") || strings.Contains(s, "data:image") {
		t.Fatalf("payload still uses OpenAI image format:\n%s", s)
	}
}

func TestBuildPayload_DocumentTextFallback(t *testing.T) {
	req := ai.Request{
		Model: "llama3.2",
		Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{
			ai.TextBlock{Text: "summarize"},
			ai.DocumentBlock{Text: "<document>pdf text</document>", MimeType: "application/pdf", Name: "paper.pdf"},
		}}},
	}
	b, err := buildPayload(req, payloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "pdf text") {
		t.Fatalf("payload missing document fallback text:\n%s", b)
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
	b, err := buildPayload(req, payloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		`"tool_calls"`,
		`"name":"bash"`,
		// Arguments must be a JSON object on the native endpoint, not a string.
		`"arguments":{"command":"ls"}`,
		// Tool result must use role:tool + tool_name (matching the original
		// tool's name, recovered from the prior assistant turn), not
		// tool_call_id.
		`"role":"tool","content":"(no output)","tool_name":"bash"`,
	} {
		if !strings.Contains(s, want) {
			t.Fatalf("payload missing %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "tool_call_id") {
		t.Fatalf("payload should not contain tool_call_id on native endpoint:\n%s", s)
	}
}

func TestBuildPayload_EmptyToolArgsBecomeEmptyObject(t *testing.T) {
	req := ai.Request{Model: "m", Messages: []ai.Message{
		{Role: ai.RoleAssistant, Content: []ai.ContentBlock{
			ai.ToolUseBlock{ID: "call_1", Name: "list"},
		}},
	}}
	b, err := buildPayload(req, payloadOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"arguments":{}`) {
		t.Fatalf("payload missing empty-object arguments fallback:\n%s", b)
	}
}
