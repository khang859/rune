package agent

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/config"
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

func TestSubagentSupervisor_AcceptsPlanningTypes(t *testing.T) {
	for _, typ := range []string{"exploration", "validator"} {
		t.Run(typ, func(t *testing.T) {
			p := faux.New().Reply("ok").Done()
			a := New(p, tools.NewRegistry(), session.New("gpt-test"), "")

			task, err := a.Subagents().Spawn(context.Background(), tools.SpawnSubagentRequest{
				Name:       typ,
				Prompt:     "do it",
				AgentType:  typ,
				Background: false,
			})
			if err != nil {
				t.Fatalf("Spawn error: %v", err)
			}
			if task.AgentType != typ {
				t.Fatalf("agent type = %q, want %q", task.AgentType, typ)
			}
		})
	}
}

func TestSubagentSystemPromptSpecializesPlanningTypes(t *testing.T) {
	if got := subagentSystemPrompt("exploration"); !strings.Contains(got, "Exploration focus") || !strings.Contains(got, "Discover the relevant files") {
		t.Fatalf("exploration prompt missing specialization:\n%s", got)
	}
	if got := subagentSystemPrompt("validator"); !strings.Contains(got, "Validator focus") || !strings.Contains(got, "Review the proposed plan") {
		t.Fatalf("validator prompt missing specialization:\n%s", got)
	}
}

func TestSubagentSupervisor_AssignsUniqueFamiliarNames(t *testing.T) {
	a := New(faux.New().Reply("ok").Done(), tools.NewRegistry(), session.New("gpt-test"), "")
	seen := map[string]struct{}{}
	var wrapped string
	for i := range len(familiarNames) + 1 {
		task, err := a.Subagents().Spawn(context.Background(), tools.SpawnSubagentRequest{
			Name:       fmt.Sprintf("task-%d", i),
			Prompt:     "do it",
			Background: false,
		})
		if err != nil {
			t.Fatalf("Spawn error: %v", err)
		}
		if task.FamiliarName == "" {
			t.Fatal("familiar name is empty")
		}
		if _, ok := seen[task.FamiliarName]; ok {
			t.Fatalf("duplicate familiar name %q", task.FamiliarName)
		}
		seen[task.FamiliarName] = struct{}{}
		wrapped = task.FamiliarName
	}
	if wrapped != "Nyx II" {
		t.Fatalf("wrapped familiar name = %q, want Nyx II", wrapped)
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
	for _, name := range []string{"spawn_subagent", "list_subagents", "get_subagent_result", "cancel_subagent"} {
		if child.Has(name) {
			t.Fatalf("child read-only registry unexpectedly has %s", name)
		}
	}
	for _, name := range []string{"write", "edit", "bash"} {
		if child.Has(name) {
			res, err := child.Run(context.Background(), ai.ToolCall{Name: name})
			if err != nil || !res.IsError {
				t.Fatalf("child read-only registry should deny %s at runtime, result=%#v err=%v", name, res, err)
			}
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

func TestSubagentConfigFromSettings(t *testing.T) {
	cfg := SubagentConfigFromSettings(config.SubagentSettings{MaxConcurrent: 2, DefaultTimeoutSecs: 30, MaxCompletedRetain: 7})
	if cfg.MaxConcurrent != 2 {
		t.Fatalf("MaxConcurrent = %d, want 2", cfg.MaxConcurrent)
	}
	if cfg.DefaultTimeout != 30*time.Second {
		t.Fatalf("DefaultTimeout = %s, want 30s", cfg.DefaultTimeout)
	}
	if cfg.MaxCompletedRetain != 7 {
		t.Fatalf("MaxCompletedRetain = %d, want 7", cfg.MaxCompletedRetain)
	}
}

func TestRegisterSubagentToolsDisabled(t *testing.T) {
	reg := tools.NewRegistry()
	a := New(faux.New(), reg, session.New("gpt-test"), "")
	a.RegisterSubagentToolsEnabled(false)

	cases := []struct {
		name string
		args []byte
	}{
		{name: "spawn_subagent", args: []byte(`{"name":"x","prompt":"y"}`)},
		{name: "list_subagents", args: []byte(`{}`)},
		{name: "get_subagent_result", args: []byte(`{"task_id":"subagent_1"}`)},
		{name: "cancel_subagent", args: []byte(`{"task_id":"subagent_1"}`)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := reg.Run(context.Background(), ai.ToolCall{Name: tc.name, Args: tc.args})
			if err != nil {
				t.Fatalf("Run error: %v", err)
			}
			if !res.IsError || !strings.Contains(res.Output, "subagents are disabled") {
				t.Fatalf("result = %+v, want disabled error", res)
			}
		})
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

func TestSubagentSupervisor_BlockedDependencyUnblocksAfterCompletion(t *testing.T) {
	p := &delayedSequenceProvider{replies: []string{"dep summary", "child summary"}, release: make(chan struct{})}
	a := New(p, tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{MaxConcurrent: 2, DefaultTimeout: time.Minute})

	dep, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "dep", Prompt: "first", Background: true})
	if err != nil {
		t.Fatalf("spawn dep: %v", err)
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if got := s.Get(dep.ID); got != nil && got.Status == string(SubagentRunning) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	child, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "child", Prompt: "second", Dependencies: []string{dep.ID}, Background: true})
	if err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	if got := s.Get(child.ID); got == nil || got.Status != string(SubagentBlocked) {
		t.Fatalf("child status = %+v, want blocked", got)
	}

	close(p.release)
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if got := s.Get(child.ID); got != nil && got.Status == string(SubagentCompleted) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child did not complete; got %+v", s.Get(child.ID))
}

func TestSubagentSupervisor_IncludesDependencySummaryInPrompt(t *testing.T) {
	p := &subagentCaptureProvider{}
	a := New(p, tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{MaxConcurrent: 1, DefaultTimeout: time.Minute})

	dep, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "dep", Prompt: "first", Background: false})
	if err != nil {
		t.Fatalf("spawn dep: %v", err)
	}
	child, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "child", Prompt: "second task", Dependencies: []string{dep.ID}, Background: false})
	if err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	if child.Status != string(SubagentCompleted) {
		t.Fatalf("child status = %s", child.Status)
	}
	if len(p.requests) != 2 {
		t.Fatalf("requests = %d, want 2", len(p.requests))
	}
	prompt := requestText(p.requests[1])
	if !strings.Contains(prompt, "Dependency results:") || !strings.Contains(prompt, "[Dependency completed: "+dep.ID+" / dep]") || !strings.Contains(prompt, "subagent summary") || !strings.Contains(prompt, "Delegated task:\n\nsecond task") {
		t.Fatalf("dependency prompt missing expected content:\n%s", prompt)
	}
}

func TestSubagentSupervisor_FailedDependencyFailsBlockedTask(t *testing.T) {
	a := New(blockingProvider{}, tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{MaxConcurrent: 1, DefaultTimeout: time.Minute})

	ctx, cancel := context.WithCancel(context.Background())
	dep, err := s.Spawn(ctx, tools.SpawnSubagentRequest{Name: "dep", Prompt: "wait", Background: true})
	if err != nil {
		t.Fatalf("spawn dep: %v", err)
	}
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		if got := s.Get(dep.ID); got != nil && got.Status == string(SubagentRunning) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	child, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "child", Prompt: "blocked", Dependencies: []string{dep.ID}, Background: true})
	if err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	cancel()
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		got := s.Get(child.ID)
		if got != nil && got.Status == string(SubagentFailed) {
			if !strings.Contains(got.Error, "dependency "+dep.ID) {
				t.Fatalf("error = %q, want dependency error", got.Error)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("child did not fail; got %+v", s.Get(child.ID))
}

func TestSubagentSupervisor_CancelBlocked(t *testing.T) {
	a := New(blockingProvider{}, tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{MaxConcurrent: 1, DefaultTimeout: time.Minute})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dep, err := s.Spawn(ctx, tools.SpawnSubagentRequest{Name: "dep", Prompt: "wait", Background: true})
	if err != nil {
		t.Fatalf("spawn dep: %v", err)
	}
	waitForSubagentStatus(t, s, dep.ID, SubagentRunning)
	child, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "child", Prompt: "blocked", Dependencies: []string{dep.ID}, Background: true})
	if err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	if err := s.Cancel(child.ID); err != nil {
		t.Fatalf("cancel child: %v", err)
	}
	if got := s.Get(child.ID); got == nil || got.Status != string(SubagentCancelled) {
		t.Fatalf("child = %+v, want cancelled", got)
	}
}

func TestSubagentSupervisor_CancelBlockedDependencyFailsDependent(t *testing.T) {
	a := New(blockingProvider{}, tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{MaxConcurrent: 1, DefaultTimeout: time.Minute})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	root, err := s.Spawn(ctx, tools.SpawnSubagentRequest{Name: "root", Prompt: "wait", Background: true})
	if err != nil {
		t.Fatalf("spawn root: %v", err)
	}
	waitForSubagentStatus(t, s, root.ID, SubagentRunning)
	dep, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "dep", Prompt: "blocked", Dependencies: []string{root.ID}, Background: true})
	if err != nil {
		t.Fatalf("spawn dep: %v", err)
	}
	waitForSubagentStatus(t, s, dep.ID, SubagentBlocked)
	child, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "child", Prompt: "blocked on dep", Dependencies: []string{dep.ID}, Background: true})
	if err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	waitForSubagentStatus(t, s, child.ID, SubagentBlocked)

	if err := s.Cancel(dep.ID); err != nil {
		t.Fatalf("cancel dep: %v", err)
	}
	got := waitForSubagentStatus(t, s, child.ID, SubagentFailed)
	if !strings.Contains(got.Error, "dependency "+dep.ID+" is cancelled") {
		t.Fatalf("child error = %q, want cancelled dependency error", got.Error)
	}
}

func TestSubagentSupervisor_CancelPendingDependencyFailsDependent(t *testing.T) {
	a := New(blockingProvider{}, tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{MaxConcurrent: 1, DefaultTimeout: time.Minute})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	running, err := s.Spawn(ctx, tools.SpawnSubagentRequest{Name: "running", Prompt: "wait", Background: true})
	if err != nil {
		t.Fatalf("spawn running: %v", err)
	}
	waitForSubagentStatus(t, s, running.ID, SubagentRunning)
	dep, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "dep", Prompt: "pending", Background: true})
	if err != nil {
		t.Fatalf("spawn dep: %v", err)
	}
	waitForSubagentStatus(t, s, dep.ID, SubagentPending)
	child, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "child", Prompt: "blocked on pending", Dependencies: []string{dep.ID}, Background: true})
	if err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	waitForSubagentStatus(t, s, child.ID, SubagentBlocked)

	if err := s.Cancel(dep.ID); err != nil {
		t.Fatalf("cancel dep: %v", err)
	}
	got := waitForSubagentStatus(t, s, child.ID, SubagentFailed)
	if !strings.Contains(got.Error, "dependency "+dep.ID+" is cancelled") {
		t.Fatalf("child error = %q, want cancelled dependency error", got.Error)
	}
}

func TestSubagentSupervisor_FailedDependencyFailsDependent(t *testing.T) {
	a := New(errorProvider{err: errors.New("provider exploded")}, tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{MaxConcurrent: 1, DefaultTimeout: time.Minute})

	dep, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "dep", Prompt: "fail", Background: true})
	if err != nil {
		t.Fatalf("spawn dep: %v", err)
	}
	child, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "child", Prompt: "blocked", Dependencies: []string{dep.ID}, Background: true})
	if err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	waitForSubagentStatus(t, s, dep.ID, SubagentFailed)
	got := waitForSubagentStatus(t, s, child.ID, SubagentFailed)
	if !strings.Contains(got.Error, "dependency "+dep.ID+" is failed") {
		t.Fatalf("child error = %q, want failed dependency error", got.Error)
	}
}

func TestSubagentSupervisor_SpawnRejectsUnknownDependency(t *testing.T) {
	a := New(faux.New(), tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{DefaultTimeout: time.Minute})

	_, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "child", Prompt: "blocked", Dependencies: []string{"subagent_missing"}, Background: true})
	if err == nil {
		t.Fatal("expected unknown dependency error")
	}
	if !strings.Contains(err.Error(), `unknown dependency task "subagent_missing"`) {
		t.Fatalf("error = %q, want unknown dependency", err.Error())
	}
}

func TestSubagentSupervisor_CleansDependencyIDs(t *testing.T) {
	p := faux.New().Reply("dep summary").Reply("child summary").Done()
	a := New(p, tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{MaxConcurrent: 1, DefaultTimeout: time.Minute})

	dep, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "dep", Prompt: "first", Background: false})
	if err != nil {
		t.Fatalf("spawn dep: %v", err)
	}
	child, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "child", Prompt: "second", Dependencies: []string{"", " " + dep.ID + " ", dep.ID, "\t"}, Background: false})
	if err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	if !reflect.DeepEqual(child.Dependencies, []string{dep.ID}) {
		t.Fatalf("dependencies = %#v, want only %s", child.Dependencies, dep.ID)
	}
}

func TestSubagentSupervisor_BlockedLifecycleEvents(t *testing.T) {
	p := &delayedSequenceProvider{replies: []string{"dep summary", "child summary"}, release: make(chan struct{})}
	a := New(p, tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{MaxConcurrent: 2, DefaultTimeout: time.Minute})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := s.Subscribe(ctx)

	dep, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "dep", Prompt: "first", Background: true})
	if err != nil {
		t.Fatalf("spawn dep: %v", err)
	}
	waitForSubagentStatus(t, s, dep.ID, SubagentRunning)
	child, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "child", Prompt: "second", Dependencies: []string{dep.ID}, Background: true})
	if err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	waitForSubagentStatus(t, s, child.ID, SubagentBlocked)
	close(p.release)
	waitForSubagentStatus(t, s, child.ID, SubagentCompleted)

	statuses := collectSubagentStatuses(t, events, child.ID, 4)
	want := []SubagentStatus{SubagentBlocked, SubagentPending, SubagentRunning, SubagentCompleted}
	if !reflect.DeepEqual(statuses, want) {
		t.Fatalf("child statuses = %v, want %v", statuses, want)
	}
}

func TestSubagentSupervisor_MultipleDependenciesWaitsForAll(t *testing.T) {
	p := &delayedSequenceProvider{replies: []string{"dep1 summary", "dep2 summary", "child summary"}, releaseByPrompt: map[string]chan struct{}{"first": make(chan struct{}), "second": make(chan struct{})}}
	a := New(p, tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{MaxConcurrent: 3, DefaultTimeout: time.Minute})

	dep1, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "dep1", Prompt: "first", Background: true})
	if err != nil {
		t.Fatalf("spawn dep1: %v", err)
	}
	dep2, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "dep2", Prompt: "second", Background: true})
	if err != nil {
		t.Fatalf("spawn dep2: %v", err)
	}
	waitForSubagentStatus(t, s, dep1.ID, SubagentRunning)
	waitForSubagentStatus(t, s, dep2.ID, SubagentRunning)
	child, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "child", Prompt: "third", Dependencies: []string{dep1.ID, dep2.ID}, Background: true})
	if err != nil {
		t.Fatalf("spawn child: %v", err)
	}
	waitForSubagentStatus(t, s, child.ID, SubagentBlocked)

	close(p.releaseByPrompt["first"])
	waitForSubagentStatus(t, s, dep1.ID, SubagentCompleted)
	time.Sleep(50 * time.Millisecond)
	if got := s.Get(child.ID); got == nil || got.Status != string(SubagentBlocked) {
		t.Fatalf("child after first dependency = %+v, want still blocked", got)
	}

	close(p.releaseByPrompt["second"])
	waitForSubagentStatus(t, s, child.ID, SubagentCompleted)
}

func TestSubagentSupervisor_ChainedFailedDependencyFailsDescendants(t *testing.T) {
	a := New(errorProvider{err: errors.New("provider exploded")}, tools.NewRegistry(), session.New("gpt-test"), "")
	s := NewSubagentSupervisor(a, SubagentConfig{MaxConcurrent: 1, DefaultTimeout: time.Minute})

	root, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "root", Prompt: "fail", Background: true})
	if err != nil {
		t.Fatalf("spawn root: %v", err)
	}
	mid, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "mid", Prompt: "blocked", Dependencies: []string{root.ID}, Background: true})
	if err != nil {
		t.Fatalf("spawn mid: %v", err)
	}
	leaf, err := s.Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "leaf", Prompt: "blocked", Dependencies: []string{mid.ID}, Background: true})
	if err != nil {
		t.Fatalf("spawn leaf: %v", err)
	}
	waitForSubagentStatus(t, s, root.ID, SubagentFailed)
	waitForSubagentStatus(t, s, mid.ID, SubagentFailed)
	got := waitForSubagentStatus(t, s, leaf.ID, SubagentFailed)
	if !strings.Contains(got.Error, "dependency "+mid.ID+" is failed") {
		t.Fatalf("leaf error = %q, want failed mid dependency", got.Error)
	}
}

type delayedSequenceProvider struct {
	mu              sync.Mutex
	replies         []string
	next            int
	release         chan struct{}
	releaseByPrompt map[string]chan struct{}
}

func (p *delayedSequenceProvider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	p.mu.Lock()
	idx := p.next
	p.next++
	text := ""
	if idx < len(p.replies) {
		text = p.replies[idx]
	}
	prompt := requestText(req)
	var release chan struct{}
	for needle, ch := range p.releaseByPrompt {
		if strings.Contains(prompt, needle) {
			release = ch
			break
		}
	}
	if idx == 0 && p.release != nil {
		release = p.release
	}
	p.mu.Unlock()

	out := make(chan ai.Event, 2)
	go func() {
		defer close(out)
		if release != nil {
			select {
			case <-release:
			case <-ctx.Done():
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case out <- ai.TextDelta{Text: text}:
		}
		select {
		case <-ctx.Done():
		case out <- ai.Done{Reason: "stop"}:
		}
	}()
	return out, nil
}

func waitForSubagentStatus(t *testing.T, s *SubagentSupervisor, id string, status SubagentStatus) *tools.SubagentTask {
	t.Helper()
	for deadline := time.Now().Add(time.Second); time.Now().Before(deadline); {
		got := s.Get(id)
		if got != nil && got.Status == string(status) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
	got := s.Get(id)
	t.Fatalf("subagent %s status = %+v, want %s", id, got, status)
	return nil
}

func collectSubagentStatuses(t *testing.T, events <-chan SubagentEvent, id string, n int) []SubagentStatus {
	t.Helper()
	statuses := make([]SubagentStatus, 0, n)
	for len(statuses) < n {
		select {
		case ev := <-events:
			if ev.Task.ID == id {
				statuses = append(statuses, ev.Status)
			}
		case <-time.After(time.Second):
			t.Fatalf("timed out waiting for %d statuses for %s; got %v", n, id, statuses)
		}
	}
	return statuses
}

type errorProvider struct{ err error }

func (p errorProvider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	_ = ctx
	_ = req
	out := make(chan ai.Event, 1)
	out <- ai.StreamError{Err: p.err}
	close(out)
	return out, nil
}
