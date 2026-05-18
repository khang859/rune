package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	DefaultEndpoint = DefaultBaseURL + "/api/chat"

	// DefaultNumCtx caps Ollama's KV cache so thinking models loaded at their
	// stock context (often 128K-256K) don't pin enough VRAM to stall the runner
	// for minutes or 500. 16384 is wide enough for typical code conversations
	// while staying cheap on workstation-class GPUs.
	DefaultNumCtx = 16384

	legacyChatCompletionsPath = "/v1/chat/completions"
	nativeChatPath            = "/api/chat"
)

// Options bundles per-provider configuration that needs to flow from settings
// into the Ollama client. Kept here (rather than on ai.Request) so the shared
// request type doesn't leak provider-specific knobs.
type Options struct {
	Endpoint string
	APIKey   string
	// NumCtx overrides the runner's KV cache size (options.num_ctx). Zero means
	// fall back to DefaultNumCtx; pass a negative value to omit num_ctx entirely
	// and let the model's modelfile decide.
	NumCtx int
	// Think enables Ollama's native thinking mode for models that support it
	// (Qwen3.x, DeepSeek-R1, etc.). When false, the model skips reasoning and
	// streams answer content directly. When true, reasoning arrives as
	// ai.Thinking events alongside ai.TextDelta.
	Think bool
}

type Provider struct {
	endpoint          string
	apiKey            string
	numCtx            int
	think             bool
	httpClient        *http.Client
	maxRetries        int
	retryBaseDelay    time.Duration
	streamIdleTimeout time.Duration
}

// New constructs a Provider. Pass an empty Options{} to get all defaults.
//
// Variadic apiKey is preserved for callers that just want endpoint+key; pass
// Options{} explicitly when you also need NumCtx/Think.
func New(endpoint string, apiKey ...string) *Provider {
	opts := Options{Endpoint: endpoint}
	if len(apiKey) > 0 {
		opts.APIKey = apiKey[0]
	}
	return NewWithOptions(opts)
}

func NewWithOptions(opts Options) *Provider {
	endpoint := strings.TrimSpace(opts.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	} else {
		endpoint = rewriteLegacyEndpoint(endpoint)
	}
	numCtx := opts.NumCtx
	if numCtx == 0 {
		numCtx = DefaultNumCtx
	}
	return &Provider{
		endpoint:          endpoint,
		apiKey:            strings.TrimSpace(opts.APIKey),
		numCtx:            numCtx,
		think:             opts.Think,
		httpClient:        &http.Client{Timeout: 0},
		maxRetries:        3,
		retryBaseDelay:    time.Second,
		streamIdleTimeout: ai.DefaultStreamIdleTimeout,
	}
}

// rewriteLegacyEndpoint maps a saved /v1/chat/completions URL onto the native
// /api/chat endpoint so existing user settings keep working without a forced
// migration. Anything else is returned unchanged. Tolerates a trailing slash
// since URLs pasted from documentation or browser autocomplete often have one.
func rewriteLegacyEndpoint(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil {
		return endpoint
	}
	cleanPath := strings.TrimRight(u.Path, "/")
	if strings.HasSuffix(cleanPath, legacyChatCompletionsPath) {
		u.Path = strings.TrimSuffix(cleanPath, legacyChatCompletionsPath) + nativeChatPath
		return u.String()
	}
	return endpoint
}

func (p *Provider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	model := strings.TrimSpace(req.Model)
	body, err := buildPayload(req, payloadOptions{NumCtx: p.numCtx, Think: p.think})
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
		if ce, ok := err.(classifiedErr); ok && ce.retryAfter > wait {
			wait = ce.retryAfter
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return lastErr
}

type classifiedErr struct {
	err        error
	class      ai.ErrorClass
	model      string
	retryAfter time.Duration
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
	req.Header.Set("Accept", "application/x-ndjson")
	req.Header.Set("User-Agent", "rune")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return classifiedErr{err: err, class: ai.ErrTransient}
	}
	respBody := ai.IdleTimeoutReader(resp.Body, p.streamIdleTimeout)
	defer respBody.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(respBody)
		details := parseErrorBody(b)
		msg := details.formatted()
		if msg == "" {
			msg = string(b)
		}
		err := fmt.Errorf("status %d: %s", resp.StatusCode, msg)
		ra := ai.ParseRetryAfter(resp.Header.Get("Retry-After"))
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			return classifiedErr{err: err, class: ai.ErrRateLimit, retryAfter: ra}
		case resp.StatusCode >= 500:
			return classifiedErr{err: err, class: ai.ErrServer, retryAfter: ra}
		case details.isModelNotFound():
			return classifiedErr{err: err, class: ai.ErrFatal}
		default:
			return classifiedErr{err: err, class: ai.ErrFatal}
		}
	}
	if err := parseNDJSON(ctx, respBody, out); err != nil {
		if errors.Is(err, ai.ErrStreamIdleTimeout) {
			// Treat idle stalls as fatal so neither the provider nor the agent
			// retries. The dominant cause is the prompt exceeding num_ctx —
			// Ollama keeps the connection open but never emits a token, so
			// retrying just multiplies the wait. Surface an actionable message
			// instead of the bare "stream idle timeout".
			return classifiedErr{
				err:   fmt.Errorf("ollama stream stalled after %s of silence (num_ctx=%d); likely the prompt exceeds the model's context window — increase num_ctx in settings or shorten the conversation: %w", p.streamIdleTimeout, p.numCtx, err),
				class: ai.ErrFatal,
			}
		}
		return err
	}
	return nil
}

type errorDetails struct {
	Message string
	Type    string
	Code    string
}

func parseErrorBody(b []byte) errorDetails {
	// Native /api/chat returns {"error":"..."} (string, not object) on failure.
	// The OpenAI-compatible shape ({"error":{"message":...}}) may still appear
	// from proxies, so handle both.
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
// chat endpoint. It uses Ollama's native /api/tags endpoint because that
// endpoint explicitly reports local models and tags.
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
