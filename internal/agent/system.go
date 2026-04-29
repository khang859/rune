package agent

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

// BasePrompt is the default rune system prompt. Centralized here so cmd/rune
// and internal/tui share one source of truth.
func BasePrompt() string {
	return "You are rune, a coding agent. Use the available tools."
}

// RuntimeContext returns a <system-context> block describing the runtime:
// date/time, cwd, os/arch, shell, user. Called per turn from loop.go so
// the date doesn't drift in long sessions.
func RuntimeContext() string {
	now := time.Now()
	zone, _ := now.Zone()

	cwd, err := os.Getwd()
	if err != nil {
		cwd = "(unknown)"
	}

	var b strings.Builder
	b.WriteString("<system-context>\n")
	fmt.Fprintf(&b, "date: %s %s\n", now.Format("2006-01-02 15:04"), zone)
	fmt.Fprintf(&b, "cwd: %s\n", cwd)
	fmt.Fprintf(&b, "os: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(&b, "shell: %s\n", os.Getenv("SHELL"))
	fmt.Fprintf(&b, "user: %s\n", os.Getenv("USER"))
	b.WriteString("</system-context>")
	return b.String()
}
