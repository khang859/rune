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

var _ = ai.RoleUser
var _ = json.Valid
var _ = context.Background
