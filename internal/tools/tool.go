package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/search"
)

type Tool interface {
	Spec() ai.ToolSpec
	Run(ctx context.Context, args json.RawMessage) (Result, error)
}

// PlanModeTool is implemented by tools that explicitly opt in to Plan Mode.
// Built-in read-only tools are still controlled by the registry allowlist below;
// this interface is primarily for external/MCP tools, which are denied by default.
type PlanModeTool interface {
	AllowedInPlanMode() bool
}

type Result struct {
	Output  string
	IsError bool
}

type PermissionMode string

const (
	PermissionModeAct  PermissionMode = "act"
	PermissionModePlan PermissionMode = "plan"
)

type Registry struct {
	tools map[string]Tool
	mode  PermissionMode
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}, mode: PermissionModeAct}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Spec().Name] = t
}

func (r *Registry) Unregister(name string) { delete(r.tools, name) }

func (r *Registry) Has(name string) bool { _, ok := r.tools[name]; return ok }

func (r *Registry) SetPermissionMode(mode PermissionMode) {
	if mode == "" {
		mode = PermissionModeAct
	}
	r.mode = mode
}

func (r *Registry) PermissionMode() PermissionMode {
	if r.mode == "" {
		return PermissionModeAct
	}
	return r.mode
}

func (r *Registry) Clone() *Registry {
	cp := NewRegistry()
	cp.SetPermissionMode(r.PermissionMode())
	for name, tool := range r.tools {
		cp.tools[name] = tool
	}
	return cp
}

func (r *Registry) CloneReadOnly() *Registry {
	cp := r.Clone()
	cp.SetPermissionMode(PermissionModePlan)
	cp.UnregisterSubagentTools()
	return cp
}

func (r *Registry) CloneForSubagentFull() *Registry {
	cp := r.Clone()
	cp.SetPermissionMode(PermissionModeAct)
	cp.UnregisterSubagentTools()
	return cp
}

func (r *Registry) UnregisterSubagentTools() {
	r.Unregister("spawn_subagent")
	r.Unregister("list_subagents")
	r.Unregister("get_subagent_result")
	r.Unregister("cancel_subagent")
}

type BuiltinOptions struct {
	WebFetchEnabled      bool
	WebFetchAllowPrivate bool
	SearchProvider       search.Provider
}

func RegisterBuiltins(r *Registry, opts BuiltinOptions) {
	r.Register(Read{})
	r.Register(ListFiles{})
	r.Register(SearchFiles{})
	r.Register(GitStatus{})
	r.Register(GitDiff{})
	r.Register(Write{})
	r.Register(Edit{})
	r.Register(Bash{})
	if opts.WebFetchEnabled {
		r.Register(WebFetch{AllowPrivate: opts.WebFetchAllowPrivate})
	}
	if opts.SearchProvider != nil {
		r.Register(WebSearch{Provider: opts.SearchProvider})
	}
}

func (r *Registry) Specs() []ai.ToolSpec {
	var names []string
	for n := range r.tools {
		if r.toolAllowed(n) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	out := make([]ai.ToolSpec, 0, len(names))
	for _, n := range names {
		out = append(out, r.tools[n].Spec())
	}
	return out
}

func (r *Registry) Run(ctx context.Context, call ai.ToolCall) (Result, error) {
	t, ok := r.tools[call.Name]
	if ok && !r.toolAllowed(call.Name) {
		return Result{Output: fmt.Sprintf("tool %q is disabled in Plan Mode", call.Name), IsError: true}, nil
	}
	if !ok {
		names := make([]string, 0, len(r.tools))
		for n := range r.tools {
			names = append(names, n)
		}
		sort.Strings(names)
		return Result{
			Output:  fmt.Sprintf("unknown tool %q. Available tools: %s.", call.Name, strings.Join(names, ", ")),
			IsError: true,
		}, nil
	}
	return t.Run(ctx, call.Args)
}

func (r *Registry) toolAllowed(name string) bool {
	if r.PermissionMode() != PermissionModePlan {
		return true
	}
	switch name {
	case "read", "list_files", "search_files", "git_status", "git_diff", "web_search", "web_fetch", "spawn_subagent", "list_subagents", "get_subagent_result", "cancel_subagent":
		return true
	}
	if t, ok := r.tools[name]; ok {
		if pm, ok := t.(PlanModeTool); ok {
			return pm.AllowedInPlanMode()
		}
	}
	return false
}
