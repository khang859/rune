package ai

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultStreamIdleTimeout is the maximum gap of silence allowed between bytes
// on a streaming response body before the stream is treated as stalled and
// torn down. Provider-level retries can then resume the request.
const DefaultStreamIdleTimeout = 60 * time.Second

// MaxRetryAfter caps server-supplied Retry-After hints. Without a cap, a
// hostile or misconfigured server could make the agent sleep arbitrarily.
const MaxRetryAfter = 60 * time.Second

// ErrStreamIdleTimeout is returned when a streaming body has been silent for
// longer than the configured idle timeout. It satisfies errors.Is checks.
var ErrStreamIdleTimeout = errors.New("stream idle timeout")

// IdleTimeoutReader wraps a streaming response body so that any gap of
// silence longer than `idle` triggers a forced Close on the underlying body,
// which unblocks any in-flight Read with an error. Reading any bytes resets
// the idle timer.
//
// The wrapper is safe to Close concurrently with Read.
func IdleTimeoutReader(body io.ReadCloser, idle time.Duration) io.ReadCloser {
	if idle <= 0 {
		return body
	}
	r := &idleReader{body: body, idle: idle}
	r.timer = time.AfterFunc(idle, r.expire)
	return r
}

type idleReader struct {
	body      io.ReadCloser
	idle      time.Duration
	timer     *time.Timer
	expired   atomic.Bool
	closeOnce sync.Once
	closeErr  error
}

func (r *idleReader) expire() {
	r.expired.Store(true)
	r.closeBody()
}

func (r *idleReader) closeBody() {
	r.closeOnce.Do(func() { r.closeErr = r.body.Close() })
}

func (r *idleReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n > 0 {
		// Refresh the deadline on any forward progress before checking
		// expired. If the timer is already racing with a successful read
		// we still surface the bytes — losing real data to a boundary
		// race would defeat the point of the wrapper.
		r.timer.Reset(r.idle)
		return n, err
	}
	if r.expired.Load() {
		return 0, ErrStreamIdleTimeout
	}
	return 0, err
}

func (r *idleReader) Close() error {
	r.timer.Stop()
	r.closeBody()
	return r.closeErr
}

// ParseRetryAfter parses an HTTP Retry-After header. Returns 0 if the value
// is missing, malformed, or non-positive. The result is clamped to
// MaxRetryAfter so a misbehaving server cannot stall the agent indefinitely.
//
// Both delta-seconds (e.g. "30") and HTTP-date (e.g. "Wed, 21 Oct 2015
// 07:28:00 GMT") forms are supported, per RFC 7231 §7.1.3.
func ParseRetryAfter(header string) time.Duration {
	v := strings.TrimSpace(header)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs <= 0 {
			return 0
		}
		d := time.Duration(secs) * time.Second
		if d > MaxRetryAfter {
			return MaxRetryAfter
		}
		return d
	}
	if t, err := http.ParseTime(v); err == nil {
		d := time.Until(t)
		if d <= 0 {
			return 0
		}
		if d > MaxRetryAfter {
			return MaxRetryAfter
		}
		return d
	}
	return 0
}
