package codeindex

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// defaultCacheCapacity caps the number of distinct (root, languages) Index
// objects retained in memory. Each entry can be tens of MB on a large repo,
// so a small cap is enough for the realistic case (a user has 1–3 repos
// open) while preventing growth from spurious distinct keys.
const defaultCacheCapacity = 4

type Cache struct {
	mu       sync.Mutex
	capacity int
	order    []string // most-recently-used first
	entries  map[string]*Index
	inflight map[string]*buildCall
	build    func(context.Context, BuildOptions) (*Index, error)
}

type buildCall struct {
	done chan struct{}
	idx  *Index
	err  error
}

var defaultCache = &Cache{capacity: defaultCacheCapacity, entries: map[string]*Index{}}

func DefaultCache() *Cache { return defaultCache }

func BuildCached(ctx context.Context, opts BuildOptions) (*Index, error) {
	return defaultCache.Build(ctx, opts)
}

func (c *Cache) Build(ctx context.Context, opts BuildOptions) (*Index, error) {
	key, err := cacheKey(opts)
	if err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.ensureLocked()
	if idx := c.entries[key]; idx != nil {
		c.touchLocked(key)
		c.mu.Unlock()
		return idx, nil
	}
	if call := c.inflight[key]; call != nil {
		c.mu.Unlock()
		return waitForBuild(ctx, call)
	}
	call := &buildCall{done: make(chan struct{})}
	c.inflight[key] = call
	c.mu.Unlock()

	call.idx, call.err = c.buildIndex(ctx, opts)

	c.mu.Lock()
	if c.inflight[key] == call {
		if call.err == nil && call.idx != nil {
			c.putLocked(key, call.idx)
		}
		delete(c.inflight, key)
	}
	c.mu.Unlock()
	close(call.done)

	return call.idx, call.err
}

func waitForBuild(ctx context.Context, call *buildCall) (*Index, error) {
	select {
	case <-call.done:
		return call.idx, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (c *Cache) buildIndex(ctx context.Context, opts BuildOptions) (*Index, error) {
	c.mu.Lock()
	build := c.build
	c.mu.Unlock()
	if build != nil {
		return build(ctx, opts)
	}
	return NewBuilder().Build(ctx, opts)
}

func (c *Cache) Get(opts BuildOptions) (*Index, bool, error) {
	key, err := cacheKey(opts)
	if err != nil {
		return nil, false, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	idx := c.entries[key]
	if idx != nil {
		c.touchLocked(key)
	}
	return idx, idx != nil, nil
}

func (c *Cache) Set(opts BuildOptions, idx *Index) error {
	key, err := cacheKey(opts)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.ensureLocked()
	c.putLocked(key, idx)
	return nil
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]*Index{}
	c.inflight = map[string]*buildCall{}
	c.order = nil
}

func (c *Cache) ensureLocked() {
	if c.entries == nil {
		c.entries = map[string]*Index{}
	}
	if c.inflight == nil {
		c.inflight = map[string]*buildCall{}
	}
	if c.capacity <= 0 {
		c.capacity = defaultCacheCapacity
	}
}

// putLocked inserts or refreshes key, evicting the least-recently-used entry
// if the cache exceeds capacity. Must be called with c.mu held.
func (c *Cache) putLocked(key string, idx *Index) {
	if _, exists := c.entries[key]; exists {
		c.entries[key] = idx
		c.touchLocked(key)
		return
	}
	c.entries[key] = idx
	c.order = append([]string{key}, c.order...)
	for len(c.order) > c.capacity {
		victim := c.order[len(c.order)-1]
		c.order = c.order[:len(c.order)-1]
		delete(c.entries, victim)
	}
}

// touchLocked promotes key to most-recently-used. Must be called with c.mu held.
func (c *Cache) touchLocked(key string) {
	for i, k := range c.order {
		if k == key {
			if i == 0 {
				return
			}
			c.order = append(append([]string{key}, c.order[:i]...), c.order[i+1:]...)
			return
		}
	}
	c.order = append([]string{key}, c.order...)
}

func cacheKey(opts BuildOptions) (string, error) {
	root := strings.TrimSpace(opts.Root)
	if root == "" {
		root = "."
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	langs := append([]string(nil), opts.Languages...)
	for i := range langs {
		langs[i] = strings.ToLower(strings.TrimSpace(langs[i]))
	}
	sort.Strings(langs)
	return abs + "\x00" + strings.Join(langs, ","), nil
}
