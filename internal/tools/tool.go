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

func (r *Registry) Unregister(name string) { delete(r.tools, name) }

func (r *Registry) Has(name string) bool { _, ok := r.tools[name]; return ok }

type BuiltinOptions struct {
	WebFetchEnabled      bool
	WebFetchAllowPrivate bool
	SearchProvider       search.Provider
}

func RegisterBuiltins(r *Registry, opts BuiltinOptions) {
	r.Register(Read{})
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
