package groq

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/khang859/rune/internal/ai"
)

const (
	DefaultBaseURL  = "https://api.groq.com/openai/v1"
	DefaultEndpoint = DefaultBaseURL + "/chat/completions"
)

type Provider struct {
	endpoint       string
	apiKey         string
	httpClient     *http.Client
	maxRetries     int
	retryBaseDelay time.Duration
}

func New(endpoint, apiKey string) *Provider {
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	return &Provider{
		endpoint:       endpoint,
		apiKey:         apiKey,
		httpClient:     &http.Client{Timeout: 0},
		maxRetries:     3,
		retryBaseDelay: time.Second,
	}
}

func (p *Provider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	if strings.TrimSpace(p.apiKey) == "" {
		return nil, fmt.Errorf("groq API key is required (set GROQ_API_KEY, RUNE_GROQ_API_KEY, or configure it in /settings)")
	}
	body, err := buildPayload(req)
	if err != nil {
		return nil, err
	}
	out := make(chan ai.Event, 64)
	go func() {
		defer close(out)
		if err := p.streamWithRetry(ctx, body, out); err != nil {
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
}

func (e classifiedErr) Error() string { return e.err.Error() }
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
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("User-Agent", "rune")

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
		case resp.StatusCode >= 500 || resp.StatusCode == 498:
			return classifiedErr{err: err, class: ai.ErrServer}
		case details.isToolGenerationFailed():
			return classifiedErr{err: err, class: ai.ErrToolGenerationFailed}
		default:
			return classifiedErr{err: err, class: ai.ErrFatal}
		}
	}
	return parseSSE(ctx, resp.Body, out)
}

// errorDetails captures fields Groq attaches to a 4xx response.
type errorDetails struct {
	Message string
	Type    string
	Code    string
}

func parseErrorBody(b []byte) errorDetails {
	var env struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(b, &env); err != nil {
		return errorDetails{}
	}
	code := ""
	switch t := env.Error.Code.(type) {
	case string:
		code = t
	case float64:
		code = fmt.Sprintf("%v", t)
	}
	return errorDetails{
		Message: env.Error.Message,
		Type:    env.Error.Type,
		Code:    code,
	}
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

// isToolGenerationFailed matches Groq's "tool_use_failed" rejection, where
// the model produced text that looked like a tool call but could not be
// parsed. We match on code first (canonical), and fall back to a substring
// check on the message in case Groq alters the code string.
func (d errorDetails) isToolGenerationFailed() bool {
	if d.Code == "tool_use_failed" {
		return true
	}
	return strings.Contains(d.Message, "Failed to call a function") &&
		strings.Contains(d.Message, "failed_generation")
}
