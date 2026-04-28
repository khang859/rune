// internal/session/browse_test.go
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestListSessions_ReadsSummaries(t *testing.T) {
	dir := t.TempDir()
	s := New("gpt-5")
	s.Name = "demo"
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
	if summaries[0].MessageCount < 1 {
		t.Fatalf("message_count = %d", summaries[0].MessageCount)
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
