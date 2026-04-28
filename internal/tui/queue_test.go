package tui

import "testing"

func TestQueue_PushAndPop(t *testing.T) {
	q := &Queue{}
	q.Push("a")
	q.Push("b")
	if got, ok := q.Pop(); !ok || got != "a" {
		t.Fatalf("pop = %q, %v", got, ok)
	}
	if got, ok := q.Pop(); !ok || got != "b" {
		t.Fatalf("pop = %q, %v", got, ok)
	}
	if _, ok := q.Pop(); ok {
		t.Fatal("expected empty")
	}
}

func TestQueue_Len(t *testing.T) {
	q := &Queue{}
	q.Push("x")
	q.Push("y")
	if q.Len() != 2 {
		t.Fatalf("len = %d", q.Len())
	}
}
