package racecounter

type Counter struct {
	n int
}

func New() *Counter {
	return &Counter{}
}

func (c *Counter) Inc() {
	c.n++
}

func (c *Counter) Value() int {
	return c.n
}
