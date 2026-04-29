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
	s.mu.RLock()
	path := s.pathToActiveLocked()
	s.mu.RUnlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()

	// Detach the active branch from Root before grafting the compacted one.
	// Compact replaces history along the active line; sibling forks under Root
	// are preserved, but the chain we just summarized is gone. Users who want
	// to keep a pre-compact view should /fork before /compact.
	oldBranchRoot := s.Active
	for oldBranchRoot.Parent != nil && oldBranchRoot.Parent != s.Root {
		oldBranchRoot = oldBranchRoot.Parent
	}
	filtered := make([]*Node, 0, len(s.Root.Children))
	for _, c := range s.Root.Children {
		if c != oldBranchRoot {
			filtered = append(filtered, c)
		}
	}
	s.Root.Children = filtered

	// Build a new branch off root: [summary, path[cut:]...]
	s.Active = s.Root
	sumNode := s.appendLocked(ai.Message{
		Role:    ai.RoleAssistant,
		Content: []ai.ContentBlock{ai.TextBlock{Text: summary}},
	})
	sumNode.CompactedCount = cut
	for _, m := range path[cut:] {
		n := s.appendLocked(m)
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
