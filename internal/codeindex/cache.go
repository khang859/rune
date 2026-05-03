package codeindex

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Cache struct {
	mu       sync.Mutex
	entries  map[string]*Index
	inflight map[string]*buildCall
	build    func(context.Context, BuildOptions) (*Index, error)
}

type buildCall struct {
	done chan struct{}
	idx  *Index
	err  error
}

var defaultCache = &Cache{entries: map[string]*Index{}}

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
			c.entries[key] = call.idx
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
	c.entries[key] = idx
	return nil
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]*Index{}
	c.inflight = map[string]*buildCall{}
}

func (c *Cache) ensureLocked() {
	if c.entries == nil {
		c.entries = map[string]*Index{}
	}
	if c.inflight == nil {
		c.inflight = map[string]*buildCall{}
	}
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
