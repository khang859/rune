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
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func TestNormalizeShiftEnterCSIU(t *testing.T) {
	msg := normalizeShiftEnterMsg(fmt.Stringer(unknownCSIString("?CSI[49 51 59 50 117]?")))
	k, ok := msg.(tea.KeyMsg)
	if !ok || k.Type != tea.KeyCtrlJ {
		t.Fatalf("expected Kitty Shift+Enter CSI-u to normalize to Ctrl+J KeyMsg, got %#v", msg)
	}
}

type unknownCSIString string

func (s unknownCSIString) String() string { return string(s) }

func TestRoot_CtrlCFirstPressDoesNotQuitAndCancelsStream(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.streaming = true
	cancelled := false
	m.cancel = func() { cancelled = true }

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.quitPrimed {
		t.Fatal("first Ctrl+C should prime the quit indicator")
	}
	if !cancelled {
		t.Fatal("first Ctrl+C should cancel an in-flight streaming turn")
	}
	if cmd == nil {
		t.Fatal("expected a quitPrimeExpired tick cmd, got nil")
	}
	if msg := cmd(); msg == (tea.QuitMsg{}) {
		t.Fatalf("first Ctrl+C must not produce tea.QuitMsg")
	}
	if !strings.Contains(m.View(), "Press Ctrl+C again to exit") {
		t.Fatal("expected primed indicator in View()")
	}
}

func TestRoot_CtrlCSecondPressQuits(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if cmd == nil {
		t.Fatal("expected Quit cmd from second Ctrl+C")
	}
	if msg := cmd(); msg != (tea.QuitMsg{}) {
		t.Fatalf("expected tea.QuitMsg, got %T", msg)
	}
}

func TestRoot_CtrlCFirstPressClearsEditorInput(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	for _, r := range "hello world" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	if !strings.Contains(m.View(), "hello world") {
		t.Fatal("setup: editor should contain typed text before Ctrl+C")
	}

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	if strings.Contains(m.View(), "hello world") {
		t.Fatal("first Ctrl+C should clear the editor input")
	}
	if !m.quitPrimed {
		t.Fatal("first Ctrl+C should prime the indicator")
	}
}

func TestRoot_NonCtrlCKeyDisarmsQuitPrime(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.quitPrimed {
		t.Fatal("setup: expected quit primed after first Ctrl+C")
	}

	// Any non-Ctrl+C key must dis-arm so a subsequent Ctrl+C primes again
	// rather than exiting.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	if m.quitPrimed {
		t.Fatal("typing should dis-arm the quit indicator")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !m.quitPrimed {
		t.Fatal("Ctrl+C after dis-arm should re-prime, not quit")
	}
	if cmd == nil {
		t.Fatal("expected primed tick cmd")
	}
	if msg := cmd(); msg == (tea.QuitMsg{}) {
		t.Fatal("Ctrl+C after dis-arm must not quit immediately")
	}
}

func TestRoot_QuitPrimeExpiresAfterTimeout(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	seq := m.quitPrimedSeq
	if !m.quitPrimed {
		t.Fatal("setup: first Ctrl+C should prime")
	}

	_, _ = m.Update(quitPrimeExpiredMsg{seq: seq})

	if m.quitPrimed {
		t.Fatal("matching expiration tick should clear the primed flag")
	}
	if strings.Contains(m.View(), "Press Ctrl+C again to exit") {
		t.Fatal("indicator should not render after expiration")
	}
}

func TestRoot_StaleQuitPrimeExpirationIgnored(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	staleSeq := m.quitPrimedSeq
	// Re-prime by dis-arming (typing) then pressing Ctrl+C again.
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})

	// A late tick from the prior priming cycle must not clear the
	// currently-primed state.
	_, _ = m.Update(quitPrimeExpiredMsg{seq: staleSeq})
	if !m.quitPrimed {
		t.Fatal("stale expiration tick must not dis-arm an active prime")
	}
}

func TestRoot_CtrlCWithModalOpenClosesModalAndPrimes(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.handleSlashCommand("/hotkeys")
	if m.modal == nil {
		t.Fatal("setup: modal should be open")
	}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	if m.modal != nil {
		t.Fatal("first Ctrl+C should close the open modal")
	}
	if !m.quitPrimed {
		t.Fatal("first Ctrl+C should prime even with a modal open")
	}
	if cmd == nil {
		t.Fatal("expected tick cmd")
	}
	if msg := cmd(); msg == (tea.QuitMsg{}) {
		t.Fatal("first Ctrl+C with modal open must not quit")
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
	out := m.msgs.Render(m.styles, false, false, time.Time{})
	if !strings.Contains(out, "queued one") {
		t.Fatalf("expected queued message in chat log; got: %q", out)
	}
}

func TestRoot_SwapSessionFallsBackToShortIDWhenNameEmpty(t *testing.T) {
	s := session.New("gpt-5")
	s.Name = "named-session"
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	ns := session.New("gpt-5")
	m.swapSession(ns)

	want := ns.ID
	if len(want) > 8 {
		want = want[:8]
	}
	if m.footer.Session != want {
		t.Fatalf("expected footer session %q, got %q", want, m.footer.Session)
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
	if got := m.msgs.Render(m.styles, false, false, time.Time{}); strings.Contains(got, "STALE") {
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
	out := m.msgs.Render(m.styles, false, false, time.Time{})
	if !strings.Contains(out, "compacted memory (4 messages)") {
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
	out := m.msgs.Render(m.styles, false, false, time.Time{})
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
	if strings.Contains(m.msgs.Render(m.styles, false, false, time.Time{}), "compacted 9 messages") {
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
	if !strings.Contains(m.msgs.Render(m.styles, false, false, time.Time{}), "queued during compact") {
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
	if !strings.Contains(m.msgs.Render(m.styles, false, false, time.Time{}), "busy") {
		t.Fatal("expected busy notice after /compact while streaming")
	}
}

func TestRoot_RendersArcaneActivityLineWhileStreaming(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.streaming = true
	m.layout()

	out := m.View()
	if !strings.Contains(out, "consulting the runes") {
		t.Fatalf("missing activity line while streaming:\n%s", out)
	}

	_, _ = m.Update(AgentChannelDoneMsg{})
	out = m.View()
	if strings.Contains(out, "consulting the runes") {
		t.Fatalf("activity line remained after done:\n%s", out)
	}
}

func TestRoot_TurnUsageShowsCurrentContextTokensNotCumulative(t *testing.T) {
	s := session.New("gpt-5.5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	m.handleEvent(agent.TurnUsage{Usage: ai.Usage{Input: 1_000_000, Output: 100_000}})
	m.handleEvent(agent.TurnUsage{Usage: ai.Usage{Input: 10_000, Output: 1_000}})

	if got, want := m.footer.Tokens, 11_000; got != want {
		t.Fatalf("footer tokens = %d, want latest usage %d", got, want)
	}
	if got, want := m.footer.ContextPct, 4; got != want {
		t.Fatalf("gpt-5.5 context pct = %d, want %d", got, want)
	}
}

func TestContextWindowForModelMatchesPiCodexList(t *testing.T) {
	for _, model := range []string{
		"gpt-5.1",
		"gpt-5.1-codex-max",
		"gpt-5.1-codex-mini",
		"gpt-5.2",
		"gpt-5.2-codex",
		"gpt-5.3-codex",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.5",
	} {
		if got, want := contextWindowForModel(model), 272_000; got != want {
			t.Fatalf("contextWindowForModel(%q) = %d, want %d", model, got, want)
		}
	}
	if got, want := contextWindowForModel("gpt-5.3-codex-spark"), 128_000; got != want {
		t.Fatalf("contextWindowForModel(spark) = %d, want %d", got, want)
	}
}

func TestCtxPctForModelUsesCodexWindow(t *testing.T) {
	if got, want := ctxPctForModel("gpt-5.5", 136_000), 50; got != want {
		t.Fatalf("ctxPctForModel(gpt-5.5, 136k) = %d, want %d", got, want)
	}
	if got, want := ctxPctForModel("gpt-5.5", 300_000), 100; got != want {
		t.Fatalf("ctxPctForModel caps at %d, want %d", got, want)
	}
}

func TestCodexModelIDsMatchesPiCodexList(t *testing.T) {
	got := strings.Join(codexModelIDs(), ",")
	want := strings.Join([]string{
		"gpt-5.5",
		"gpt-5.4",
		"gpt-5.4-mini",
		"gpt-5.3-codex",
		"gpt-5.3-codex-spark",
		"gpt-5.2",
		"gpt-5.2-codex",
		"gpt-5.1",
		"gpt-5.1-codex-max",
		"gpt-5.1-codex-mini",
	}, ",")
	if got != want {
		t.Fatalf("codexModelIDs() = %q, want %q", got, want)
	}
}

func TestRoot_RendersSimpleActivityLineWhenConfigured(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.settings.ActivityMode = "simple"
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.compacting = true
	m.layout()

	out := m.View()
	if !strings.Contains(out, "running") {
		t.Fatalf("missing simple activity line while compacting:\n%s", out)
	}
}

func TestRoot_SettingsPreserveCurrentEffortByDefault(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m.handleSlashCommand("/settings")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected settings result command")
	}
	_, _ = m.Update(cmd())

	if got := a.ReasoningEffort(); got != "medium" {
		t.Fatalf("reasoning effort changed after applying default settings: %q", got)
	}
}

func TestRoot_IgnoresStaleActivityTicks(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.streaming = true
	m.activitySeq = 1
	m.activityTicking = true
	m.activityFrame = 0

	_, _ = m.Update(activityTickMsg{seq: 0})

	if m.activityFrame != 0 {
		t.Fatalf("stale activity tick advanced frame to %d", m.activityFrame)
	}
	if !m.activityTicking {
		t.Fatal("stale activity tick cleared the active tick state")
	}
}

var _ = ai.RoleUser
var _ = json.Valid
var _ = context.Background
