// internal/session/compact.go
package session

import (
	"context"
	"time"

	"github.com/khang859/rune/internal/ai"
)

type Summarizer func(ctx context.Context, history []ai.Message, instructions string) (string, error)

// Compact replaces the active path's prefix up to (but not including) the most recent
// user message with a single synthetic assistant summary message.
func (s *Session) Compact(ctx context.Context, instructions string, summarize Summarizer) error {
	path := s.PathToActive()
	if len(path) == 0 {
		return nil
	}
	cut := lastUserIndex(path)
	if cut <= 0 {
		return nil // nothing to compact (no prior user msgs before the most recent)
	}
	summary, err := summarize(ctx, path[:cut], instructions)
	if err != nil {
		return err
	}
	// Build a new branch off root: [summary, path[cut:]...]
	s.Active = s.Root
	s.Append(ai.Message{
		Role:    ai.RoleAssistant,
		Content: []ai.ContentBlock{ai.TextBlock{Text: summary}},
	})
	for _, m := range path[cut:] {
		n := s.Append(m)
		n.Created = time.Now()
	}
	return nil
}

func lastUserIndex(path []ai.Message) int {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i].Role == ai.RoleUser {
			return i
		}
	}
	return -1
}
