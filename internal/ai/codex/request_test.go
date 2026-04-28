package codex

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

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
