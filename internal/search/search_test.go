package search

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/khang859/rune/internal/config"
)

func TestTavilySearchRequestAndResponse(t *testing.T) {
	var gotAuth string
	var gotBody struct {
		Query             string `json:"query"`
		SearchDepth       string `json:"search_depth"`
		Topic             string `json:"topic"`
		MaxResults        int    `json:"max_results"`
		IncludeAnswer     bool   `json:"include_answer"`
		IncludeRawContent bool   `json:"include_raw_content"`
		IncludeImages     bool   `json:"include_images"`
		IncludeUsage      bool   `json:"include_usage"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q, want application/json", got)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"results":[{"title":"One","url":"https://example.com/1","content":"Snippet one"},{"title":"Two","url":"https://example.com/2","content":"Snippet two"}]}`))
	}))
	defer srv.Close()

	p, err := NewTavily("tvly-" + strings.Repeat("x", 24))
	if err != nil {
		t.Fatal(err)
	}
	p.Endpoint = srv.URL
	results, err := p.Search(context.Background(), "go tavily", 99)
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer "+p.APIKey {
		t.Fatalf("Authorization = %q", gotAuth)
	}
	if gotBody.Query != "go tavily" || gotBody.SearchDepth != "basic" || gotBody.Topic != "general" || gotBody.MaxResults != 10 {
		t.Fatalf("unexpected request body: %+v", gotBody)
	}
	if gotBody.IncludeAnswer || gotBody.IncludeRawContent || gotBody.IncludeImages || gotBody.IncludeUsage {
		t.Fatalf("unexpected include flags: %+v", gotBody)
	}
	if len(results) != 2 || results[0].Title != "One" || results[0].Snippet != "Snippet one" {
		t.Fatalf("results = %+v", results)
	}
}

func TestResolveProviderTavily(t *testing.T) {
	store := config.NewSecretStore(filepath.Join(t.TempDir(), "secrets.json"))
	if err := store.SetTavilyAPIKey("tvly-" + strings.Repeat("t", 24)); err != nil {
		t.Fatal(err)
	}
	p, status, err := ResolveProvider(ResolveOptions{SearchEnabled: "on", SearchProvider: "tavily", SecretStore: store})
	if err != nil {
		t.Fatal(err)
	}
	if status != "tavily" {
		t.Fatalf("status = %q, want tavily", status)
	}
	if _, ok := p.(*Tavily); !ok {
		t.Fatalf("provider = %T, want *Tavily", p)
	}
}

func TestResolveProviderAutoPrefersBraveThenTavily(t *testing.T) {
	store := config.NewSecretStore(filepath.Join(t.TempDir(), "secrets.json"))
	if err := store.SetBraveSearchAPIKey(strings.Repeat("b", 24)); err != nil {
		t.Fatal(err)
	}
	if err := store.SetTavilyAPIKey("tvly-" + strings.Repeat("t", 24)); err != nil {
		t.Fatal(err)
	}
	p, status, err := ResolveProvider(ResolveOptions{SearchEnabled: "auto", SearchProvider: "auto", SecretStore: store})
	if err != nil {
		t.Fatal(err)
	}
	if status != "brave" {
		t.Fatalf("status = %q, want brave", status)
	}
	if _, ok := p.(*Brave); !ok {
		t.Fatalf("provider = %T, want *Brave", p)
	}
}
