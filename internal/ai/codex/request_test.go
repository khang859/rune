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

func TestBuildPayload_EmptyToolOutputSerializesPlaceholder(t *testing.T) {
	req := ai.Request{
		Model: "gpt-5",
		Messages: []ai.Message{
			{Role: ai.RoleAssistant, Content: []ai.ContentBlock{
				ai.ToolUseBlock{ID: "fc_AAA", Name: "bash", Args: json.RawMessage(`{"command":"grep zzz"}`)},
			}},
			{Role: ai.RoleToolResult, ToolCallID: "fc_AAA", Content: []ai.ContentBlock{
				ai.ToolResultBlock{ToolCallID: "fc_AAA", Output: ""},
			}},
		},
	}
	b, err := buildPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Input []map[string]any `json:"input"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, item := range decoded.Input {
		if item["type"] != "function_call_output" {
			continue
		}
		found = true
		out, ok := item["output"]
		if !ok {
			t.Fatalf("function_call_output is missing the required 'output' field — the API rejects this with 400:\n%s", string(b))
		}
		if out == "" {
			t.Fatalf("function_call_output.output is empty — should be a non-empty placeholder:\n%s", string(b))
		}
	}
	if !found {
		t.Fatalf("no function_call_output item in payload:\n%s", string(b))
	}
}

func TestBuildPayload_IncludesReasoningNone(t *testing.T) {
	req := ai.Request{
		Model:     "gpt-5.5",
		Messages:  []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hi"}}}},
		Reasoning: ai.ReasoningConfig{Effort: "none"},
	}
	b, err := buildPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"reasoning":{"effort":"none"`) {
		t.Fatalf("payload missing effort none:\n%s", string(b))
	}
}

func TestBuildPayload_IncludesUserImages(t *testing.T) {
	req := ai.Request{
		Model: "gpt-5",
		Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{
			ai.TextBlock{Text: "what is this?"},
			ai.ImageBlock{Data: []byte("png"), MimeType: "image/png"},
		}}},
	}
	b, err := buildPayload(req)
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Input []struct {
			Content []struct {
				Type     string `json:"type"`
				Text     string `json:"text"`
				ImageURL string `json:"image_url"`
				Detail   string `json:"detail"`
			} `json:"content"`
		} `json:"input"`
	}
	if err := json.Unmarshal(b, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Input) != 1 || len(decoded.Input[0].Content) != 2 {
		t.Fatalf("unexpected content in payload: %s", b)
	}
	img := decoded.Input[0].Content[1]
	if img.Type != "input_image" || img.ImageURL != "data:image/png;base64,cG5n" || img.Detail != "auto" {
		t.Fatalf("image content = %#v; payload: %s", img, b)
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
