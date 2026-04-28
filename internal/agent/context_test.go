package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadAgentsMD_WalksUp(t *testing.T) {
	base := t.TempDir()
	inner := filepath.Join(base, "a", "b", "c")
	_ = os.MkdirAll(inner, 0o755)
	_ = os.WriteFile(filepath.Join(base, "AGENTS.md"), []byte("ROOT"), 0o644)
	_ = os.WriteFile(filepath.Join(base, "a", "AGENTS.md"), []byte("MID"), 0o644)
	_ = os.WriteFile(filepath.Join(inner, "AGENTS.md"), []byte("LEAF"), 0o644)

	got := LoadAgentsMD(inner, base)

	if !strings.Contains(got, "ROOT") || !strings.Contains(got, "MID") || !strings.Contains(got, "LEAF") {
		t.Fatalf("missing layers: %q", got)
	}
	rootIdx := strings.Index(got, "ROOT")
	leafIdx := strings.Index(got, "LEAF")
	if rootIdx > leafIdx {
		t.Fatalf("ordering wrong: root=%d leaf=%d", rootIdx, leafIdx)
	}
}

func TestLoadAgentsMD_NoneFound(t *testing.T) {
	dir := t.TempDir()
	if got := LoadAgentsMD(dir, dir); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}
