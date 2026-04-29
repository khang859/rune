package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/search"
)

type WebSearch struct{ Provider search.Provider }

func (w WebSearch) Spec() ai.ToolSpec {
	return ai.ToolSpec{Name: "web_search", Description: "Search the web and return ranked results with titles, URLs, and snippets.", Schema: json.RawMessage(`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer","default":5}},"required":["query"]}`)}
}
func (w WebSearch) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"query": string, "limit"?: number}.`, err), IsError: true}, nil
	}
	a.Query = strings.TrimSpace(a.Query)
	if a.Query == "" {
		return Result{Output: "query is required", IsError: true}, nil
	}
	if a.Limit <= 0 {
		a.Limit = 5
	}
	if a.Limit > 10 {
		a.Limit = 10
	}
	if w.Provider == nil {
		return Result{Output: "web_search provider is not configured", IsError: true}, nil
	}
	rs, err := w.Provider.Search(ctx, a.Query, a.Limit)
	if err != nil {
		return Result{Output: fmt.Sprintf("web_search failed: %v", err), IsError: true}, nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Search results for: %q\n\n", a.Query)
	if len(rs) == 0 {
		b.WriteString("No results.\n")
		return Result{Output: b.String()}, nil
	}
	for i, r := range rs {
		fmt.Fprintf(&b, "%d. %s\n   URL: %s\n", i+1, r.Title, r.URL)
		if r.Snippet != "" {
			fmt.Fprintf(&b, "   Snippet: %s\n", r.Snippet)
		}
		if i < len(rs)-1 {
			b.WriteByte('\n')
		}
	}
	return Result{Output: b.String()}, nil
}
