package tui

import "testing"

func TestParseGitStatusPorcelain(t *testing.T) {
	status := "## main...origin/main [ahead 1]\n M internal/tui/root.go\nM  staged.go\n?? new.txt\nR  old.go -> new.go\n"
	files := parseGitStatusPorcelain(status)
	if len(files) != 4 {
		t.Fatalf("len = %d, want 4: %#v", len(files), files)
	}
	if files[0].Path != "internal/tui/root.go" || files[0].IndexStatus != ' ' || files[0].WorkStatus != 'M' {
		t.Fatalf("first file = %#v", files[0])
	}
	if files[1].Path != "staged.go" || files[1].IndexStatus != 'M' || files[1].WorkStatus != ' ' {
		t.Fatalf("second file = %#v", files[1])
	}
	if files[2].Path != "new.txt" || files[2].IndexStatus != '?' || files[2].WorkStatus != '?' {
		t.Fatalf("third file = %#v", files[2])
	}
	if files[3].OriginalPath != "old.go" || files[3].Path != "new.go" || files[3].IndexStatus != 'R' {
		t.Fatalf("rename file = %#v", files[3])
	}
}

func TestParseGitBranchLine(t *testing.T) {
	got := parseGitBranchLine("## main...origin/main [ahead 1]\n M file.go\n")
	if got != "main...origin/main [ahead 1]" {
		t.Fatalf("branch = %q", got)
	}
}
