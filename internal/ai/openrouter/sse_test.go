package openrouter

import (
	"context"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func collectSSE(t *testing.T, input string) []ai.Event {
	t.Helper()
	return collectSSEModel(t, "", input)
}

func collectSSEModel(t *testing.T, model, input string) []ai.Event {
	t.Helper()
	out := make(chan ai.Event, 64)
	if err := parseSSE(context.Background(), strings.NewReader(input), out, model); err != nil {
		t.Fatal(err)
	}
	close(out)
	var events []ai.Event
	for ev := range out {
		events = append(events, ev)
	}
	return events
}

// joinText concatenates all TextDelta payloads; joinThinking does the same for Thinking.
func joinText(events []ai.Event) string {
	var b strings.Builder
	for _, ev := range events {
		if v, ok := ev.(ai.TextDelta); ok {
			b.WriteString(v.Text)
		}
	}
	return b.String()
}

func joinThinking(events []ai.Event) string {
	var b strings.Builder
	for _, ev := range events {
		if v, ok := ev.(ai.Thinking); ok {
			b.WriteString(v.Text)
		}
	}
	return b.String()
}

func dataLine(payload string) string { return "data: " + payload + "\n\n" }

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

func TestParseSSEKimiOrphanCloseThinkInContent(t *testing.T) {
	// kimi leaks reasoning into content with no opening tag, terminated by </think>.
	events := collectSSEModel(t, "moonshotai/kimi-k2.7-code",
		dataLine(`{"choices":[{"delta":{"content":"Let me plan this out. "}}]}`)+
			dataLine(`{"choices":[{"delta":{"content":"Step 1, step 2. </think> Here is the answer."}}]}`)+
			dataLine(`{"choices":[{"finish_reason":"stop"}]}`)+
			"data: [DONE]\n\n")
	if got := joinThinking(events); got != "Let me plan this out. Step 1, step 2. " {
		t.Fatalf("thinking = %q", got)
	}
	if got := joinText(events); got != "Here is the answer." {
		t.Fatalf("text = %q", got)
	}
}

func TestParseSSEKimiExplicitThinkPair(t *testing.T) {
	events := collectSSEModel(t, "moonshotai/kimi-k2.7-code",
		dataLine(`{"choices":[{"delta":{"content":"<think>reasoning here</think>real answer"}}]}`)+
			dataLine(`{"choices":[{"finish_reason":"stop"}]}`))
	if got := joinThinking(events); got != "reasoning here" {
		t.Fatalf("thinking = %q", got)
	}
	if got := joinText(events); got != "real answer" {
		t.Fatalf("text = %q", got)
	}
}

func TestParseSSEKimiCloseTagSplitAcrossChunks(t *testing.T) {
	events := collectSSEModel(t, "moonshotai/kimi-k2.7-code",
		dataLine(`{"choices":[{"delta":{"content":"thinking text </thi"}}]}`)+
			dataLine(`{"choices":[{"delta":{"content":"nk> the answer"}}]}`)+
			dataLine(`{"choices":[{"finish_reason":"stop"}]}`))
	if got := joinThinking(events); got != "thinking text " {
		t.Fatalf("thinking = %q", got)
	}
	if got := joinText(events); got != "the answer" {
		t.Fatalf("text = %q", got)
	}
}

func TestParseSSEKimiReasoningFieldGateLeavesContentAlone(t *testing.T) {
	// Provider separates reasoning correctly: content must stream as-is, even if it
	// happens to contain a </think> (no stripping once reasoning field is seen).
	events := collectSSEModel(t, "moonshotai/kimi-k2.7-code",
		dataLine(`{"choices":[{"delta":{"reasoning":"clean reasoning"}}]}`)+
			dataLine(`{"choices":[{"delta":{"content":"the answer with a </think> literal"}}]}`)+
			dataLine(`{"choices":[{"finish_reason":"stop"}]}`))
	if got := joinThinking(events); got != "clean reasoning" {
		t.Fatalf("thinking = %q", got)
	}
	if got := joinText(events); got != "the answer with a </think> literal" {
		t.Fatalf("text = %q", got)
	}
}

func TestParseSSENonKimiContentNeverStripped(t *testing.T) {
	// Non-kimi models pass content through verbatim regardless of think tags.
	events := collectSSEModel(t, "openai/gpt-oss-120b",
		dataLine(`{"choices":[{"delta":{"content":"discussing the </think> tag inline"}}]}`)+
			dataLine(`{"choices":[{"finish_reason":"stop"}]}`))
	if got := joinThinking(events); got != "" {
		t.Fatalf("thinking = %q, want empty", got)
	}
	if got := joinText(events); got != "discussing the </think> tag inline" {
		t.Fatalf("text = %q", got)
	}
}

func TestParseSSEKimiNoThinkTagStillStreams(t *testing.T) {
	// kimi content with no reasoning field and no think tag must not be hidden.
	events := collectSSEModel(t, "moonshotai/kimi-k2.7-code",
		dataLine(`{"choices":[{"delta":{"content":"just a direct answer"}}]}`)+
			dataLine(`{"choices":[{"finish_reason":"stop"}]}`))
	if got := joinText(events); got != "just a direct answer" {
		t.Fatalf("text = %q", got)
	}
	if got := joinThinking(events); got != "" {
		t.Fatalf("thinking = %q, want empty", got)
	}
}

func TestParseSSEKimiBufferedContentFlushedOnDoneSentinel(t *testing.T) {
	// kimi content with no </think> and no finish_reason chunk, terminated only by
	// [DONE]: buffered content must still be flushed, not dropped.
	events := collectSSEModel(t, "moonshotai/kimi-k2.7-code",
		dataLine(`{"choices":[{"delta":{"content":"buffered answer with no think tag"}}]}`)+
			"data: [DONE]\n\n")
	if got := joinText(events); got != "buffered answer with no think tag" {
		t.Fatalf("text = %q", got)
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
