package repomap

import (
	"sort"
	"strings"

	"github.com/khang859/rune/internal/codeindex"
)

// RenderItem is a (file, symbol) pair to render. Order matters: callers pass
// items in priority order; RenderBudgeted binary-searches the prefix that fits.
type RenderItem struct {
	File   string
	Symbol *codeindex.Symbol
}

// estimateTokens is the placeholder tokenizer until we wire provider-specific
// tokenizers. ~4 chars per token is a reasonable cross-tokenizer average.
func estimateTokens(s string) int {
	return (len(s) + 3) / 4
}

// RenderBudgeted renders the largest prefix of items that fits in maxTokens.
// Symbols are grouped by file in input order; within a group, symbols are
// rendered in their original priority order (caller decides).
func RenderBudgeted(items []RenderItem, maxTokens int) string {
	if len(items) == 0 || maxTokens <= 0 {
		return ""
	}

	lower, upper := 0, len(items)
	middle := upper
	var bestOut string
	var bestTokens int
	const okPctErr = 0.15

	for lower <= upper {
		out := renderPrefix(items[:middle])
		tokens := estimateTokens(out)
		within := tokens <= maxTokens
		if (within && tokens > bestTokens) || pctErr(tokens, maxTokens) < okPctErr {
			bestOut = out
			bestTokens = tokens
			if pctErr(tokens, maxTokens) < okPctErr {
				break
			}
		}
		if tokens < maxTokens {
			lower = middle + 1
		} else {
			upper = middle - 1
		}
		middle = (lower + upper) / 2
		if middle <= 0 {
			break
		}
	}
	return bestOut
}

func pctErr(got, want int) float64 {
	diff := got - want
	if diff < 0 {
		diff = -diff
	}
	return float64(diff) / float64(want)
}

func renderPrefix(items []RenderItem) string {
	if len(items) == 0 {
		return ""
	}
	// Group by file, preserving first-seen order.
	type group struct {
		file string
		syms []*codeindex.Symbol
	}
	order := []string{}
	groups := map[string]*group{}
	for _, it := range items {
		g, ok := groups[it.File]
		if !ok {
			g = &group{file: it.File}
			groups[it.File] = g
			order = append(order, it.File)
		}
		g.syms = append(g.syms, it.Symbol)
	}
	for _, g := range groups {
		sort.SliceStable(g.syms, func(i, j int) bool {
			return g.syms[i].StartLine < g.syms[j].StartLine
		})
	}

	var b strings.Builder
	for _, file := range order {
		b.WriteString(file)
		b.WriteString(":\n")
		for _, sym := range groups[file].syms {
			b.WriteString("  ")
			if sym.Signature != "" {
				b.WriteString(sym.Signature)
			} else {
				b.WriteString(string(sym.Kind))
				b.WriteString(" ")
				b.WriteString(sym.Name)
			}
			b.WriteString("\n")
		}
	}
	return b.String()
}
