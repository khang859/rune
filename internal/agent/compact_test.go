// internal/agent/compact_test.go
package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

func TestCompact_UsesProviderForSummary(t *testing.T) {
	s := session.New("gpt-5")
	s.Append(userMsg("u1"))
	s.Append(asstMsg("a1"))
	s.Append(userMsg("u2"))

	f := faux.New().Reply("here is a summary").Done()
	a := New(f, tools.NewRegistry(), s, "")
	if err := a.Compact(context.Background(), ""); err != nil {
		t.Fatal(err)
	}
	path := s.PathToActive()
	if len(path) < 2 {
		t.Fatalf("path len = %d", len(path))
	}
	if !strings.Contains(path[0].Content[0].(ai.TextBlock).Text, "summary") {
		t.Fatalf("first msg not summary: %#v", path[0])
	}
}

// reuse helpers from loop_test.go (same package)
func asstMsg(text string) ai.Message {
	return ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}}
}
