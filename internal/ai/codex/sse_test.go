package codex

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func collect(t *testing.T, ch <-chan ai.Event) []ai.Event {
	t.Helper()
	var out []ai.Event
	for e := range ch {
		out = append(out, e)
	}
	return out
}

func TestParseSSE_TextOnly(t *testing.T) {
	b, _ := os.ReadFile("testdata/stream_text_only.sse")
	out := make(chan ai.Event, 32)
	err := parseSSE(context.Background(), strings.NewReader(string(b)), out)
	close(out)
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(t, out)

	var text strings.Builder
	var sawDone bool
	var usage ai.Usage
	for _, e := range evs {
		switch v := e.(type) {
		case ai.TextDelta:
			text.WriteString(v.Text)
		case ai.Usage:
			usage = v
		case ai.Done:
			sawDone = true
			if v.Reason != "stop" {
				t.Fatalf("done reason = %q", v.Reason)
			}
		}
	}
	if text.String() != "hello world" {
		t.Fatalf("text = %q", text.String())
	}
	if !sawDone {
		t.Fatal("missing Done")
	}
	if usage.Input != 10 || usage.Output != 2 {
		t.Fatalf("usage = %#v", usage)
	}
}

func TestParseSSE_ToolCall(t *testing.T) {
	b, _ := os.ReadFile("testdata/stream_tool_call.sse")
	out := make(chan ai.Event, 32)
	if err := parseSSE(context.Background(), strings.NewReader(string(b)), out); err != nil {
		t.Fatal(err)
	}
	close(out)
	evs := collect(t, out)
	var found bool
	for _, e := range evs {
		if c, ok := e.(ai.ToolCall); ok {
			found = true
			if c.Name != "read" {
				t.Fatalf("tool name = %q", c.Name)
			}
			var args map[string]string
			if err := json.Unmarshal(c.Args, &args); err != nil {
				t.Fatal(err)
			}
			if args["path"] != "/tmp/x" {
				t.Fatalf("args = %v", args)
			}
		}
	}
	if !found {
		t.Fatal("no ToolCall emitted")
	}
}

func TestParseSSE_ContextOverflow(t *testing.T) {
	b, _ := os.ReadFile("testdata/stream_overflow.sse")
	out := make(chan ai.Event, 32)
	if err := parseSSE(context.Background(), strings.NewReader(string(b)), out); err != nil {
		t.Fatal(err)
	}
	close(out)
	evs := collect(t, out)
	for _, e := range evs {
		if d, ok := e.(ai.Done); ok && d.Reason == "context_overflow" {
			return
		}
	}
	t.Fatal("missing Done{context_overflow}")
}
