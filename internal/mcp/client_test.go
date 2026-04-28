package mcp

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func buildStubServer(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "stub")
	cmd := exec.Command("go", "build", "-o", bin, "./testdata/stub_server.go")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build stub failed: %v\n%s", err, out)
	}
	return bin
}

func TestClient_InitializeAndListTools(t *testing.T) {
	bin := buildStubServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := Spawn(ctx, "stub", bin, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.Initialize(ctx); err != nil {
		t.Fatal(err)
	}
	tools, err := c.ListTools(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(tools) != 1 || tools[0].Name != "echo" {
		t.Fatalf("tools = %#v", tools)
	}
}

func TestClient_CallTool(t *testing.T) {
	bin := buildStubServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	c, err := Spawn(ctx, "stub", bin, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	_ = c.Initialize(ctx)

	res, err := c.CallTool(ctx, "echo", []byte(`{"text":"hi"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Content) != 1 || res.Content[0].Text != "echo: hi" {
		t.Fatalf("res = %#v", res)
	}
}
