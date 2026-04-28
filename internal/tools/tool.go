package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/khang859/rune/internal/ai"
)

type Tool interface {
	Spec() ai.ToolSpec
	Run(ctx context.Context, args json.RawMessage) (Result, error)
}

type Result struct {
	Output  string
	IsError bool
}

type Registry struct {
	tools map[string]Tool
}

func NewRegistry() *Registry {
	return &Registry{tools: map[string]Tool{}}
}

func (r *Registry) Register(t Tool) {
	r.tools[t.Spec().Name] = t
}

func (r *Registry) Specs() []ai.ToolSpec {
	var names []string
	for n := range r.tools {
		names = append(names, n)
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
	if !ok {
		return Result{}, fmt.Errorf("unknown tool: %q", call.Name)
	}
	return t.Run(ctx, call.Args)
}
