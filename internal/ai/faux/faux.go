package faux

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/khang859/rune/internal/ai"
)

// Faux is a scriptable ai.Provider for tests.
//
// Build a script with chained methods; each Done() ends one turn.
// Stream returns the next turn's events in order.
type Faux struct {
	mu    sync.Mutex
	turns [][]ai.Event
	next  int
}

func New() *Faux { return &Faux{} }

func (f *Faux) Reply(text string) *Faux {
	f.cur().push(ai.TextDelta{Text: text})
	return f
}

func (f *Faux) Thinking(text string) *Faux {
	f.cur().push(ai.Thinking{Text: text})
	return f
}

func (f *Faux) CallTool(name, jsonArgs string) *Faux {
	f.cur().push(ai.ToolCall{
		ID:   randID(),
		Name: name,
		Args: json.RawMessage(jsonArgs),
	})
	return f
}

func (f *Faux) Usage(in, out int) *Faux {
	f.cur().push(ai.Usage{Input: in, Output: out})
	return f
}

// Done finishes the current turn (default reason "stop"; "tool_use" if any tool calls in this turn).
func (f *Faux) Done() *Faux {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.turns) == 0 {
		f.turns = append(f.turns, nil)
	}
	cur := f.turns[len(f.turns)-1]
	reason := "stop"
	for _, e := range cur {
		if _, ok := e.(ai.ToolCall); ok {
			reason = "tool_use"
			break
		}
	}
	// Always emit a usage event before Done so consumers see token counts.
	cur = append(cur, ai.Usage{Input: 1, Output: 1})
	cur = append(cur, ai.Done{Reason: reason})
	f.turns[len(f.turns)-1] = cur
	f.turns = append(f.turns, nil) // start a new turn buffer for next chained call
	return f
}

// DoneOverflow finishes the current turn with reason "context_overflow".
func (f *Faux) DoneOverflow() *Faux {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.turns) == 0 {
		f.turns = append(f.turns, nil)
	}
	cur := f.turns[len(f.turns)-1]
	cur = append(cur, ai.Done{Reason: "context_overflow"})
	f.turns[len(f.turns)-1] = cur
	f.turns = append(f.turns, nil)
	return f
}

func (f *Faux) Stream(ctx context.Context, req ai.Request) (<-chan ai.Event, error) {
	f.mu.Lock()
	if f.next >= len(f.turns) || len(f.turns[f.next]) == 0 {
		f.mu.Unlock()
		// Empty turn: return a closed channel after a single Done.
		out := make(chan ai.Event, 1)
		out <- ai.Done{Reason: "stop"}
		close(out)
		return out, nil
	}
	events := f.turns[f.next]
	f.next++
	f.mu.Unlock()

	out := make(chan ai.Event, len(events))
	go func() {
		defer close(out)
		for _, e := range events {
			select {
			case <-ctx.Done():
				return
			case out <- e:
			}
		}
	}()
	return out, nil
}

// --- helpers ---

type turnBuf struct{ events *[]ai.Event }

func (f *Faux) cur() turnBuf {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.turns) == 0 {
		f.turns = append(f.turns, nil)
	}
	last := len(f.turns) - 1
	return turnBuf{events: &f.turns[last]}
}

func (b turnBuf) push(e ai.Event) {
	*b.events = append(*b.events, e)
}

var idCounter int

func randID() string {
	idCounter++
	return string(rune('a'+idCounter%26)) + string(rune('0'+(idCounter/26)%10))
}
