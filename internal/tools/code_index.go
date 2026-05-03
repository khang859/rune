package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/codeindex"
)

type CodeIndexSummary struct{}
type CodeFindSymbol struct{}
type CodeSymbolContext struct{}
type CodeGraphNeighbors struct{}

func (CodeIndexSummary) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "code_index_summary",
		Description: "Build an in-memory multi-language Tree-sitter AST/graph index and summarize files and symbols. Supports Go, JavaScript, TypeScript, and Python.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "path":{"type":"string","description":"Repository or directory to index. Defaults to the current working directory."},
                "languages":{"type":"array","items":{"type":"string"},"description":"Optional languages to include: go, javascript, typescript, python."},
                "max_symbols":{"type":"integer","description":"Maximum symbols to show. Defaults to 300."}
            }
        }`),
	}
}

func (CodeIndexSummary) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Path       string   `json:"path"`
		Languages  []string `json:"languages"`
		MaxSymbols int      `json:"max_symbols"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"path"?: string, "languages"?: string[], "max_symbols"?: int}.`, err), IsError: true}, nil
	}
	idx, err := buildCodeIndex(ctx, a.Path, a.Languages)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	return Result{Output: codeindex.FormatSummary(idx, a.MaxSymbols)}, nil
}

func (CodeFindSymbol) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "code_find_symbol",
		Description: "Find symbols in a multi-language Tree-sitter AST index by name, signature, or file path.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "query":{"type":"string","description":"Symbol query. Matches names, qualified names, signatures, and file paths."},
                "kind":{"type":"string","description":"Optional kind filter: function, method, class, struct, interface, type, variable, constant."},
                "path":{"type":"string","description":"Repository or directory to index. Defaults to the current working directory."},
                "languages":{"type":"array","items":{"type":"string"},"description":"Optional languages to include: go, javascript, typescript, python."},
                "limit":{"type":"integer","description":"Maximum results. Defaults to 20."}
            }
        }`),
	}
}

func (CodeFindSymbol) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Query     string   `json:"query"`
		Kind      string   `json:"kind"`
		Path      string   `json:"path"`
		Languages []string `json:"languages"`
		Limit     int      `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"query"?: string, "kind"?: string, "path"?: string, "languages"?: string[], "limit"?: int}.`, err), IsError: true}, nil
	}
	idx, err := buildCodeIndex(ctx, a.Path, a.Languages)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	return Result{Output: codeindex.FormatSymbolList(codeindex.FindSymbols(idx, a.Query, a.Kind, a.Limit))}, nil
}

func (CodeSymbolContext) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "code_symbol_context",
		Description: "Show AST/graph context for a symbol id from code_find_symbol, including incoming and outgoing graph edges.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "symbol_id":{"type":"string","description":"Symbol id returned by code_find_symbol or code_index_summary."},
                "path":{"type":"string","description":"Repository or directory to index. Defaults to the current working directory."},
                "languages":{"type":"array","items":{"type":"string"},"description":"Optional languages to include: go, javascript, typescript, python."},
                "include_callers":{"type":"boolean","description":"Include incoming graph edges. Defaults to true."},
                "include_callees":{"type":"boolean","description":"Include outgoing graph edges. Defaults to true."}
            },
            "required":["symbol_id"]
        }`),
	}
}

func (CodeSymbolContext) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		SymbolID       string   `json:"symbol_id"`
		Path           string   `json:"path"`
		Languages      []string `json:"languages"`
		IncludeCallers bool     `json:"include_callers"`
		IncludeCallees bool     `json:"include_callees"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"symbol_id": string, "path"?: string, "languages"?: string[], "include_callers"?: bool, "include_callees"?: bool}.`, err), IsError: true}, nil
	}
	if strings.TrimSpace(a.SymbolID) == "" {
		return Result{Output: "symbol_id is required", IsError: true}, nil
	}
	idx, err := buildCodeIndex(ctx, a.Path, a.Languages)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	callers, callees := a.IncludeCallers, a.IncludeCallees
	if !callers && !callees {
		callers, callees = true, true
	}
	return Result{Output: codeindex.FormatSymbolContext(idx, a.SymbolID, callers, callees)}, nil
}

func (CodeGraphNeighbors) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "code_graph_neighbors",
		Description: "Show graph neighbors for a file, import, name, or symbol id from the Tree-sitter code index.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "id":{"type":"string","description":"Graph node id, e.g. a symbol id, file:path, import:module, or name:identifier."},
                "path":{"type":"string","description":"Repository or directory to index. Defaults to the current working directory."},
                "languages":{"type":"array","items":{"type":"string"},"description":"Optional languages to include: go, javascript, typescript, python."},
                "depth":{"type":"integer","description":"Traversal depth, max 3. Defaults to 1."},
                "limit":{"type":"integer","description":"Maximum edges. Defaults to 50."}
            },
            "required":["id"]
        }`),
	}
}

func (CodeGraphNeighbors) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		ID        string   `json:"id"`
		Path      string   `json:"path"`
		Languages []string `json:"languages"`
		Depth     int      `json:"depth"`
		Limit     int      `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"id": string, "path"?: string, "languages"?: string[], "depth"?: int, "limit"?: int}.`, err), IsError: true}, nil
	}
	if strings.TrimSpace(a.ID) == "" {
		return Result{Output: "id is required", IsError: true}, nil
	}
	idx, err := buildCodeIndex(ctx, a.Path, a.Languages)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	return Result{Output: codeindex.FormatNeighbors(idx, a.ID, a.Depth, a.Limit)}, nil
}

func buildCodeIndex(ctx context.Context, path string, languages []string) (*codeindex.Index, error) {
	root, err := resolveExplorePath(path)
	if err != nil {
		return nil, err
	}
	return codeindex.BuildCached(ctx, codeindex.BuildOptions{Root: root, Languages: languages})
}
