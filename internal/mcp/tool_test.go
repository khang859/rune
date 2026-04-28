package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestMCPTool_RunsAndStringifies(t *testing.T) {
	bin := buildStubServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	c, _ := Spawn(ctx, "stub", bin, nil, nil)
	defer c.Close()
	_ = c.Initialize(ctx)

	tt := NewTool(c, Tool{
		Name:        "echo",
		Description: "echo",
		InputSchema: json.RawMessage(`{"type":"object"}`),
	})
	spec := tt.Spec()
	if spec.Name != "stub:echo" {
		t.Fatalf("spec.Name = %q", spec.Name)
	}
	res, err := tt.Run(ctx, json.RawMessage(`{"text":"hello"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "echo: hello") {
		t.Fatalf("output = %q", res.Output)
	}
}
