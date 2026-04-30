package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
	"github.com/khang859/rune/internal/tui/modal"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "rune-tui-tests-*")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer os.RemoveAll(dir)
	_ = os.Setenv("RUNE_DIR", dir)
	os.Exit(m.Run())
}

func TestRoot_SavesOnlyAfterFirstUserMessage(t *testing.T) {
	dir := t.TempDir()
	s := session.New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	a := agent.New(faux.New().Reply("hello back").Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	if _, err := os.Stat(s.Path()); !os.IsNotExist(err) {
		t.Fatalf("new empty session should not be saved yet, stat err=%v", err)
	}

	cmd := m.startTurn("hi", nil)
	if cmd == nil {
		t.Fatal("expected startTurn command")
	}
	if _, err := os.Stat(s.Path()); err != nil {
		t.Fatalf("session should be saved after first user message: %v", err)
	}

	loaded, err := session.Load(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	msgs := loaded.PathToActive()
	if len(msgs) != 1 || msgs[0].Role != ai.RoleUser {
		t.Fatalf("saved messages after startTurn = %#v, want one user message", msgs)
	}
}

func TestRoot_AutoCompactsAtConfiguredContextThreshold(t *testing.T) {
	s := session.New("gpt-5.5")
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "u1"}}})
	s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "a1"}}})
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "u2"}}})
	a := agent.New(faux.New().Reply("summary").Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.currentTokens = 272000 * 80 / 100
	m.footer.Tokens = m.currentTokens
	m.eventCh = make(chan agent.Event)

	_, cmd := m.Update(AgentChannelDoneMsg{Ch: m.eventCh})
	if cmd == nil {
		t.Fatal("expected auto-compact command at threshold")
	}
	if !m.compacting {
		t.Fatal("expected compacting flag after auto-compact trigger")
	}
}

func TestRoot_AutoCompactDoesNotRunBeforeFirstCompactableCut(t *testing.T) {
	s := session.New("gpt-5.5")
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "u1"}}})
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.currentTokens = 272000
	m.eventCh = make(chan agent.Event)

	_, cmd := m.Update(AgentChannelDoneMsg{Ch: m.eventCh})
	if cmd != nil {
		t.Fatal("did not expect auto-compact when only one user message exists")
	}
	if m.compacting {
		t.Fatal("compacting flag should remain false")
	}
}

func TestRoot_ExpandsFileReferenceWhenSendingButDisplaysOriginal(t *testing.T) {
	dir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Project"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := session.New("gpt-5")
	a := agent.New(faux.New().Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	m.editor.SetValue("review @README.md")
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected turn command")
	}
	msgs := s.PathToActive()
	if len(msgs) == 0 {
		t.Fatal("expected at least one message")
	}
	text, ok := msgs[0].Content[0].(ai.TextBlock)
	if !ok {
		t.Fatalf("first block = %#v, want text", msgs[0].Content[0])
	}
	if !strings.Contains(text.Text, "<file name=\"") || !strings.Contains(text.Text, filepath.Join(dir, "README.md")) || !strings.Contains(text.Text, ">\n# Project\n</file>") {
		t.Fatalf("sent text did not include expanded file:\n%s", text.Text)
	}
	out := m.msgs.Render(DefaultStylesWithIconMode("nerd"), false, false, time.Time{})
	if !strings.Contains(out, "review @README.md") {
		t.Fatalf("display should keep original reference, got:\n%s", out)
	}
	if strings.Contains(out, "<file name=") {
		t.Fatalf("display should not show expanded file contents, got:\n%s", out)
	}
}

func TestRoot_StartTurnWarnsButSendsImagesForUnsupportedModel(t *testing.T) {
	s := session.New("llama-3.3-70b-versatile")
	s.Provider = "groq"
	a := agent.New(faux.New().Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	cmd := m.startTurn("describe", []ai.ContentBlock{ai.ImageBlock{Data: []byte("x"), MimeType: "image/png"}})
	if cmd == nil {
		t.Fatal("expected start turn command")
	}
	msgs := s.PathToActive()
	if len(msgs) != 1 {
		t.Fatalf("messages len = %d, want 1", len(msgs))
	}
	if len(msgs[0].Content) != 2 {
		t.Fatalf("content len = %d, want text + image", len(msgs[0].Content))
	}
	if _, ok := msgs[0].Content[1].(ai.ImageBlock); !ok {
		t.Fatalf("image was not sent: %#v", msgs[0].Content[1])
	}
	out := m.msgs.Render(DefaultStylesWithIconMode("nerd"), false, false, time.Time{})
	if !strings.Contains(out, "not documented") {
		t.Fatalf("expected unsupported model warning in messages, got:\n%s", out)
	}
}

func TestFormatUserMessageForDisplayIncludesImageCount(t *testing.T) {
	if got := formatUserMessageForDisplay("hi", 2); got != "hi\n[2 image(s) attached]" {
		t.Fatalf("display = %q", got)
	}
	if got := formatUserMessageForDisplay("", 1); got != "[1 image(s) attached]" {
		t.Fatalf("display = %q", got)
	}
}

func TestRoot_SavesAssistantMessageWhenTurnCompletes(t *testing.T) {
	dir := t.TempDir()
	s := session.New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.eventCh = make(chan agent.Event)
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hi"}}})
	s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "hello"}}})

	_, _ = m.Update(AgentChannelDoneMsg{Ch: m.eventCh})

	loaded, err := session.Load(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	msgs := loaded.PathToActive()
	if len(msgs) != 2 || msgs[1].Role != ai.RoleAssistant {
		t.Fatalf("saved messages after done = %#v, want user + assistant", msgs)
	}
}

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

func TestRoot_PlanModeSlashCommands(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	m.handleSlashCommand("/plan")
	if a.Mode() != agent.ModePlan || a.Tools().PermissionMode() != tools.PermissionModePlan || m.footer.Mode != "plan" || !m.planPending {
		t.Fatalf("/plan did not enter plan mode: mode=%q toolMode=%q footer=%q pending=%v", a.Mode(), a.Tools().PermissionMode(), m.footer.Mode, m.planPending)
	}

	m.handleSlashCommand("/cancel-plan")
	if a.Mode() != agent.ModePlan || m.planPending {
		t.Fatalf("/cancel-plan should keep plan mode and clear pending: mode=%q pending=%v", a.Mode(), m.planPending)
	}

	m.handleSlashCommand("/approve")
	if a.Mode() != agent.ModeAct || a.Tools().PermissionMode() != tools.PermissionModeAct || m.footer.Mode != "" || m.planPending {
		t.Fatalf("/approve did not switch to act mode: mode=%q toolMode=%q footer=%q pending=%v", a.Mode(), a.Tools().PermissionMode(), m.footer.Mode, m.planPending)
	}

	m.handleSlashCommand("/plan")
	m.handleSlashCommand("/act")
	if a.Mode() != agent.ModeAct || m.footer.Mode != "" || m.planPending {
		t.Fatalf("/act did not switch to act mode: mode=%q footer=%q pending=%v", a.Mode(), m.footer.Mode, m.planPending)
	}
}

func TestRoot_PlanModeSlashCommandsBlockedWhileStreaming(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.streaming = true
	m.handleSlashCommand("/plan")
	if a.Mode() != agent.ModeAct {
		t.Fatalf("/plan while streaming changed mode to %q", a.Mode())
	}
}

func TestRoot_PlanModeBannerRendersAndReservesLayoutRow(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	actHeight := m.viewport.Height

	m.handleSlashCommand("/plan")
	if !strings.Contains(m.View(), "Plan Mode: edits and bash disabled") {
		t.Fatalf("missing plan mode banner:\n%s", m.View())
	}
	if got, want := m.viewport.Height, actHeight-1; got != want {
		t.Fatalf("plan mode viewport height = %d, want %d", got, want)
	}

	m.handleSlashCommand("/act")
	if strings.Contains(m.View(), "Plan Mode: edits and bash disabled") {
		t.Fatalf("plan mode banner remained after /act:\n%s", m.View())
	}
	if got, want := m.viewport.Height, actHeight; got != want {
		t.Fatalf("act mode viewport height = %d, want %d", got, want)
	}
}

func TestRoot_ShiftTabCyclesNormalPlanCopyNormal(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if a.Mode() != agent.ModeAct || m.copyMode || m.footer.Mode != "" {
		t.Fatalf("initial mode = agent %q copy=%v footer=%q, want normal/act", a.Mode(), m.copyMode, m.footer.Mode)
	}

	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if a.Mode() != agent.ModePlan || !m.planPending || m.copyMode || m.footer.Mode != "plan" {
		t.Fatalf("first Shift+Tab did not enter plan: mode=%q pending=%v copy=%v footer=%q", a.Mode(), m.planPending, m.copyMode, m.footer.Mode)
	}
	if !strings.Contains(m.View(), "Plan Mode: edits and bash disabled") {
		t.Fatalf("missing plan banner after first Shift+Tab:\n%s", m.View())
	}

	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if a.Mode() != agent.ModeAct || m.planPending || !m.copyMode || m.footer.Mode != "" {
		t.Fatalf("second Shift+Tab did not enter copy mode from plan: mode=%q pending=%v copy=%v footer=%q", a.Mode(), m.planPending, m.copyMode, m.footer.Mode)
	}
	if !strings.Contains(m.View(), "[copy mode]") {
		t.Fatalf("missing copy banner after second Shift+Tab:\n%s", m.View())
	}

	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if a.Mode() != agent.ModeAct || m.planPending || m.copyMode || m.footer.Mode != "" {
		t.Fatalf("third Shift+Tab did not return to normal: mode=%q pending=%v copy=%v footer=%q", a.Mode(), m.planPending, m.copyMode, m.footer.Mode)
	}
	out := m.View()
	if strings.Contains(out, "Plan Mode: edits and bash disabled") || strings.Contains(out, "[copy mode]") {
		t.Fatalf("normal mode still shows mode banner:\n%s", out)
	}
}

func TestRoot_ShiftTabCycleBlockedWhileStreaming(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.streaming = true

	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if a.Mode() != agent.ModeAct || m.copyMode {
		t.Fatalf("Shift+Tab while streaming changed normal mode: mode=%q copy=%v", a.Mode(), m.copyMode)
	}
	if !strings.Contains(m.View(), "busy — wait for current turn to finish") {
		t.Fatalf("missing busy message after blocked Shift+Tab:\n%s", m.View())
	}
}

func TestRoot_PlanModeBlocksShellShortcuts(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.handleSlashCommand("/plan")

	for _, r := range "!echo nope" {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("blocked shell shortcut should not start a command")
	}
	if len(s.PathToActive()) != 0 {
		t.Fatalf("blocked shell shortcut should not append session messages")
	}
	if !strings.Contains(m.View(), "shell shortcuts are disabled in Plan Mode") {
		t.Fatalf("missing shell shortcut block message in view")
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

func TestRoot_SessionSlashCommandIsInfoNotError(t *testing.T) {
	s := session.New("gpt-5.5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	m.handleSlashCommand("/session")

	if len(m.msgs.blocks) == 0 {
		t.Fatal("expected /session to add a message")
	}
	last := m.msgs.blocks[len(m.msgs.blocks)-1]
	if last.kind != bkInfo {
		t.Fatalf("/session should render as info, got kind %v", last.kind)
	}
	if !strings.Contains(last.text, "session id=") || !strings.Contains(last.text, "model=gpt-5.5") {
		t.Fatalf("unexpected /session text: %q", last.text)
	}
}

func TestRoot_ClearSlashCommandStartsNewSession(t *testing.T) {
	s := session.New("gpt-5.5")
	s.Provider = "groq"
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hello"}}})
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	oldID := m.sess.ID

	m.handleSlashCommand("/clear")

	if m.sess.ID == oldID {
		t.Fatal("/clear should swap to a new session")
	}
	if m.sess.Model != "gpt-5.5" || m.sess.Provider != "groq" {
		t.Fatalf("/clear should preserve model/provider, got model=%q provider=%q", m.sess.Model, m.sess.Provider)
	}
	if len(m.sess.PathToActive()) != 0 {
		t.Fatalf("/clear should start with an empty session, got %d messages", len(m.sess.PathToActive()))
	}
	if len(m.msgs.blocks) != 0 {
		t.Fatalf("/clear should clear rendered messages, got %d blocks", len(m.msgs.blocks))
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

func TestThinkingLevelsForKnownModels(t *testing.T) {
	cases := map[string][]string{
		"gpt-5.5":       {"none", "low", "medium", "high", "xhigh"},
		"gpt-5.4":       {"none", "low", "medium", "high", "xhigh"},
		"gpt-5.3-codex": {"low", "medium", "high", "xhigh"},
		"gpt-5.2":       {"none", "low", "medium", "high", "xhigh"},
		"gpt-5.2-codex": {"low", "medium", "high", "xhigh"},
		"gpt-5.1":       {"none", "low", "medium", "high"},
	}
	for model, want := range cases {
		got := thinkingLevelsForModel(model)
		if strings.Join(got, ",") != strings.Join(want, ",") {
			t.Fatalf("thinkingLevelsForModel(%q) = %v, want %v", model, got, want)
		}
	}
	if got := thinkingLevelsForModel("gpt-5.4-mini"); len(got) != 0 {
		t.Fatalf("thinkingLevelsForModel unknown = %v, want none", got)
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

func TestRoot_ForkEmptySessionDoesNotOpenModal(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	cmd := m.handleSlashCommand("/fork")
	if cmd != nil {
		t.Fatal("/fork on empty session returned a command")
	}
	if m.modal != nil {
		t.Fatal("/fork on empty session opened a modal")
	}
	if got := m.viewport.View(); !strings.Contains(got, "nothing to fork yet") {
		t.Fatalf("expected empty fork notice, got:\n%s", got)
	}
}

func TestRoot_ThinkingUnknownModelDoesNotOpenModal(t *testing.T) {
	s := session.New("gpt-5.4-mini")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	cmd := m.handleSlashCommand("/thinking")
	if cmd != nil {
		t.Fatal("/thinking for unknown model returned a command")
	}
	if m.modal != nil {
		t.Fatal("/thinking for unknown model opened a modal")
	}
}

func TestRoot_ThinkingPickerUpdatesEffort(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	s := session.New("gpt-5.5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	if cmd := m.handleSlashCommand("/thinking"); cmd != nil {
		_, _ = m.Update(cmd())
	}
	if _, ok := m.modal.(*modal.ThinkingPicker); !ok {
		t.Fatalf("modal = %T, want ThinkingPicker", m.modal)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyDown}) // high
	if cmd != nil {
		_, _ = m.Update(cmd())
	}
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected thinking picker result command")
	}
	_, _ = m.Update(cmd())

	if got := a.ReasoningEffort(); got != "high" {
		t.Fatalf("reasoning effort = %q, want high", got)
	}
}

func TestRoot_ModelSwitchClampsThinkingEffort(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	s := session.New("gpt-5.5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	a.SetReasoningEffort("xhigh")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	m.applyModalResult(modal.NewModelPicker(nil, ""), "gpt-5.1")

	if got := a.ReasoningEffort(); got != "medium" {
		t.Fatalf("reasoning effort = %q, want medium", got)
	}
}

func TestRoot_LoadsSavedSettingsOnStartup(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	if err := config.SaveSettings(config.SettingsPath(), config.Settings{
		ReasoningEffort: "high",
		IconMode:        "ascii",
		ActivityMode:    "simple",
		Web: config.WebSettings{
			FetchEnabled:      false,
			FetchAllowPrivate: true,
			SearchEnabled:     "off",
			SearchProvider:    "brave",
		},
	}); err != nil {
		t.Fatal(err)
	}

	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	if got := a.ReasoningEffort(); got != "high" {
		t.Fatalf("reasoning effort = %q, want high", got)
	}
	if got := m.settings.IconMode; got != "ascii" {
		t.Fatalf("settings icon mode = %q, want ascii", got)
	}
	if got := m.settings.ActivityMode; got != "simple" {
		t.Fatalf("settings activity mode = %q, want simple", got)
	}
	if got := m.settings.WebFetch; got != "off" {
		t.Fatalf("settings web fetch = %q, want off", got)
	}
	if got := m.settings.SearchProvider; got != "brave" {
		t.Fatalf("settings search provider = %q, want brave", got)
	}
}

func TestRoot_SettingsPreserveCurrentEffortByDefault(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
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

func TestRoot_FooterThinkingEffortForSupportedModel(t *testing.T) {
	s := session.New("gpt-5.5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	if got, want := m.footer.ThinkingEffort, "medium"; got != want {
		t.Fatalf("footer thinking effort = %q, want %q", got, want)
	}
}

func TestRoot_FooterThinkingEffortHiddenForUnsupportedModel(t *testing.T) {
	s := session.New("gpt-5.4-mini")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	if got := m.footer.ThinkingEffort; got != "" {
		t.Fatalf("footer thinking effort = %q, want empty", got)
	}
}

func TestRoot_FooterThinkingEffortUpdatesFromPicker(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	s := session.New("gpt-5.5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	m.applyModalResult(modal.NewThinkingPicker(nil, ""), "high")

	if got, want := m.footer.ThinkingEffort, "high"; got != want {
		t.Fatalf("footer thinking effort = %q, want %q", got, want)
	}
}

func TestRoot_FooterThinkingEffortRefreshesOnModelSwitch(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	s := session.New("gpt-5.5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	a.SetReasoningEffort("xhigh")
	m := NewRootModel(a, s)

	m.applyModalResult(modal.NewModelPicker(nil, ""), "gpt-5.1")
	if got, want := m.footer.ThinkingEffort, "medium"; got != want {
		t.Fatalf("footer thinking effort after clamp = %q, want %q", got, want)
	}

	m.applyModalResult(modal.NewModelPicker(nil, ""), "gpt-5.4-mini")
	if got := m.footer.ThinkingEffort; got != "" {
		t.Fatalf("footer thinking effort for unsupported model = %q, want empty", got)
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

func TestRoot_SubagentCompletionStartsContinuationWhenIdle(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New().Reply("continued from subagent").Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	ev := agent.SubagentEvent{
		Status: agent.SubagentCompleted,
		Task: tools.SubagentTask{
			ID:        "subagent_1",
			Name:      "repo-plan",
			AgentType: "general",
			Status:    string(agent.SubagentCompleted),
			CreatedAt: time.Now(),
			Summary:   "## Summary\nneeds follow-up",
		},
	}
	_, cmd := m.Update(SubagentEventMsg{Event: ev, Ch: m.subagentCh})
	if cmd == nil {
		t.Fatal("expected continuation command after completed subagent")
	}
	if !m.streaming {
		t.Fatal("expected continuation turn to start")
	}
	path := s.PathToActive()
	if len(path) < 1 || path[0].Role != ai.RoleUser {
		t.Fatalf("expected synthetic continuation user message, got %#v", path)
	}
	if got := path[0].Content[0].(ai.TextBlock).Text; got != subagentContinuationPrompt {
		t.Fatalf("continuation prompt = %q, want %q", got, subagentContinuationPrompt)
	}
}

func TestRoot_SubagentCompletionQueuesContinuationWhenBusy(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New().Reply("continued from subagent").Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.streaming = true

	ev := agent.SubagentEvent{
		Status: agent.SubagentCompleted,
		Task: tools.SubagentTask{
			ID:        "subagent_1",
			Name:      "repo-plan",
			AgentType: "general",
			Status:    string(agent.SubagentCompleted),
			CreatedAt: time.Now(),
			Summary:   "## Summary\nneeds follow-up",
		},
	}
	_, _ = m.Update(SubagentEventMsg{Event: ev, Ch: m.subagentCh})
	if !m.pendingSubagentContinuation {
		t.Fatal("expected continuation to be queued while streaming")
	}
}

var _ = ai.RoleUser
var _ = json.Valid
var _ = context.Background

func TestRoot_ActivityLineShowsSubagentsOnRightWhileStreaming(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.streaming = true
	m.subagents["subagent_1"] = agent.SubagentEvent{
		Status: agent.SubagentRunning,
		Task:   tools.SubagentTask{ID: "subagent_1", Name: "repo-plan", AgentType: "general", Status: string(agent.SubagentRunning), CreatedAt: time.Now()},
	}

	line := m.renderActivityLine()
	if !strings.Contains(line, "consulting the runes") || !strings.Contains(line, "1 familiar scrying") {
		t.Fatalf("combined activity line missing main/familiar indicators: %q", line)
	}
	if strings.Index(line, "consulting the runes") > strings.Index(line, "1 familiar scrying") {
		t.Fatalf("subagent indicator should render to the right of main activity: %q", line)
	}
}

func TestRoot_SubagentSlashCommandsListAndCancel(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New().Reply("working").Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	task, err := a.Subagents().Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "repo-plan", Prompt: "inspect", AgentType: "general", Background: true})
	if err != nil {
		t.Fatal(err)
	}
	m.subagents[task.ID] = agent.SubagentEvent{Status: agent.SubagentRunning, Task: *task}

	m.handleSlashCommand("/subagents")
	out := m.msgs.Render(m.styles, false, false, time.Now())
	if !strings.Contains(out, task.ID) || !strings.Contains(out, "repo-plan") {
		t.Fatalf("/subagents did not render task list: %q", out)
	}

	m.handleSlashCommand("/subagent-cancel all")
	out = m.msgs.Render(m.styles, false, false, time.Now())
	if !strings.Contains(out, "cancelled 1 subagents") {
		t.Fatalf("/subagent-cancel all did not report cancellation: %q", out)
	}
}

func TestRoot_SubagentEventsRenderAndTrackActivity(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New().Reply("continued").Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	created := time.Now()
	ev := agent.SubagentEvent{
		Status: agent.SubagentRunning,
		Task:   tools.SubagentTask{ID: "subagent_1", Name: "repo-plan", AgentType: "general", Status: string(agent.SubagentRunning), CreatedAt: created},
	}
	_, _ = m.Update(SubagentEventMsg{Event: ev, Ch: m.subagentCh})
	out := m.msgs.Render(m.styles, false, false, time.Now())
	if !strings.Contains(out, "familiar of repo-plan") || !strings.Contains(out, "scrying") {
		t.Fatalf("running familiar not rendered: %q", out)
	}
	if m.activeSubagentCount() != 1 {
		t.Fatalf("activeSubagentCount = %d, want 1", m.activeSubagentCount())
	}
	if !strings.Contains(m.View(), "1 familiar scrying") {
		t.Fatalf("familiar activity indicator not rendered in view: %q", m.View())
	}
	before := m.renderSubagentActivityIndicator()
	seq := m.activitySeq
	_, cmd := m.Update(activityTickMsg{seq: seq})
	if cmd == nil {
		t.Fatal("expected continuing activity tick while subagent is active")
	}
	after := m.renderSubagentActivityIndicator()
	if before == after {
		t.Fatalf("subagent spinner did not advance: before=%q after=%q", before, after)
	}

	doneAt := time.Now()
	ev.Status = agent.SubagentCompleted
	ev.Task.Status = string(agent.SubagentCompleted)
	ev.Task.CompletedAt = &doneAt
	ev.Task.Summary = "## Summary\nall done"
	_, _ = m.Update(SubagentEventMsg{Event: ev, Ch: m.subagentCh})
	out = m.msgs.Render(m.styles, false, false, time.Now())
	if !strings.Contains(out, "returned") || !strings.Contains(out, "findings added to") || !strings.Contains(out, "context") {
		t.Fatalf("completed familiar not rendered collapsed: %q", out)
	}
	if strings.Contains(out, "all done") {
		t.Fatalf("completed subagent summary should be collapsed by default: %q", out)
	}
	if m.activeSubagentCount() != 0 {
		t.Fatalf("activeSubagentCount = %d, want 0", m.activeSubagentCount())
	}
	if strings.Contains(m.View(), "familiar scrying") {
		t.Fatalf("familiar activity indicator still rendered after completion: %q", m.View())
	}
}

func TestRoot_BlockedSubagentCountsAsActivity(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	ev := agent.SubagentEvent{
		Status: agent.SubagentBlocked,
		Task:   tools.SubagentTask{ID: "subagent_1", Name: "blocked-plan", AgentType: "general", Status: string(agent.SubagentBlocked), CreatedAt: time.Now()},
	}
	_, _ = m.Update(SubagentEventMsg{Event: ev, Ch: m.subagentCh})

	if m.activeSubagentCount() != 1 {
		t.Fatalf("activeSubagentCount = %d, want 1", m.activeSubagentCount())
	}
	if !strings.Contains(m.View(), "1 familiar scrying") {
		t.Fatalf("blocked familiar activity indicator not rendered in view: %q", m.View())
	}
}
