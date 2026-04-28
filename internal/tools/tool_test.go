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
	_, err := r.Run(context.Background(), ai.ToolCall{Name: "missing"})
	if err == nil || !strings.Contains(err.Error(), "unknown tool") {
		t.Fatalf("err = %v", err)
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
