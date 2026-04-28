package faux

import (
	"context"
	"encoding/json"
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

func TestFaux_TextThenDone(t *testing.T) {
	f := New().Reply("hello world").Done()
	ch, err := f.Stream(context.Background(), ai.Request{})
	if err != nil {
		t.Fatal(err)
	}
	evs := collect(t, ch)
	// Expect: TextDelta("hello world"), Usage, Done
	if len(evs) != 3 {
		t.Fatalf("events = %d, want 3: %#v", len(evs), evs)
	}
	if td, ok := evs[0].(ai.TextDelta); !ok || td.Text != "hello world" {
		t.Fatalf("evs[0] = %#v", evs[0])
	}
	if d, ok := evs[2].(ai.Done); !ok || d.Reason != "stop" {
		t.Fatalf("evs[2] = %#v", evs[2])
	}
}

func TestFaux_ToolCallThenDone(t *testing.T) {
	f := New().CallTool("read", `{"path":"foo"}`).Done()
	ch, _ := f.Stream(context.Background(), ai.Request{})
	evs := collect(t, ch)
	var foundCall bool
	for _, e := range evs {
		if c, ok := e.(ai.ToolCall); ok {
			foundCall = true
			if c.Name != "read" {
				t.Fatalf("name = %q", c.Name)
			}
			var args map[string]string
			if err := json.Unmarshal(c.Args, &args); err != nil {
				t.Fatal(err)
			}
			if args["path"] != "foo" {
				t.Fatalf("args = %v", args)
			}
		}
	}
	if !foundCall {
		t.Fatal("no ToolCall emitted")
	}
}

func TestFaux_TurnsAdvanceAcrossStreamCalls(t *testing.T) {
	f := New().
		Reply("first turn").Done().
		Reply("second turn").Done()
	// First call returns turn 1.
	ch1, _ := f.Stream(context.Background(), ai.Request{})
	e1 := collect(t, ch1)
	if td := e1[0].(ai.TextDelta); td.Text != "first turn" {
		t.Fatal("turn 1 wrong")
	}
	// Second call returns turn 2.
	ch2, _ := f.Stream(context.Background(), ai.Request{})
	e2 := collect(t, ch2)
	if td := e2[0].(ai.TextDelta); td.Text != "second turn" {
		t.Fatal("turn 2 wrong")
	}
}
