package codeindex

import (
	"fmt"
	"sort"
	"strings"
)

func FormatSummary(idx *Index, maxSymbols int) string {
	if maxSymbols <= 0 {
		maxSymbols = 300
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Indexed %d files, %d symbols.\n", len(idx.Files), len(idx.Symbols))
	count := 0
	for _, file := range idx.SortedFiles() {
		if len(file.Symbols) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s [%s]\n", file.Path, file.Language)
		syms := append([]string(nil), file.Symbols...)
		sort.Slice(syms, func(i, j int) bool {
			return idx.Symbols[syms[i]].StartLine < idx.Symbols[syms[j]].StartLine
		})
		for _, id := range syms {
			if count >= maxSymbols {
				fmt.Fprintf(&b, "\n[showing first %d symbols. Narrow path/languages or increase max_symbols.]\n", maxSymbols)
				return strings.TrimSpace(b.String())
			}
			sym := idx.Symbols[id]
			fmt.Fprintf(&b, "  %s %s:%d", sym.Kind, sym.Qualified, sym.StartLine)
			if sym.Signature != "" {
				fmt.Fprintf(&b, " — %s", sym.Signature)
			}
			b.WriteByte('\n')
			count++
		}
	}
	return strings.TrimSpace(b.String())
}

func FindSymbols(idx *Index, query, kind string, limit int) []*Symbol {
	query = strings.ToLower(strings.TrimSpace(query))
	kind = strings.ToLower(strings.TrimSpace(kind))
	if limit <= 0 {
		limit = 20
	}
	var matches []*Symbol
	for _, sym := range idx.Symbols {
		if kind != "" && string(sym.Kind) != kind {
			continue
		}
		haystack := strings.ToLower(sym.Qualified + " " + sym.Name + " " + sym.Signature + " " + sym.File)
		if query == "" || strings.Contains(haystack, query) || wordsMatch(haystack, query) {
			matches = append(matches, sym)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		si, sj := scoreSymbol(matches[i], query), scoreSymbol(matches[j], query)
		if si != sj {
			return si > sj
		}
		if matches[i].File != matches[j].File {
			return matches[i].File < matches[j].File
		}
		return matches[i].StartLine < matches[j].StartLine
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

func FormatSymbolList(symbols []*Symbol) string {
	if len(symbols) == 0 {
		return "(no symbols found)"
	}
	var b strings.Builder
	for i, sym := range symbols {
		fmt.Fprintf(&b, "%d. %s %s\n", i+1, sym.Kind, sym.Qualified)
		fmt.Fprintf(&b, "   %s:%d-%d\n", sym.File, sym.StartLine, sym.EndLine)
		fmt.Fprintf(&b, "   id: %s\n", sym.ID)
		if sym.Signature != "" {
			fmt.Fprintf(&b, "   %s\n", sym.Signature)
		}
	}
	return strings.TrimSpace(b.String())
}

func FormatSymbolContext(idx *Index, id string, callers, callees bool) string {
	sym := idx.Symbols[id]
	if sym == nil {
		return "symbol not found"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Symbol:\n  %s %s\n  %s:%d-%d\n  id: %s\n", sym.Kind, sym.Qualified, sym.File, sym.StartLine, sym.EndLine, sym.ID)
	if sym.Signature != "" {
		fmt.Fprintf(&b, "  %s\n", sym.Signature)
	}
	if callees {
		b.WriteString("\nOutgoing:\n")
		writeEdges(&b, idx, idx.Graph.Out(id))
	}
	if callers {
		b.WriteString("\nIncoming:\n")
		writeEdges(&b, idx, idx.Graph.In(id))
	}
	return strings.TrimSpace(b.String())
}

func FormatNeighbors(idx *Index, id string, depth, limit int) string {
	if depth <= 0 {
		depth = 1
	}
	if depth > 3 {
		depth = 3
	}
	if limit <= 0 {
		limit = 50
	}
	var b strings.Builder
	seen := map[string]bool{id: true}
	frontier := []string{id}
	count := 0
	for d := 0; d < depth; d++ {
		var next []string
		for _, cur := range frontier {
			for _, e := range append(idx.Graph.Out(cur), idx.Graph.In(cur)...) {
				if count >= limit {
					fmt.Fprintf(&b, "[showing first %d edges]\n", limit)
					return strings.TrimSpace(b.String())
				}
				fmt.Fprintf(&b, "%s --%s--> %s", label(idx, e.From), e.Relation, label(idx, e.To))
				if e.Label != "" {
					fmt.Fprintf(&b, " (%s)", e.Label)
				}
				b.WriteByte('\n')
				count++
				other := e.To
				if other == cur {
					other = e.From
				}
				if !seen[other] {
					seen[other] = true
					next = append(next, other)
				}
			}
		}
		frontier = next
	}
	if count == 0 {
		return "(no graph neighbors found)"
	}
	return strings.TrimSpace(b.String())
}

func writeEdges(b *strings.Builder, idx *Index, edges []Edge) {
	if len(edges) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	for _, e := range edges {
		fmt.Fprintf(b, "  %s → %s", e.Relation, label(idx, e.To))
		if e.Label != "" {
			fmt.Fprintf(b, " (%s)", e.Label)
		}
		b.WriteByte('\n')
	}
}

func label(idx *Index, id string) string {
	if sym := idx.Symbols[id]; sym != nil {
		return sym.Qualified
	}
	if strings.HasPrefix(id, "file:") || strings.HasPrefix(id, "import:") || strings.HasPrefix(id, "name:") {
		return id
	}
	return id
}

func wordsMatch(haystack, query string) bool {
	for _, word := range strings.Fields(query) {
		if !strings.Contains(haystack, word) {
			return false
		}
	}
	return true
}

func scoreSymbol(sym *Symbol, query string) int {
	if query == "" {
		return 0
	}
	name := strings.ToLower(sym.Name)
	qualified := strings.ToLower(sym.Qualified)
	sig := strings.ToLower(sym.Signature)
	switch {
	case name == query:
		return 100
	case qualified == query:
		return 90
	case strings.Contains(name, query):
		return 75
	case strings.Contains(qualified, query):
		return 60
	case strings.Contains(sig, query):
		return 40
	case wordsMatch(strings.ToLower(sym.File+" "+sym.Qualified+" "+sym.Signature), query):
		return 30
	default:
		return 1
	}
}
