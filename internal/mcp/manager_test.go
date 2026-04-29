package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
		if s.Name == "stub_echo" {
			found = true
		}
	}
	if !found {
		t.Fatalf("stub_echo not registered: %v", specs)
	}
	statuses := mgr.Statuses()
	if len(statuses) != 1 || !statuses[0].Connected || statuses[0].ToolCount != 1 || len(statuses[0].Tools) != 1 || statuses[0].Tools[0] != "echo" {
		t.Fatalf("statuses = %#v", statuses)
	}

	res, err := reg.Run(ctx, ai.ToolCall{Name: "stub_echo", Args: json.RawMessage(`{"text":"x"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || res.Output == "" {
		t.Fatalf("res = %#v", res)
	}
}

func TestManager_RegistersToolsFromHTTPServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"protocolVersion":"2024-11-05","capabilities":{}}`)})
		case "notifications/initialized":
			w.WriteHeader(http.StatusAccepted)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"tools":[{"name":"echo","inputSchema":{"type":"object"}}]}`)})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(Response{JSONRPC: "2.0", ID: req.ID, Result: json.RawMessage(`{"content":[{"type":"text","text":"echo: http"}]}`)})
		default:
			t.Fatalf("unexpected method %q", req.Method)
		}
	}))
	defer srv.Close()

	cfgPath := filepath.Join(t.TempDir(), "mcp.json")
	cfg := fmt.Sprintf(`{"servers":{"context7":{"type":"http","url":%q}}}`, srv.URL)
	_ = os.WriteFile(cfgPath, []byte(cfg), 0o644)

	reg := tools.NewRegistry()
	mgr := NewManager(cfgPath)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := mgr.Start(ctx, reg); err != nil {
		t.Fatal(err)
	}
	defer mgr.Shutdown()

	res, err := reg.Run(ctx, ai.ToolCall{Name: "context7_echo", Args: json.RawMessage(`{"text":"x"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError || res.Output != "echo: http" {
		t.Fatalf("res = %#v", res)
	}
}

func TestManager_RecordsFailedStatus(t *testing.T) {
	cfgPath := filepath.Join(t.TempDir(), "mcp.json")
	cfg := `{"servers":{"missing":{"command":"definitely-not-a-real-mcp-binary"}}}`
	_ = os.WriteFile(cfgPath, []byte(cfg), 0o644)

	reg := tools.NewRegistry()
	mgr := NewManager(cfgPath)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := mgr.Start(ctx, reg); err != nil {
		t.Fatal(err)
	}
	statuses := mgr.Statuses()
	if len(statuses) != 1 || statuses[0].Name != "missing" || statuses[0].Connected || statuses[0].Error == "" {
		t.Fatalf("statuses = %#v", statuses)
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
