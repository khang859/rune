package ai

import (
	"encoding/json"
	"testing"
)

func TestMessage_RoundTripJSON(t *testing.T) {
	m := Message{
		Role: RoleAssistant,
		Content: []ContentBlock{
			TextBlock{Text: "hello"},
			ToolUseBlock{ID: "t1", Name: "read", Args: json.RawMessage(`{"path":"x"}`)},
		},
	}
	b, err := json.Marshal(m)
	if err != nil {
		t.Fatal(err)
	}
	var got Message
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.Role != RoleAssistant {
		t.Fatalf("role = %q", got.Role)
	}
	if len(got.Content) != 2 {
		t.Fatalf("content len = %d", len(got.Content))
	}
	if tx, ok := got.Content[0].(TextBlock); !ok || tx.Text != "hello" {
		t.Fatalf("content[0] = %#v", got.Content[0])
	}
	if tu, ok := got.Content[1].(ToolUseBlock); !ok || tu.Name != "read" {
		t.Fatalf("content[1] = %#v", got.Content[1])
	}
}

func TestToolResultMessage_HasToolCallID(t *testing.T) {
	m := Message{
		Role:       RoleToolResult,
		ToolCallID: "t1",
		Content:    []ContentBlock{ToolResultBlock{ToolCallID: "t1", Output: "ok"}},
	}
	b, _ := json.Marshal(m)
	var got Message
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if got.ToolCallID != "t1" {
		t.Fatalf("toolCallID = %q", got.ToolCallID)
	}
}
