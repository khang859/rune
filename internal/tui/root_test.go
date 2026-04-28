package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

func TestRoot_TextOnlyTurnRendersAssistantText(t *testing.T) {
	f := faux.New().Reply("hello back").Done()
	s := session.New("gpt-5")
	a := agent.New(f, tools.NewRegistry(), s, "")

	m := NewRootModel(a, s)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	typeText(tm, "hi")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "hello back")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestRoot_CtrlCQuitsEvenWhileStreaming(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.streaming = true

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected Quit cmd while streaming, got nil")
	}
	if msg := cmd(); msg != (tea.QuitMsg{}) {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func typeText(tm *teatest.TestModel, s string) {
	for _, r := range s {
		tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestRoot_RefreshDoesNotJumpWhenScrolledUp(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	for i := 0; i < 50; i++ {
		m.msgs.AppendUser(fmt.Sprintf("line %d", i))
	}
	m.refreshViewport()
	m.viewport.GotoTop()
	if m.viewport.AtBottom() {
		t.Fatal("expected viewport not at bottom after GotoTop")
	}
	m.msgs.AppendUser("incoming streamed line")
	m.refreshViewport()
	if m.viewport.AtBottom() {
		t.Fatal("refresh snapped to bottom while user was scrolled up")
	}
}

func TestRoot_QueuedMessageAppendsAndDrainsAfterTurn(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m.streaming = true
	m.queue.Push(QueueItem{Text: "queued one"})

	_, cmd := m.Update(AgentChannelDoneMsg{})
	if cmd == nil {
		t.Fatal("expected cmd from drain (startTurn)")
	}
	out := m.msgs.Render(m.styles)
	if !strings.Contains(out, "queued one") {
		t.Fatalf("expected queued message in chat log; got: %q", out)
	}
}

func TestRoot_StaleEventsAfterSwapSessionAreDropped(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// Simulate an active stream owned by the previous session.
	oldCh := make(chan agent.Event)
	m.streaming = true
	m.eventCh = oldCh
	m.cancel = func() {}
	m.queue.Push(QueueItem{Text: "queued before swap"})

	// Swap to a fresh session (mirrors what /resume or /new does).
	ns := session.New("gpt-5")
	m.swapSession(ns)

	if m.streaming {
		t.Fatal("swapSession must clear streaming flag")
	}
	if m.eventCh != nil {
		t.Fatal("swapSession must clear eventCh")
	}
	if m.queue.Len() != 0 {
		t.Fatal("swapSession must drop queued items so they don't bleed into the new session")
	}

	// A stale event from the old channel must not reach handleEvent.
	_, _ = m.Update(AgentEventMsg{Event: agent.AssistantText{Delta: "STALE"}, Ch: oldCh})
	if got := m.msgs.Render(m.styles); strings.Contains(got, "STALE") {
		t.Fatalf("stale event leaked into messages: %q", got)
	}

	// A stale done from the old channel must not pop the (now empty) queue
	// or otherwise mutate state.
	m.queue.Push(QueueItem{Text: "should-stay-queued"})
	_, _ = m.Update(AgentChannelDoneMsg{Ch: oldCh})
	if m.queue.Len() != 1 {
		t.Fatal("stale AgentChannelDoneMsg must not drain the queue")
	}
}

func TestRoot_SlashCommandInfoFlushesToViewportImmediately(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	// /copy with no assistant message must surface the notice in the viewport,
	// not buffer it until the next user prompt.
	m.handleSlashCommand("/copy")

	if got := m.viewport.View(); !strings.Contains(got, "no assistant message to copy") {
		t.Fatalf("expected /copy notice flushed to viewport, got:\n%s", got)
	}
}

func TestRoot_CompactDoneRendersSummaryAndInfo(t *testing.T) {
	s := session.New("gpt-5")
	// Build a session with a pre-existing summary node so rebuild has work to do.
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "u1"}}})
	sum := s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "the summary"}}})
	sum.CompactedCount = 4
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "u2"}}})

	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.compacting = true

	_, _ = m.Update(compactDoneMsg{sess: s, count: 4})
	out := m.msgs.Render(m.styles)
	if !strings.Contains(out, "compacted summary (4 messages)") {
		t.Fatalf("missing summary header: %q", out)
	}
	if !strings.Contains(out, "the summary") {
		t.Fatalf("missing summary body: %q", out)
	}
	if !strings.Contains(out, "(compacted 4 messages)") {
		t.Fatalf("missing post-compact info: %q", out)
	}
	if m.compacting {
		t.Fatal("compacting flag should be cleared after compactDoneMsg")
	}
}

func TestRoot_CompactDoneSurfacesError(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.compacting = true

	_, _ = m.Update(compactDoneMsg{sess: s, err: fmt.Errorf("boom")})
	out := m.msgs.Render(m.styles)
	if !strings.Contains(out, "compact failed") || !strings.Contains(out, "boom") {
		t.Fatalf("error not surfaced: %q", out)
	}
	if m.compacting {
		t.Fatal("compacting flag should be cleared even on error")
	}
}

func TestRoot_CompactDoneFromSwappedSessionIsDropped(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.compacting = true

	stale := session.New("gpt-5")
	_, _ = m.Update(compactDoneMsg{sess: stale, count: 9})

	if !m.compacting {
		t.Fatal("stale compactDoneMsg must not clear compacting flag on the active session")
	}
	if strings.Contains(m.msgs.Render(m.styles), "compacted 9 messages") {
		t.Fatal("stale compactDoneMsg leaked into messages")
	}
}

func TestRoot_CompactQueuesUserInputAndDrainsOnDone(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.compacting = true
	m.queue.Push(QueueItem{Text: "queued during compact"})

	_, cmd := m.Update(compactDoneMsg{sess: s, count: 1})
	if cmd == nil {
		t.Fatal("expected drain cmd after compactDoneMsg with queued item")
	}
	if !strings.Contains(m.msgs.Render(m.styles), "queued during compact") {
		t.Fatal("queued message did not appear in chat after compact done")
	}
}

func TestRoot_RefusesCompactWhileStreaming(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.streaming = true

	got := m.handleSlashCommand("/compact")
	if got != nil {
		t.Fatal("/compact must be a no-op while streaming")
	}
	if !strings.Contains(m.msgs.Render(m.styles), "busy") {
		t.Fatal("expected busy notice after /compact while streaming")
	}
}

var _ = ai.RoleUser
var _ = json.Valid
var _ = context.Background
