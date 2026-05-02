// internal/session/browse_test.go
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListSessions_ReadsSummaries(t *testing.T) {
	dir := t.TempDir()
	s := New("gpt-5")
	s.Name = "demo"
	s.Cwd = filepath.Join(dir, "project")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	s.Append(userMsg("hi"))
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	summaries, err := ListSessions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len = %d", len(summaries))
	}
	if summaries[0].Name != "demo" {
		t.Fatalf("name = %q", summaries[0].Name)
	}
	if summaries[0].ID != s.ID {
		t.Fatalf("id = %q", summaries[0].ID)
	}
	if summaries[0].Preview != "hi" {
		t.Fatalf("preview = %q", summaries[0].Preview)
	}
	if summaries[0].Updated.IsZero() {
		t.Fatal("updated time is zero")
	}
	if summaries[0].MessageCount < 1 {
		t.Fatalf("message_count = %d", summaries[0].MessageCount)
	}
	if summaries[0].Cwd != filepath.Join(dir, "project") {
		t.Fatalf("cwd = %q", summaries[0].Cwd)
	}
}

func TestListSessionsForCWD_FiltersByExactCWD(t *testing.T) {
	dir := t.TempDir()
	project := filepath.Join(dir, "project")
	otherProject := filepath.Join(dir, "other")

	match := New("gpt-5")
	match.Name = "match"
	match.Cwd = project
	match.SetPath(filepath.Join(dir, match.ID+".json"))
	match.Append(userMsg("matching project"))
	if err := match.Save(); err != nil {
		t.Fatal(err)
	}

	other := New("gpt-5")
	other.Name = "other"
	other.Cwd = otherProject
	other.SetPath(filepath.Join(dir, other.ID+".json"))
	other.Append(userMsg("other project"))
	if err := other.Save(); err != nil {
		t.Fatal(err)
	}

	legacy := New("gpt-5")
	legacy.Name = "legacy"
	legacy.SetPath(filepath.Join(dir, legacy.ID+".json"))
	legacy.Append(userMsg("legacy project"))
	if err := legacy.Save(); err != nil {
		t.Fatal(err)
	}

	summaries, err := ListSessionsForCWD(dir, project)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("len = %d", len(summaries))
	}
	if summaries[0].ID != match.ID {
		t.Fatalf("id = %q, want %q", summaries[0].ID, match.ID)
	}
}

func TestListSessions_SortsByUpdatedTime(t *testing.T) {
	dir := t.TempDir()
	oldSession := New("gpt-5")
	oldSession.SetPath(filepath.Join(dir, oldSession.ID+".json"))
	oldSession.Append(userMsg("old"))
	if err := oldSession.Save(); err != nil {
		t.Fatal(err)
	}

	newSession := New("gpt-5")
	newSession.SetPath(filepath.Join(dir, newSession.ID+".json"))
	newSession.Append(userMsg("new"))
	if err := newSession.Save(); err != nil {
		t.Fatal(err)
	}

	oldTime := time.Date(2026, 4, 29, 12, 0, 0, 0, time.Local)
	newTime := oldTime.Add(time.Hour)
	if err := os.Chtimes(oldSession.Path(), oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(newSession.Path(), newTime, newTime); err != nil {
		t.Fatal(err)
	}

	summaries, err := ListSessions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("len = %d", len(summaries))
	}
	if summaries[0].ID != newSession.ID {
		t.Fatalf("first id = %q, want %q", summaries[0].ID, newSession.ID)
	}
}

func TestListSessions_PreviewIsFirstUserMessage(t *testing.T) {
	dir := t.TempDir()
	s := New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	s.Append(asstMsg("assistant first"))
	s.Append(userMsg("  please   fix\nresume sessions  "))
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	summaries, err := ListSessions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := summaries[0].Preview; got != "please fix resume sessions" {
		t.Fatalf("preview = %q", got)
	}
}

func TestListSessions_PreviewUsesActiveBranch(t *testing.T) {
	dir := t.TempDir()
	s := New("gpt-5")
	s.SetPath(filepath.Join(dir, s.ID+".json"))
	s.Append(userMsg("inactive branch prompt"))
	s.Append(asstMsg("inactive branch answer"))
	s.Fork(s.Root)
	s.Append(userMsg("active branch prompt"))
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	summaries, err := ListSessions(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := summaries[0].Preview; got != "active branch prompt" {
		t.Fatalf("preview = %q", got)
	}
}

func TestListSessions_SkipsBadFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not json"), 0o644)
	summaries, err := ListSessions(dir)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("expected 0, got %d", len(summaries))
	}
	var unused json.RawMessage
	_ = unused
}
