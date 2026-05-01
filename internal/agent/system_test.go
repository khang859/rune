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
