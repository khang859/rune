package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

type stubTool struct{ name string }

type planStubTool struct {
	stubTool
	allowed bool
}

func (p planStubTool) AllowedInPlanMode() bool { return p.allowed }

func (s stubTool) Spec() ai.ToolSpec {
	return ai.ToolSpec{Name: s.name, Description: "stub", Schema: json.RawMessage(`{}`)}
}
func (s stubTool) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	return Result{Output: "ran " + s.name}, nil
}

func TestRegistry_RegisterAndRun(t *testing.T) {
	r := NewRegistry()
	r.Register(stubTool{name: "x"})
	res, err := r.Run(context.Background(), ai.ToolCall{Name: "x", Args: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "ran x" {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestRegistry_UnknownTool(t *testing.T) {
	r := NewRegistry()
	r.Register(stubTool{name: "alpha"})
	r.Register(stubTool{name: "beta"})
	res, err := r.Run(context.Background(), ai.ToolCall{Name: "missing"})
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError=true, got %#v", res)
	}
	if !strings.Contains(res.Output, `unknown tool "missing"`) {
		t.Fatalf("output should name the missing tool: %q", res.Output)
	}
	if !strings.Contains(res.Output, "alpha") || !strings.Contains(res.Output, "beta") {
		t.Fatalf("output should list available tools: %q", res.Output)
	}
}

func TestRegistry_Specs(t *testing.T) {
	r := NewRegistry()
	r.Register(stubTool{name: "a"})
	r.Register(stubTool{name: "b"})
	specs := r.Specs()
	if len(specs) != 2 {
		t.Fatalf("specs len = %d", len(specs))
	}
}

func TestRegistry_PlanModeFiltersAndDeniesTools(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"read", "list_files", "search_files", "git_status", "git_diff", "write", "edit", "bash", "web_search", "external_tool"} {
		r.Register(stubTool{name: name})
	}
	r.Register(planStubTool{stubTool: stubTool{name: "plan_external"}, allowed: true})
	r.Register(planStubTool{stubTool: stubTool{name: "unsafe_external"}, allowed: false})
	r.SetPermissionMode(PermissionModePlan)

	for _, spec := range r.Specs() {
		switch spec.Name {
		case "write", "edit", "bash", "external_tool", "unsafe_external":
			t.Fatalf("plan mode exposed denied tool %q", spec.Name)
		}
	}
	for _, name := range []string{"read", "list_files", "search_files", "git_status", "git_diff", "web_search", "plan_external"} {
		res, err := r.Run(context.Background(), ai.ToolCall{Name: name, Args: json.RawMessage(`{}`)})
		if err != nil || res.IsError {
			t.Fatalf("allowed tool %q result=%#v err=%v", name, res, err)
		}
	}
	for _, name := range []string{"write", "edit", "bash", "external_tool", "unsafe_external"} {
		res, err := r.Run(context.Background(), ai.ToolCall{Name: name, Args: json.RawMessage(`{}`)})
		if err != nil {
			t.Fatalf("denial should not be go error for %q: %v", name, err)
		}
		if !res.IsError || !strings.Contains(res.Output, "disabled in Plan Mode") {
			t.Fatalf("denied tool %q result=%#v", name, res)
		}
	}
}

func TestRegistry_ActModeExposesAndRunsMutatingTools(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"write", "edit", "bash"} {
		r.Register(stubTool{name: name})
	}
	if got := len(r.Specs()); got != 3 {
		t.Fatalf("act specs len=%d, want 3", got)
	}
	for _, name := range []string{"write", "edit", "bash"} {
		res, err := r.Run(context.Background(), ai.ToolCall{Name: name, Args: json.RawMessage(`{}`)})
		if err != nil || res.IsError || res.Output != "ran "+name {
			t.Fatalf("act run %q result=%#v err=%v", name, res, err)
		}
	}
}

func TestRegistry_CloneReadOnlyUsesPlanPolicyAndDisablesChildSubagents(t *testing.T) {
	r := NewRegistry()
	for _, name := range []string{"read", "write", "edit", "bash", "spawn_subagent", "list_subagents", "get_subagent_result", "cancel_subagent"} {
		r.Register(stubTool{name: name})
	}
	cp := r.CloneReadOnly()
	if got := cp.PermissionMode(); got != PermissionModePlan {
		t.Fatalf("clone mode=%q, want plan", got)
	}
	if cp.Has("spawn_subagent") || cp.Has("list_subagents") || cp.Has("get_subagent_result") || cp.Has("cancel_subagent") {
		t.Fatal("read-only child registry should not include subagent tools")
	}
	res, err := cp.Run(context.Background(), ai.ToolCall{Name: "write", Args: json.RawMessage(`{}`)})
	if err != nil || !res.IsError {
		t.Fatalf("write should be denied by read-only clone: result=%#v err=%v", res, err)
	}
}

func TestRegistry_DoesNotSwallowToolErrors(t *testing.T) {
	r := NewRegistry()
	r.Register(errTool{})
	_, err := r.Run(context.Background(), ai.ToolCall{Name: "boom"})
	if !errors.Is(err, errBoom) {
		t.Fatalf("err = %v", err)
	}
}

var errBoom = errors.New("boom")

type errTool struct{}

func (errTool) Spec() ai.ToolSpec {
	return ai.ToolSpec{Name: "boom", Schema: json.RawMessage(`{}`)}
}
func (errTool) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	return Result{}, errBoom
}
