package agent

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

func TestSubagentSupervisor_PersistsCompletedTaskMetadataToSession(t *testing.T) {
	p := faux.New().Reply("## Summary\npersisted result").Done()
	sess := session.New("gpt-test")
	a := New(p, tools.NewRegistry(), sess, "")

	task, err := a.Subagents().Spawn(t.Context(), tools.SpawnSubagentRequest{Name: "inspect", Prompt: "look", Background: false})
	if err != nil {
		t.Fatal(err)
	}
	if task.Status != string(SubagentCompleted) {
		t.Fatalf("status = %q", task.Status)
	}

	persisted := sess.SubagentTasks()
	if len(persisted) != 1 {
		t.Fatalf("persisted len = %d", len(persisted))
	}
	if persisted[0].ID != task.ID || persisted[0].Name != "inspect" || persisted[0].Status != string(SubagentCompleted) {
		t.Fatalf("unexpected persisted task: %+v", persisted[0])
	}
	if persisted[0].Summary != "## Summary\npersisted result" {
		t.Fatalf("summary = %q", persisted[0].Summary)
	}
}

func TestSubagentSupervisor_RestoresHistoricalTasksFromSession(t *testing.T) {
	created := time.Now().Add(-time.Hour).Round(0)
	completed := time.Now().Add(-time.Minute).Round(0)
	sess := session.New("gpt-test")
	sess.SetSubagents([]session.SubagentTask{{
		ID:          "subagent_saved",
		Name:        "saved",
		AgentType:   "general",
		Status:      string(SubagentCompleted),
		CreatedAt:   created,
		CompletedAt: &completed,
		Summary:     "saved summary",
	}})

	a := New(faux.New(), tools.NewRegistry(), sess, "")
	got := a.Subagents().List()
	if len(got) != 1 {
		t.Fatalf("list len = %d", len(got))
	}
	if got[0].ID != "subagent_saved" || got[0].Summary != "saved summary" || got[0].Status != string(SubagentCompleted) {
		t.Fatalf("unexpected restored task: %+v", got[0])
	}
}

func TestSubagentSupervisor_SaveLoadRestoreCompletedTask(t *testing.T) {
	dir := t.TempDir()
	sess := session.New("gpt-test")
	sess.SetPath(filepath.Join(dir, sess.ID+".json"))
	a := New(faux.New().Reply("saved summary").Done(), tools.NewRegistry(), sess, "")

	created, err := a.Subagents().Spawn(t.Context(), tools.SpawnSubagentRequest{Name: "persist", Prompt: "inspect", Background: false})
	if err != nil {
		t.Fatal(err)
	}
	if err := sess.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := session.Load(sess.Path())
	if err != nil {
		t.Fatal(err)
	}
	reloadedAgent := New(faux.New(), tools.NewRegistry(), loaded, "")
	restored := reloadedAgent.Subagents().Get(created.ID)
	if restored == nil {
		t.Fatalf("restored task %s missing", created.ID)
	}
	if restored.Status != string(SubagentCompleted) || restored.Summary != "saved summary" || restored.CompletedAt == nil {
		t.Fatalf("unexpected restored task: %+v", restored)
	}
}

func TestSubagentSupervisor_RestoredIDsDoNotCollideWithNewSpawns(t *testing.T) {
	sess := session.New("gpt-test")
	sess.SetSubagents([]session.SubagentTask{{
		ID:        "subagent_99999",
		Name:      "saved",
		AgentType: "general",
		Status:    string(SubagentCompleted),
		CreatedAt: time.Now().Add(-time.Hour),
		Summary:   "saved",
	}})
	a := New(faux.New().Reply("new summary").Done(), tools.NewRegistry(), sess, "")

	spawned, err := a.Subagents().Spawn(t.Context(), tools.SpawnSubagentRequest{Name: "new", Prompt: "new", Background: false})
	if err != nil {
		t.Fatal(err)
	}
	if spawned.ID == "subagent_99999" || spawned.ID == "subagent_1" {
		t.Fatalf("spawned colliding/regressed ID: %s", spawned.ID)
	}
	if got := a.Subagents().Get("subagent_99999"); got == nil || got.Summary != "saved" {
		t.Fatalf("restored task was overwritten: %+v", got)
	}
}

func TestSubagentSupervisor_RestoresNonTerminalTasksAsCancelled(t *testing.T) {
	for _, status := range []SubagentStatus{SubagentPending, SubagentRunning, SubagentBlocked} {
		t.Run(string(status), func(t *testing.T) {
			sess := session.New("gpt-test")
			sess.SetSubagents([]session.SubagentTask{{
				ID:        "subagent_stale_" + string(status),
				Name:      "stale",
				AgentType: "general",
				Status:    string(status),
				CreatedAt: time.Now().Add(-time.Hour),
			}})

			a := New(faux.New(), tools.NewRegistry(), sess, "")
			got := a.Subagents().List()
			if len(got) != 1 {
				t.Fatalf("list len = %d", len(got))
			}
			if got[0].Status != string(SubagentCancelled) {
				t.Fatalf("status = %q, want cancelled", got[0].Status)
			}
			if got[0].CompletedAt == nil || !strings.Contains(got[0].Error, "session restored") {
				t.Fatalf("unexpected restored stale task: %+v", got[0])
			}
		})
	}
}

func TestSubagentSupervisor_RestoresStaleActiveTaskAsCancelled(t *testing.T) {
	sess := session.New("gpt-test")
	sess.SetSubagents([]session.SubagentTask{{
		ID:        "subagent_running",
		Name:      "running",
		AgentType: "general",
		Status:    string(SubagentRunning),
		CreatedAt: time.Now().Add(-time.Hour),
	}})

	a := New(faux.New(), tools.NewRegistry(), sess, "")
	got := a.Subagents().Get("subagent_running")
	if got == nil {
		t.Fatal("restored task missing")
	}
	if got.Status != string(SubagentCancelled) {
		t.Fatalf("status = %q, want cancelled", got.Status)
	}
	if got.CompletedAt == nil {
		t.Fatal("completed_at not set")
	}
	if !strings.Contains(got.Error, "session restored") {
		t.Fatalf("error = %q", got.Error)
	}
}
