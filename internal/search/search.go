package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/khang859/rune/internal/config"
)

type Result struct {
	Title   string
	URL     string
	Snippet string
}
type Provider interface {
	Search(ctx context.Context, query string, limit int) ([]Result, error)
}

type Brave struct {
	APIKey   string
	Endpoint string
	Client   *http.Client
}

func NewBrave(apiKey string) (*Brave, error) {
	apiKey = config.NormalizeBraveAPIKeyInput(apiKey)
	if err := config.ValidateBraveAPIKey(apiKey); err != nil {
		return nil, fmt.Errorf("invalid Brave Search API key: %w", err)
	}
	return &Brave{APIKey: apiKey, Endpoint: "https://api.search.brave.com/res/v1/web/search", Client: &http.Client{Timeout: 15 * time.Second}}, nil
}
func (b *Brave) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = 5
	}
	if limit > 10 {
		limit = 10
	}
	endpoint := b.Endpoint
	if endpoint == "" {
		endpoint = "https://api.search.brave.com/res/v1/web/search"
	}
	u, err := url.Parse(endpoint)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("q", query)
	q.Set("count", fmt.Sprint(limit))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Subscription-Token", b.APIKey)
	c := b.Client
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("brave search failed: %s", resp.Status)
	}
	var body struct {
		Web struct {
			Results []struct {
				Title       string `json:"title"`
				URL         string `json:"url"`
				Description string `json:"description"`
			} `json:"results"`
		} `json:"web"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(body.Web.Results))
	for _, r := range body.Web.Results {
		out = append(out, Result{Title: r.Title, URL: r.URL, Snippet: r.Description})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type Tavily struct {
	APIKey   string
	Endpoint string
	Client   *http.Client
}

func NewTavily(apiKey string) (*Tavily, error) {
	apiKey = config.NormalizeTavilyAPIKeyInput(apiKey)
	if err := config.ValidateTavilyAPIKey(apiKey); err != nil {
		return nil, fmt.Errorf("invalid Tavily API key: %w", err)
	}
	return &Tavily{APIKey: apiKey, Endpoint: "https://api.tavily.com/search", Client: &http.Client{Timeout: 15 * time.Second}}, nil
}

func (t *Tavily) Search(ctx context.Context, query string, limit int) ([]Result, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("query is required")
	}
	if limit <= 0 {
		limit = 5
	}
	if limit > 10 {
		limit = 10
	}
	endpoint := t.Endpoint
	if endpoint == "" {
		endpoint = "https://api.tavily.com/search"
	}
	body := struct {
		Query                    string `json:"query"`
		SearchDepth              string `json:"search_depth"`
		Topic                    string `json:"topic"`
		MaxResults               int    `json:"max_results"`
		IncludeAnswer            bool   `json:"include_answer"`
		IncludeRawContent        bool   `json:"include_raw_content"`
		IncludeImages            bool   `json:"include_images"`
		IncludeImageDescriptions bool   `json:"include_image_descriptions"`
		IncludeFavicon           bool   `json:"include_favicon"`
		IncludeUsage             bool   `json:"include_usage"`
	}{
		Query:                    query,
		SearchDepth:              "basic",
		Topic:                    "general",
		MaxResults:               limit,
		IncludeAnswer:            false,
		IncludeRawContent:        false,
		IncludeImages:            false,
		IncludeImageDescriptions: false,
		IncludeFavicon:           false,
		IncludeUsage:             false,
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+t.APIKey)
	c := t.Client
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("tavily search failed: %s", resp.Status)
	}
	var outBody struct {
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Content string `json:"content"`
		} `json:"results"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&outBody); err != nil {
		return nil, err
	}
	out := make([]Result, 0, len(outBody.Results))
	for _, r := range outBody.Results {
		out = append(out, Result{Title: r.Title, URL: r.URL, Snippet: r.Content})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

type ResolveOptions struct {
	SearchEnabled  string
	SearchProvider string
	SecretStore    *config.SecretStore
	SearXNGURL     string
}

func ResolveProvider(opts ResolveOptions) (Provider, string, error) {
	enabled := opts.SearchEnabled
	if enabled == "" {
		enabled = "auto"
	}
	provider := opts.SearchProvider
	if provider == "" {
		provider = "auto"
	}
	if ev := strings.TrimSpace(getenv("RUNE_WEB_SEARCH_PROVIDER")); ev != "" && provider == "auto" {
		provider = ev
	}
	if enabled == "off" {
		return nil, "web_search disabled", nil
	}
	store := opts.SecretStore
	if store == nil {
		store = config.NewSecretStore(config.SecretsPath())
	}
	tryBrave := func() (Provider, error) {
		key, err := store.BraveSearchAPIKey()
		if err != nil {
			return nil, err
		}
		if key == "" {
			return nil, nil
		}
		return NewBrave(key)
	}
	tryTavily := func() (Provider, error) {
		key, err := store.TavilyAPIKey()
		if err != nil {
			return nil, err
		}
		if key == "" {
			return nil, nil
		}
		return NewTavily(key)
	}
	switch provider {
	case "", "auto":
		if p, err := tryBrave(); err != nil {
			if enabled == "on" {
				return nil, "", err
			}
		} else if p != nil {
			return p, "brave", nil
		}
		if p, err := tryTavily(); err != nil {
			if enabled == "on" {
				return nil, "", err
			}
		} else if p != nil {
			return p, "tavily", nil
		}
		if enabled == "on" {
			return nil, "", fmt.Errorf("web_search enabled but no provider is configured; set RUNE_BRAVE_SEARCH_API_KEY, RUNE_TAVILY_API_KEY, or configure a search API key in /settings")
		}
		return nil, "web_search unavailable: no provider configured", nil
	case "brave":
		p, err := tryBrave()
		if err != nil {
			return nil, "", err
		}
		if p == nil {
			return nil, "", fmt.Errorf("Brave Search API key missing; set RUNE_BRAVE_SEARCH_API_KEY or configure it in /settings")
		}
		return p, "brave", nil
	case "tavily":
		p, err := tryTavily()
		if err != nil {
			return nil, "", err
		}
		if p == nil {
			return nil, "", fmt.Errorf("Tavily API key missing; set RUNE_TAVILY_API_KEY or configure it in /settings")
		}
		return p, "tavily", nil
	case "searxng":
		return nil, "", fmt.Errorf("SearXNG provider is not implemented yet")
	default:
		return nil, "", fmt.Errorf("unknown web search provider %q", provider)
	}
}

var getenv = os.Getenv
