package tui

import (
	"bytes"
	"os/exec"
	"strings"
)

func currentGitBranch(cwd string) string {
	if cwd == "" {
		return ""
	}

	// symbolic-ref works for both normal repositories and newly initialized
	// repositories whose current branch is still unborn (no commits yet).
	cmd := exec.Command("git", "-C", cwd, "symbolic-ref", "--quiet", "--short", "HEAD")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
