// internal/agent/compact.go
package agent

import (
	"context"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

const compactSystemPrompt = "You are a context compactor. Produce a concise (~300 token) summary of the conversation so far. Preserve decisions, file paths, and unresolved questions. Output the summary only."

func (a *Agent) Compact(ctx context.Context, userInstructions string) error {
	return a.session.Compact(ctx, userInstructions, func(ctx context.Context, history []ai.Message, instr string) (string, error) {
		sys := compactSystemPrompt
		if instr != "" {
			sys += "\n\nUser instructions: " + instr
		}
		// Build a single Request that summarizes `history`.
		req := ai.Request{
			Model:    a.session.Model,
			System:   sys,
			Messages: history,
		}
		ch, err := a.provider.Stream(ctx, req)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for ev := range ch {
			switch v := ev.(type) {
			case ai.TextDelta:
				b.WriteString(v.Text)
			case ai.StreamError:
				return "", v.Err
			case ai.Done:
				return strings.TrimSpace(b.String()), nil
			}
		}
		return strings.TrimSpace(b.String()), nil
	})
}
