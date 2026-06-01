package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

// With --require-tool set, a model that ends a turn with plain text is nudged
// to keep going. Once it calls the required tool, the next text-only turn ends
// the run cleanly (no incomplete signal).
func TestRun_RequireTool_NudgesUntilToolCalled(t *testing.T) {
	f := faux.New().
		Reply("I'll implement this. Should I proceed?").Done(). // text only → nudge
		CallTool("kanban_complete", `{"summary":"done"}`).Done().
		Reply("all set").Done() // text only, but required tool already called → stop
	s := session.New("gpt-5")
	reg := tools.NewRegistry()
	reg.Register(stubTool{name: "kanban_complete"})
	a := New(f, reg, s, "system")
	a.SetRequireTools(map[string]bool{"kanban_complete": true})

	evs := collect(t, a.Run(context.Background(), userMsg("do the task")))

	var nudges int
	var toolRan, sawStop, sawIncomplete bool
	for _, e := range evs {
		switch v := e.(type) {
		case RequiredToolPending:
			nudges++
		case ToolFinished:
			if v.Call.Name == "kanban_complete" {
				toolRan = true
			}
		case TurnDone:
			switch v.Reason {
			case ReasonIncompleteRequiredTool:
				sawIncomplete = true
			case "stop":
				sawStop = true
			}
		}
	}
	if nudges != 1 {
		t.Fatalf("nudges=%d, want 1", nudges)
	}
	if !toolRan {
		t.Fatal("required tool never ran")
	}
	if sawIncomplete {
		t.Fatal("unexpected incomplete after required tool was called")
	}
	if !sawStop {
		t.Fatal("expected clean TurnDone(stop)")
	}
}

// A model that refuses to ever call the required tool is nudged up to the cap,
// then the run ends with the incomplete signal (mapped to exit 3 upstream).
func TestRun_RequireTool_IncompleteAfterCap(t *testing.T) {
	f := faux.New()
	for i := 0; i < maxHeadlessNudges+1; i++ {
		f = f.Reply("still thinking — should I proceed?").Done()
	}
	s := session.New("gpt-5")
	a := New(f, tools.NewRegistry(), s, "system")
	a.SetRequireTools(map[string]bool{"kanban_complete": true, "kanban_block": true})

	evs := collect(t, a.Run(context.Background(), userMsg("do the task")))

	var nudges int
	var sawIncomplete, sawStop bool
	var lastAttempt int
	for _, e := range evs {
		switch v := e.(type) {
		case RequiredToolPending:
			nudges++
			lastAttempt = v.Attempt
		case TurnDone:
			switch v.Reason {
			case ReasonIncompleteRequiredTool:
				sawIncomplete = true
			case "stop":
				sawStop = true
			}
		}
	}
	if nudges != maxHeadlessNudges {
		t.Fatalf("nudges=%d, want %d", nudges, maxHeadlessNudges)
	}
	if lastAttempt != maxHeadlessNudges {
		t.Fatalf("last attempt=%d, want %d", lastAttempt, maxHeadlessNudges)
	}
	if !sawIncomplete {
		t.Fatal("expected incomplete TurnDone after exhausting nudge budget")
	}
	if sawStop {
		t.Fatal("unexpected clean stop — model never completed")
	}
}

// Without --require-tool, a text-only turn ends immediately (unchanged behavior).
func TestRun_RequireTool_DisabledKeepsOneShot(t *testing.T) {
	f := faux.New().Reply("here is the answer").Done()
	s := session.New("gpt-5")
	a := New(f, tools.NewRegistry(), s, "system")

	evs := collect(t, a.Run(context.Background(), userMsg("what is 2+2?")))

	var nudges int
	var sawStop bool
	for _, e := range evs {
		switch v := e.(type) {
		case RequiredToolPending:
			nudges++
		case TurnDone:
			if v.Reason == "stop" {
				sawStop = true
			}
		}
	}
	if nudges != 0 {
		t.Fatalf("nudges=%d, want 0 when require-tool disabled", nudges)
	}
	if !sawStop {
		t.Fatal("expected immediate TurnDone(stop)")
	}
}

// The headless contract must be injected into the system prompt when enabled.
func TestRun_RequireTool_InjectsSystemPrompt(t *testing.T) {
	cp := &captureProvider{}
	a := New(cp, tools.NewRegistry(), session.New("gpt-5"), "base prompt")
	a.SetRequireTools(map[string]bool{"kanban_complete": true})

	_ = collect(t, a.Run(context.Background(), userMsg("hi")))

	if !strings.Contains(cp.gotReq.System, "<headless-execution>") {
		t.Fatalf("headless contract not injected: %q", cp.gotReq.System)
	}
	if !strings.Contains(cp.gotReq.System, "kanban_complete") {
		t.Fatalf("required tool name missing from prompt: %q", cp.gotReq.System)
	}
}

func TestParseRequireTools(t *testing.T) {
	got := ParseRequireTools(" kanban_complete , kanban_block ,")
	if len(got) != 2 || !got["kanban_complete"] || !got["kanban_block"] {
		t.Fatalf("got %v", got)
	}
	if ParseRequireTools("") != nil || ParseRequireTools(" , ") != nil {
		t.Fatal("empty input should yield nil")
	}
}
