# Auto-Context Repo Map Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Inject an always-on, token-budgeted repo map into rune's system prompt every turn, ranked by personalized PageRank over a file graph projected from the existing `codeindex` symbol graph.

**Architecture:** New `internal/codeindex/repomap/` subpackage holds the ranker, focus tracker, renderer, and cache. The agent calls `repomap.Build(...)` before each turn; result is wrapped in `<repo_map>` and appended to the system prompt next to `RuntimeContext()`. Session gains `FilesRead []string`; the read tool reports successful reads via a callback wired at registry construction.

**Tech Stack:** Go 1.x, existing `internal/codeindex` (tree-sitter symbol graph), `internal/agent`, `internal/session`, `internal/tools`, `internal/config`, `internal/tui`.

**Spec:** `docs/superpowers/specs/2026-05-22-auto-context-repomap-design.md`

---

## File Structure

**Created:**
- `internal/codeindex/repomap/pagerank.go` — power-iteration PageRank
- `internal/codeindex/repomap/pagerank_test.go`
- `internal/codeindex/repomap/focus.go` — mentioned-ident scanner, focus type
- `internal/codeindex/repomap/focus_test.go`
- `internal/codeindex/repomap/rank.go` — file-graph projection + symbol selection
- `internal/codeindex/repomap/rank_test.go`
- `internal/codeindex/repomap/render.go` — token-budgeted tree renderer
- `internal/codeindex/repomap/render_test.go`
- `internal/codeindex/repomap/cache.go` — LRU map cache
- `internal/codeindex/repomap/cache_test.go`
- `internal/codeindex/repomap/repomap.go` — public `Build` entry point
- `internal/codeindex/repomap/repomap_test.go` — integration

**Modified:**
- `internal/session/session.go` — add `FilesRead []string`, `RecordFileRead(string)` method
- `internal/session/persist.go` — persist `FilesRead` in `wireSession`
- `internal/tools/read.go` — add `OnRead func(string)` field, invoke on success
- `internal/tools/tool.go` — add `OnRead` to `BuiltinOptions`, wire into `Read` registration
- `internal/config/settings.go` — add `RepoMap RepoMapSettings`
- `internal/agent/agent.go` — store `RepoMapEnabled bool`, `RepoMapBudget int`
- `internal/agent/system.go` — helper to build the `<repo_map>` block
- `internal/agent/loop.go` — call the helper alongside `RuntimeContext()`
- `internal/tui/root.go` — register `/repomap` in `knownSlashCommands`, handle in `handleSlashCommand`

---

## Task 1: PageRank with personalization

**Files:**
- Create: `internal/codeindex/repomap/pagerank.go`
- Test: `internal/codeindex/repomap/pagerank_test.go`

- [ ] **Step 1: Write the failing test**

```go
package repomap

import (
	"math"
	"testing"
)

func TestPageRankUniformWithoutPersonalization(t *testing.T) {
	nodes := []string{"a", "b", "c", "d"}
	edges := []WeightedEdge{
		{From: "a", To: "b", Weight: 1},
		{From: "b", To: "c", Weight: 1},
		{From: "c", To: "d", Weight: 1},
		{From: "d", To: "a", Weight: 1},
	}
	scores := PageRank(nodes, edges, nil)
	if len(scores) != 4 {
		t.Fatalf("want 4 scores, got %d", len(scores))
	}
	// Symmetric ring graph: all scores should be ~0.25.
	for _, n := range nodes {
		if math.Abs(scores[n]-0.25) > 0.01 {
			t.Errorf("score[%s] = %f, want ~0.25", n, scores[n])
		}
	}
}

func TestPageRankPersonalizationShiftsRanks(t *testing.T) {
	nodes := []string{"a", "b", "c", "d"}
	edges := []WeightedEdge{
		{From: "a", To: "b", Weight: 1},
		{From: "b", To: "c", Weight: 1},
		{From: "c", To: "d", Weight: 1},
		{From: "d", To: "a", Weight: 1},
	}
	pers := map[string]float64{"a": 1.0}
	scores := PageRank(nodes, edges, pers)
	if scores["a"] <= scores["c"] {
		t.Errorf("personalized node a (%f) should rank above c (%f)", scores["a"], scores["c"])
	}
}

func TestPageRankEmptyGraphReturnsNil(t *testing.T) {
	scores := PageRank(nil, nil, nil)
	if scores != nil {
		t.Errorf("want nil for empty input, got %v", scores)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/codeindex/repomap/ -run TestPageRank -v`
Expected: compile error — `PageRank` undefined.

- [ ] **Step 3: Implement PageRank**

```go
package repomap

const (
	pagerankDamping       = 0.85
	pagerankMaxIterations = 50
	pagerankTolerance     = 1e-6
)

type WeightedEdge struct {
	From   string
	To     string
	Weight float64
}

// PageRank runs personalized PageRank over nodes and weighted directed edges.
// personalization may be nil (uniform). Returns nil for empty input.
func PageRank(nodes []string, edges []WeightedEdge, personalization map[string]float64) map[string]float64 {
	if len(nodes) == 0 {
		return nil
	}

	n := float64(len(nodes))
	outWeight := map[string]float64{}
	outEdges := map[string][]WeightedEdge{}
	for _, e := range edges {
		outEdges[e.From] = append(outEdges[e.From], e)
		outWeight[e.From] += e.Weight
	}

	// Normalize personalization to a probability distribution; default uniform.
	pers := map[string]float64{}
	if len(personalization) == 0 {
		for _, node := range nodes {
			pers[node] = 1.0 / n
		}
	} else {
		var sum float64
		for _, node := range nodes {
			sum += personalization[node]
		}
		if sum <= 0 {
			for _, node := range nodes {
				pers[node] = 1.0 / n
			}
		} else {
			for _, node := range nodes {
				pers[node] = personalization[node] / sum
			}
		}
	}

	score := map[string]float64{}
	for _, node := range nodes {
		score[node] = 1.0 / n
	}

	for iter := 0; iter < pagerankMaxIterations; iter++ {
		next := map[string]float64{}
		var dangling float64
		for _, node := range nodes {
			if outWeight[node] == 0 {
				dangling += score[node]
			}
		}
		for _, node := range nodes {
			// Teleport contribution: (1 - d) * pers + d * (dangling redistributed via pers).
			next[node] = (1.0-pagerankDamping)*pers[node] + pagerankDamping*dangling*pers[node]
		}
		for _, src := range nodes {
			if outWeight[src] == 0 {
				continue
			}
			for _, e := range outEdges[src] {
				next[e.To] += pagerankDamping * score[src] * (e.Weight / outWeight[src])
			}
		}

		var delta float64
		for _, node := range nodes {
			d := next[node] - score[node]
			if d < 0 {
				d = -d
			}
			delta += d
		}
		score = next
		if delta < pagerankTolerance {
			break
		}
	}
	return score
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/codeindex/repomap/ -run TestPageRank -v`
Expected: PASS (all three subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/codeindex/repomap/pagerank.go internal/codeindex/repomap/pagerank_test.go
git commit -m "feat(repomap): add personalized PageRank"
```

---

## Task 2: Mentioned-ident scanner & Focus type

**Files:**
- Create: `internal/codeindex/repomap/focus.go`
- Test: `internal/codeindex/repomap/focus_test.go`

- [ ] **Step 1: Write the failing test**

```go
package repomap

import (
	"testing"

	"github.com/khang859/rune/internal/ai"
)

func TestExtractMentionedIdents(t *testing.T) {
	msgs := []ai.Message{
		{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "Please fix parseConfig in loop.go"}}},
		{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextBlock{Text: "Looking at HandleTool now"}}},
	}
	symbols := map[string]bool{
		"parseConfig": true,
		"HandleTool":  true,
		"unused":      true,
	}
	got := ExtractMentionedIdents(msgs, symbols)
	if !got["parseConfig"] {
		t.Errorf("missing parseConfig: %v", got)
	}
	if !got["HandleTool"] {
		t.Errorf("missing HandleTool: %v", got)
	}
	if got["unused"] {
		t.Errorf("should not include symbols absent from chat: %v", got)
	}
}

func TestExtractMentionedIdentsFiltersStopwords(t *testing.T) {
	msgs := []ai.Message{
		{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "func error return string"}}},
	}
	symbols := map[string]bool{"func": true, "error": true, "return": true, "string": true}
	got := ExtractMentionedIdents(msgs, symbols)
	if len(got) != 0 {
		t.Errorf("stopwords should be filtered, got %v", got)
	}
}

func TestExtractMentionedIdentsRequiresMinLength(t *testing.T) {
	msgs := []ai.Message{
		{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "use x or ab; try parseConfig"}}},
	}
	symbols := map[string]bool{"x": true, "ab": true, "parseConfig": true}
	got := ExtractMentionedIdents(msgs, symbols)
	if got["x"] || got["ab"] {
		t.Errorf("short idents should be filtered: %v", got)
	}
	if !got["parseConfig"] {
		t.Errorf("parseConfig missing: %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/codeindex/repomap/ -run TestExtractMentionedIdents -v`
Expected: compile error — `ExtractMentionedIdents` undefined.

- [ ] **Step 3: Implement focus extraction**

```go
package repomap

import (
	"regexp"

	"github.com/khang859/rune/internal/ai"
)

// Focus is the per-turn signal fed into ranking. InFocusFiles biases the
// PageRank personalization vector; MentionedIdents boosts edge weights and
// per-file symbol selection.
type Focus struct {
	InFocusFiles    []string
	MentionedIdents map[string]bool
}

var identRegexp = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]{2,}`)

// goKeywords and friends are stopwords too common to ever be useful as
// mentioned idents even if the index happens to contain a symbol with the
// same name (rare but possible for fields like "type").
var stopwords = map[string]bool{
	"func": true, "return": true, "error": true, "string": true, "int": true,
	"bool": true, "true": true, "false": true, "nil": true, "type": true,
	"struct": true, "interface": true, "package": true, "import": true,
	"var": true, "const": true, "for": true, "range": true, "switch": true,
	"case": true, "default": true, "break": true, "continue": true,
	"if": true, "else": true, "this": true, "that": true, "the": true,
	"and": true, "you": true, "with": true, "from": true, "into": true,
	"will": true, "should": true, "would": true, "could": true, "have": true,
	"been": true, "they": true, "them": true, "what": true, "when": true,
	"where": true, "which": true, "while": true, "your": true, "but": true,
}

// ExtractMentionedIdents scans message text for identifier-shaped tokens,
// filters stopwords, and keeps only those present in symbolNames.
func ExtractMentionedIdents(messages []ai.Message, symbolNames map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, m := range messages {
		for _, c := range m.Content {
			text := textOf(c)
			if text == "" {
				continue
			}
			for _, match := range identRegexp.FindAllString(text, -1) {
				if stopwords[match] {
					continue
				}
				if symbolNames[match] {
					out[match] = true
				}
			}
		}
	}
	return out
}

func textOf(c ai.ContentBlock) string {
	switch v := c.(type) {
	case ai.TextBlock:
		return v.Text
	case ai.ToolResultBlock:
		return v.Output
	default:
		return ""
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/codeindex/repomap/ -run TestExtractMentionedIdents -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/codeindex/repomap/focus.go internal/codeindex/repomap/focus_test.go
git commit -m "feat(repomap): add mentioned-ident scanner and Focus type"
```

---

## Task 3: File-graph projection with edge weighting

**Files:**
- Create: `internal/codeindex/repomap/rank.go`
- Test: `internal/codeindex/repomap/rank_test.go`

- [ ] **Step 1: Write the failing test**

```go
package repomap

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/khang859/rune/internal/codeindex"
)

func writeFile(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestProjectFileGraphEdges(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a
import "fmt"
func Caller() { Callee(); fmt.Println("x") }
func Callee() {}
`)
	writeFile(t, dir, "b.go", `package a
func Other() { Callee() }
`)
	idx, err := codeindex.NewBuilder().Build(context.Background(), codeindex.BuildOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	focus := Focus{}
	nodes, edges := ProjectFileGraph(idx, focus)
	if len(nodes) < 2 {
		t.Fatalf("want >=2 file nodes, got %d", len(nodes))
	}
	var found bool
	for _, e := range edges {
		if filepath.Base(e.From) == "b.go" && filepath.Base(e.To) == "a.go" {
			found = true
			if e.Weight <= 0 {
				t.Errorf("edge b.go->a.go has non-positive weight %f", e.Weight)
			}
		}
	}
	if !found {
		t.Fatalf("expected cross-file edge b.go -> a.go (Callee reference), got edges %+v", edges)
	}
}

func TestProjectFileGraphFocusBoost(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a
func Callee() {}
`)
	writeFile(t, dir, "b.go", `package a
func Other() { Callee() }
`)
	idx, err := codeindex.NewBuilder().Build(context.Background(), codeindex.BuildOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	baseFocus := Focus{}
	_, baseEdges := ProjectFileGraph(idx, baseFocus)

	bPath := filepath.Join(dir, "b.go")
	focused := Focus{InFocusFiles: []string{bPath}}
	_, boostedEdges := ProjectFileGraph(idx, focused)

	baseWeight := edgeWeight(baseEdges, bPath, filepath.Join(dir, "a.go"))
	boostedWeight := edgeWeight(boostedEdges, bPath, filepath.Join(dir, "a.go"))
	if boostedWeight <= baseWeight {
		t.Errorf("focus boost should raise edge weight, base=%f boosted=%f", baseWeight, boostedWeight)
	}
}

func edgeWeight(edges []WeightedEdge, from, to string) float64 {
	for _, e := range edges {
		if e.From == from && e.To == to {
			return e.Weight
		}
	}
	return 0
}
```

Add this import to the test file:

```go
import "os"
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/codeindex/repomap/ -run TestProjectFileGraph -v`
Expected: compile error — `ProjectFileGraph` undefined.

- [ ] **Step 3: Implement file-graph projection**

```go
package repomap

import (
	"math"
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

// ProjectFileGraph collapses the symbol graph into a file-to-file graph
// suitable for PageRank. Each resolved cross-file call/reference becomes a
// weighted edge from referencer file to definer file, with weights derived
// from Aider's repomap heuristics (see spec section "Edge weighting").
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
		counts[edgeKey{from: from.File, to: to.File, ident: to.Name}]++
	}

	nodeSet := map[string]bool{}
	for f := range idx.Files {
		nodeSet[f] = true
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/codeindex/repomap/ -run TestProjectFileGraph -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/codeindex/repomap/rank.go internal/codeindex/repomap/rank_test.go
git commit -m "feat(repomap): project file graph with Aider-style edge weights"
```

---

## Task 4: Symbol selection within file

**Files:**
- Modify: `internal/codeindex/repomap/rank.go`
- Modify: `internal/codeindex/repomap/rank_test.go`

- [ ] **Step 1: Write the failing test (append to rank_test.go)**

```go
func TestSelectSymbolsForFilePrioritizesMentioned(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "a.go", `package a
func Apple() {}
func Banana() {}
func Cherry() {}
`)
	idx, err := codeindex.NewBuilder().Build(context.Background(), codeindex.BuildOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	aPath := filepath.Join(dir, "a.go")
	focus := Focus{MentionedIdents: map[string]bool{"Cherry": true}}
	syms := SelectSymbolsForFile(idx, aPath, focus, 10)
	if len(syms) == 0 {
		t.Fatalf("expected symbols, got none")
	}
	if syms[0].Name != "Cherry" {
		t.Errorf("Cherry should rank first when mentioned, got %s", syms[0].Name)
	}
}

func TestSelectSymbolsForFileCaps(t *testing.T) {
	dir := t.TempDir()
	body := "package a\n"
	for i := 0; i < 30; i++ {
		body += "func F" + string(rune('0'+i%10)) + string(rune('0'+i/10)) + "() {}\n"
	}
	writeFile(t, dir, "a.go", body)
	idx, err := codeindex.NewBuilder().Build(context.Background(), codeindex.BuildOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	syms := SelectSymbolsForFile(idx, filepath.Join(dir, "a.go"), Focus{}, 5)
	if len(syms) > 5 {
		t.Errorf("cap not enforced: got %d symbols, want <=5", len(syms))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/codeindex/repomap/ -run TestSelectSymbolsForFile -v`
Expected: compile error — `SelectSymbolsForFile` undefined.

- [ ] **Step 3: Append to rank.go**

```go
// SelectSymbolsForFile picks up to capPerFile symbols from a single file,
// always including any whose name is in focus.MentionedIdents, then padding
// with the most-referenced symbols by in-degree.
func SelectSymbolsForFile(idx *codeindex.Index, file string, focus Focus, capPerFile int) []*codeindex.Symbol {
	if idx == nil || capPerFile <= 0 {
		return nil
	}
	fileInfo, ok := idx.Files[file]
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
```

Add the `sort` import to rank.go:

```go
import (
	"math"
	"sort"
	"strings"
	"unicode"

	"github.com/khang859/rune/internal/codeindex"
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/codeindex/repomap/ -run TestSelectSymbolsForFile -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/codeindex/repomap/rank.go internal/codeindex/repomap/rank_test.go
git commit -m "feat(repomap): add per-file symbol selection"
```

---

## Task 5: Token-budgeted tree renderer

**Files:**
- Create: `internal/codeindex/repomap/render.go`
- Test: `internal/codeindex/repomap/render_test.go`

- [ ] **Step 1: Write the failing test**

```go
package repomap

import (
	"strings"
	"testing"

	"github.com/khang859/rune/internal/codeindex"
)

func sym(name, file, sig string, line uint) *codeindex.Symbol {
	return &codeindex.Symbol{
		ID:        name + "@" + file,
		Name:      name,
		File:      file,
		Signature: sig,
		StartLine: line,
		Kind:      codeindex.SymbolFunction,
	}
}

func TestRenderBudgetedGroupsByFile(t *testing.T) {
	items := []RenderItem{
		{File: "internal/agent/loop.go", Symbol: sym("Run", "internal/agent/loop.go", "func (a *Agent) Run(...) error", 17)},
		{File: "internal/agent/loop.go", Symbol: sym("runTurn", "internal/agent/loop.go", "func (a *Agent) runTurn(...)", 33)},
		{File: "internal/session/session.go", Symbol: sym("Append", "internal/session/session.go", "func (s *Session) Append(m ai.Message)", 10)},
	}
	out := RenderBudgeted(items, 10000)
	if !strings.Contains(out, "internal/agent/loop.go:") {
		t.Errorf("missing file header for loop.go:\n%s", out)
	}
	if !strings.Contains(out, "func (a *Agent) Run") {
		t.Errorf("missing Run signature:\n%s", out)
	}
	if strings.Count(out, "internal/agent/loop.go:") != 1 {
		t.Errorf("file header should appear once per file, got:\n%s", out)
	}
}

func TestRenderBudgetedRespectsBudget(t *testing.T) {
	items := make([]RenderItem, 200)
	for i := range items {
		items[i] = RenderItem{
			File:   "f.go",
			Symbol: sym("S"+string(rune('a'+i%26)), "f.go", "func S() {}", uint(i)),
		}
	}
	out := RenderBudgeted(items, 200) // 200 tokens ~= 800 chars
	if estimateTokens(out) > 230 {     // within 15%
		t.Errorf("output exceeded budget: tokens=%d, budget=200", estimateTokens(out))
	}
}

func TestRenderBudgetedEmpty(t *testing.T) {
	if RenderBudgeted(nil, 1000) != "" {
		t.Error("empty input should render empty string")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/codeindex/repomap/ -run TestRenderBudgeted -v`
Expected: compile error — `RenderBudgeted`, `RenderItem`, `estimateTokens` undefined.

- [ ] **Step 3: Implement renderer**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/codeindex/repomap/ -run TestRenderBudgeted -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/codeindex/repomap/render.go internal/codeindex/repomap/render_test.go
git commit -m "feat(repomap): add token-budgeted tree renderer"
```

---

## Task 6: LRU cache

**Files:**
- Create: `internal/codeindex/repomap/cache.go`
- Test: `internal/codeindex/repomap/cache_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
	c.Get("a") // make 'a' most-recent
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/codeindex/repomap/ -run TestCache -v`
Expected: compile error — `NewCache` undefined.

- [ ] **Step 3: Implement cache**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/codeindex/repomap/ -run TestCache -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/codeindex/repomap/cache.go internal/codeindex/repomap/cache_test.go
git commit -m "feat(repomap): add small LRU cache"
```

---

## Task 7: Build() entry point + integration test

**Files:**
- Create: `internal/codeindex/repomap/repomap.go`
- Test: `internal/codeindex/repomap/repomap_test.go`

- [ ] **Step 1: Write the failing integration test**

```go
package repomap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/codeindex"
)

func TestBuildSurfacesMentionedSymbol(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "lib.go"), []byte(`package lib
func ParseConfig() error { return nil }
func unused() {}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte(`package lib
func Run() error { return ParseConfig() }
`), 0o644); err != nil {
		t.Fatal(err)
	}

	idx, err := codeindex.NewBuilder().Build(context.Background(), codeindex.BuildOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}

	focus := Focus{
		InFocusFiles:    []string{filepath.Join(dir, "main.go")},
		MentionedIdents: map[string]bool{"ParseConfig": true},
	}
	out, err := Build(context.Background(), idx, focus, Options{MaxTokens: 1000})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "ParseConfig") {
		t.Errorf("expected ParseConfig in output, got:\n%s", out)
	}
	if !strings.Contains(out, "lib.go") {
		t.Errorf("expected lib.go file header, got:\n%s", out)
	}
}

func TestBuildReturnsEmptyForNilIndex(t *testing.T) {
	out, err := Build(context.Background(), nil, Focus{}, Options{MaxTokens: 1000})
	if err != nil {
		t.Errorf("nil index should not error, got %v", err)
	}
	if out != "" {
		t.Errorf("nil index should return empty string, got %q", out)
	}
}

func TestBuildRespectsZeroBudget(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\nfunc F() {}\n"), 0o644)
	idx, _ := codeindex.NewBuilder().Build(context.Background(), codeindex.BuildOptions{Root: dir})
	out, _ := Build(context.Background(), idx, Focus{}, Options{MaxTokens: 0})
	if out != "" {
		t.Errorf("zero budget should return empty string, got %q", out)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/codeindex/repomap/ -run TestBuild -v`
Expected: compile error — `Build`, `Options` undefined.

- [ ] **Step 3: Implement Build**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/codeindex/repomap/ -run TestBuild -v`
Expected: PASS.

- [ ] **Step 5: Run the whole package**

Run: `go test ./internal/codeindex/repomap/ -v`
Expected: all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/codeindex/repomap/repomap.go internal/codeindex/repomap/repomap_test.go
git commit -m "feat(repomap): add public Build entry point"
```

---

## Task 8: Session.FilesRead + read-tool hook

**Files:**
- Modify: `internal/session/session.go`
- Modify: `internal/session/persist.go`
- Modify: `internal/session/session_test.go` (or create persist_test additions)
- Modify: `internal/tools/read.go`
- Modify: `internal/tools/read_test.go`
- Modify: `internal/tools/tool.go`

- [ ] **Step 1: Write failing test for session**

Append to `internal/session/session_test.go`:

```go
func TestRecordFileReadDedupAndCap(t *testing.T) {
	s := New("model")
	for i := 0; i < 60; i++ {
		s.RecordFileRead(fmt.Sprintf("/tmp/f%d.go", i))
	}
	if len(s.FilesRead) > 50 {
		t.Errorf("cap not enforced: got %d files, want <=50", len(s.FilesRead))
	}
	// Most recent should be first.
	if s.FilesRead[0] != "/tmp/f59.go" {
		t.Errorf("want newest first, got %q", s.FilesRead[0])
	}

	s.RecordFileRead("/tmp/f59.go")
	if len(s.FilesRead) > 50 {
		t.Error("dedup failed")
	}
	if s.FilesRead[0] != "/tmp/f59.go" {
		t.Error("re-recording should move to front")
	}
}
```

Add `import "fmt"` if not already present.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/session/ -run TestRecordFileRead -v`
Expected: compile error — `RecordFileRead`/`FilesRead` undefined.

- [ ] **Step 3: Add field and method to session.go**

Locate the `type Session struct {` declaration and add field:

```go
type Session struct {
	// ... existing fields ...
	FilesRead []string
}
```

Add the method at the bottom of the file:

```go
const maxFilesRead = 50

// RecordFileRead prepends path to FilesRead, deduping and capping at 50.
// Called by tools/read.go on every successful read.
func (s *Session) RecordFileRead(path string) {
	if path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []string{path}
	for _, p := range s.FilesRead {
		if p == path {
			continue
		}
		out = append(out, p)
		if len(out) >= maxFilesRead {
			break
		}
	}
	s.FilesRead = out
}
```

- [ ] **Step 4: Persist FilesRead in wireSession**

In `internal/session/persist.go`, add to `wireSession`:

```go
type wireSession struct {
	// ... existing fields ...
	FilesRead []string `json:"files_read,omitempty"`
}
```

In `snapshotForSave`, populate it:

```go
w := wireSession{
	// ... existing initialization ...
	FilesRead: append([]string(nil), s.FilesRead...),
}
```

In `Load`, restore it:

```go
return &Session{
	// ... existing fields ...
	FilesRead: append([]string(nil), w.FilesRead...),
	path:      path,
}, nil
```

- [ ] **Step 5: Run session test**

Run: `go test ./internal/session/ -run TestRecordFileRead -v`
Expected: PASS.

- [ ] **Step 6: Write failing test for Read tool callback**

Append to `internal/tools/read_test.go`:

```go
func TestReadCallsOnReadCallback(t *testing.T) {
	tmp, err := os.CreateTemp(t.TempDir(), "*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmp.WriteString("hello")
	tmp.Close()

	var got string
	r := Read{OnRead: func(p string) { got = p }}
	args, _ := json.Marshal(struct {
		Path string `json:"path"`
	}{Path: tmp.Name()})
	res, err := r.Run(context.Background(), args)
	if err != nil || res.IsError {
		t.Fatalf("read failed: %v %+v", err, res)
	}
	if got != tmp.Name() {
		t.Errorf("OnRead got %q, want %q", got, tmp.Name())
	}
}

func TestReadDoesNotCallOnReadOnFailure(t *testing.T) {
	var called bool
	r := Read{OnRead: func(p string) { called = true }}
	args, _ := json.Marshal(struct {
		Path string `json:"path"`
	}{Path: "/no/such/file"})
	res, _ := r.Run(context.Background(), args)
	if !res.IsError {
		t.Fatal("expected IsError=true for missing file")
	}
	if called {
		t.Error("OnRead must not fire on read failure")
	}
}
```

Ensure imports include `"os"`, `"context"`, `"encoding/json"`.

- [ ] **Step 7: Run test to verify it fails**

Run: `go test ./internal/tools/ -run TestReadCallsOnReadCallback -v`
Expected: compile error — `Read` has no `OnRead` field.

- [ ] **Step 8: Modify Read in read.go**

Change `type Read struct{}` to:

```go
type Read struct {
	// OnRead, if non-nil, is invoked with the resolved path after a successful
	// read. Used by the agent to track FilesRead for repo-map focus.
	OnRead func(path string)
}
```

In the `Run` method, after the successful read paths (both `read_all` branch and the truncation branch), invoke the callback. The cleanest spot: just before `return Result{...}` for success cases. Concretely, restructure the bottom of `Run` so all success paths flow through a single return:

Find the `return Result{Output: string(b)}, nil` line in the `if a.ReadAll {` branch and the final success `return Result{Output: out}, nil` — change them to:

```go
// In ReadAll branch:
if r.OnRead != nil {
	r.OnRead(a.Path)
}
return Result{Output: string(b)}, nil

// At the final success return:
if r.OnRead != nil {
	r.OnRead(a.Path)
}
return Result{Output: out}, nil
```

Receiver must be `(r Read)` not `(Read)`. Update both method signatures:

```go
func (r Read) Spec() ai.ToolSpec { ... }
func (r Read) Run(ctx context.Context, args json.RawMessage) (Result, error) { ... }
```

- [ ] **Step 9: Run Read tests**

Run: `go test ./internal/tools/ -run TestRead -v`
Expected: all PASS.

- [ ] **Step 10: Wire OnRead into BuiltinOptions and registration**

Modify `internal/tools/tool.go`:

```go
type BuiltinOptions struct {
	WebFetchEnabled      bool
	WebFetchAllowPrivate bool
	SearchProvider       search.Provider
	OnRead               func(path string)
}

func RegisterBuiltins(r *Registry, opts BuiltinOptions) {
	r.Register(Read{OnRead: opts.OnRead})
	// ... rest unchanged ...
}
```

- [ ] **Step 11: Run full tool package tests**

Run: `go test ./internal/tools/ -v`
Expected: all PASS.

- [ ] **Step 12: Commit**

```bash
git add internal/session/session.go internal/session/persist.go internal/session/session_test.go internal/tools/read.go internal/tools/read_test.go internal/tools/tool.go
git commit -m "feat: track FilesRead per session via Read tool callback"
```

---

## Task 9: Agent-side repo map injection + settings

**Files:**
- Modify: `internal/config/settings.go`
- Modify: `internal/agent/agent.go`
- Modify: `internal/agent/system.go`
- Modify: `internal/agent/system_test.go`
- Modify: `internal/agent/loop.go`

- [ ] **Step 1: Add settings field**

In `internal/config/settings.go`, add at the end of the `Settings` struct:

```go
type Settings struct {
	// ... existing fields ...
	RepoMap RepoMapSettings `json:"repo_map,omitempty"`
}

type RepoMapSettings struct {
	Enabled   bool `json:"enabled,omitempty"`
	MaxTokens int  `json:"max_tokens,omitempty"`
}
```

- [ ] **Step 2: Add Agent fields and accessors**

In `internal/agent/agent.go`, add to the `Agent` struct:

```go
type Agent struct {
	// ... existing fields ...
	repomapEnabled bool
	repomapBudget  int
	codeIndex      *codeindex.Index
}
```

Add an import `"github.com/khang859/rune/internal/codeindex"` if missing.

Add setters used by the TUI layer:

```go
func (a *Agent) SetRepoMapEnabled(enabled bool) { a.repomapEnabled = enabled }
func (a *Agent) SetRepoMapBudget(tokens int)    { a.repomapBudget = tokens }
func (a *Agent) RepoMapEnabled() bool           { return a.repomapEnabled }
func (a *Agent) RepoMapBudget() int             { return a.repomapBudget }
func (a *Agent) SetCodeIndex(idx *codeindex.Index) { a.codeIndex = idx }
```

- [ ] **Step 3: Write failing test for system-prompt block**

Append to `internal/agent/system_test.go`:

```go
func TestBuildRepoMapBlockDisabled(t *testing.T) {
	got := BuildRepoMapBlock(nil, nil, false, 1000)
	if got != "" {
		t.Errorf("disabled should return empty, got %q", got)
	}
}

func TestBuildRepoMapBlockWrapsOutput(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte(`package a
func Helper() {}
`), 0o644)
	os.WriteFile(filepath.Join(dir, "b.go"), []byte(`package a
func Caller() { Helper() }
`), 0o644)
	idx, err := codeindex.NewBuilder().Build(context.Background(), codeindex.BuildOptions{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New("m")
	sess.RecordFileRead(filepath.Join(dir, "b.go"))
	sess.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "Look at Helper"}}})

	got := BuildRepoMapBlock(sess, idx, true, 1000)
	if got == "" {
		t.Fatal("expected non-empty repo map block")
	}
	if !strings.HasPrefix(got, "<repo_map>\n") || !strings.HasSuffix(got, "\n</repo_map>") {
		t.Errorf("missing wrapping tags:\n%s", got)
	}
}
```

Ensure these imports exist at the top of the test file:

```go
import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/codeindex"
	"github.com/khang859/rune/internal/session"
)
```

- [ ] **Step 4: Run test to verify it fails**

Run: `go test ./internal/agent/ -run TestBuildRepoMapBlock -v`
Expected: compile error — `BuildRepoMapBlock` undefined.

- [ ] **Step 5: Add helper to system.go**

Append to `internal/agent/system.go`:

```go
// BuildRepoMapBlock assembles the per-turn <repo_map> system-prompt block.
// Returns "" silently on any failure path — never fails a turn over the map.
func BuildRepoMapBlock(s *session.Session, idx *codeindex.Index, enabled bool, maxTokens int) string {
	if !enabled || s == nil || idx == nil {
		return ""
	}
	symbolNames := make(map[string]bool, len(idx.Symbols))
	for _, sym := range idx.Symbols {
		symbolNames[sym.Name] = true
	}
	focus := repomap.Focus{
		InFocusFiles:    append([]string(nil), s.FilesRead...),
		MentionedIdents: repomap.ExtractMentionedIdents(s.PathToActive(), symbolNames),
	}
	out, err := repomap.Build(context.Background(), idx, focus, repomap.Options{MaxTokens: maxTokens})
	if err != nil || out == "" {
		return ""
	}
	return "<repo_map>\n" + out + "</repo_map>"
}
```

Add imports:

```go
import (
	"context"
	// ... existing imports ...
	"github.com/khang859/rune/internal/codeindex"
	"github.com/khang859/rune/internal/codeindex/repomap"
	"github.com/khang859/rune/internal/session"
)
```

- [ ] **Step 6: Run system test**

Run: `go test ./internal/agent/ -run TestBuildRepoMapBlock -v`
Expected: PASS.

- [ ] **Step 7: Wire into loop.go**

In `internal/agent/loop.go`, locate this block around line 46:

```go
if sys != "" {
    sys += "\n\n" + RuntimeContext()
}
```

Change it to:

```go
if sys != "" {
    sys += "\n\n" + RuntimeContext()
}
if block := BuildRepoMapBlock(a.session, a.codeIndex, a.repomapEnabled, a.repomapBudget); block != "" {
    if sys != "" {
        sys += "\n\n"
    }
    sys += block
}
```

- [ ] **Step 8: Wire OnRead callback into agent construction**

Find where rune constructs its `BuiltinOptions` (likely in `cmd/rune/` or wherever the agent is wired up — search with `grep -rn "BuiltinOptions{" cmd/ internal/`). At that callsite, set:

```go
opts := tools.BuiltinOptions{
    // ... existing options ...
    OnRead: sess.RecordFileRead,
}
```

Also at the agent construction site, propagate settings:

```go
agent.SetRepoMapEnabled(settings.RepoMap.Enabled || /*default*/ true)
budget := settings.RepoMap.MaxTokens
if budget == 0 {
    budget = 2000
}
agent.SetRepoMapBudget(budget)
// If the code index is built/available, attach it:
agent.SetCodeIndex(idx)
```

Default-on matches the spec ("Always-on with /repomap toggle"). Settings override.

- [ ] **Step 9: Run full agent and tools tests**

Run: `go test ./internal/agent/ ./internal/tools/ ./internal/session/ ./internal/codeindex/... -v`
Expected: all PASS.

- [ ] **Step 10: Commit**

```bash
git add internal/config/settings.go internal/agent/agent.go internal/agent/system.go internal/agent/system_test.go internal/agent/loop.go cmd/
git commit -m "feat: inject repo map into system prompt per turn"
```

---

## Task 10: /repomap slash command

**Files:**
- Modify: `internal/tui/root.go`
- Modify: `internal/tui/root_test.go`

- [ ] **Step 1: Write failing test**

Append to `internal/tui/root_test.go` (use any existing test that constructs a `RootModel` as a template; the assertion is the new bit):

```go
func TestSlashRepomapToggle(t *testing.T) {
	m := newTestRootModel(t) // existing helper; if absent, mirror nearest existing test setup
	m.agent.SetRepoMapEnabled(true)

	m.handleSlashCommand("/repomap off")
	if m.agent.RepoMapEnabled() {
		t.Error("expected repo map disabled after /repomap off")
	}
	m.handleSlashCommand("/repomap on")
	if !m.agent.RepoMapEnabled() {
		t.Error("expected repo map enabled after /repomap on")
	}
	m.handleSlashCommand("/repomap budget 3000")
	if m.agent.RepoMapBudget() != 3000 {
		t.Errorf("budget = %d, want 3000", m.agent.RepoMapBudget())
	}
}
```

(If `newTestRootModel` doesn't exist, find the existing test for `/compact` at `internal/tui/root_test.go:1083` and reuse whatever setup it uses to construct `m`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tui/ -run TestSlashRepomapToggle -v`
Expected: FAIL (unknown command).

- [ ] **Step 3: Register /repomap in knownSlashCommands**

In `internal/tui/root.go:105`, the line:

```go
"/compact", "/reload", "/hotkeys", "/skill-creator", "/feature-dev",
```

becomes:

```go
"/compact", "/reload", "/hotkeys", "/skill-creator", "/feature-dev",
"/repomap",
```

- [ ] **Step 4: Handle /repomap in handleSlashCommand**

In the `switch name {` block (around line 758 of root.go), add a new case:

```go
case "/repomap":
	switch {
	case arg == "" || arg == "status":
		state := "off"
		if m.agent.RepoMapEnabled() {
			state = "on"
		}
		m.msgs.OnInfo(fmt.Sprintf("(repomap: %s, budget=%d tokens)", state, m.agent.RepoMapBudget()))
	case arg == "on":
		m.agent.SetRepoMapEnabled(true)
		m.msgs.OnInfo("(repomap enabled)")
	case arg == "off":
		m.agent.SetRepoMapEnabled(false)
		m.msgs.OnInfo("(repomap disabled)")
	case strings.HasPrefix(arg, "budget "):
		raw := strings.TrimSpace(strings.TrimPrefix(arg, "budget "))
		n, err := strconv.Atoi(raw)
		if err != nil || n < 0 {
			m.msgs.OnInfo(fmt.Sprintf("(usage: /repomap budget N — N is non-negative integer, got %q)", raw))
			break
		}
		m.agent.SetRepoMapBudget(n)
		m.msgs.OnInfo(fmt.Sprintf("(repomap budget = %d tokens)", n))
	default:
		m.msgs.OnInfo("(usage: /repomap [status|on|off|budget N])")
	}
```

Ensure imports include `"strconv"` and `"strings"` (likely already present).

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/tui/ -run TestSlashRepomapToggle -v`
Expected: PASS.

- [ ] **Step 6: Run all TUI tests**

Run: `go test ./internal/tui/... -v`
Expected: all PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/tui/root.go internal/tui/root_test.go
git commit -m "feat(tui): add /repomap slash command (status, on, off, budget)"
```

---

## Task 11: Full-project smoke test

- [ ] **Step 1: Run the full project test + build**

Run: `make all`
Expected: vet + fmt + test + build all succeed.

If any test fails or `go vet` flags an issue, fix it in place. Common issues:
- Missing imports in modified files
- Unused imports after additions
- A test asserting a slash command list contents that needs `/repomap` added

- [ ] **Step 2: Manual smoke check**

Run rune in interactive mode against any small project:

```bash
./rune
> /repomap status
# expect: (repomap: on, budget=2000 tokens)
> /repomap budget 1000
# expect: (repomap budget = 1000 tokens)
```

Send a real request like "find the function that handles X" and observe — there is no automatic assertion here, but the agent's first turn should have access to a `<repo_map>` block in its system prompt (visible if you trace requests, but not required for sign-off).

- [ ] **Step 3: Update README (one bullet under Customization or Commands)**

In `README.md`, under the `## Commands` highlights list, add one line:

```
- `/repomap` — show/toggle the always-on code repo map (status, on, off, budget N)
```

No deeper docs page needed yet.

- [ ] **Step 4: Final commit**

```bash
git add README.md
git commit -m "docs: mention /repomap in README"
```

---

## Self-Review Notes

- **Spec coverage:** All seven components from the spec map to tasks 1–7. Agent integration covered by task 8 (FilesRead), task 9 (system-prompt injection + settings), task 10 (/repomap UX). Eval task (spec section "Testing → Eval") is deliberately deferred — it's a separate evaluation effort, not a blocker for shipping the feature.
- **Type consistency:** `WeightedEdge` used uniformly. `Focus`, `Options`, `RenderItem` consistent across tasks. `BuildRepoMapBlock` is the single integration helper.
- **Placeholder scan:** No "TBD" or "handle edge cases." All code is concrete. The one fuzzy spot is task 9 step 8 (wire `OnRead` at agent construction) — the file path isn't pinned because rune's wiring varies; the engineer will grep for `BuiltinOptions{` and find the one or two callsites. That's appropriately specific given the ambiguity.
- **Cap-per-file constant:** Spec says 20, code uses `defaultCapPerFile = 20`. Consistent.
- **Default token budget:** Spec says 2k default, 8k no-focus. Implemented as `defaultMaxTokens = 2000`, `defaultNoFocusBudget = 8000`. Consistent.
