package agent

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/codeindex"
	"github.com/khang859/rune/internal/session"
)

func TestBasePrompt_IncludesApprovalGuidance(t *testing.T) {
	got := BasePrompt()
	if got == "" {
		t.Fatal("BasePrompt() returned empty string")
	}
	for _, want := range []string{
		"You are rune, a coding agent.",
		"Default behavior:",
		"small, obvious change",
		"first inspect the relevant code and clarify the goal before editing files",
		"ask the user exactly one clarifying question at a time",
		"include your recommended answer and brief rationale",
		"present a concise implementation plan",
		"wait for the user's approval before proceeding",
		"Preserve user work",
		"Validate with targeted tests or checks when practical",
		"Prefer `read`, `write`, and `edit` over `bash`",
		"Use AST/code-index tools for codebase navigation when available",
		"code_find_symbol",
		"Use literal file search for exact strings",
		"do not call get_subagent_result immediately after starting it",
		"Cite source URLs when relying on web information",
		"summarize what changed, where, and how it was validated",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("BasePrompt() missing %q in: %q", want, got)
		}
	}
}

func TestPlanModePrompt_IncludesSafetyGuidance(t *testing.T) {
	got := PlanModePrompt()
	for _, want := range []string{
		"You are in PLAN MODE",
		"Do not edit",
		"run shell commands",
		"mutating tools",
		"Use only read-only tools",
		"If a question can be answered by reading/searching/inspecting the codebase, do that instead of asking the user",
		"Walk the design tree systematically",
		"Ask exactly one question at a time",
		"Include your recommended answer and a brief rationale",
		"Stop and wait for the user's answer",
		"produce a concise plan",
		"Approval request",
		"approve before implementation",
		"Act Mode",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("PlanModePrompt() missing %q in %q", want, got)
		}
	}
}

func TestRuntimeContext_ContainsExpectedFields(t *testing.T) {
	got := RuntimeContext()

	if !strings.HasPrefix(got, "<system-context>") || !strings.HasSuffix(got, "</system-context>") {
		t.Fatalf("missing wrapper tags: %q", got)
	}

	today := time.Now().Format("2006-01-02")
	if !strings.Contains(got, "date: "+today) {
		t.Errorf("missing today's date %q in: %s", today, got)
	}

	for _, key := range []string{"cwd:", "os:", "shell:", "user:"} {
		if !strings.Contains(got, key) {
			t.Errorf("missing %q in: %s", key, got)
		}
	}

	osArch := runtime.GOOS + "/" + runtime.GOARCH
	if !strings.Contains(got, osArch) {
		t.Errorf("missing %q in: %s", osArch, got)
	}
}

func TestBuildRepoMapBlockDisabled(t *testing.T) {
	got := BuildRepoMapBlock(nil, nil, false, 1000)
	if got != "" {
		t.Errorf("disabled should return empty, got %q", got)
	}
}

func TestBuildRepoMapBlockWrapsOutput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(`package a
func Helper() {}
`), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte(`package a
func Caller() { Helper() }
`), 0o644)
	idx, err := codeindex.NewBuilder().Build(context.Background(), codeindex.BuildOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New("m")
	sess.RecordFileRead(filepath.Join(dir, "b.go"))
	sess.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "Look at Helper"}}})

	got := BuildRepoMapBlock(sess, idx, true, 1000)
	if got == "" {
		t.Fatal("expected non-empty repo map block")
	}
	if !strings.HasPrefix(got, "<repo_map>\n") || !strings.HasSuffix(got, "</repo_map>") {
		t.Errorf("missing wrapping tags:\n%s", got)
	}
}
