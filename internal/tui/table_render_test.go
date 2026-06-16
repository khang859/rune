package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
)

func TestMessages_TableWrapsWithinWidthWithoutBreakingBorders(t *testing.T) {
	md := `| Option | Task | Why |
|---|---|---|
| A | Review/land PR #8 (auto-memory) | It's the only open PR and the oldest work in flight. |
| B | Implement issue #44 (Kimi tool-call token leak) | It's a real bug surfaced in a session log, tightly scoped to internal/ai/openrouter/sse.go. |
| C | Implement issue #45 (reasoning feedback-loop hardening) | Defense-in-depth after #46; touches loop persistence and maybe provider routing guidance. |
`
	width := 80
	m := NewMessages(width)
	m.AppendUser(md)
	out := m.Render(DefaultStyles(), false, false, time.Time{})

	lines := strings.Split(out, "\n")
	inTable := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Option") || strings.HasPrefix(trimmed, "|") {
			inTable = true
		}
		if inTable && trimmed != "" {
			if ansi.StringWidth(line) > width {
				t.Errorf("table row %d exceeds message width %d: %q", i, width, line)
			}
			if !strings.Contains(line, "|") {
				t.Errorf("table row %d appears broken (no pipe separator): %q", i, line)
			}
		}
	}
}
