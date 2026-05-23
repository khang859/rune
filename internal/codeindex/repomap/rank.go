package repomap

import (
	"math"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/khang859/rune/internal/codeindex"
)

const (
	weightMulMentioned     = 10.0
	weightMulInterestingID = 10.0
	weightMulPrivate       = 0.1
	weightMulGeneric       = 0.1
	weightMulInFocus       = 50.0
	genericDefThreshold    = 5
	minInterestingIDLen    = 8
)

// absFile joins idx.Root with a relative slash-path stored in the index
// (idx.Files keys and Symbol.File). Repomap emits absolute paths everywhere
// so callers (and Focus.InFocusFiles) can use platform-native absolute paths.
func absFile(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}

// ProjectFileGraph collapses the symbol graph into a file-to-file graph
// suitable for PageRank. Each resolved cross-file call/reference becomes a
// weighted edge from referencer file to definer file, with weights derived
// from Aider's repomap heuristics (see spec section "Edge weighting").
// All node and edge endpoint paths are absolute (joined against idx.Root).
func ProjectFileGraph(idx *codeindex.Index, focus Focus) ([]string, []WeightedEdge) {
	if idx == nil {
		return nil, nil
	}

	inFocus := map[string]bool{}
	for _, f := range focus.InFocusFiles {
		inFocus[f] = true
	}

	// Count how many files each ident is defined in, for the "generic" demotion.
	defsPerIdent := map[string]int{}
	for _, sym := range idx.Symbols {
		defsPerIdent[sym.Name]++
	}

	type edgeKey struct {
		from, to, ident string
	}
	counts := map[edgeKey]int{}
	for _, e := range idx.Graph.Edges {
		if e.Relation != codeindex.RelCalls && e.Relation != codeindex.RelReferences {
			continue
		}
		from := idx.Symbols[e.From]
		to := idx.Symbols[e.To]
		if from == nil || to == nil {
			continue
		}
		if from.File == to.File {
			continue
		}
		counts[edgeKey{from: absFile(idx.Root, from.File), to: absFile(idx.Root, to.File), ident: to.Name}]++
	}

	nodeSet := map[string]bool{}
	for f := range idx.Files {
		nodeSet[absFile(idx.Root, f)] = true
	}
	nodes := make([]string, 0, len(nodeSet))
	for n := range nodeSet {
		nodes = append(nodes, n)
	}

	edges := make([]WeightedEdge, 0, len(counts))
	for k, n := range counts {
		base := math.Sqrt(float64(n))
		mul := 1.0
		if focus.MentionedIdents[k.ident] {
			mul *= weightMulMentioned
		}
		if isInterestingIdent(k.ident) {
			mul *= weightMulInterestingID
		}
		if strings.HasPrefix(k.ident, "_") {
			mul *= weightMulPrivate
		}
		if defsPerIdent[k.ident] > genericDefThreshold {
			mul *= weightMulGeneric
		}
		if inFocus[k.from] {
			mul *= weightMulInFocus
		}
		edges = append(edges, WeightedEdge{From: k.from, To: k.to, Weight: base * mul})
	}
	return nodes, edges
}

func isInterestingIdent(s string) bool {
	if len(s) < minInterestingIDLen {
		return false
	}
	hasUpper, hasLower, hasUnder, hasDash := false, false, false, false
	for _, r := range s {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case r == '_':
			hasUnder = true
		case r == '-':
			hasDash = true
		}
	}
	isCamel := hasUpper && hasLower
	isSnake := hasUnder && (hasLower || hasUpper)
	isKebab := hasDash && (hasLower || hasUpper)
	return isCamel || isSnake || isKebab
}

// relFile converts an absolute file path back to the relative slash-path
// used as a key in idx.Files. Inverse of absFile.
func relFile(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(rel)
}

// SelectSymbolsForFile picks up to capPerFile symbols from a single file,
// always including any whose name is in focus.MentionedIdents, then padding
// with the most-referenced symbols by in-degree.
// The file argument is an absolute path (as emitted by ProjectFileGraph).
func SelectSymbolsForFile(idx *codeindex.Index, file string, focus Focus, capPerFile int) []*codeindex.Symbol {
	if idx == nil || capPerFile <= 0 {
		return nil
	}
	fileInfo, ok := idx.Files[relFile(idx.Root, file)]
	if !ok {
		return nil
	}

	inDegree := map[string]int{}
	for _, e := range idx.Graph.Edges {
		if e.Relation == codeindex.RelCalls || e.Relation == codeindex.RelReferences {
			inDegree[e.To]++
		}
	}

	var mentioned []*codeindex.Symbol
	var others []*codeindex.Symbol
	for _, symID := range fileInfo.Symbols {
		sym := idx.Symbols[symID]
		if sym == nil {
			continue
		}
		if focus.MentionedIdents[sym.Name] {
			mentioned = append(mentioned, sym)
		} else {
			others = append(others, sym)
		}
	}

	sort.Slice(others, func(i, j int) bool {
		di, dj := inDegree[others[i].ID], inDegree[others[j].ID]
		if di != dj {
			return di > dj
		}
		return others[i].StartLine < others[j].StartLine
	})

	out := append([]*codeindex.Symbol(nil), mentioned...)
	for _, sym := range others {
		if len(out) >= capPerFile {
			break
		}
		out = append(out, sym)
	}
	if len(out) > capPerFile {
		out = out[:capPerFile]
	}
	return out
}
