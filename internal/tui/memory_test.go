package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/memory"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

func TestMaybeStartMemoryUpdateWritesProjectMemory(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	cwd := t.TempDir()
	if err := os.Mkdir(filepath.Join(cwd, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	s := session.New("gpt-5")
	s.Cwd = cwd
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "remember test command"}}})
	s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "ok"}}})
	a := agent.New(faux.New().Reply("- Use go test ./...").Done(), tools.NewRegistry(), s, "")
	a.SetMemoryStore(memory.NewStore(cwd, 25000))
	m := NewRootModel(a, s)

	cmd := m.maybeStartMemoryUpdate()
	if cmd == nil {
		t.Fatal("expected memory update command")
	}
	msg, ok := cmd().(memoryUpdateDoneMsg)
	if !ok {
		t.Fatalf("msg = %T", msg)
	}
	if _, _ = m.Update(msg); m.memoryWriting {
		t.Fatal("memoryWriting still true after completion")
	}
	content, err := a.MemoryStore().Load()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(content, "go test ./...") {
		t.Fatalf("memory = %q", content)
	}
}

func TestAgentDoneWithQueuedItemDoesNotStartDroppedMemoryUpdate(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	cwd := t.TempDir()
	s := session.New("gpt-5")
	s.Cwd = cwd
	s.SetPath(filepath.Join(t.TempDir(), s.ID+".json"))
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "first"}}})
	s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "done"}}})
	a := agent.New(faux.New(), tools.NewRegistry(), s, "")
	a.SetMemoryStore(memory.NewStore(cwd, 25000))
	m := NewRootModel(a, s)
	ch := make(chan agent.Event)
	m.eventCh = ch
	m.queue.Push(QueueItem{Text: "next"})

	_, _ = m.Update(AgentChannelDoneMsg{Ch: ch})
	if m.memoryWriting {
		t.Fatal("memoryWriting should not be set when queued turn starts instead")
	}
}

func TestMemoryOffBeforeCompletionPreventsWrite(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	cwd := t.TempDir()
	s := session.New("gpt-5")
	s.Cwd = cwd
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "remember test command"}}})
	s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "ok"}}})
	a := agent.New(faux.New().Reply("- Use go test ./...").Done(), tools.NewRegistry(), s, "")
	a.SetMemoryStore(memory.NewStore(cwd, 25000))
	m := NewRootModel(a, s)

	cmd := m.maybeStartMemoryUpdate()
	msg := cmd().(memoryUpdateDoneMsg)
	m.agent.SetMemoryEnabled(false)
	_, _ = m.Update(msg)
	content, err := a.MemoryStore().Load()
	if err != nil {
		t.Fatal(err)
	}
	if content != "" {
		t.Fatalf("memory should not be written after disable, got %q", content)
	}
}

func TestStaleMemoryCompletionAfterSessionSwapDoesNotBlockFutureUpdates(t *testing.T) {
	t.Setenv("RUNE_DIR", t.TempDir())
	cwd := t.TempDir()
	s := session.New("gpt-5")
	s.Cwd = cwd
	s.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "first"}}})
	s.Append(ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "done"}}})
	a := agent.New(faux.New().Reply("- first memory").Done(), tools.NewRegistry(), s, "")
	a.SetMemoryStore(memory.NewStore(cwd, 25000))
	m := NewRootModel(a, s)
	cmd := m.maybeStartMemoryUpdate()

	next := session.New("gpt-5")
	next.Cwd = cwd
	m.swapSession(next)
	_, _ = m.Update(cmd().(memoryUpdateDoneMsg))
	if m.memoryWriting {
		t.Fatal("stale memory completion should not leave memoryWriting set")
	}
}
