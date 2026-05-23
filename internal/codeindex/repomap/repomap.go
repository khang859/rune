package repomap

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/khang859/rune/internal/codeindex"
)

type Options struct {
	MaxTokens     int
	NoFocusBudget int
	CapPerFile    int
	TopFiles      int
}

const (
	defaultMaxTokens     = 2000
	defaultNoFocusBudget = 8000
	defaultCapPerFile    = 20
	defaultTopFiles      = 40
	defaultCacheSize     = 4
)

var (
	defaultCache   *Cache
	defaultCacheMu sync.Mutex
)

func getCache() *Cache {
	defaultCacheMu.Lock()
	defer defaultCacheMu.Unlock()
	if defaultCache == nil {
		defaultCache = NewCache(defaultCacheSize)
	}
	return defaultCache
}

func cacheKey(idxRoot string, focus Focus, budget int) string {
	files := append([]string(nil), focus.InFocusFiles...)
	sort.Strings(files)
	idents := make([]string, 0, len(focus.MentionedIdents))
	for k := range focus.MentionedIdents {
		idents = append(idents, k)
	}
	sort.Strings(idents)
	h := sha1.New()
	h.Write([]byte(idxRoot))
	h.Write([]byte{0})
	h.Write([]byte(strconv.Itoa(budget)))
	for _, f := range files {
		h.Write([]byte{0})
		h.Write([]byte(f))
	}
	h.Write([]byte{1})
	for _, id := range idents {
		h.Write([]byte{0})
		h.Write([]byte(id))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// Build assembles a token-budgeted repo map for the agent's system prompt.
// Returns "" (no error) when the index is nil/empty, when budget is 0, or
// when no edges resolve. Never fails a turn.
func Build(_ context.Context, idx *codeindex.Index, focus Focus, opts Options) (string, error) {
	if idx == nil {
		return "", nil
	}
	if opts.MaxTokens == 0 {
		// 0 is "use default"; -1 (or any negative) is "disabled".
		opts.MaxTokens = defaultMaxTokens
	}
	if opts.MaxTokens < 0 {
		return "", nil
	}
	if opts.NoFocusBudget <= 0 {
		opts.NoFocusBudget = defaultNoFocusBudget
	}
	if opts.CapPerFile <= 0 {
		opts.CapPerFile = defaultCapPerFile
	}
	if opts.TopFiles <= 0 {
		opts.TopFiles = defaultTopFiles
	}

	budget := opts.MaxTokens
	if len(focus.InFocusFiles) == 0 {
		budget = opts.NoFocusBudget
	}

	key := cacheKey(idx.Root, focus, budget)
	if v, ok := getCache().Get(key); ok {
		return v, nil
	}

	nodes, edges := ProjectFileGraph(idx, focus)
	if len(nodes) == 0 || len(edges) == 0 {
		return "", nil
	}

	pers := map[string]float64{}
	for _, f := range focus.InFocusFiles {
		pers[f] += 1.0
	}
	// Path-component match against mentioned idents (Aider's heuristic).
	for ident := range focus.MentionedIdents {
		for _, file := range nodes {
			if pathContains(file, ident) {
				pers[file] += 1.0
			}
		}
	}

	scores := PageRank(nodes, edges, pers)
	type rankedFile struct {
		file  string
		score float64
	}
	rf := make([]rankedFile, 0, len(scores))
	for f, s := range scores {
		rf = append(rf, rankedFile{file: f, score: s})
	}
	sort.Slice(rf, func(i, j int) bool { return rf[i].score > rf[j].score })
	if len(rf) > opts.TopFiles {
		rf = rf[:opts.TopFiles]
	}

	items := []RenderItem{}
	for _, r := range rf {
		for _, sym := range SelectSymbolsForFile(idx, r.file, focus, opts.CapPerFile) {
			items = append(items, RenderItem{File: r.file, Symbol: sym})
		}
	}
	out := RenderBudgeted(items, budget)
	getCache().Put(key, out)
	return out, nil
}

func pathContains(path, needle string) bool {
	return strings.Contains(strings.ToLower(path), strings.ToLower(needle))
}
