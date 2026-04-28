package codex

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/khang859/rune/internal/ai"
)

type AuthSource interface {
	Token(ctx context.Context) (string, error)
	Refresh(ctx context.Context) error
}

type Provider struct {
	endpoint       string
	auth           AuthSource
	httpClient     *http.Client
	maxRetries     int
	retryBaseDelay time.Duration
}

func New(endpoint string, auth AuthSource) *Provider {
	return &Provider{
		endpoint:       endpoint,
		auth:           auth,
		httpClient:     &http.Client{Timeout: 0},
		maxRetries:     3,
		retryBaseDelay: 1 * time.Second,
	}
}

func (p *Provider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	body, err := buildPayload(req)
	if err != nil {
		return nil, err
	}
	out := make(chan ai.Event, 64)
	go func() {
		defer close(out)
		if err := p.streamWithRetry(ctx, body, out); err != nil {
			select {
			case out <- ai.StreamError{Err: err, Retryable: false}:
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

type retryableErr struct{ err error }

func (e retryableErr) Error() string { return e.err.Error() }
func (e retryableErr) Unwrap() error { return e.err }

func isRetryable(err error) bool {
	_, ok := err.(retryableErr)
	return ok
}

func (p *Provider) streamOnce(ctx context.Context, body []byte, out chan<- ai.Event) error {
	token, err := p.auth.Token(ctx)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("OpenAI-Beta", "responses=v1")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return retryableErr{err}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		if rerr := p.auth.Refresh(ctx); rerr != nil {
			return fmt.Errorf("auth refresh failed: %w", rerr)
		}
		return retryableErr{fmt.Errorf("401 unauthorized, refreshed and retrying")}
	}
	if resp.StatusCode == 429 || resp.StatusCode >= 500 {
		b, _ := io.ReadAll(resp.Body)
		return retryableErr{fmt.Errorf("status %d: %s", resp.StatusCode, string(b))}
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("status %d: %s", resp.StatusCode, string(b))
	}
	return parseSSE(ctx, resp.Body, out)
}
