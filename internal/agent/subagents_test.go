package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

func TestSubagentSupervisor_SpawnGeneralForeground(t *testing.T) {
	p := faux.New().Reply("## Summary\nsubagent result").Done()
	a := New(p, tools.NewRegistry(), session.New("gpt-test"), "base")

	task, err := a.Subagents().Spawn(context.Background(), tools.SpawnSubagentRequest{
		Name:       "inspect",
		Prompt:     "inspect something",
		AgentType:  "general",
		Background: false,
	})
	if err != nil {
		t.Fatalf("Spawn error: %v", err)
	}
	if task.Status != string(SubagentCompleted) {
		t.Fatalf("status = %q, want completed", task.Status)
	}
	if task.Summary != "## Summary\nsubagent result" {
		t.Fatalf("summary = %q", task.Summary)
	}
	if task.AgentType != "general" {
		t.Fatalf("agent type = %q", task.AgentType)
	}
}

func TestSubagentSupervisor_DefaultsToGeneral(t *testing.T) {
	p := faux.New().Reply("ok").Done()
	a := New(p, tools.NewRegistry(), session.New("gpt-test"), "")

	task, err := a.Subagents().Spawn(context.Background(), tools.SpawnSubagentRequest{
		Name:       "general by default",
		Prompt:     "do it",
		Background: false,
	})
	if err != nil {
		t.Fatalf("Spawn error: %v", err)
	}
	if task.AgentType != "general" {
		t.Fatalf("agent type = %q, want general", task.AgentType)
	}
}

func TestSubagentSupervisor_RejectsUnknownType(t *testing.T) {
	a := New(faux.New(), tools.NewRegistry(), session.New("gpt-test"), "")
	_, err := a.Subagents().Spawn(context.Background(), tools.SpawnSubagentRequest{
		Name:      "bad",
		Prompt:    "do it",
		AgentType: "security",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSubagentToolsRegisteredAndHiddenFromChildren(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(tools.Read{})
	reg.Register(tools.Write{})
	a := New(faux.New(), reg, session.New("gpt-test"), "")
	a.RegisterSubagentTools()

	for _, name := range []string{"spawn_subagent", "list_subagents", "get_subagent_result", "cancel_subagent"} {
		if !reg.Has(name) {
			t.Fatalf("parent registry missing %s", name)
		}
	}
	child := reg.CloneReadOnly()
	for _, name := range []string{"write", "edit", "bash", "spawn_subagent", "list_subagents", "get_subagent_result", "cancel_subagent"} {
		if child.Has(name) {
			t.Fatalf("child read-only registry unexpectedly has %s", name)
		}
	}
	if !child.Has("read") {
		t.Fatal("child read-only registry should keep read")
	}
}

func TestSubagentSupervisor_PublishesLifecycleEvents(t *testing.T) {
	p := faux.New().Reply("done").Done()
	a := New(p, tools.NewRegistry(), session.New("gpt-test"), "")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := a.Subagents().Subscribe(ctx)

	_, err := a.Subagents().Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "worker", Prompt: "do it", Background: false})
	if err != nil {
		t.Fatalf("Spawn error: %v", err)
	}

	var statuses []SubagentStatus
	for len(statuses) < 3 {
		select {
		case ev := <-events:
			statuses = append(statuses, ev.Status)
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for lifecycle events; got %v", statuses)
		}
	}
	want := []SubagentStatus{SubagentPending, SubagentRunning, SubagentCompleted}
	for i := range want {
		if statuses[i] != want[i] {
			t.Fatalf("statuses = %v, want prefix %v", statuses, want)
		}
	}
}

func TestSubagentSupervisor_CancelPending(t *testing.T) {
	a := New(blockingProvider{}, tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{MaxConcurrent: 1, DefaultTimeout: time.Minute})

	ctx, block := context.WithCancel(context.Background())
	defer block()
	t1, err := s.Spawn(ctx, tools.SpawnSubagentRequest{Name: "first", Prompt: "wait", Background: true})
	if err != nil {
		t.Fatalf("spawn first: %v", err)
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if got := s.Get(t1.ID); got != nil && got.Status == string(SubagentRunning) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	t2, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "second", Prompt: "queued", Background: true})
	if err != nil {
		t.Fatalf("spawn second: %v", err)
	}
	if err := s.Cancel(t2.ID); err != nil {
		t.Fatalf("cancel second: %v", err)
	}
	if got := s.Get(t2.ID); got == nil || got.Status != string(SubagentCancelled) {
		t.Fatalf("second status = %+v, want cancelled", got)
	}
}

func TestRun_InjectsCompletedSubagentSummaryOnce(t *testing.T) {
	p := &subagentCaptureProvider{}
	a := New(p, tools.NewRegistry(), session.New("gpt-test"), "")
	_, err := a.Subagents().Spawn(context.Background(), tools.SpawnSubagentRequest{
		Name:       "inspect",
		Prompt:     "inspect something",
		Background: false,
	})
	if err != nil {
		t.Fatalf("Spawn error: %v", err)
	}

	collect(t, a.Run(context.Background(), userMsg("use the result")))
	collect(t, a.Run(context.Background(), userMsg("again")))

	if len(p.requests) != 3 {
		t.Fatalf("requests = %d, want 3", len(p.requests))
	}
	first := requestText(p.requests[1])
	if !strings.Contains(first, "[Subagent completed: subagent_") || !strings.Contains(first, "inspect") || !strings.Contains(first, "subagent summary") {
		t.Fatalf("first request missing injected summary:\n%s", first)
	}
	second := requestText(p.requests[2])
	if strings.Count(second, "subagent summary") != 1 {
		t.Fatalf("summary injected more than once; second request text:\n%s", second)
	}
}

type subagentCaptureProvider struct {
	requests []ai.Request
}

func (p *subagentCaptureProvider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	out := make(chan ai.Event, 2)
	text := "parent reply"
	if len(p.requests) == 0 {
		text = "subagent summary"
	}
	p.requests = append(p.requests, req)
	out <- ai.TextDelta{Text: text}
	out <- ai.Done{Reason: "stop"}
	close(out)
	return out, nil
}

func requestText(req ai.Request) string {
	var b strings.Builder
	for _, msg := range req.Messages {
		for _, block := range msg.Content {
			if text, ok := block.(ai.TextBlock); ok {
				b.WriteString(text.Text)
				b.WriteString("\n")
			}
		}
	}
	return b.String()
}

type blockingProvider struct{}

func (blockingProvider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	out := make(chan ai.Event)
	go func() {
		defer close(out)
		<-ctx.Done()
	}()
	return out, nil
}
