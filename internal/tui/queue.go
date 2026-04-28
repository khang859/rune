package tui

import "github.com/khang859/rune/internal/ai"

type QueueItem struct {
	Text   string
	Images []ai.ImageBlock
}

type Queue struct {
	items []QueueItem
}

func (q *Queue) Push(item QueueItem) { q.items = append(q.items, item) }
func (q *Queue) Pop() (QueueItem, bool) {
	if len(q.items) == 0 {
		return QueueItem{}, false
	}
	s := q.items[0]
	q.items = q.items[1:]
	return s, true
}
func (q *Queue) Len() int { return len(q.items) }
