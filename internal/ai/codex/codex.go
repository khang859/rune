package codex

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/khang859/rune/internal/ai"
)

type AuthSource interface {
	Token(ctx context.Context) (string, error)
	AccountID(ctx context.Context) (string, error)
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

// classifiedErr attaches an ai.ErrorClass so the agent layer can decide
// retry / heal behavior, and so streamWithRetry can decide whether to retry
// internally. Errors without classification surface as ErrFatal.
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
	token, err := p.auth.Token(ctx)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	accountID, err := p.auth.AccountID(ctx)
	if err != nil {
		return err
	}
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("OpenAI-Beta", "responses=experimental")
	httpReq.Header.Set("chatgpt-account-id", accountID)
	httpReq.Header.Set("originator", "rune")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return classifiedErr{err: err, class: ai.ErrTransient}
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		if rerr := p.auth.Refresh(ctx); rerr != nil {
			return fmt.Errorf("auth refresh failed: %w", rerr)
		}
		return classifiedErr{err: fmt.Errorf("401 unauthorized, refreshed and retrying"), class: ai.ErrTransient}
	}
	if resp.StatusCode == 429 {
		b, _ := io.ReadAll(resp.Body)
		return classifiedErr{err: fmt.Errorf("status 429: %s", string(b)), class: ai.ErrRateLimit}
	}
	if resp.StatusCode >= 500 {
		b, _ := io.ReadAll(resp.Body)
		return classifiedErr{err: fmt.Errorf("status %d: %s", resp.StatusCode, string(b)), class: ai.ErrServer}
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		body := string(b)
		class := ai.ErrFatal
		if isOrphanOutputRejection(body) {
			class = ai.ErrOrphanOutput
		}
		return classifiedErr{err: fmt.Errorf("status %d: %s", resp.StatusCode, body), class: class}
	}
	return parseSSE(ctx, resp.Body, out)
}

// isOrphanOutputRejection matches the OpenAI Responses 400 returned when an
// input array is missing a function_call_output for a function_call. The
// agent loop heals this by appending synthetic tool_results, then retries.
func isOrphanOutputRejection(body string) bool {
	return strings.Contains(body, "missing_required_parameter") &&
		strings.Contains(body, "input[") &&
		strings.Contains(body, ".output")
}
