package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/khang859/rune/internal/ai"
)

const (
	DefaultBaseURL  = "http://localhost:11434"
	DefaultEndpoint = DefaultBaseURL + "/v1/chat/completions"
)

type Provider struct {
	endpoint       string
	apiKey         string
	httpClient     *http.Client
	maxRetries     int
	retryBaseDelay time.Duration
}

func New(endpoint string, apiKey ...string) *Provider {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	key := ""
	if len(apiKey) > 0 {
		key = strings.TrimSpace(apiKey[0])
	}
	return &Provider{
		endpoint:       endpoint,
		apiKey:         key,
		httpClient:     &http.Client{Timeout: 0},
		maxRetries:     3,
		retryBaseDelay: time.Second,
	}
}

func (p *Provider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	model := strings.TrimSpace(req.Model)
	body, err := buildPayload(req)
	if err != nil {
		return nil, err
	}
	out := make(chan ai.Event, 64)
	go func() {
		defer close(out)
		if err := p.streamWithRetry(ctx, body, out); err != nil {
			if ce, ok := err.(classifiedErr); ok && ce.model == "" {
				ce.model = model
				err = ce
			}
			select {
			case out <- ai.StreamError{Err: err, Class: classify(err)}:
			case <-ctx.Done():
			}
		}
	}()
	return out, nil
}

func (p *Provider) streamWithRetry(ctx context.Context, body []byte, out chan<- ai.Event) error {
	var lastErr error
	for attempt := 0; attempt <= p.maxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err := p.streamOnce(ctx, body, out)
		if err == nil {
			return nil
		}
		lastErr = err
		if !isRetryable(err) {
			return err
		}
		wait := p.retryBaseDelay * (1 << attempt)
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return lastErr
}

type classifiedErr struct {
	err   error
	class ai.ErrorClass
	model string
}

func (e classifiedErr) Error() string {
	if e.model != "" {
		details := errorDetails{Message: e.err.Error()}
		if details.isModelNotFound() {
			return fmt.Sprintf("ollama model %q not found (run `ollama pull %s`)", e.model, e.model)
		}
	}
	return e.err.Error()
}
func (e classifiedErr) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	if ce, ok := err.(classifiedErr); ok {
		switch ce.class {
		case ai.ErrTransient, ai.ErrRateLimit, ai.ErrServer:
			return true
		}
	}
	return false
}

func classify(err error) ai.ErrorClass {
	if ce, ok := err.(classifiedErr); ok {
		return ce.class
	}
	return ai.ErrFatal
}

func (p *Provider) streamOnce(ctx context.Context, body []byte, out chan<- ai.Event) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "rune")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return classifiedErr{err: err, class: ai.ErrTransient}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		details := parseErrorBody(b)
		msg := details.formatted()
		if msg == "" {
			msg = string(b)
		}
		err := fmt.Errorf("status %d: %s", resp.StatusCode, msg)
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			return classifiedErr{err: err, class: ai.ErrRateLimit}
		case resp.StatusCode >= 500:
			return classifiedErr{err: err, class: ai.ErrServer}
		case details.isModelNotFound():
			return classifiedErr{err: err, class: ai.ErrFatal}
		default:
			return classifiedErr{err: err, class: ai.ErrFatal}
		}
	}
	return parseSSE(ctx, resp.Body, out)
}

type errorDetails struct {
	Message string
	Type    string
	Code    string
}

func parseErrorBody(b []byte) errorDetails {
	var env struct {
		Error any `json:"error"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return errorDetails{}
	}
	switch e := env.Error.(type) {
	case string:
		return errorDetails{Message: e}
	case map[string]any:
		d := errorDetails{}
		if v, ok := e["message"].(string); ok {
			d.Message = v
		}
		if v, ok := e["type"].(string); ok {
			d.Type = v
		}
		switch v := e["code"].(type) {
		case string:
			d.Code = v
		case float64:
			d.Code = fmt.Sprintf("%v", v)
		}
		return d
	}
	return errorDetails{}
}

func (d errorDetails) formatted() string {
	if d.Message == "" {
		return ""
	}
	if d.Type != "" {
		return d.Message + " (" + d.Type + ")"
	}
	return d.Message
}

func (d errorDetails) isModelNotFound() bool {
	msg := strings.ToLower(d.Message)
	return strings.Contains(msg, "model") && (strings.Contains(msg, "not found") || strings.Contains(msg, "pull"))
}

// ListModels returns model names installed on the Ollama instance backing a
// chat-completions endpoint. It uses Ollama's native /api/tags endpoint because
// that endpoint explicitly reports local models and tags.
func ListModels(ctx context.Context, chatEndpoint string, apiKey ...string) ([]string, error) {
	if chatEndpoint == "" {
		chatEndpoint = DefaultEndpoint
	}
	key := ""
	if len(apiKey) > 0 {
		key = strings.TrimSpace(apiKey[0])
	}
	tags, err := tagsEndpoint(chatEndpoint)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tags, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "rune")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama list models: status %d: %s", resp.StatusCode, string(b))
	}
	var body struct {
		Models []struct {
			Name string `json:"name"`
			ID   string `json:"id"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range body.Models {
		name := strings.TrimSpace(m.Name)
		if name == "" {
			name = strings.TrimSpace(m.ID)
		}
		if name != "" && !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out, nil
}

func tagsEndpoint(chatEndpoint string) (string, error) {
	u, err := url.Parse(chatEndpoint)
	if err != nil {
		return "", err
	}
	if u.Scheme == "" || u.Host == "" {
		return "", fmt.Errorf("invalid ollama endpoint %q", chatEndpoint)
	}
	u.Path = "/api/tags"
	u.RawQuery = ""
	u.Fragment = ""
	return u.String(), nil
}
