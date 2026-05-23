package repomap

import (
	"strings"
	"testing"

	"github.com/khang859/rune/internal/codeindex"
)

func sym(name, file, sig string, line uint) *codeindex.Symbol {
	return &codeindex.Symbol{
		ID:        name + "@" + file,
		Name:      name,
		File:      file,
		Signature: sig,
		StartLine: line,
		Kind:      codeindex.SymbolFunction,
	}
}

func TestRenderBudgetedGroupsByFile(t *testing.T) {
	items := []RenderItem{
		{File: "internal/agent/loop.go", Symbol: sym("Run", "internal/agent/loop.go", "func (a *Agent) Run(...) error", 17)},
		{File: "internal/agent/loop.go", Symbol: sym("runTurn", "internal/agent/loop.go", "func (a *Agent) runTurn(...)", 33)},
		{File: "internal/session/session.go", Symbol: sym("Append", "internal/session/session.go", "func (s *Session) Append(m ai.Message)", 10)},
	}
	out := RenderBudgeted(items, 10000)
	if !strings.Contains(out, "internal/agent/loop.go:") {
		t.Errorf("missing file header for loop.go:\n%s", out)
	}
	if !strings.Contains(out, "func (a *Agent) Run") {
		t.Errorf("missing Run signature:\n%s", out)
	}
	if strings.Count(out, "internal/agent/loop.go:") != 1 {
		t.Errorf("file header should appear once per file, got:\n%s", out)
	}
}

func TestRenderBudgetedRespectsBudget(t *testing.T) {
	items := make([]RenderItem, 200)
	for i := range items {
		items[i] = RenderItem{
			File:   "f.go",
			Symbol: sym("S"+string(rune('a'+i%26)), "f.go", "func S() {}", uint(i)),
		}
	}
	out := RenderBudgeted(items, 200) // 200 tokens ~= 800 chars
	if estimateTokens(out) > 230 {     // within 15%
		t.Errorf("output exceeded budget: tokens=%d, budget=200", estimateTokens(out))
	}
}

func TestRenderBudgetedEmpty(t *testing.T) {
	if RenderBudgeted(nil, 1000) != "" {
		t.Error("empty input should render empty string")
	}
}
