package codeindex

import (
	"context"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Cache struct {
	mu      sync.Mutex
	entries map[string]*Index
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
	if c.entries == nil {
		c.entries = map[string]*Index{}
	}
	if idx := c.entries[key]; idx != nil {
		c.mu.Unlock()
		return idx, nil
	}
	c.mu.Unlock()

	idx, err := NewBuilder().Build(ctx, opts)
	if err != nil {
		return nil, err
	}
	c.Set(opts, idx)
	return idx, nil
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
	if c.entries == nil {
		c.entries = map[string]*Index{}
	}
	c.entries[key] = idx
	return nil
}

func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = map[string]*Index{}
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
