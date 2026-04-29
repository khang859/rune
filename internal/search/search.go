package search

import (
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
	switch provider {
	case "", "auto":
		if p, err := tryBrave(); err != nil {
			if enabled == "on" {
				return nil, "", err
			}
		} else if p != nil {
			return p, "brave", nil
		}
		if enabled == "on" {
			return nil, "", fmt.Errorf("web_search enabled but no provider is configured; set RUNE_BRAVE_SEARCH_API_KEY or configure Brave in /settings")
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
	case "searxng":
		return nil, "", fmt.Errorf("SearXNG provider is not implemented yet")
	default:
		return nil, "", fmt.Errorf("unknown web search provider %q", provider)
	}
}

var getenv = os.Getenv
