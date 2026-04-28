// internal/session/compact_test.go
package session

import (
	"context"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestCompact_ReplacesPreCutWithSummary(t *testing.T) {
	s := New("gpt-5")
	s.Append(userMsg("u1"))
	s.Append(asstMsg("a1"))
	s.Append(userMsg("u2"))
	s.Append(asstMsg("a2"))

	summarizer := func(ctx context.Context, msgs []ai.Message, instructions string) (string, error) {
		var b strings.Builder
		for _, m := range msgs {
			for _, c := range m.Content {
				if tx, ok := c.(ai.TextBlock); ok {
					b.WriteString(tx.Text + " ")
				}
			}
		}
		return "SUMMARY: " + strings.TrimSpace(b.String()), nil
	}

	if err := s.Compact(context.Background(), "be brief", summarizer); err != nil {
		t.Fatal(err)
	}

	path := s.PathToActive()
	// Expect: [summary, u2, a2]
	if len(path) != 3 {
		t.Fatalf("path len after compact = %d", len(path))
	}
	if !strings.Contains(path[0].Content[0].(ai.TextBlock).Text, "SUMMARY") {
		t.Fatalf("first msg not a summary: %#v", path[0])
	}
	if path[1].Content[0].(ai.TextBlock).Text != "u2" {
		t.Fatalf("second msg should be u2: %#v", path[1])
	}
	if path[2].Content[0].(ai.TextBlock).Text != "a2" {
		t.Fatalf("third msg should be a2: %#v", path[2])
	}
}

func TestCompact_NoCutPoint_ReturnsNoOp(t *testing.T) {
	s := New("gpt-5")
	summarizer := func(ctx context.Context, msgs []ai.Message, _ string) (string, error) { return "x", nil }
	if err := s.Compact(context.Background(), "", summarizer); err != nil {
		t.Fatal(err)
	}
	if got := len(s.PathToActive()); got != 0 {
		t.Fatalf("path len = %d", got)
	}
}
