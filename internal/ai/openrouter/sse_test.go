package openrouter

import (
	"context"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func collectSSE(t *testing.T, input string) []ai.Event {
	t.Helper()
	out := make(chan ai.Event, 16)
	if err := parseSSE(context.Background(), strings.NewReader(input), out); err != nil {
		t.Fatal(err)
	}
	close(out)
	var events []ai.Event
	for ev := range out {
		events = append(events, ev)
	}
	return events
}

func TestParseSSEIgnoresCommentsAndStreamsTextUsageDone(t *testing.T) {
	events := collectSSE(t, ": OPENROUTER PROCESSING\n\n"+
		"data: {\"choices\":[{\"delta\":{\"content\":\"hi\",\"reasoning\":\"think\"}}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3}}\n\n"+
		"data: [DONE]\n\n")
	var text, thinking string
	var usage bool
	var done bool
	for _, ev := range events {
		switch v := ev.(type) {
		case ai.TextDelta:
			text += v.Text
		case ai.Thinking:
			thinking += v.Text
		case ai.Usage:
			usage = v.Input == 2 && v.Output == 3
		case ai.Done:
			done = v.Reason == "stop"
		}
	}
	if text != "hi" || thinking != "think" || !usage || !done {
		t.Fatalf("events = %#v", events)
	}
}

func TestParseSSEAccumulatesToolCalls(t *testing.T) {
	events := collectSSE(t,
		"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"call_1\",\"function\":{\"name\":\"bash\",\"arguments\":\"{\\\"cmd\\\"\"}}]}}]}\n\n"+
			"data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"function\":{\"arguments\":\":\\\"pwd\\\"}\"}}]},\"finish_reason\":\"tool_calls\"}]}\n\n")
	var call ai.ToolCall
	var done ai.Done
	for _, ev := range events {
		switch v := ev.(type) {
		case ai.ToolCall:
			call = v
		case ai.Done:
			done = v
		}
	}
	if call.ID != "call_1" || call.Name != "bash" || string(call.Args) != `{"cmd":"pwd"}` || done.Reason != "tool_use" {
		t.Fatalf("events = %#v", events)
	}
}

func TestParseSSETopLevelError(t *testing.T) {
	events := collectSSE(t, "data: {\"error\":{\"message\":\"bad\"}}\n\n")
	if len(events) != 1 {
		t.Fatalf("events = %#v", events)
	}
	if ev, ok := events[0].(ai.StreamError); !ok || ev.Class != ai.ErrFatal || ev.Err.Error() != "bad" {
		t.Fatalf("event = %#v", events[0])
	}
}
