package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/memory"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

func TestExtractMemoryUpdate(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	p := faux.New().Reply("- Use go test ./...\n").Done()
	s := session.New("test-model")
	a := New(p, tools.NewRegistry(), s, "")
	a.SetMemoryStore(memory.NewStore(t.TempDir(), 25000))
	updated, changed, err := a.ExtractMemoryUpdate(context.Background(), []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "Use go test ./..."}}}})
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !strings.Contains(updated, "go test ./...") {
		t.Fatalf("updated=%q changed=%v", updated, changed)
	}
}

func TestExtractMemoryUpdateNoChange(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	p := faux.New().Reply("NO_CHANGE").Done()
	s := session.New("test-model")
	a := New(p, tools.NewRegistry(), s, "")
	a.SetMemoryStore(memory.NewStore(t.TempDir(), 25000))
	updated, changed, err := a.ExtractMemoryUpdate(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed || updated != "" {
		t.Fatalf("updated=%q changed=%v", updated, changed)
	}
}
