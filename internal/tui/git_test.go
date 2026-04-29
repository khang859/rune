package tui

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestCurrentGitBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "main")
	if got := currentGitBranch(dir); got != "main" {
		t.Fatalf("currentGitBranch() = %q, want main", got)
	}
}

func TestCurrentGitBranchOutsideRepo(t *testing.T) {
	if got := currentGitBranch(t.TempDir()); got != "" {
		t.Fatalf("currentGitBranch() = %q, want empty", got)
	}
}

func TestCurrentGitBranchInSubdirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", "feature")
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := currentGitBranch(subdir); got != "feature" {
		t.Fatalf("currentGitBranch() = %q, want feature", got)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
