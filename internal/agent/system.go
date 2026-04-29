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
	return strings.Join([]string{
		"You are rune, a coding agent. Use the available tools.",
		"When asked to implement a new feature or make a non-trivial change, first explore the codebase to understand the request, present a concise plan, and wait for the user's approval before editing files or running implementation steps.",
		"For current or unknown web information, use web_search first to discover relevant sources, then use web_fetch only on search results or URLs explicitly provided by the user. Do not guess URLs. Cite source URLs when relying on web information.",
	}, "\n")
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
