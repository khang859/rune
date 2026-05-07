package counter

type Counter struct {
	counts map[string]int
}

func New() *Counter {
	return &Counter{}
}

func (c *Counter) Inc(key string) {
	c.counts[key]++
}

func (c *Counter) Get(key string) int {
	return c.counts[key]
}
