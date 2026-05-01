package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGHValidateReadonlyAllowsExpectedCommands(t *testing.T) {
	cases := [][]string{
		{"issue", "list"},
		{"issue", "view", "123", "--json", "title,body"},
		{"pr", "list", "--repo", "owner/repo"},
		{"pr", "view", "123"},
		{"pr", "diff", "123"},
		{"repo", "view", "owner/repo"},
		{"search", "issues", "bug"},
		{"release", "list"},
		{"run", "view", "123"},
		{"workflow", "list"},
		{"api", "repos/owner/repo/pulls/1"},
		{"api", "--method", "GET", "repos/owner/repo/issues"},
		{"api", "-XGET", "repos/owner/repo/issues"},
	}
	for _, tc := range cases {
		if err := validateGHReadonlyArgs(tc); err != nil {
			t.Fatalf("validateGHReadonlyArgs(%v) returned error: %v", tc, err)
		}
	}
}

func TestGHValidateReadonlyRejectsMutatingCommands(t *testing.T) {
	cases := [][]string{
		{"issue", "create"},
		{"issue", "edit", "123"},
		{"pr", "merge", "123"},
		{"repo", "delete", "owner/repo"},
		{"run", "rerun", "123"},
		{"api", "--method", "POST", "repos/owner/repo/issues"},
		{"api", "-XPATCH", "repos/owner/repo/issues/1"},
		{"api", "repos/owner/repo/issues", "--field", "title=x"},
		{"api", "repos/owner/repo/issues", "-ftitle=x"},
		{"api", "graphql"},
		{"pr", "view", "--web", "123"},
	}
	for _, tc := range cases {
		if err := validateGHReadonlyArgs(tc); err == nil {
			t.Fatalf("validateGHReadonlyArgs(%v) returned nil, want error", tc)
		}
	}
}

func TestGHRunUsesDirectExecutableAndTruncates(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("test helper script uses sh syntax")
	}
	dir := t.TempDir()
	ghPath := filepath.Join(dir, "gh")
	if err := os.WriteFile(ghPath, []byte("#!/bin/sh\nprintf '0123456789abcdef'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", dir+string(os.PathListSeparator)+oldPath)

	res, err := (GH{}).Run(context.Background(), json.RawMessage(`{"args":["issue","list"],"max_bytes":8}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("result error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "01234567") || !strings.Contains(res.Output, "truncated gh output to 8 bytes") {
		t.Fatalf("unexpected output: %q", res.Output)
	}
}

func TestGHRunRejectsDeniedCommandBeforeExecution(t *testing.T) {
	res, err := (GH{}).Run(context.Background(), json.RawMessage(`{"args":["issue","create"]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Output, "not allowed in read-only mode") {
		t.Fatalf("unexpected result: %#v", res)
	}
}
