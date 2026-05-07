package counter

import "testing"

func TestCounter(t *testing.T) {
	c := New()
	c.Inc("a")
	c.Inc("a")
	c.Inc("b")
	if got := c.Get("a"); got != 2 {
		t.Errorf("Get(a) = %d, want 2", got)
	}
	if got := c.Get("b"); got != 1 {
		t.Errorf("Get(b) = %d, want 1", got)
	}
	if got := c.Get("missing"); got != 0 {
		t.Errorf("Get(missing) = %d, want 0", got)
	}
}
