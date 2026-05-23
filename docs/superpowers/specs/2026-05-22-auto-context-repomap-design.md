# Auto-Context Repo Map — Design

**Status:** Approved for planning
**Date:** 2026-05-22
**Inspired by:** [Aider's repository map](https://aider.chat/docs/repomap.html)

## Problem

Rune already builds a code index (`internal/codeindex/`) with tree-sitter
symbol extraction, a typed graph (`RelDefines`, `RelCalls`, `RelReferences`),
and resolved cross-symbol edges (`resolveLocalCalls`). But the index is only
reachable through agent-invoked tools (`code_index_summary`, `find_symbols`,
`symbol_context`, `neighbors`). The agent must remember to call them, must
guess the right query, and pays a tool-call round-trip for every lookup.

Result: in practice, rune doesn't take advantage of its own index unless
explicitly prompted to. Compared to Aider, Cursor, and Cody — which all surface
relevant code automatically — this is the single biggest agent-quality gap.

## Goal

Inject a small, always-on "repo map" into rune's system prompt every turn:
a token-budgeted list of the most relevant symbols across the project,
ranked by personalized PageRank biased toward files the agent has touched
this session and identifiers mentioned in the current conversation.

Tool-based lookups (`find_symbols`, `symbol_context`, etc.) remain available
for deeper, agent-driven exploration. The map is the *ambient* layer; tools
are the *precise* layer.

## Non-goals

- No embeddings or vector DB. Rune already has a symbol graph; we use it.
- No multi-repo support. Single-project scope, matching the existing index.
- No new languages. Whatever the existing `codeindex/builder.go` parses, the
  map covers — no more, no less.
- No incremental graph updates. The index already invalidates by file mtime;
  we reuse that.

## Approach

**Hybrid file/symbol ranking** (decided in brainstorming):

1. Project rune's symbol-level graph down to a file→file graph. Each
   cross-file symbol reference contributes a weighted edge.
2. Run personalized PageRank on the file graph (fast, proven by Aider).
3. Within each top-ranked file, select symbols by local in-degree and
   mentioned-ident overlap.
4. Render `(file, symbol)` pairs token-budgeted, signature-only, until the
   budget is hit.

This keeps PageRank cheap (file graphs are small even in big repos) while
exploiting rune's richer symbol-level metadata for the actual rendered output.

## Architecture

New subpackage `internal/codeindex/repomap/`:

```
internal/codeindex/repomap/
  pagerank.go       Power-iteration PageRank with personalization (~80 LOC)
  focus.go          Track files-read-this-session, extract mentioned idents (~50 LOC)
  rank.go           Project file graph, apply weights, score symbols within files (~60 LOC)
  render.go         Token-budgeted tree renderer (~80 LOC)
  cache.go          LRU cache keyed by focus state (~40 LOC)
  repomap.go        Public Build() entry point (~40 LOC)
  *_test.go
```

Public API:

```go
type Focus struct {
    InFocusFiles    []string         // session.FilesRead, dedup'd
    MentionedIdents map[string]bool  // intersected with idx.Symbols
}

type Options struct {
    MaxTokens     int  // default 2000
    NoFocusBudget int  // default MaxTokens * 4 = 8000 when InFocusFiles is empty
    Verbose       bool
}

func Build(ctx context.Context, idx *codeindex.Index, focus Focus, opts Options) (string, error)
```

Agent integration:

- `internal/session/session.go` gains `FilesRead []string` (cap 50, dedup,
  most-recent-first).
- Read-tool dispatch in `internal/tools/` appends the resolved path on success.
- `internal/agent/system.go` calls `repomap.Build(...)` and wraps the result in
  `<repo_map>...</repo_map>` before appending to the system prompt.
- New `/repomap` slash command toggles enabled flag and budget (persisted in
  rune's settings).

## Algorithm details

### Edge weighting (adapted from Aider's `repomap.py:487-514`)

For each resolved symbol reference `A → B` where A and B are in different files,
add a file-graph edge from `file(A)` to `file(B)` with weight:

```
base   = sqrt(num_refs_A_to_B)
mul    = 1.0
mul   *= 10  if ident ∈ mentioned_idents
mul   *= 10  if len(ident) ≥ 8 and ident is snake/kebab/camel case
mul   *= 0.1 if ident starts with '_'
mul   *= 0.1 if ident is defined in >5 files (generic name)
mul   *= 50  if file(A) ∈ in_focus_files
weight = base * mul
```

Same-file references are skipped — they contribute nothing to file ranking.

### Personalization vector

For each file in `InFocusFiles`, set personalization to `100/N` where N is
the total file count. Files whose path basename matches an identifier in
`MentionedIdents` also get the boost. PageRank uses this vector as both
the `personalization` argument and the `dangling` argument (Aider's pattern).

### Symbol selection within a file

After PageRank produces per-file scores, walk files in descending order. For
each file, collect symbols using this priority:

1. Symbols whose `Name` is in `MentionedIdents` (always included)
2. Symbols sorted by in-degree count in `idx.Graph`
3. Cap at 20 symbols per file to prevent one massive file from dominating

### Token-budget rendering

Binary search over a prefix of the `(file, symbol)` list. For each candidate
prefix:

1. Render to tree format (file path, then indented signatures)
2. Count tokens via `len(text) / 4` (placeholder; later: provider tokenizer)
3. Accept if within 15% of budget, else adjust bounds

Render format:

```
internal/agent/loop.go:
  func (a *Agent) Run(ctx context.Context) error
  func (a *Agent) handleTool(ctx context.Context, call ai.ToolCall) (Result, error)
internal/session/session.go:
  type Session struct { ... }
  func (s *Session) Compact(ctx context.Context, instr string, summarize SummarizeFunc) error
```

Signatures come from `Symbol.Signature` (already populated by builder).

### Caching

Cache key: `hash(focus.InFocusFiles_sorted, focus.MentionedIdents_sorted, opts.MaxTokens, idx.Root)`.
LRU capacity 4. The index's own cache (file mtimes) handles upstream
invalidation; if the index reference changes, our cache key changes implicitly
via the focus snapshot.

Aider's "skip rebuild if last build took <1s" trick is added as an optional
optimization once we have timing data.

## Data flow per turn

```
User message arrives
  ↓
agent/loop.go: assembling request
  ↓
session.FilesRead, session.Messages available
  ↓
focus.Extract(session, idx) → Focus{
    InFocusFiles:    session.FilesRead,
    MentionedIdents: regex_scan(last 10 messages) ∩ idx.Symbols.Names,
}
  ↓
cache key from (focus, opts)
  ↓
hit  → reuse cached string
miss → repomap.Build(ctx, idx, focus, opts) {
         project file graph from idx.Graph
         apply edge weights
         personalized PageRank
         pick top files, then top symbols per file
         binary-search render to MaxTokens
       }
  ↓
agent/system.go prepends "<repo_map>\n{result}\n</repo_map>\n\n" to system prompt
  ↓
Provider stream
```

## Failure modes

- **Index nil / still building.** Skip the map silently; agent works without it.
- **Build error (empty graph, no resolved edges in a fresh repo).** Log at
  debug level, return empty string. Never fail a turn over the repo map.
- **PageRank divergence (rare; cyclic graph with all-zero personalization).**
  Fall back to in-degree ranking of files.
- **Budget too small (e.g., user sets 100 tokens).** Render best-effort, may
  include zero symbols; that's fine.
- **`/repomap off`.** Build short-circuits, returns "" before any computation.

## Testing

### Unit

- `pagerank_test.go` — 4-node hand-computed graph; personalization shifts ranks
  predictably; convergence within 50 iterations; divergence on a pathological
  graph falls back to in-degree ranking without panicking.
- `focus_test.go` — ident extraction from sample chat; stopwords filtered;
  unknown idents (not in index) dropped.
- `rank_test.go` — fixture graph; assert ordering with/without focus boost;
  symbol selection inside file prioritizes mentioned idents.
- `render_test.go` — never exceeds budget; hits 15% target on typical input;
  graceful degradation when budget too tight.
- `cache_test.go` — same focus = hit; different focus = miss; LRU eviction.

### Integration

- `repomap_test.go` — build full `codeindex.Index` over an in-tree fixture
  (3-5 .go files with explicit cross-file references), call `Build()`, assert
  expected symbol appears when mentioned.

### Agent-level

- `agent/system_test.go` — when enabled and `session.FilesRead` non-empty,
  assembled system prompt contains `<repo_map>` block. When disabled, no block.

### Eval

- New eval task `11-large-codebase-context`: small project (~10 files) where
  the bug fix requires knowing a function defined in another file. Compare
  pass rate with and without repo map enabled.

### Out of scope

- No benchmarks vs Aider.
- No fuzz/property tests for the `len/4` tokenizer heuristic — known rough.
- No language-specific regression — `codeindex/builder.go` tests already cover
  extraction.

## Configuration & UX

- Default: enabled, 2000-token budget, 8000-token no-focus budget.
- `/repomap` — show current state (enabled, budget, last build time, file count
  in last map).
- `/repomap off` / `/repomap on` — toggle.
- `/repomap budget N` — set token budget (0 = disabled).
- Settings file: `repomap.enabled`, `repomap.max_tokens`,
  `repomap.no_focus_budget`.
- Debug: `--repomap-verbose` flag prints which files/symbols were chosen and
  why on each build.

## Open items deferred to implementation

- Provider tokenizer wiring. The `len/4` heuristic is the placeholder; once
  we have a per-provider tokenizer interface, swap it in. Tracked but not
  blocking.
- Whether to include unresolved name references (`RelCallsName`,
  `RelReferenceName`) when no resolved equivalent exists. Lean toward "no" to
  avoid noise, but revisit if maps look thin in cross-language repos.
- Persistence of `FilesRead` across `/resume`. Probably yes (it's part of
  session state) but verify it doesn't bloat session files.
