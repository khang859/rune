package ollama

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/khang859/rune/internal/ai"
)

// parseNDJSON consumes Ollama's native /api/chat stream, which is
// newline-delimited JSON (one complete object per line) — not SSE. Each frame
// looks like:
//
//	{"model":"...","message":{"role":"assistant","content":"hello","thinking":"..."},"done":false}
//
// terminating with a `done:true` frame that may carry the final tool_calls
// and aggregate usage counters (prompt_eval_count, eval_count).
func parseNDJSON(ctx context.Context, r io.Reader, out chan<- ai.Event) error {
	scanner := bufio.NewScanner(r)
	// Tool-call args and big assistant chunks can be large; bump well past
	// bufio's 64KB default.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var f frame
		if err := json.Unmarshal(line, &f); err != nil {
			// Skip malformed frames rather than aborting the stream — partial
			// lines from proxies occasionally slip through.
			continue
		}
		if f.Error != "" {
			return send(ctx, out, ai.StreamError{Err: errString(f.Error), Class: ai.ErrFatal})
		}
		if f.Message.Content != "" {
			if err := send(ctx, out, ai.TextDelta{Text: f.Message.Content}); err != nil {
				return err
			}
		}
		if f.Message.Thinking != "" {
			if err := send(ctx, out, ai.Thinking{Text: f.Message.Thinking}); err != nil {
				return err
			}
		}
		// Tool calls arrive whole, typically on the terminal `done:true` frame.
		// Emit one ToolCall event per call.
		for i, tc := range f.Message.ToolCalls {
			args := tc.Function.Arguments
			if len(args) == 0 {
				args = json.RawMessage(`{}`)
			}
			if err := send(ctx, out, ai.ToolCall{
				ID:   fmt.Sprintf("call_%d", i),
				Name: tc.Function.Name,
				Args: args,
			}); err != nil {
				return err
			}
		}
		if f.Done {
			if f.PromptEvalCount > 0 || f.EvalCount > 0 {
				if err := send(ctx, out, ai.Usage{Input: f.PromptEvalCount, Output: f.EvalCount}); err != nil {
					return err
				}
			}
			reason := "stop"
			if len(f.Message.ToolCalls) > 0 {
				reason = "tool_use"
			} else if f.DoneReason == "length" {
				reason = "max_tokens"
			}
			return send(ctx, out, ai.Done{Reason: reason})
		}
	}
	return scanner.Err()
}

type frame struct {
	Message         frameMessage `json:"message"`
	Done            bool         `json:"done"`
	DoneReason      string       `json:"done_reason,omitempty"`
	PromptEvalCount int          `json:"prompt_eval_count,omitempty"`
	EvalCount       int          `json:"eval_count,omitempty"`
	Error           string       `json:"error,omitempty"`
}

type frameMessage struct {
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	Thinking  string          `json:"thinking,omitempty"`
	ToolCalls []frameToolCall `json:"tool_calls,omitempty"`
}

type frameToolCall struct {
	Function frameToolFunction `json:"function"`
}

type frameToolFunction struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

func send(ctx context.Context, out chan<- ai.Event, e ai.Event) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- e:
		return nil
	}
}

type errString string

func (e errString) Error() string { return string(e) }
