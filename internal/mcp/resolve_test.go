package mcp

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMergeConfig_LocalOverridesAndExtends(t *testing.T) {
	global := Config{Servers: map[string]ServerConfig{
		"a": {Command: "global-a"},
		"b": {Command: "global-b"},
	}}
	local := Config{Servers: map[string]ServerConfig{
		"b": {Command: "local-b"},
		"c": {Command: "local-c"},
	}}

	got := MergeConfig(global, local)

	if len(got.Servers) != 3 {
		t.Fatalf("merged servers = %d, want 3: %#v", len(got.Servers), got.Servers)
	}
	if got.Servers["a"].Command != "global-a" {
		t.Errorf("a = %q, want global-a", got.Servers["a"].Command)
	}
	if got.Servers["b"].Command != "local-b" {
		t.Errorf("b = %q, want local-b (local wins)", got.Servers["b"].Command)
	}
	if got.Servers["c"].Command != "local-c" {
		t.Errorf("c = %q, want local-c", got.Servers["c"].Command)
	}
}

func TestMergeConfig_DoesNotMutateInputs(t *testing.T) {
	global := Config{Servers: map[string]ServerConfig{"a": {Command: "global-a"}}}
	local := Config{Servers: map[string]ServerConfig{"a": {Command: "local-a"}}}

	_ = MergeConfig(global, local)

	if global.Servers["a"].Command != "global-a" {
		t.Errorf("global mutated: %q", global.Servers["a"].Command)
	}
}

func writeConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolveConfig_EnvOverrideWinsAlone(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global.json")
	local := filepath.Join(dir, "local.json")
	env := filepath.Join(dir, "env.json")
	writeConfig(t, global, `{"servers":{"g":{"command":"g"}}}`)
	writeConfig(t, local, `{"servers":{"l":{"command":"l"}}}`)
	writeConfig(t, env, `{"servers":{"e":{"command":"e"}}}`)

	cfg, err := ResolveConfig(global, local, env)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 1 || cfg.Servers["e"].Command != "e" {
		t.Fatalf("env override should be used alone: %#v", cfg.Servers)
	}
}

func TestResolveConfig_MergesLocalOverGlobal(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global.json")
	local := filepath.Join(dir, "local.json")
	writeConfig(t, global, `{"servers":{"g":{"command":"g"},"shared":{"command":"global"}}}`)
	writeConfig(t, local, `{"servers":{"l":{"command":"l"},"shared":{"command":"local"}}}`)

	cfg, err := ResolveConfig(global, local, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 3 {
		t.Fatalf("merged servers = %d, want 3: %#v", len(cfg.Servers), cfg.Servers)
	}
	if cfg.Servers["shared"].Command != "local" {
		t.Errorf("shared = %q, want local", cfg.Servers["shared"].Command)
	}
}

func TestResolveConfig_BothAbsentReturnsEmpty(t *testing.T) {
	dir := t.TempDir()
	cfg, err := ResolveConfig(filepath.Join(dir, "g.json"), filepath.Join(dir, "l.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 0 {
		t.Fatalf("want empty, got %#v", cfg.Servers)
	}
}

func TestResolveConfig_GlobalOnlyWhenLocalAbsent(t *testing.T) {
	dir := t.TempDir()
	global := filepath.Join(dir, "global.json")
	writeConfig(t, global, `{"servers":{"g":{"command":"g"}}}`)

	cfg, err := ResolveConfig(global, filepath.Join(dir, "missing.json"), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Servers) != 1 || cfg.Servers["g"].Command != "g" {
		t.Fatalf("want global only: %#v", cfg.Servers)
	}
}
