package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

func userMsg(s string) ai.Message {
	return ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: s}}}
}

func collect(t *testing.T, ch <-chan Event) []Event {
	t.Helper()
	var out []Event
	for e := range ch {
		out = append(out, e)
	}
	return out
}

func TestRun_TextOnlyTurn(t *testing.T) {
	f := faux.New().Reply("hi there").Done()
	s := session.New("gpt-5")
	a := New(f, tools.NewRegistry(), s, "system")
	evs := collect(t, a.Run(context.Background(), userMsg("hello")))

	var sawText, sawDone bool
	for _, e := range evs {
		switch v := e.(type) {
		case AssistantText:
			if v.Delta == "hi there" {
				sawText = true
			}
		case TurnDone:
			if v.Reason == "stop" {
				sawDone = true
			}
		}
	}
	if !sawText {
		t.Fatal("missing AssistantText")
	}
	if !sawDone {
		t.Fatal("missing TurnDone")
	}
	// Session must contain user msg + assistant msg.
	if got := len(s.PathToActive()); got != 2 {
		t.Fatalf("path len = %d", got)
	}
}

func TestRun_DispatchesToolThenContinues(t *testing.T) {
	f := faux.New().
		CallTool("read", `{"path":"/tmp/x"}`).Done().
		Reply("file said hi").Done()
	s := session.New("gpt-5")
	reg := tools.NewRegistry()
	reg.Register(stubReadTool{output: "hi"})
	a := New(f, reg, s, "")
	evs := collect(t, a.Run(context.Background(), userMsg("look at /tmp/x")))

	var started, finished, doneStop bool
	for _, e := range evs {
		switch v := e.(type) {
		case ToolStarted:
			if v.Call.Name == "read" {
				started = true
			}
		case ToolFinished:
			if v.Result.Output == "hi" {
				finished = true
			}
		case TurnDone:
			if v.Reason == "stop" {
				doneStop = true
			}
		}
	}
	if !started || !finished || !doneStop {
		t.Fatalf("missing events: started=%v finished=%v done=%v", started, finished, doneStop)
	}

	// Session: user, assistant(tool_use), tool_result, assistant(text)
	path := s.PathToActive()
	if len(path) != 4 {
		t.Fatalf("path len = %d, want 4", len(path))
	}
	if path[1].Role != ai.RoleAssistant {
		t.Fatalf("path[1] role = %s", path[1].Role)
	}
	if path[2].Role != ai.RoleToolResult {
		t.Fatalf("path[2] role = %s", path[2].Role)
	}
}

type stubReadTool struct{ output string }

func (stubReadTool) Spec() ai.ToolSpec {
	return ai.ToolSpec{Name: "read", Schema: json.RawMessage(`{}`)}
}
func (s stubReadTool) Run(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	return tools.Result{Output: s.output}, nil
}

type slowProvider struct{}

func (slowProvider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	out := make(chan ai.Event)
	go func() {
		defer close(out)
		select {
		case <-ctx.Done():
			return
		case <-time.After(5 * time.Second):
			out <- ai.Done{Reason: "stop"}
		}
	}()
	return out, nil
}

func TestRun_AbortViaCtx(t *testing.T) {
	s := session.New("gpt-5")
	a := New(slowProvider{}, tools.NewRegistry(), s, "")
	ctx, cancel := context.WithCancel(context.Background())
	ch := a.Run(ctx, userMsg("anything"))

	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	deadline := time.After(2 * time.Second)
	var sawAbort bool
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				if !sawAbort {
					t.Fatal("channel closed without TurnAborted")
				}
				return
			}
			if _, ok := e.(TurnAborted); ok {
				sawAbort = true
			}
		case <-deadline:
			t.Fatal("agent did not abort within deadline")
		}
	}
}

func TestRun_ContextOverflow(t *testing.T) {
	f := faux.New().Reply("partial").DoneOverflow()
	s := session.New("gpt-5")
	a := New(f, tools.NewRegistry(), s, "")
	evs := collect(t, a.Run(context.Background(), userMsg("hi")))

	var sawOverflow, sawDoneOverflow bool
	for _, e := range evs {
		switch v := e.(type) {
		case ContextOverflow:
			sawOverflow = true
		case TurnDone:
			if v.Reason == "context_overflow" {
				sawDoneOverflow = true
			}
		}
	}
	if !sawOverflow || !sawDoneOverflow {
		t.Fatalf("overflow=%v doneOverflow=%v", sawOverflow, sawDoneOverflow)
	}
}
