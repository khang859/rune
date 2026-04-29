package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/search"
)

type fakeSearchProvider struct {
	limit int
	err   error
}

func (f *fakeSearchProvider) Search(ctx context.Context, query string, limit int) ([]search.Result, error) {
	f.limit = limit
	if f.err != nil {
		return nil, f.err
	}
	return []search.Result{{Title: "Title", URL: "https://example.com", Snippet: "Snippet"}}, nil
}

func TestWebSearchDefaultAndCapLimit(t *testing.T) {
	p := &fakeSearchProvider{}
	res, err := (WebSearch{Provider: p}).Run(context.Background(), json.RawMessage(`{"query":"go","limit":99}`))
	if err != nil || res.IsError {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	if p.limit != 10 {
		t.Fatalf("limit = %d, want 10", p.limit)
	}
	if !strings.Contains(res.Output, "URL: https://example.com") {
		t.Fatalf("missing formatted result: %s", res.Output)
	}
}

func TestWebSearchProviderErrorIsToolError(t *testing.T) {
	res, err := (WebSearch{Provider: &fakeSearchProvider{err: errors.New("boom")}}).Run(context.Background(), json.RawMessage(`{"query":"go"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Output, "boom") {
		t.Fatalf("expected tool error, got %+v", res)
	}
}
