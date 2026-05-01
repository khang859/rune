package modal

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestGitStatusViewShowsFilesAndDiffstat(t *testing.T) {
	md := NewGitStatus(GitStatusData{
		Repo:   "/tmp/repo",
		Branch: "main...origin/main [ahead 1]",
		Files: []GitFileChange{
			{Path: "internal/tui/root.go", WorkStatus: 'M', WorkDiff: "diff --git a/internal/tui/root.go b/internal/tui/root.go\n+hello\n"},
			{Path: "new.txt", IndexStatus: '?', WorkStatus: '?'},
		},
		Diffstat: " internal/tui/root.go | 2 +-\n 1 file changed, 1 insertion(+), 1 deletion(-)",
	})

	out := md.View(100, 30)
	for _, want := range []string{"Git Status", "main...origin/main", "internal/tui/root.go", "new.txt", "Diffstat", "1 file changed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view missing %q:\n%s", want, out)
		}
	}
}

func TestGitStatusEnterShowsDiff(t *testing.T) {
	md := NewGitStatus(GitStatusData{
		Branch: "main",
		Files:  []GitFileChange{{Path: "file.go", WorkStatus: 'M', WorkDiff: "diff --git a/file.go b/file.go\n+changed\n"}},
	})
	m, cmd := md.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatal("unexpected command")
	}
	out := m.View(80, 24)
	for _, want := range []string{"Git Diff", "file.go", "+changed"} {
		if !strings.Contains(out, want) {
			t.Fatalf("detail missing %q:\n%s", want, out)
		}
	}
}

func TestGitStatusUntrackedDetailMessage(t *testing.T) {
	md := NewGitStatus(GitStatusData{Files: []GitFileChange{{Path: "new.txt", IndexStatus: '?', WorkStatus: '?'}}})
	m, _ := md.Update(tea.KeyMsg{Type: tea.KeyEnter})
	out := m.View(80, 24)
	if !strings.Contains(out, "Untracked file") {
		t.Fatalf("missing untracked message:\n%s", out)
	}
}
