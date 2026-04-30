package ollama

import (
	"context"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestParseSSE_TextUsageDone(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"content":"hel"},"finish_reason":null}]}

data: {"choices":[{"delta":{"content":"lo"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2,"prompt_tokens_details":{"cached_tokens":1}}}

data: [DONE]

`
	out := make(chan ai.Event, 10)
	if err := parseSSE(context.Background(), strings.NewReader(sse), out); err != nil {
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
	if usage.Input != 3 || usage.Output != 2 || usage.CacheRead != 1 {
		t.Fatalf("usage = %+v", usage)
	}
	if !done {
		t.Fatal("missing done")
	}
}

func TestParseSSE_ToolCallStreaming(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"{\"pa","name":"read"},"id":"call_1","index":0,"type":"function"}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"function":{"arguments":"th\":\"/tmp/x\"}"},"index":0}]},"finish_reason":"tool_calls"}]}

`
	out := make(chan ai.Event, 10)
	if err := parseSSE(context.Background(), strings.NewReader(sse), out); err != nil {
		t.Fatal(err)
	}
	close(out)
	var call ai.ToolCall
	var saw bool
	for ev := range out {
		if v, ok := ev.(ai.ToolCall); ok {
			call = v
			saw = true
		}
	}
	if !saw {
		t.Fatal("missing tool call")
	}
	if call.ID != "call_1" || call.Name != "read" || string(call.Args) != `{"path":"/tmp/x"}` {
		t.Fatalf("call = %+v args=%s", call, string(call.Args))
	}
}

func TestParseSSE_Reasoning(t *testing.T) {
	sse := "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"think\"}}]}\n\n"
	out := make(chan ai.Event, 1)
	if err := parseSSE(context.Background(), strings.NewReader(sse), out); err != nil {
		t.Fatal(err)
	}
	close(out)
	ev := <-out
	v, ok := ev.(ai.Thinking)
	if !ok || v.Text != "think" {
		t.Fatalf("event = %#v", ev)
	}
}
