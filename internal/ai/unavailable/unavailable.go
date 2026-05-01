package unavailable

import (
	"context"
	"fmt"

	"github.com/khang859/rune/internal/ai"
)

type Provider struct {
	Message string
}

func New(message string) *Provider {
	return &Provider{Message: message}
}

func (p *Provider) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	msg := p.Message
	if msg == "" {
		msg = "no active provider configured"
	}
	return nil, fmt.Errorf("%s", msg)
}
