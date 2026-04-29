package agent

import (
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBasePrompt_NonEmpty(t *testing.T) {
	if got := BasePrompt(); got == "" {
		t.Fatal("BasePrompt() returned empty string")
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
