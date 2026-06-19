package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/ai/oauth"
	"github.com/khang859/rune/internal/ai/unavailable"
	"github.com/khang859/rune/internal/codeindex"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/providers"
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

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	os.Stdout = w
	defer func() { os.Stdout = original }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEmitFleetReadyMarker(t *testing.T) {
	t.Setenv("FLEET_SESSION", "1")

	out := captureStdout(t, func() {
		emitFleetReadyMarker()
	})

	if out != fleetReadyMarkerOSC {
		t.Fatalf("emitFleetReadyMarker() = %q, want %q", out, fleetReadyMarkerOSC)
	}
}

func TestEmitFleetReadyMarkerOutsideFleet(t *testing.T) {
	t.Setenv("FLEET_SESSION", "")

	out := captureStdout(t, func() {
		emitFleetReadyMarker()
	})

	if out != "" {
		t.Fatalf("emitFleetReadyMarker() = %q, want empty output", out)
	}
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
	m.streaming = true

	_, cmd := m.Update(AgentChannelDoneMsg{Ch: m.eventCh})
	if cmd == nil {
		t.Fatal("expected auto-compact command at threshold")
	}
	if !m.compacting {
		t.Fatal("expected compacting flag after auto-compact trigger")
	}
}

func TestRoot_RunsPendingSubagentContinuationAfterAutoCompact(t *testing.T) {
	s := session.New("gpt-5.5")
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "u1"}}})
	s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "a1"}}})
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "u2"}}})
	a := agent.New(faux.New().Reply("summary").Done().Reply("continued").Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.currentTokens = 272000 * 90 / 100
	m.footer.Tokens = m.currentTokens
	m.eventCh = make(chan agent.Event)
	m.streaming = true
	m.pendingSubagentContinuation = true

	_, cmd := m.Update(AgentChannelDoneMsg{Ch: m.eventCh})
	if cmd == nil {
		t.Fatal("expected auto-compact command after streaming turn")
	}
	compactMsg := cmd().(tea.BatchMsg)[0]().(compactDoneMsg)

	_, cmd = m.Update(compactMsg)
	if cmd == nil {
		t.Fatal("expected pending subagent continuation to start after compact")
	}
	if m.pendingSubagentContinuation {
		t.Fatal("pending subagent continuation should be consumed")
	}
	if !m.streaming {
		t.Fatal("expected subagent continuation turn to be streaming")
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

func TestRoot_RefreshesGitBranchAfterToolFinished(t *testing.T) {
	dir := initTestGitRepo(t, "main")
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	if got := m.footer.GitBranch; got != "main" {
		t.Fatalf("initial GitBranch = %q, want main", got)
	}

	runGit(t, dir, "checkout", "-b", "feature")
	m.handleEvent(agent.ToolFinished{
		Call:   ai.ToolCall{Name: "bash"},
		Result: tools.Result{Output: "switched"},
	})

	if got := m.footer.GitBranch; got != "feature" {
		t.Fatalf("GitBranch after ToolFinished = %q, want feature", got)
	}
}

func TestRoot_RefreshesGitBranchAfterTurnDone(t *testing.T) {
	dir := initTestGitRepo(t, "main")
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	if got := m.footer.GitBranch; got != "main" {
		t.Fatalf("initial GitBranch = %q, want main", got)
	}

	runGit(t, dir, "checkout", "-b", "feature")
	m.handleEvent(agent.TurnDone{Reason: "stop"})

	if got := m.footer.GitBranch; got != "feature" {
		t.Fatalf("GitBranch after TurnDone = %q, want feature", got)
	}
}

func initTestGitRepo(t *testing.T, branch string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not found")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-b", branch)
	return dir
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
	prewarmCodeIndexForCwd(t)
	f := faux.New().Reply("hello back").Done()
	s := session.New("gpt-5")
	a := agent.New(f, tools.NewRegistry(), s, "")

	m := NewRootModel(a, s)
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))

	tm.Send(tea.WindowSizeMsg{Width: 80, Height: 24})
	tm.Send(codeIndexDoneMsg{})
	typeText(tm, "hi")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})

	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		return strings.Contains(string(b), "hello back")
	}, teatest.WithDuration(2*time.Second))

	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.WaitFinished(t, teatest.WithFinalTimeout(2*time.Second))
}

func prewarmCodeIndexForCwd(t *testing.T) {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := codeindex.DefaultCache().Set(codeindex.BuildOptions{Root: cwd}, codeindex.New(cwd)); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeShiftEnterCSIU(t *testing.T) {
	msg := normalizeShiftEnterMsg(fmt.Stringer(unknownCSIString("?CSI[49 51 59 50 117]?")))
	k, ok := msg.(tea.KeyMsg)
	if !ok || k.Type != tea.KeyCtrlJ {
		t.Fatalf("expected Kitty Shift+Enter CSI-u to normalize to Ctrl+J KeyMsg, got %#v", msg)
	}
}

func TestSlashRepomapFilesAndContext(t *testing.T) {
	dir := t.TempDir()
	s := session.New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	s.RecordFileRead(filepath.Join(dir, "a.go"))
	a := agent.New(faux.New().Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	a.SetRepoMapEnabled(true)
	a.SetRepoMapBudget(2000)
	m.currentInput = 10
	m.currentOutput = 5
	m.currentCache = 3
	m.currentTokens = 15

	m.handleSlashCommand("/repomap files")
	m.handleSlashCommand("/context")
	out := m.msgs.Render(m.styles, false, false, time.Now())
	if !strings.Contains(out, "repomap focus files") || !strings.Contains(out, "a.go") {
		t.Fatalf("missing repomap files output: %q", out)
	}
	if !strings.Contains(out, "context —") || !strings.Contains(out, "latest usage: in=10 out=5 cache=3") {
		t.Fatalf("missing context output: %q", out)
	}
}

func TestSlashRememberAndMemory(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	s := session.New("gpt-5")
	a := agent.New(faux.New().Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	m.handleSlashCommand("/remember prefer table-driven tests")
	m.handleSlashCommand("/memory")
	out := m.msgs.Render(m.styles, false, false, time.Now())
	if !strings.Contains(out, "remembered") || !strings.Contains(out, "prefer table-driven tests") {
		t.Fatalf("missing memory output: %q", out)
	}
	if a.MemoryPath() != config.MemoryPath() {
		t.Fatalf("MemoryPath = %q, want %q", a.MemoryPath(), config.MemoryPath())
	}
}

func TestSlashRememberRejectedInPlanMode(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	s := session.New("gpt-5")
	a := agent.New(faux.New().Done(), tools.NewRegistry(), s, "")
	a.SetMode(agent.ModePlan)
	m := NewRootModel(a, s)

	m.handleSlashCommand("/remember prefer table-driven tests")
	out := m.msgs.Render(m.styles, false, false, time.Now())
	if !strings.Contains(out, "disabled in plan mode") {
		t.Fatalf("expected plan-mode rejection: %q", out)
	}
	if _, err := os.Stat(config.MemoryPath()); err == nil {
		t.Fatal("memory file should not be created in plan mode")
	}
}

func TestSlashRepomapToggle(t *testing.T) {
	dir := t.TempDir()
	s := session.New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	a := agent.New(faux.New().Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	a.SetRepoMapEnabled(true)

	m.handleSlashCommand("/repomap off")
	if a.RepoMapEnabled() {
		t.Error("expected repo map disabled after /repomap off")
	}
	m.handleSlashCommand("/repomap on")
	if !a.RepoMapEnabled() {
		t.Error("expected repo map enabled after /repomap on")
	}
	m.handleSlashCommand("/repomap budget 3000")
	if a.RepoMapBudget() != 3000 {
		t.Errorf("budget = %d, want 3000", a.RepoMapBudget())
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

func TestRoot_BaseSlashCommandsIncludeGitStatus(t *testing.T) {
	for _, want := range []string{"/git-status", "/feature-dev", "/plan", "/approve", "/cancel-plan", "/context", "/memory", "/remember"} {
		found := false
		for _, cmd := range baseSlashCmds {
			if cmd == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("baseSlashCmds missing %s", want)
		}
	}
	for _, cmd := range baseSlashCmds {
		if cmd == "/act" {
			t.Fatal("baseSlashCmds should not include /act")
		}
	}
}

func TestRoot_FeatureDevSlashCommandArmsPrompt(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	m.handleSlashCommand("/feature-dev")
	if m.pendingSkillBody != featureDevPrompt {
		t.Fatal("/feature-dev did not arm feature-dev prompt")
	}
	out := m.msgs.Render(m.styles, false, false, time.Now())
	if !strings.Contains(out, "feature-dev armed") {
		t.Fatalf("missing armed message: %q", out)
	}
}

func TestRoot_FeatureDevSlashCommandWithArgumentStartsTurn(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New().Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	cmd := m.handleSlashCommand("/feature-dev add previews")
	if cmd == nil {
		t.Fatal("expected /feature-dev with argument to start a turn")
	}
	msgs := s.PathToActive()
	if len(msgs) == 0 || msgs[0].Role != ai.RoleUser {
		t.Fatalf("messages = %#v, want user message first", msgs)
	}
	text, ok := msgs[0].Content[0].(ai.TextBlock)
	if !ok || !strings.Contains(text.Text, featureDevPrompt) || !strings.Contains(text.Text, "Feature request:\nadd previews") {
		t.Fatalf("feature-dev prompt message = %#v", msgs[0].Content)
	}
}

func TestRoot_FeatureDevSlashCommandWithArgumentQueuesWhenBusy(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.streaming = true

	cmd := m.handleSlashCommand("/feature-dev add previews")
	if cmd != nil {
		t.Fatal("busy /feature-dev should queue without starting a turn")
	}
	if got := m.queue.Len(); got != 1 {
		t.Fatalf("queue len = %d, want 1", got)
	}
	item, ok := m.queue.Pop()
	if !ok || !strings.Contains(item.Text, featureDevPrompt) || !strings.Contains(item.Text, "Feature request:\nadd previews") {
		t.Fatalf("queued item = %#v", item)
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
}

func TestRoot_ApproveStartsImplementationInNewSessionWithLatestPlan(t *testing.T) {
	s := session.New("gpt-5")
	s.Provider = "groq"
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "make a plan"}}})
	s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "1. Edit the file\n2. Run tests"}}})
	a := agent.New(blockingTUIProvider{}, tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.handleSlashCommand("/plan")

	oldID := m.sess.ID
	cmd := m.handleSlashCommand("/approve")
	if cmd == nil {
		t.Fatal("/approve should start an implementation turn")
	}
	if m.sess.ID == oldID {
		t.Fatal("/approve should switch to a new session")
	}
	if m.sess.Provider != "groq" || m.sess.Model != "gpt-5" {
		t.Fatalf("new session provider/model = %q/%q, want groq/gpt-5", m.sess.Provider, m.sess.Model)
	}
	if m.agent.Mode() != agent.ModeAct || m.agent.Tools().PermissionMode() != tools.PermissionModeAct || m.footer.Mode != "" || m.planPending {
		t.Fatalf("/approve did not enter act mode: new agent=%q toolMode=%q footer=%q pending=%v", m.agent.Mode(), m.agent.Tools().PermissionMode(), m.footer.Mode, m.planPending)
	}
	msgs := m.sess.PathToActive()
	if len(msgs) != 1 || msgs[0].Role != ai.RoleUser {
		t.Fatalf("new session messages = %#v, want one user message", msgs)
	}
	text, ok := msgs[0].Content[0].(ai.TextBlock)
	if !ok || text.Text != "1. Edit the file\n2. Run tests" {
		t.Fatalf("approved plan prompt = %#v", msgs[0].Content)
	}
	if !m.streaming {
		t.Fatal("/approve should mark implementation turn as streaming")
	}
}

func TestRoot_ApproveWithoutAssistantPlanDoesNotSwapOrStart(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.handleSlashCommand("/plan")

	oldID := m.sess.ID
	cmd := m.handleSlashCommand("/approve")
	if cmd != nil {
		t.Fatal("/approve without an assistant plan should not start a turn")
	}
	if m.sess.ID != oldID || a.Mode() != agent.ModePlan || !m.planPending {
		t.Fatalf("/approve without plan changed state: session %s->%s mode=%q pending=%v", oldID, m.sess.ID, a.Mode(), m.planPending)
	}
	if got := m.msgs.Render(m.styles, false, false, time.Now()); !strings.Contains(got, "no assistant plan") {
		t.Fatalf("missing no-plan notice: %q", got)
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

	m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	if strings.Contains(m.View(), "Plan Mode: edits and bash disabled") {
		t.Fatalf("plan mode banner remained after Shift+Tab:\n%s", m.View())
	}
	if !strings.Contains(m.View(), "[copy mode]") {
		t.Fatalf("Shift+Tab from plan should enter copy mode:\n%s", m.View())
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

func TestRoot_RebuildMessagesFromLoadedSession(t *testing.T) {
	s := session.New("gpt-5")
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "saved prompt"}}})
	s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "saved answer"}}})
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.rebuildMessagesFromSession()

	view := m.msgs.Render(m.styles, false, false, time.Time{})
	if !strings.Contains(view, "saved prompt") || !strings.Contains(view, "saved answer") {
		t.Fatalf("expected saved transcript in messages:\n%s", view)
	}
}

func TestRoot_ResumeListsOnlyCurrentCWD(t *testing.T) {
	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	project := filepath.Join(t.TempDir(), "project")
	otherProject := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(otherProject, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(project); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	currentCwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	current := session.New("gpt-5")
	current.Name = "current project"
	current.Cwd = currentCwd
	current.SetPath(filepath.Join(config.SessionsDir(), current.ID+".json"))
	current.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "current"}}})
	if err := current.Save(); err != nil {
		t.Fatal(err)
	}

	other := session.New("gpt-5")
	other.Name = "other project"
	other.Cwd = otherProject
	other.SetPath(filepath.Join(config.SessionsDir(), other.ID+".json"))
	other.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "other"}}})
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}

	legacy := session.New("gpt-5")
	legacy.Name = "legacy global"
	legacy.SetPath(filepath.Join(config.SessionsDir(), legacy.ID+".json"))
	legacy.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "legacy"}}})
	if err := legacy.Save(); err != nil {
		t.Fatal(err)
	}

	s := session.New("gpt-5")
	s.Cwd = currentCwd
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.handleSlashCommand("/resume")

	if m.modal == nil {
		t.Fatal("expected resume modal")
	}
	view := m.modal.View(80, 24)
	if !strings.Contains(view, "current project") {
		t.Fatalf("expected current project session in resume modal:\n%s", view)
	}
	if strings.Contains(view, "other project") || strings.Contains(view, "legacy global") {
		t.Fatalf("resume modal included non-current sessions:\n%s", view)
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

func TestAutoSessionName_ExtractsFeatureRequest(t *testing.T) {
	text := "You are running Rune's /feature-dev workflow for a non-trivial feature request.\n\nFeature request:\nits very hard to tell which previous session i was on"
	if got := autoSessionName(text); got != "its very hard to tell which previous session i was on" {
		t.Fatalf("name = %q", got)
	}
}

func TestAutoSessionName_StripsFileBlocksAndSkipsShellPrompts(t *testing.T) {
	if got := autoSessionName("fix this <file name=\"a.go\">\npackage main\n</file> please"); got != "fix this please" {
		t.Fatalf("name = %q", got)
	}
	if got := autoSessionName("I ran `ls` and it produced:\nAGENTS.md README.md"); got != "" {
		t.Fatalf("shell prompt name = %q", got)
	}
}

func TestRoot_AutoNamesSessionOnFirstTurn(t *testing.T) {
	dir := t.TempDir()
	s := session.New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	a := agent.New(faux.New().Done(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	cmd := m.startTurn("fix resume session naming", nil)
	if cmd == nil {
		t.Fatal("expected start turn command")
	}
	if s.Name != "fix resume session naming" {
		t.Fatalf("session name = %q", s.Name)
	}
	loaded, err := session.Load(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "fix resume session naming" {
		t.Fatalf("loaded name = %q", loaded.Name)
	}
}

func TestRoot_NameSlashCommandSetsSessionName(t *testing.T) {
	dir := t.TempDir()
	s := session.New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hello"}}})
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	m.handleSlashCommand("/name useful resume fix")

	if s.Name != "useful resume fix" {
		t.Fatalf("session name = %q", s.Name)
	}
	loaded, err := session.Load(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Name != "useful resume fix" {
		t.Fatalf("loaded name = %q", loaded.Name)
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

func TestRoot_LoginGroqExplainsAPIKey(t *testing.T) {
	s := session.New("gpt-5.5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	cmd := m.handleSlashCommand("/login groq")

	if cmd != nil {
		t.Fatal("/login groq should not start an async command")
	}
	out := m.msgs.Render(m.styles, false, false, time.Time{})
	if !strings.Contains(out, "Groq uses an API key") {
		t.Fatalf("missing Groq API key guidance: %q", out)
	}
}

func TestRoot_LoginDefaultStartsCodexBrowserFlow(t *testing.T) {
	old := startCodexLoginForTUI
	defer func() { startCodexLoginForTUI = old }()
	started := false
	startCodexLoginForTUI = func(m *RootModel) tea.Cmd {
		started = true
		m.msgs.OnInfo("(starting Codex login — opening your browser)\nIf it does not open, copy this URL:\nhttp://127.0.0.1/auth?code_challenge=test")
		return func() tea.Msg { return loginDoneMsg{provider: providers.Codex} }
	}
	s := session.New("gpt-5.5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	cmd := m.handleSlashCommand("/login")
	if cmd == nil || !started {
		t.Fatal("/login should start an async login command")
	}
	out := m.msgs.Render(m.styles, false, false, time.Time{})
	if !strings.Contains(out, "starting Codex login") || !strings.Contains(out, "If it does not open") {
		t.Fatalf("missing login guidance: %q", out)
	}
	if !strings.Contains(out, "http://127.0.0.1/auth?") || !strings.Contains(out, "code_challenge=") {
		t.Fatalf("missing auth URL: %q", out)
	}
}

func TestRoot_LoginDoneRefreshesCodexProvider(t *testing.T) {
	t.Setenv("RUNE_CODEX_ENDPOINT", "http://127.0.0.1/codex/responses")
	s := session.New("gpt-5.5")
	s.Provider = providers.Codex
	a := agent.New(unavailable.New("not logged in"), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	writeCodexTestCredentials(t)

	_, _ = m.Update(loginDoneMsg{provider: providers.Codex, account: "user@example.com"})

	if providerUnavailable(m.agent.Provider()) {
		t.Fatal("expected active provider to refresh after login")
	}
	out := m.msgs.Render(m.styles, false, false, time.Time{})
	if !strings.Contains(out, "logged in to codex as user@example.com") {
		t.Fatalf("missing success message: %q", out)
	}
}

func TestRoot_LoginDoneActivatesCodexFromNoProvider(t *testing.T) {
	t.Setenv("RUNE_CODEX_ENDPOINT", "http://127.0.0.1/codex/responses")
	s := session.New("")
	a := agent.New(unavailable.New("no active provider configured"), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	writeCodexTestCredentials(t)

	_, _ = m.Update(loginDoneMsg{provider: providers.Codex})

	if providerUnavailable(m.agent.Provider()) {
		t.Fatal("expected /login success to activate Codex from no-provider state")
	}
	if m.sess.Provider != providers.Codex || m.sess.Model == "" {
		t.Fatalf("provider/model = %q/%q, want Codex with model", m.sess.Provider, m.sess.Model)
	}
	m.editor.SetValue("hello")
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.streaming {
		t.Fatal("send after /login should start a turn")
	}
}

func writeCodexTestCredentials(t *testing.T) {
	t.Helper()
	if err := oauth.NewStore(config.AuthPath()).Set("openai-codex", oauth.Credentials{AccessToken: "token", RefreshToken: "refresh", ExpiresAt: time.Now().Add(10 * time.Minute)}); err != nil {
		t.Fatal(err)
	}
}

func TestRoot_AddProviderProfileDefaultsToOllamaWhenNoProvider(t *testing.T) {
	runeDir := t.TempDir()
	t.Setenv("RUNE_DIR", runeDir)
	s := session.New("")
	a := agent.New(unavailable.New("no active provider configured"), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	m.addProviderProfile("Local")

	settings, err := config.LoadSettings(config.SettingsPath())
	if err != nil {
		t.Fatal(err)
	}
	p := config.FindProviderProfile(settings.Profiles, settings.ActiveProfile)
	if p == nil {
		t.Fatalf("active profile %q not found in %+v", settings.ActiveProfile, settings.Profiles)
	}
	if p.Provider != providers.Ollama || settings.Provider != providers.Ollama {
		t.Fatalf("provider/profile = %q/%q, want ollama/ollama", settings.Provider, p.Provider)
	}
}

func TestRoot_NoProviderBlocksSend(t *testing.T) {
	s := session.New("")
	a := agent.New(unavailable.New("no active provider configured"), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.editor.SetValue("hello")

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if len(s.PathToActive()) != 0 {
		t.Fatalf("send without provider should not append to session, got %d messages", len(s.PathToActive()))
	}
	if m.streaming {
		t.Fatal("send without provider should not start streaming")
	}
	if !strings.Contains(m.msgs.Render(m.styles, false, false, time.Time{}), "no active provider configured") {
		t.Fatalf("missing no-provider message: %q", m.msgs.Render(m.styles, false, false, time.Time{}))
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

func TestRoot_ContextOverflowNoticeMentionsAutoCompactRetry(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)

	m.handleEvent(agent.ContextOverflow{})

	out := m.msgs.Render(m.styles, false, false, time.Time{})
	if !strings.Contains(out, "auto-compacting and retrying") {
		t.Fatalf("overflow notice did not mention auto-compacting retry: %q", out)
	}
	if strings.Contains(out, "manual /compact recommended") {
		t.Fatalf("overflow notice still recommends manual compact: %q", out)
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
		// kimi omits "medium" so the global default resolves to none, not an
		// indistinguishable explicit choice.
		"moonshotai/kimi-k2.7-code": {"none", "low", "high"},
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

func TestResolveModelEffort(t *testing.T) {
	cases := []struct {
		name   string
		model  string
		effort string
		want   string
	}{
		{"kimi default medium -> none", "moonshotai/kimi-k2.7-code", "medium", "none"},
		{"kimi explicit high kept", "moonshotai/kimi-k2.7-code", "high", "high"},
		{"kimi explicit low kept", "moonshotai/kimi-k2.7-code", "low", "low"},
		{"kimi none kept", "moonshotai/kimi-k2.7-code", "none", "none"},
		{"gpt medium supported kept", "gpt-5.5", "medium", "medium"},
		{"unconstrained model passes through", "some/other-model", "medium", "medium"},
	}
	for _, tc := range cases {
		if got := ResolveModelEffort(tc.model, tc.effort); got != tc.want {
			t.Fatalf("%s: ResolveModelEffort(%q,%q) = %q, want %q", tc.name, tc.model, tc.effort, got, tc.want)
		}
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

func waitForSubagentStatus(t *testing.T, a *agent.Agent, id, status string) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if got := a.Subagents().Get(id); got != nil && got.Status == status {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("subagent %s did not reach status %q; got %+v", id, status, a.Subagents().Get(id))
}

type blockingTUIProvider struct{}

func (blockingTUIProvider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	out := make(chan ai.Event)
	go func() {
		defer close(out)
		<-ctx.Done()
	}()
	return out, nil
}

func TestRoot_ArcaneActivityDoesNotRenderSideRail(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.msgs.OnInfo("hello")
	m.streaming = true
	m.layout()
	m.refreshViewport()

	out := m.View()
	if strings.Contains(out, "╎ᚱ╎") {
		t.Fatalf("arcane activity rail rendered: %q", out)
	}
	if m.viewport.Width != 80 {
		t.Fatalf("viewport width = %d, want 80", m.viewport.Width)
	}
}

func TestRoot_ArcaneActivityRailHiddenForSimpleAndNarrow(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.msgs.OnInfo("hello")
	m.streaming = true
	m.settings.ActivityMode = "simple"
	m.layout()
	m.refreshViewport()
	if strings.Contains(m.View(), "╎ᚱ╎") {
		t.Fatalf("arcane activity rail rendered in simple mode: %q", m.View())
	}
	if m.viewport.Width != 80 {
		t.Fatalf("viewport width = %d, want 80", m.viewport.Width)
	}

	m.settings.ActivityMode = "arcane"
	m.Update(tea.WindowSizeMsg{Width: 79, Height: 24})
	m.layout()
	m.refreshViewport()
	if strings.Contains(m.View(), "╎ᚱ╎") {
		t.Fatalf("arcane activity rail rendered at narrow width: %q", m.View())
	}
	if m.viewport.Width != 79 {
		t.Fatalf("viewport width = %d, want 79", m.viewport.Width)
	}
}

func TestRoot_ActivityLineShowsSubagentsOnRightWhileStreaming(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m.streaming = true
	started := time.Now().Add(-75 * time.Second)
	m.subagents["subagent_1"] = agent.SubagentEvent{
		Status: agent.SubagentRunning,
		Task: tools.SubagentTask{
			ID: "subagent_1", Name: "repo-plan", AgentType: "general", Status: string(agent.SubagentRunning), CreatedAt: time.Now(),
			StartedAt: &started, InputTokens: 1_000, OutputTokens: 234,
		},
	}

	line := m.renderActivityLine()
	if !strings.Contains(line, "consulting the runes") || !strings.Contains(line, "1 familiar working") || !strings.Contains(line, "1m") || !strings.Contains(line, "1.2k tok") {
		t.Fatalf("combined activity line missing main/familiar indicators: %q", line)
	}
	if strings.Index(line, "consulting the runes") > strings.Index(line, "1 familiar working") {
		t.Fatalf("subagent indicator should render to the right of main activity: %q", line)
	}
}

func TestRoot_SubagentSlashCommandsListAndCancel(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(blockingTUIProvider{}, tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	task, err := a.Subagents().Spawn(context.Background(), tools.SpawnSubagentRequest{Name: "repo-plan", Prompt: "inspect", AgentType: "general", Background: true})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = a.Subagents().Cancel(task.ID) }()
	waitForSubagentStatus(t, a, task.ID, string(agent.SubagentRunning))
	if latest := a.Subagents().Get(task.ID); latest != nil {
		m.subagents[task.ID] = agent.SubagentEvent{Status: agent.SubagentRunning, Task: *latest}
	}

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
	started := created.Add(-2 * time.Minute)
	ev := agent.SubagentEvent{
		Status: agent.SubagentRunning,
		Task: tools.SubagentTask{
			ID: "subagent_1", Name: "repo-plan", AgentType: "general", Status: string(agent.SubagentRunning), CreatedAt: created,
			StartedAt: &started, InputTokens: 2_000, OutputTokens: 345,
		},
	}
	_, _ = m.Update(SubagentEventMsg{Event: ev, Ch: m.subagentCh})
	out := m.msgs.Render(m.styles, false, false, time.Now())
	if !strings.Contains(out, "familiar of repo-plan") || !strings.Contains(out, "working") || !strings.Contains(out, "2.3k tok") {
		t.Fatalf("running familiar not rendered: %q", out)
	}
	if m.activeSubagentCount() != 1 {
		t.Fatalf("activeSubagentCount = %d, want 1", m.activeSubagentCount())
	}
	if !strings.Contains(m.View(), "1 familiar working") || !strings.Contains(m.View(), "2.3k tok") {
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
	if !strings.Contains(out, "returned") || !strings.Contains(out, "2 lines") {
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

func TestRoot_ActivityTickRefreshesRunningSubagentElapsed(t *testing.T) {
	s := session.New("gpt-5")
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	m := NewRootModel(a, s)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})

	started := time.Now().Add(-75 * time.Second)
	ev := agent.SubagentEvent{
		Status: agent.SubagentRunning,
		Task: tools.SubagentTask{
			ID: "subagent_1", Name: "repo-plan", AgentType: "general", Status: string(agent.SubagentRunning), CreatedAt: time.Now(),
			StartedAt: &started,
		},
	}
	_, _ = m.Update(SubagentEventMsg{Event: ev, Ch: m.subagentCh})
	if got := m.viewport.View(); !strings.Contains(got, "1m15s") {
		t.Fatalf("viewport missing initial elapsed: %q", got)
	}

	// Simulate time passing with no new subagent event arriving.
	earlier := time.Now().Add(-10 * time.Minute)
	for i := range m.msgs.blocks {
		if m.msgs.blocks[i].taskID == "subagent_1" {
			m.msgs.blocks[i].task.StartedAt = &earlier
		}
	}
	_, _ = m.Update(activityTickMsg{seq: m.activitySeq})
	if got := m.viewport.View(); !strings.Contains(got, "10m") {
		t.Fatalf("activity tick did not refresh running familiar elapsed: %q", got)
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
	if !strings.Contains(m.View(), "1 familiar working") {
		t.Fatalf("blocked familiar activity indicator not rendered in view: %q", m.View())
	}
}
