package agent

import (
	"time"

	"github.com/khang859/rune/internal/ai"
)

const maxStreamRetries = 2

type retryDirective struct {
	wait time.Duration
	heal bool
}

// classifyRetry returns a non-nil directive when an ai.ErrorClass should
// trigger an in-loop retry. Returns nil if the error should be surfaced.
//
// Providers handle their own transport-level retries (rate limit, 5xx)
// before emitting StreamError, so the agent loop only retries classes a
// provider cannot fix on its own: ErrOrphanOutput (needs session healing)
// and ErrTransient (a generic fallback for providers that don't ship retry
// logic of their own).
func classifyRetry(class ai.ErrorClass, attempt int) *retryDirective {
	if attempt >= maxStreamRetries {
		return nil
	}
	switch class {
	case ai.ErrOrphanOutput:
		return &retryDirective{heal: true}
	case ai.ErrTransient:
		return &retryDirective{wait: 500 * time.Millisecond}
	}
	return nil
}
