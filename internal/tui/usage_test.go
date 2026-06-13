package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/khang859/rune/internal/session"
)

func TestFormatUsageStats_FullAndEmpty(t *testing.T) {
	full := formatUsageStats(session.UsageStats{
		Provider: "codex", Model: "gpt-5", Effort: "high",
		Created: time.Now(), Updated: time.Now(),
		Turns: 2, Input: 300, Output: 100, CacheRead: 40, DurationMs: 4000,
		SubagentCount: 1, SubagentInput: 500, SubagentOutput: 80,
	})
	joined := strings.Join(full, "\n")
	for _, want := range []string{"model=gpt-5", "effort=high", "turns=2", "total 440", "subagents=1", "created"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q in:\n%s", want, joined)
		}
	}

	// Zero turns must not panic on avg division and effort falls back to em dash.
	empty := formatUsageStats(session.UsageStats{Model: "gpt-5"})
	ej := strings.Join(empty, "\n")
	if !strings.Contains(ej, "effort=—") || !strings.Contains(ej, "turns=0") {
		t.Fatalf("unexpected empty output:\n%s", ej)
	}
	if strings.Contains(ej, "subagents=") {
		t.Fatalf("should not show subagents line when none:\n%s", ej)
	}
}
