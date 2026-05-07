package racecounter

import (
	"sync"
	"testing"
)

func TestCounterConcurrent(t *testing.T) {
	c := New()
	const n = 1000
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			c.Inc()
		}()
	}
	wg.Wait()
	if got := c.Value(); got != n {
		t.Errorf("Value() = %d, want %d", got, n)
	}
}
