package tui

type Queue struct {
	items []string
}

func (q *Queue) Push(s string) { q.items = append(q.items, s) }
func (q *Queue) Pop() (string, bool) {
	if len(q.items) == 0 {
		return "", false
	}
	s := q.items[0]
	q.items = q.items[1:]
	return s, true
}
func (q *Queue) Len() int { return len(q.items) }
