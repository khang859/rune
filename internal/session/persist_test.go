package session

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai"
)

func TestSave_AndLoad_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	s := New("gpt-5")
	s.Name = "demo"
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	s.Append(userMsg("hi"))
	s.Append(asstMsg("hello"))

	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	loaded, err := Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ID != s.ID {
		t.Fatalf("id mismatch")
	}
	if loaded.Name != "demo" {
		t.Fatalf("name = %q", loaded.Name)
	}
	if got := len(loaded.PathToActive()); got != 2 {
		t.Fatalf("path len = %d", got)
	}
	// Parent pointers must be reconstructed.
	if loaded.Active.Parent == nil || loaded.Active.Parent.Parent == nil {
		t.Fatal("parent pointers not reconstructed")
	}
	if loaded.Active.Parent.Parent != loaded.Root {
		t.Fatal("parent pointers do not chain to root")
	}
}

func TestSave_AndLoad_PreservesOllamaProvider(t *testing.T) {
	dir := t.TempDir()
	s := New("qwen3:4b")
	s.Provider = "ollama"
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	s.Append(userMsg("hi"))
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Provider != "ollama" || loaded.Model != "qwen3:4b" {
		t.Fatalf("loaded provider/model = %s/%s", loaded.Provider, loaded.Model)
	}
}

func TestSave_AndLoad_PreservesCompactedCount(t *testing.T) {
	dir := t.TempDir()
	s := New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	s.Append(userMsg("hi"))
	n := s.Append(asstMsg("compact summary"))
	n.CompactedCount = 7

	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	nodes := loaded.PathToActiveNodes()
	if len(nodes) != 2 {
		t.Fatalf("nodes len = %d", len(nodes))
	}
	if got := nodes[1].CompactedCount; got != 7 {
		t.Fatalf("CompactedCount = %d after round-trip, want 7", got)
	}
}

func TestSave_AndLoad_PreservesSubagentTaskMetadata(t *testing.T) {
	dir := t.TempDir()
	s := New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	created := time.Now().Add(-time.Hour).Round(0)
	started := time.Now().Add(-30 * time.Minute).Round(0)
	completed := time.Now().Add(-time.Minute).Round(0)
	s.SetSubagents([]SubagentTask{
		{
			ID:           "subagent_1",
			Name:         "inspect",
			AgentType:    "general",
			Status:       "completed",
			Dependencies: []string{"subagent_0"},
			CreatedAt:    created,
			StartedAt:    &started,
			CompletedAt:  &completed,
			Summary:      "done",
		},
		{
			ID:        "subagent_2",
			Name:      "review",
			AgentType: "general",
			Status:    "failed",
			CreatedAt: created.Add(time.Minute),
			Error:     "boom",
		},
	})

	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(s.path)
	if err != nil {
		t.Fatal(err)
	}
	got := loaded.SubagentTasks()
	if len(got) != 2 {
		t.Fatalf("subagents len = %d", len(got))
	}
	if got[0].ID != "subagent_1" || got[0].Name != "inspect" || got[0].AgentType != "general" || got[0].Status != "completed" || got[0].Summary != "done" {
		t.Fatalf("unexpected subagent[0]: %+v", got[0])
	}
	if len(got[0].Dependencies) != 1 || got[0].Dependencies[0] != "subagent_0" {
		t.Fatalf("dependencies = %#v", got[0].Dependencies)
	}
	if !got[0].CreatedAt.Equal(created) || got[0].StartedAt == nil || !got[0].StartedAt.Equal(started) || got[0].CompletedAt == nil || !got[0].CompletedAt.Equal(completed) {
		t.Fatalf("timestamps not preserved: %+v", got[0])
	}
	if got[1].ID != "subagent_2" || got[1].Status != "failed" || got[1].Error != "boom" {
		t.Fatalf("unexpected subagent[1]: %+v", got[1])
	}
}

func TestSession_SubagentTasksAreCopied(t *testing.T) {
	s := New("gpt-5")
	created := time.Now().Round(0)
	started := created.Add(time.Second)
	deps := []string{"subagent_dep"}
	tasks := []SubagentTask{{ID: "subagent_1", Name: "one", AgentType: "general", Status: "running", Dependencies: deps, CreatedAt: created, StartedAt: &started}}
	s.SetSubagents(tasks)
	wantStarted := started
	deps[0] = "mutated"
	tasks[0].Name = "mutated"
	*tasks[0].StartedAt = started.Add(time.Hour)

	got := s.SubagentTasks()
	if got[0].Name != "one" || got[0].Dependencies[0] != "subagent_dep" || got[0].StartedAt == nil || got[0].StartedAt.UnixNano() != wantStarted.UnixNano() {
		t.Fatalf("SetSubagents did not copy input: %+v", got[0])
	}
	got[0].Name = "mutated again"
	got[0].Dependencies[0] = "mutated again"
	*got[0].StartedAt = started.Add(2 * time.Hour)
	again := s.SubagentTasks()
	if again[0].Name != "one" || again[0].Dependencies[0] != "subagent_dep" || again[0].StartedAt == nil || again[0].StartedAt.UnixNano() != wantStarted.UnixNano() {
		t.Fatalf("SubagentTasks did not copy output: %+v", again[0])
	}
}

func TestSave_IsAtomic(t *testing.T) {
	// After Save, the temp file must be gone — only the final file exists.
	dir := t.TempDir()
	s := New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	s.Append(userMsg("hi"))
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 file, got %d", len(entries))
	}
}

// keep ai import alive for test compilation
var _ = ai.RoleUser
