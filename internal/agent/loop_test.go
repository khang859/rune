package agent

import (
	"context"
	"testing"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

func userMsg(s string) ai.Message {
	return ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: s}}}
}

func collect(t *testing.T, ch <-chan Event) []Event {
	t.Helper()
	var out []Event
	for e := range ch {
		out = append(out, e)
	}
	return out
}

func TestRun_TextOnlyTurn(t *testing.T) {
	f := faux.New().Reply("hi there").Done()
	s := session.New("gpt-5")
	a := New(f, tools.NewRegistry(), s, "system")
	evs := collect(t, a.Run(context.Background(), userMsg("hello")))

	var sawText, sawDone bool
	for _, e := range evs {
		switch v := e.(type) {
		case AssistantText:
			if v.Delta == "hi there" {
				sawText = true
			}
		case TurnDone:
			if v.Reason == "stop" {
				sawDone = true
			}
		}
	}
	if !sawText {
		t.Fatal("missing AssistantText")
	}
	if !sawDone {
		t.Fatal("missing TurnDone")
	}
	// Session must contain user msg + assistant msg.
	if got := len(s.PathToActive()); got != 2 {
		t.Fatalf("path len = %d", got)
	}
}
