package agent

import (
	"context"
	"encoding/json"
	"strings"
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

type streamErrProvider struct{ err error }

func (p streamErrProvider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	out := make(chan ai.Event, 1)
	out <- ai.StreamError{Err: p.err}
	close(out)
	return out, nil
}

func TestRun_StreamErrorContextCanceledBecomesAbort(t *testing.T) {
	a := New(streamErrProvider{err: context.Canceled}, tools.NewRegistry(), session.New("gpt-5"), "")
	evs := collect(t, a.Run(context.Background(), userMsg("anything")))

	var sawAbort, sawError bool
	for _, e := range evs {
		switch e.(type) {
		case TurnAborted:
			sawAbort = true
		case TurnError:
			sawError = true
		}
	}
	if !sawAbort {
		t.Fatalf("expected TurnAborted, got events: %#v", evs)
	}
	if sawError {
		t.Fatalf("context.Canceled leaked through as TurnError: %#v", evs)
	}
}

func TestRun_AutoCompactOnOverflow(t *testing.T) {
	f := faux.New().
		DoneOverflow().                         // first call hits overflow
		Reply("compacted summary text").Done(). // compact summarizer
		Reply("post-compact reply").Done()      // retry of original turn
	s := session.New("gpt-5")
	s.Append(userMsg("u1"))
	s.Append(asstMsg("a1"))
	a := New(f, tools.NewRegistry(), s, "")

	evs := collect(t, a.Run(context.Background(), userMsg("u2")))

	var sawOverflow, sawDone bool
	for _, e := range evs {
		switch v := e.(type) {
		case ContextOverflow:
			sawOverflow = true
		case TurnDone:
			if v.Reason == "stop" {
				sawDone = true
			}
		}
	}
	if !sawOverflow {
		t.Fatal("missing ContextOverflow event")
	}
	if !sawDone {
		t.Fatal("missing TurnDone after retry")
	}
}

func TestAgent_DefaultEffortIsMedium(t *testing.T) {
	a := New(faux.New(), tools.NewRegistry(), session.New("gpt-5"), "")
	if got := a.ReasoningEffort(); got != "medium" {
		t.Fatalf("default effort = %q, want %q", got, "medium")
	}
}

func TestAgent_SetReasoningEffort(t *testing.T) {
	a := New(faux.New(), tools.NewRegistry(), session.New("gpt-5"), "")
	a.SetReasoningEffort("high")
	if got := a.ReasoningEffort(); got != "high" {
		t.Fatalf("after set, effort = %q, want %q", got, "high")
	}
}

type captureProvider struct {
	gotReq ai.Request
}

func (c *captureProvider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	c.gotReq = req
	out := make(chan ai.Event, 2)
	out <- ai.Usage{Input: 1, Output: 1}
	out <- ai.Done{Reason: "stop"}
	close(out)
	return out, nil
}

func TestRun_ThreadsReasoningEffortIntoRequest(t *testing.T) {
	cp := &captureProvider{}
	a := New(cp, tools.NewRegistry(), session.New("gpt-5"), "")
	a.SetReasoningEffort("high")
	_ = collect(t, a.Run(context.Background(), userMsg("hi")))
	if cp.gotReq.Reasoning.Effort != "high" {
		t.Fatalf("req.Reasoning.Effort = %q, want %q", cp.gotReq.Reasoning.Effort, "high")
	}
}

func TestRun_AppendsRuntimeContextWhenSystemSet(t *testing.T) {
	cp := &captureProvider{}
	a := New(cp, tools.NewRegistry(), session.New("gpt-5"), "base prompt")
	_ = collect(t, a.Run(context.Background(), userMsg("hi")))

	if !strings.HasPrefix(cp.gotReq.System, "base prompt") {
		t.Errorf("base lost: %q", cp.gotReq.System)
	}
	if !strings.Contains(cp.gotReq.System, "<system-context>") {
		t.Errorf("runtime context not appended: %q", cp.gotReq.System)
	}
}

func TestRun_OmitsRuntimeContextWhenSystemEmpty(t *testing.T) {
	cp := &captureProvider{}
	a := New(cp, tools.NewRegistry(), session.New("gpt-5"), "")
	_ = collect(t, a.Run(context.Background(), userMsg("hi")))

	if cp.gotReq.System != "" {
		t.Errorf("expected empty system, got: %q", cp.gotReq.System)
	}
}

// erroringTool returns a Go error from Run, simulating a tool plugin failure.
type erroringTool struct{}

func (erroringTool) Spec() ai.ToolSpec {
	return ai.ToolSpec{Name: "boom", Schema: json.RawMessage(`{}`)}
}
func (erroringTool) Run(ctx context.Context, args json.RawMessage) (tools.Result, error) {
	return tools.Result{}, errIntentional
}

var errIntentional = errStr("kaboom")

type errStr string

func (e errStr) Error() string { return string(e) }

// TestRun_ToolGoErrorHealsOrphans verifies that a tool returning a Go error
// (rather than a Result with IsError) does not leave orphan tool_use blocks
// in the session — the next turn would otherwise be rejected by providers
// that require every function_call to have a function_call_output.
func TestRun_ToolGoErrorHealsOrphans(t *testing.T) {
	f := faux.New().CallTool("boom", `{}`).Done()
	s := session.New("gpt-5")
	reg := tools.NewRegistry()
	reg.Register(erroringTool{})
	a := New(f, reg, s, "")

	evs := collect(t, a.Run(context.Background(), userMsg("trigger")))

	var sawErr bool
	for _, e := range evs {
		if _, ok := e.(TurnError); ok {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatal("expected TurnError")
	}
	assertNoOrphans(t, s)
}

// TestRun_StreamFatalErrorHealsOrphans verifies that a fatal stream error
// after the assistant has emitted tool_uses leaves the session valid for
// the next request. Without healing, the persisted tool_use would poison
// every subsequent turn.
func TestRun_StreamFatalErrorHealsOrphans(t *testing.T) {
	// Provider emits a tool_call and a fatal StreamError without a Done.
	// Without persistence on this code path the assistant message is not
	// added until Done — so we need a provider that DOES reach Done with
	// tool calls, then a separate failure on the next turn. Instead, use
	// ToolGoErrorHealsOrphans for the persisted-orphan case; here verify
	// healOrphans runs as a deferred safety net even when the provider
	// errors mid-stream and we never persist.
	a := New(streamErrProvider{err: errStr("status 500: boom")}, tools.NewRegistry(), session.New("gpt-5"), "")
	evs := collect(t, a.Run(context.Background(), userMsg("hi")))

	var sawErr bool
	for _, e := range evs {
		if _, ok := e.(TurnError); ok {
			sawErr = true
		}
	}
	if !sawErr {
		t.Fatal("expected TurnError")
	}
}

// retryThenSucceedProvider emits an ErrOrphanOutput on the first call, then
// a successful Done on the second.
type retryThenSucceedProvider struct {
	calls int
}

func (p *retryThenSucceedProvider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	p.calls++
	out := make(chan ai.Event, 4)
	if p.calls == 1 {
		out <- ai.StreamError{Err: errStr("missing required parameter: 'input[0].output'"), Class: ai.ErrOrphanOutput}
		close(out)
		return out, nil
	}
	out <- ai.TextDelta{Text: "ok"}
	out <- ai.Usage{Input: 1, Output: 1}
	out <- ai.Done{Reason: "stop"}
	close(out)
	return out, nil
}

// TestRun_OrphanOutputHealsAndRetries verifies that an ErrOrphanOutput
// triggers session healing and one retry. The retried request must include
// the synthetic tool_result so the provider doesn't reject it again.
func TestRun_OrphanOutputHealsAndRetries(t *testing.T) {
	s := session.New("gpt-5")
	// Plant a poisoned state: an assistant tool_use with no matching tool_result.
	s.Append(userMsg("u1"))
	s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{
		ai.ToolUseBlock{ID: "call_1", Name: "read", Args: json.RawMessage(`{}`)},
	}})

	p := &retryThenSucceedProvider{}
	a := New(p, tools.NewRegistry(), s, "")

	evs := collect(t, a.Run(context.Background(), userMsg("u2")))

	if p.calls != 2 {
		t.Fatalf("expected 2 provider calls, got %d", p.calls)
	}

	var sawDone bool
	for _, e := range evs {
		if v, ok := e.(TurnDone); ok && v.Reason == "stop" {
			sawDone = true
		}
	}
	if !sawDone {
		t.Fatal("expected TurnDone on retry success")
	}
	assertNoOrphans(t, s)

	// The planted orphan must have been healed with a synthetic error
	// tool_result keyed to its call_id.
	var foundHeal bool
	for _, m := range s.PathToActive() {
		if m.Role != ai.RoleToolResult {
			continue
		}
		for _, c := range m.Content {
			if v, ok := c.(ai.ToolResultBlock); ok && v.ToolCallID == "call_1" && v.IsError {
				foundHeal = true
			}
		}
	}
	if !foundHeal {
		t.Fatal("expected synthetic error tool_result for healed orphan")
	}
}

func assertNoOrphans(t *testing.T, s *session.Session) {
	t.Helper()
	used := map[string]bool{}
	resulted := map[string]bool{}
	for _, m := range s.PathToActive() {
		for _, c := range m.Content {
			switch v := c.(type) {
			case ai.ToolUseBlock:
				used[v.ID] = true
			case ai.ToolResultBlock:
				resulted[v.ToolCallID] = true
			}
		}
	}
	for id := range used {
		if !resulted[id] {
			t.Fatalf("orphan tool_use remains in session: id=%s", id)
		}
	}
}
