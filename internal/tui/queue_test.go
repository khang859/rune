package tui

import "testing"

func TestQueue_PushAndPop(t *testing.T) {
	q := &Queue{}
	q.Push(QueueItem{Text: "a"})
	q.Push(QueueItem{Text: "b"})
	if got, ok := q.Pop(); !ok || got.Text != "a" {
		t.Fatalf("pop = %q, %v", got.Text, ok)
	}
	if got, ok := q.Pop(); !ok || got.Text != "b" {
		t.Fatalf("pop = %q, %v", got.Text, ok)
	}
	if _, ok := q.Pop(); ok {
		t.Fatal("expected empty")
	}
}

func TestQueue_Len(t *testing.T) {
	q := &Queue{}
	q.Push(QueueItem{Text: "x"})
	q.Push(QueueItem{Text: "y"})
	if q.Len() != 2 {
		t.Fatalf("len = %d", q.Len())
	}
}
