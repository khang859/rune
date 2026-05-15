package ollama

import (
	"context"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestParseNDJSON_TextUsageDone(t *testing.T) {
	stream := strings.Join([]string{
		`{"message":{"role":"assistant","content":"hel"},"done":false}`,
		`{"message":{"role":"assistant","content":"lo"},"done":false}`,
		`{"message":{"role":"assistant","content":""},"done":true,"done_reason":"stop","prompt_eval_count":3,"eval_count":2}`,
	}, "\n") + "\n"

	out := make(chan ai.Event, 10)
	if err := parseNDJSON(context.Background(), strings.NewReader(stream), out); err != nil {
		t.Fatal(err)
	}
	close(out)

	var text string
	var usage ai.Usage
	var done bool
	for ev := range out {
		switch v := ev.(type) {
		case ai.TextDelta:
			text += v.Text
		case ai.Usage:
			usage = v
		case ai.Done:
			done = true
		}
	}
	if text != "hello" {
		t.Fatalf("text = %q", text)
	}
	if usage.Input != 3 || usage.Output != 2 {
		t.Fatalf("usage = %+v", usage)
	}
	if !done {
		t.Fatal("missing done")
	}
}

func TestParseNDJSON_ToolCallOnTerminalFrame(t *testing.T) {
	// Native /api/chat delivers tool_calls whole on the final done:true frame,
	// not streamed token-by-token like OpenAI deltas.
	stream := strings.Join([]string{
		`{"message":{"role":"assistant","content":""},"done":false}`,
		`{"message":{"role":"assistant","content":"","tool_calls":[{"function":{"name":"read","arguments":{"path":"/tmp/x"}}}]},"done":true,"done_reason":"stop"}`,
	}, "\n") + "\n"

	out := make(chan ai.Event, 10)
	if err := parseNDJSON(context.Background(), strings.NewReader(stream), out); err != nil {
		t.Fatal(err)
	}
	close(out)

	var call ai.ToolCall
	var doneReason string
	for ev := range out {
		switch v := ev.(type) {
		case ai.ToolCall:
			call = v
		case ai.Done:
			doneReason = v.Reason
		}
	}
	if call.Name != "read" {
		t.Fatalf("tool name = %q", call.Name)
	}
	if string(call.Args) != `{"path":"/tmp/x"}` {
		t.Fatalf("tool args = %s", string(call.Args))
	}
	if doneReason != "tool_use" {
		t.Fatalf("done reason = %q, want tool_use", doneReason)
	}
}

func TestParseNDJSON_ThinkingFieldSurfacesAsThinkingEvent(t *testing.T) {
	stream := `{"message":{"role":"assistant","content":"","thinking":"reasoning..."},"done":false}` + "\n"
	out := make(chan ai.Event, 4)
	if err := parseNDJSON(context.Background(), strings.NewReader(stream), out); err != nil {
		t.Fatal(err)
	}
	close(out)

	var thinking string
	for ev := range out {
		if v, ok := ev.(ai.Thinking); ok {
			thinking = v.Text
		}
	}
	if thinking != "reasoning..." {
		t.Fatalf("thinking = %q", thinking)
	}
}

func TestParseNDJSON_ErrorFrameSurfacesStreamError(t *testing.T) {
	stream := `{"error":"model not found"}` + "\n"
	out := make(chan ai.Event, 4)
	if err := parseNDJSON(context.Background(), strings.NewReader(stream), out); err != nil {
		t.Fatal(err)
	}
	close(out)

	var got string
	for ev := range out {
		if v, ok := ev.(ai.StreamError); ok {
			got = v.Err.Error()
		}
	}
	if got != "model not found" {
		t.Fatalf("stream error = %q", got)
	}
}

func TestParseNDJSON_MaxTokensDoneReason(t *testing.T) {
	stream := `{"message":{"role":"assistant","content":"truncated"},"done":true,"done_reason":"length"}` + "\n"
	out := make(chan ai.Event, 4)
	if err := parseNDJSON(context.Background(), strings.NewReader(stream), out); err != nil {
		t.Fatal(err)
	}
	close(out)

	var reason string
	for ev := range out {
		if v, ok := ev.(ai.Done); ok {
			reason = v.Reason
		}
	}
	if reason != "max_tokens" {
		t.Fatalf("done reason = %q, want max_tokens", reason)
	}
}
