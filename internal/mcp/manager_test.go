package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/tools"
)

func TestManager_RegistersToolsFromAllServers(t *testing.T) {
	bin := buildStubServer(t)

	cfgPath := filepath.Join(t.TempDir(), "mcp.json")
	cfg := fmt.Sprintf(`{"servers":{"stub":{"command":%q}}}`, bin)
	_ = os.WriteFile(cfgPath, []byte(cfg), 0o644)

	reg := tools.NewRegistry()
	mgr := NewManager(cfgPath)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.Start(ctx, reg); err != nil {
		t.Fatal(err)
	}
	defer mgr.Shutdown()

	specs := reg.Specs()
	var found bool
	for _, s := range specs {
		if s.Name == "stub:echo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stub:echo not registered: %v", specs)
	}

	res, err := reg.Run(ctx, ai.ToolCall{Name: "stub:echo", Args: json.RawMessage(`{"text":"x"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || res.Output == "" {
		t.Fatalf("res = %#v", res)
	}
}

func TestManager_MissingConfigIsNoOp(t *testing.T) {
	reg := tools.NewRegistry()
	mgr := NewManager(filepath.Join(t.TempDir(), "does-not-exist.json"))
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := mgr.Start(ctx, reg); err != nil {
		t.Fatalf("missing config should be no-op, got %v", err)
	}
	if len(reg.Specs()) != 0 {
		t.Fatalf("expected empty registry, got %v", reg.Specs())
	}
}
