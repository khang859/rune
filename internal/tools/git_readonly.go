package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

const defaultGitDiffMaxBytes = 60000

type GitStatus struct{}

type GitDiff struct{}

func (GitStatus) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "git_status",
		Description: "Inspect git repository status without running a shell. Runs a fixed read-only git status command.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "path":{"type":"string","description":"Repository or subdirectory path. Defaults to the current working directory."}
            }
        }`),
	}
}

func (GitStatus) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"path"?: string}.`, err), IsError: true}, nil
	}
	repo, err := resolveGitPath(a.Path)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	out, err := runGitReadonly(ctx, repo, "status", "--short", "--branch")
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(out) == "" {
		out = "(clean)"
	}
	return Result{Output: out}, nil
}

func (GitDiff) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "git_diff",
		Description: "Inspect git diffs without running a shell. Runs fixed read-only git diff commands with optional path filtering, staged diff, stat output, and bounded output size.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "repo":{"type":"string","description":"Repository or subdirectory path. Defaults to the current working directory."},
                "path":{"type":"string","description":"Optional file or directory pathspec to filter the diff."},
                "staged":{"type":"boolean","description":"Show staged changes using git diff --cached."},
                "stat":{"type":"boolean","description":"Show diffstat instead of the full patch."},
                "max_bytes":{"type":"integer","description":"Maximum output bytes. Defaults to 60000."}
            }
        }`),
	}
}

func (GitDiff) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Repo     string `json:"repo"`
		Path     string `json:"path"`
		Staged   bool   `json:"staged"`
		Stat     bool   `json:"stat"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"repo"?: string, "path"?: string, "staged"?: bool, "stat"?: bool, "max_bytes"?: int}.`, err), IsError: true}, nil
	}
	if a.MaxBytes < 0 {
		return Result{Output: "max_bytes must be non-negative", IsError: true}, nil
	}
	maxBytes := a.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultGitDiffMaxBytes
	}
	repo, err := resolveGitPath(a.Repo)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	gitArgs := []string{"diff"}
	if a.Staged {
		gitArgs = append(gitArgs, "--cached")
	}
	if a.Stat {
		gitArgs = append(gitArgs, "--stat")
	}
	if strings.TrimSpace(a.Path) != "" {
		gitArgs = append(gitArgs, "--", filepath.ToSlash(strings.TrimSpace(a.Path)))
	}
	out, err := runGitReadonly(ctx, repo, gitArgs...)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	if strings.TrimSpace(out) == "" {
		out = "(no diff)"
	}
	if len(out) > maxBytes {
		out = out[:maxBytes] + fmt.Sprintf("\n\n[truncated git diff output to %d bytes. Use stat=true, path, or increase max_bytes to inspect more.]", maxBytes)
	}
	return Result{Output: out}, nil
}

func resolveGitPath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("couldn't determine current working directory: %v", err)
		}
		return wd, nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("couldn't resolve %s: %v", p, err)
	}
	return abs, nil
}

func runGitReadonly(ctx context.Context, repo string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git executable not found")
	}
	argv := append([]string{"-C", repo}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(buf.String())
		if ctx.Err() != nil {
			if out != "" {
				return "", fmt.Errorf("%s\n(canceled)", out)
			}
			return "", ctx.Err()
		}
		if out != "" {
			return "", fmt.Errorf("git %s failed: %s", strings.Join(args, " "), out)
		}
		return "", fmt.Errorf("git %s failed: %v", strings.Join(args, " "), err)
	}
	return buf.String(), nil
}
