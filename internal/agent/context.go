package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// LoadAgentsMD walks from start up to (and including) stop, collecting AGENTS.md files.
// Returns them concatenated, root-first (broadest scope first).
func LoadAgentsMD(start, stop string) string {
	var collected []string
	cur, _ := filepath.Abs(start)
	stopAbs, _ := filepath.Abs(stop)
	for {
		p := filepath.Join(cur, "AGENTS.md")
		if b, err := os.ReadFile(p); err == nil {
			collected = append([]string{string(b)}, collected...)
		}
		if cur == stopAbs {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return strings.Join(collected, "\n\n---\n\n")
}
