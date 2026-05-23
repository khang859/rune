package repomap

import "testing"

func TestCacheHitMiss(t *testing.T) {
	c := NewCache(2)
	if _, ok := c.Get("k1"); ok {
		t.Error("empty cache should miss")
	}
	c.Put("k1", "v1")
	if v, ok := c.Get("k1"); !ok || v != "v1" {
		t.Errorf("want v1, got (%q, %v)", v, ok)
	}
}

func TestCacheLRUEviction(t *testing.T) {
	c := NewCache(2)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Get("a")      // make 'a' most-recent
	c.Put("c", "3") // should evict 'b'
	if _, ok := c.Get("b"); ok {
		t.Error("b should have been evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("a should still be present")
	}
	if _, ok := c.Get("c"); !ok {
		t.Error("c should be present")
	}
}
