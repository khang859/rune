package tui

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/khang859/rune/internal/tui/modal"
)

const gitStatusDiffMaxBytes = 40000

func loadGitStatusData(cwd string) (modal.GitStatusData, error) {
	if strings.TrimSpace(cwd) == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return modal.GitStatusData{}, fmt.Errorf("couldn't determine current working directory: %v", err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	root, err := runGitForStatus(ctx, cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return modal.GitStatusData{}, err
	}
	root = strings.TrimSpace(root)
	branchOut, _ := runGitForStatus(ctx, root, "status", "--short", "--branch")
	files := parseGitStatusPorcelain(branchOut)
	diffstat, _ := runGitForStatus(ctx, root, "diff", "--stat")
	stagedStat, _ := runGitForStatus(ctx, root, "diff", "--cached", "--stat")
	if strings.TrimSpace(stagedStat) != "" {
		if strings.TrimSpace(diffstat) != "" {
			diffstat = strings.TrimRight(stagedStat, "\n") + "\n" + diffstat
		} else {
			diffstat = stagedStat
		}
	}

	for i := range files {
		if files[i].IndexStatus != 0 && files[i].IndexStatus != ' ' && files[i].IndexStatus != '?' {
			files[i].StagedDiff, _ = boundedGitDiff(ctx, root, gitStatusDiffMaxBytes, true, files[i].Path)
		}
		if files[i].WorkStatus != 0 && files[i].WorkStatus != ' ' && files[i].WorkStatus != '?' {
			files[i].WorkDiff, _ = boundedGitDiff(ctx, root, gitStatusDiffMaxBytes, false, files[i].Path)
		}
	}

	return modal.GitStatusData{
		Repo:     root,
		Branch:   parseGitBranchLine(branchOut),
		Files:    files,
		Diffstat: diffstat,
	}, nil
}

func parseGitBranchLine(status string) string {
	for _, line := range strings.Split(status, "\n") {
		if strings.HasPrefix(line, "## ") {
			return strings.TrimSpace(strings.TrimPrefix(line, "## "))
		}
	}
	return ""
}

func parseGitStatusPorcelain(status string) []modal.GitFileChange {
	var files []modal.GitFileChange
	for _, line := range strings.Split(status, "\n") {
		if strings.TrimSpace(line) == "" || strings.HasPrefix(line, "## ") {
			continue
		}
		if len(line) < 4 {
			continue
		}
		file := modal.GitFileChange{IndexStatus: line[0], WorkStatus: line[1]}
		pathPart := strings.TrimSpace(line[3:])
		if strings.Contains(pathPart, " -> ") {
			parts := strings.SplitN(pathPart, " -> ", 2)
			file.OriginalPath = strings.TrimSpace(parts[0])
			file.Path = strings.TrimSpace(parts[1])
		} else {
			file.Path = pathPart
		}
		files = append(files, file)
	}
	return files
}

func boundedGitDiff(ctx context.Context, repo string, maxBytes int, staged bool, path string) (string, error) {
	args := []string{"diff"}
	if staged {
		args = append(args, "--cached")
	}
	args = append(args, "--", filepath.ToSlash(path))
	out, err := runGitForStatus(ctx, repo, args...)
	if err != nil {
		return "", err
	}
	if len(out) > maxBytes {
		out = out[:maxBytes] + fmt.Sprintf("\n\n[truncated git diff output to %d bytes]", maxBytes)
	}
	return out, nil
}

func runGitForStatus(ctx context.Context, repo string, args ...string) (string, error) {
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
