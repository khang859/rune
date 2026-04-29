package agent

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBasePrompt_IncludesApprovalGuidance(t *testing.T) {
	got := BasePrompt()
	if got == "" {
		t.Fatal("BasePrompt() returned empty string")
	}
	for _, want := range []string{
		"You are rune, a coding agent.",
		"present a concise plan",
		"wait for the user's approval before editing files",
		"Prefer `read`, `write`, and `edit` over `bash`",
		"do not call get_subagent_result immediately after starting it",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("BasePrompt() missing %q in: %q", want, got)
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
