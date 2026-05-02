package session

import (
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func userMsg(text string) ai.Message {
	return ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}}
}

func asstMsg(text string) ai.Message {
	return ai.Message{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}}
}

func TestSession_AppendBuildsLinearPath(t *testing.T) {
	s := New("gpt-5")
	s.Append(userMsg("hi"))
	s.Append(asstMsg("hello"))
	s.Append(userMsg("again"))
	path := s.PathToActive()
	if len(path) != 3 {
		t.Fatalf("path len = %d", len(path))
	}
	if got := path[0].Content[0].(ai.TextBlock).Text; got != "hi" {
		t.Fatalf("path[0] text = %q", got)
	}
	if got := path[2].Content[0].(ai.TextBlock).Text; got != "again" {
		t.Fatalf("path[2] text = %q", got)
	}
}

func TestSession_ForkCreatesBranch(t *testing.T) {
	s := New("gpt-5")
	s.Append(userMsg("a"))
	a := s.Active
	s.Append(asstMsg("first reply"))
	s.Append(userMsg("b1"))
	// Fork back to "a" and add a different child.
	s.Fork(a)
	s.Append(asstMsg("second reply"))
	if len(a.Children) != 2 {
		t.Fatalf("a.Children len = %d", len(a.Children))
	}
	path := s.PathToActive()
	last := path[len(path)-1].Content[0].(ai.TextBlock).Text
	if last != "second reply" {
		t.Fatalf("last on active branch = %q", last)
	}
}

func TestSession_CloneCopiesActiveBranch(t *testing.T) {
	s := New("gpt-5")
	s.Cwd = "/tmp/project"
	s.Append(userMsg("a"))
	s.Append(asstMsg("b"))
	c := s.Clone()
	if c.ID == s.ID {
		t.Fatal("cloned session must have a different ID")
	}
	if len(c.PathToActive()) != 2 {
		t.Fatalf("cloned path len = %d", len(c.PathToActive()))
	}
	if c.Cwd != "/tmp/project" {
		t.Fatalf("cloned cwd = %q", c.Cwd)
	}
	// Mutating original must not affect clone.
	s.Append(userMsg("c"))
	if len(c.PathToActive()) != 2 {
		t.Fatal("clone aliased to original")
	}
}
