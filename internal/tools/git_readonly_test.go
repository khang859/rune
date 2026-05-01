package tools

import (
	"context"
	"encoding/json"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestGitStatusShowsBranchAndChanges(t *testing.T) {
	dir := initTestGitRepo(t)
	writeTestFile(t, dir, "tracked.txt", "changed\n")
	writeTestFile(t, dir, "untracked.txt", "new\n")

	res, err := (GitStatus{}).Run(context.Background(), json.RawMessage(`{"path":`+quoteJSON(dir)+`}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("result error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "##") || !strings.Contains(res.Output, " M tracked.txt") || !strings.Contains(res.Output, "?? untracked.txt") {
		t.Fatalf("unexpected git status output:\n%s", res.Output)
	}
}

func TestGitDiffShowsUnstagedAndStagedChanges(t *testing.T) {
	dir := initTestGitRepo(t)
	writeTestFile(t, dir, "tracked.txt", "changed\n")

	res, err := (GitDiff{}).Run(context.Background(), json.RawMessage(`{"repo":`+quoteJSON(dir)+`}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unstaged result error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "diff --git") || !strings.Contains(res.Output, "+changed") {
		t.Fatalf("unexpected unstaged diff:\n%s", res.Output)
	}

	runGitTestCmd(t, dir, "add", "tracked.txt")
	res, err = (GitDiff{}).Run(context.Background(), json.RawMessage(`{"repo":`+quoteJSON(dir)+`,"staged":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("staged result error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "diff --git") || !strings.Contains(res.Output, "+changed") {
		t.Fatalf("unexpected staged diff:\n%s", res.Output)
	}
}

func TestGitDiffStatAndPathFilter(t *testing.T) {
	dir := initTestGitRepo(t)
	writeTestFile(t, dir, "tracked.txt", "changed\n")
	writeTestFile(t, dir, "other.txt", "other\n")
	runGitTestCmd(t, dir, "add", "other.txt")
	runGitTestCmd(t, dir, "commit", "-m", "add other")
	writeTestFile(t, dir, "other.txt", "other changed\n")

	res, err := (GitDiff{}).Run(context.Background(), json.RawMessage(`{"repo":`+quoteJSON(dir)+`,"path":"tracked.txt","stat":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("result error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "tracked.txt") || strings.Contains(res.Output, "other.txt") {
		t.Fatalf("unexpected filtered stat:\n%s", res.Output)
	}
}

func TestGitDiffTruncatesOutput(t *testing.T) {
	dir := initTestGitRepo(t)
	writeTestFile(t, dir, "tracked.txt", "changed line one\nchanged line two\n")

	res, err := (GitDiff{}).Run(context.Background(), json.RawMessage(`{"repo":`+quoteJSON(dir)+`,"max_bytes":20}`))
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("result error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "truncated git diff output to 20 bytes") {
		t.Fatalf("missing truncation footer:\n%s", res.Output)
	}
}

func TestPlanModeAllowsGitReadonlyToolsButDeniesBash(t *testing.T) {
	r := NewRegistry()
	RegisterBuiltins(r, BuiltinOptions{})
	r.SetPermissionMode(PermissionModePlan)

	var names []string
	for _, spec := range r.Specs() {
		names = append(names, spec.Name)
	}
	for _, want := range []string{"git_status", "git_diff", "gh"} {
		if !containsString(names, want) {
			t.Fatalf("plan specs missing %q in %v", want, names)
		}
	}
	if containsString(names, "bash") {
		t.Fatalf("plan specs exposed bash: %v", names)
	}

	res, err := r.Run(context.Background(), ai.ToolCall{Name: "bash", Args: json.RawMessage(`{"command":"git status"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Output, "disabled in Plan Mode") {
		t.Fatalf("bash should be denied in plan mode: %#v", res)
	}
}

func initTestGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	dir := t.TempDir()
	runGitTestCmd(t, dir, "init")
	runGitTestCmd(t, dir, "config", "user.email", "test@example.com")
	runGitTestCmd(t, dir, "config", "user.name", "Test User")
	writeTestFile(t, dir, "tracked.txt", "original\n")
	runGitTestCmd(t, dir, "add", "tracked.txt")
	runGitTestCmd(t, dir, "commit", "-m", "initial")
	return dir
}

func runGitTestCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", filepath.Clean(dir)}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
