package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/mcp"
)

func TestRunMCPAddCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", dir)

	var out, stderr bytes.Buffer
	if err := runMCP([]string{"add", "filesystem", "--", "npx", "-y", "@modelcontextprotocol/server-filesystem", "/tmp/work"}, &out, &stderr); err != nil {
		t.Fatal(err)
	}

	cfg := readMCPConfig(t)
	sc, ok := cfg.Servers["filesystem"]
	if !ok {
		t.Fatalf("filesystem server missing: %#v", cfg.Servers)
	}
	if sc.Command != "npx" {
		t.Fatalf("Command = %q, want npx", sc.Command)
	}
	wantArgs := []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp/work"}
	if strings.Join(sc.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("Args = %#v, want %#v", sc.Args, wantArgs)
	}
}

func TestRunMCPAddRefusesDuplicateWithoutForce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", dir)

	if err := runMCP([]string{"add", "filesystem", "--", "npx", "old"}, ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatal(err)
	}
	err := runMCP([]string{"add", "filesystem", "--", "npx", "new"}, ioDiscard{}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want already exists", err)
	}

	cfg := readMCPConfig(t)
	if got := cfg.Servers["filesystem"].Args[0]; got != "old" {
		t.Fatalf("server overwritten without force: arg = %q", got)
	}
}

func TestRunMCPAddForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", dir)

	if err := runMCP([]string{"add", "filesystem", "--", "npx", "old"}, ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatal(err)
	}
	if err := runMCP([]string{"add", "filesystem", "--force", "--", "npx", "new"}, ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatal(err)
	}

	cfg := readMCPConfig(t)
	if got := cfg.Servers["filesystem"].Args[0]; got != "new" {
		t.Fatalf("arg = %q, want new", got)
	}
}

func TestRunMCPAddHTTPCreatesConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", dir)

	if err := runMCP([]string{"add-http", "context7", "--url", "https://mcp.context7.com/mcp", "--header", "CONTEXT7_API_KEY=secret"}, ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatal(err)
	}

	cfg := readMCPConfig(t)
	sc, ok := cfg.Servers["context7"]
	if !ok {
		t.Fatalf("context7 server missing: %#v", cfg.Servers)
	}
	if sc.Type != "http" || sc.URL != "https://mcp.context7.com/mcp" {
		t.Fatalf("server = %#v, want http context7", sc)
	}
	if sc.Headers["CONTEXT7_API_KEY"] != "secret" {
		t.Fatalf("headers = %#v, want CONTEXT7_API_KEY", sc.Headers)
	}
}

func TestRunMCPAddHTTPRefusesDuplicateWithoutForce(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", dir)

	if err := runMCP([]string{"add-http", "context7", "--url", "https://old.example"}, ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatal(err)
	}
	err := runMCP([]string{"add-http", "context7", "--url", "https://new.example"}, ioDiscard{}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("err = %v, want already exists", err)
	}

	cfg := readMCPConfig(t)
	if got := cfg.Servers["context7"].URL; got != "https://old.example" {
		t.Fatalf("server overwritten without force: url = %q", got)
	}
}

func TestRunMCPList(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", dir)
	writeMCPConfig(t, mcp.Config{Servers: map[string]mcp.ServerConfig{
		"sqlite":     {Command: "uvx", Args: []string{"mcp-server-sqlite", "--db-path", "/tmp/db.sqlite"}},
		"filesystem": {Command: "npx", Args: []string{"-y", "@modelcontextprotocol/server-filesystem", "/tmp/work"}},
		"context7":   {Type: "http", URL: "https://mcp.context7.com/mcp"},
	}})

	var out bytes.Buffer
	if err := runMCP([]string{"list"}, &out, ioDiscard{}); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "filesystem") || !strings.Contains(got, "npx -y @modelcontextprotocol/server-filesystem /tmp/work") {
		t.Fatalf("list output missing filesystem: %q", got)
	}
	if !strings.Contains(got, "sqlite") || !strings.Contains(got, "uvx mcp-server-sqlite --db-path /tmp/db.sqlite") {
		t.Fatalf("list output missing sqlite: %q", got)
	}
	if !strings.Contains(got, "context7") || !strings.Contains(got, "http https://mcp.context7.com/mcp") {
		t.Fatalf("list output missing context7: %q", got)
	}
	if strings.Index(got, "filesystem") > strings.Index(got, "sqlite") {
		t.Fatalf("list output not sorted: %q", got)
	}
}

func TestRunMCPRemove(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", dir)
	writeMCPConfig(t, mcp.Config{Servers: map[string]mcp.ServerConfig{
		"filesystem": {Command: "npx"},
		"sqlite":     {Command: "uvx"},
	}})

	if err := runMCP([]string{"remove", "filesystem"}, ioDiscard{}, ioDiscard{}); err != nil {
		t.Fatal(err)
	}

	cfg := readMCPConfig(t)
	if _, ok := cfg.Servers["filesystem"]; ok {
		t.Fatalf("filesystem server still present")
	}
	if _, ok := cfg.Servers["sqlite"]; !ok {
		t.Fatalf("sqlite server should be preserved")
	}
}

func TestRunMCPRemoveMissingErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", dir)

	err := runMCP([]string{"remove", "missing"}, ioDiscard{}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestRunMCPMalformedJSON(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", dir)
	if err := os.WriteFile(config.MCPConfig(), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := runMCP([]string{"list"}, ioDiscard{}, ioDiscard{})
	if err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("err = %v, want parse error", err)
	}
}

func TestRunMCPPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", dir)

	var out bytes.Buffer
	if err := runMCP([]string{"path"}, &out, ioDiscard{}); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "mcp.json") + "\n"
	if out.String() != want {
		t.Fatalf("path output = %q, want %q", out.String(), want)
	}
}

func TestRunMCPValidate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RUNE_DIR", dir)
	writeMCPConfig(t, mcp.Config{Servers: map[string]mcp.ServerConfig{
		"filesystem": {Command: "npx"},
	}})

	var out bytes.Buffer
	if err := runMCP([]string{"validate"}, &out, ioDiscard{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "valid") {
		t.Fatalf("validate output = %q, want valid", out.String())
	}
}

func readMCPConfig(t *testing.T) mcp.Config {
	t.Helper()
	b, err := os.ReadFile(config.MCPConfig())
	if err != nil {
		t.Fatal(err)
	}
	var cfg mcp.Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg
}

func writeMCPConfig(t *testing.T, cfg mcp.Config) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(config.MCPConfig()), 0o755); err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(config.MCPConfig(), b, 0o644); err != nil {
		t.Fatal(err)
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
