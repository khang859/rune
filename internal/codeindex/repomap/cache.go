package repomap

import "container/list"

type Cache struct {
	cap   int
	items map[string]*list.Element
	order *list.List
}

type cacheEntry struct {
	key string
	val string
}

func NewCache(capacity int) *Cache {
	if capacity < 1 {
		capacity = 1
	}
	return &Cache{
		cap:   capacity,
		items: map[string]*list.Element{},
		order: list.New(),
	}
}

func (c *Cache) Get(key string) (string, bool) {
	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.order.MoveToFront(el)
	return el.Value.(*cacheEntry).val, true
}

func (c *Cache) Put(key, val string) {
	if el, ok := c.items[key]; ok {
		el.Value.(*cacheEntry).val = val
		c.order.MoveToFront(el)
		return
	}
	el := c.order.PushFront(&cacheEntry{key: key, val: val})
	c.items[key] = el
	if c.order.Len() > c.cap {
		back := c.order.Back()
		if back != nil {
			delete(c.items, back.Value.(*cacheEntry).key)
			c.order.Remove(back)
		}
	}
}
