package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

// parseSSE reads SSE from r and emits ai.Event values to out.
// out is owned by the caller; parseSSE does NOT close it.
func parseSSE(ctx context.Context, r io.Reader, out chan<- ai.Event) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)

	var (
		eventName string
		dataBuf   strings.Builder
		state     = newStreamState()
	)

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Text()
		if line == "" {
			if dataBuf.Len() > 0 || eventName != "" {
				if err := dispatchEvent(ctx, eventName, dataBuf.String(), out, state); err != nil {
					return err
				}
			}
			eventName = ""
			dataBuf.Reset()
			continue
		}
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			eventName = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if dataBuf.Len() > 0 {
				dataBuf.WriteByte('\n')
			}
			dataBuf.WriteString(strings.TrimPrefix(line, "data:"))
			continue
		}
	}
	if dataBuf.Len() > 0 || eventName != "" {
		if err := dispatchEvent(ctx, eventName, dataBuf.String(), out, state); err != nil {
			return err
		}
	}
	return scanner.Err()
}

type usageWire struct {
	InputTokens        int `json:"input_tokens"`
	OutputTokens       int `json:"output_tokens"`
	InputTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"input_tokens_details"`
}

type respCompleted struct {
	Type     string `json:"type"`
	Response struct {
		Status string    `json:"status"`
		Usage  usageWire `json:"usage"`
	} `json:"response"`
}

type respIncomplete struct {
	Type     string `json:"type"`
	Response struct {
		IncompleteDetails struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
	} `json:"response"`
}

type respFailed struct {
	Type  string `json:"type"`
	Error struct {
		Message string `json:"message"`
		Code    string `json:"code"`
	} `json:"error"`
}

type textDelta struct {
	Type  string `json:"type"`
	Delta string `json:"delta"`
}

type itemAdded struct {
	Type string `json:"type"`
	Item struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"item"`
}

type itemDone struct {
	Type string `json:"type"`
	Item struct {
		Type      string `json:"type"`
		ID        string `json:"id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"item"`
}

type fnArgsDelta struct {
	Type   string `json:"type"`
	ItemID string `json:"item_id"`
	Delta  string `json:"delta"`
}

type fnArgsDone struct {
	Type      string `json:"type"`
	ItemID    string `json:"item_id"`
	Arguments string `json:"arguments"`
}

// streamState accumulates streamed function-call arguments across SSE events.
// The Responses API streams a tool call as: output_item.added (empty arguments)
// → function_call_arguments.delta* → function_call_arguments.done. We buffer
// until we have the full arguments JSON, then emit a single ai.ToolCall.
//
// It also tracks whether any reasoning_summary_text.delta has been seen since
// the last summary part boundary, so we can emit a "\n\n" separator between
// non-empty parts without producing a leading separator.
type streamState struct {
	pendingCalls    map[string]*pendingCall
	order           []string
	seenSummaryText bool
}

type pendingCall struct {
	id   string
	name string
	args strings.Builder
}

func newStreamState() *streamState {
	return &streamState{pendingCalls: map[string]*pendingCall{}}
}

func (s *streamState) start(itemID, name, initialArgs string) {
	if _, exists := s.pendingCalls[itemID]; exists {
		return
	}
	pc := &pendingCall{id: itemID, name: name}
	if initialArgs != "" {
		pc.args.WriteString(initialArgs)
	}
	s.pendingCalls[itemID] = pc
	s.order = append(s.order, itemID)
}

func (s *streamState) appendDelta(itemID, delta string) {
	if pc, ok := s.pendingCalls[itemID]; ok {
		pc.args.WriteString(delta)
	}
}

func (s *streamState) emit(ctx context.Context, itemID, finalArgs string, out chan<- ai.Event) error {
	pc, ok := s.pendingCalls[itemID]
	if !ok {
		return nil
	}
	delete(s.pendingCalls, itemID)
	for i, id := range s.order {
		if id == itemID {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	args := finalArgs
	if args == "" {
		args = pc.args.String()
	}
	if args == "" {
		args = "{}"
	}
	return send(ctx, out, ai.ToolCall{
		ID:   pc.id,
		Name: pc.name,
		Args: json.RawMessage(args),
	})
}

func (s *streamState) flushAll(ctx context.Context, out chan<- ai.Event) error {
	ids := append([]string(nil), s.order...)
	for _, id := range ids {
		if err := s.emit(ctx, id, "", out); err != nil {
			return err
		}
	}
	return nil
}

func dispatchEvent(ctx context.Context, name, data string, out chan<- ai.Event, state *streamState) error {
	data = strings.TrimSpace(data)
	if data == "" {
		return nil
	}
	switch name {
	case "response.output_text.delta":
		var d textDelta
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			return nil
		}
		return send(ctx, out, ai.TextDelta{Text: d.Delta})

	case "response.reasoning_summary_text.delta":
		var d textDelta
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			return nil
		}
		state.seenSummaryText = true
		return send(ctx, out, ai.Thinking{Text: d.Delta})

	case "response.reasoning_summary_part.added":
		if state.seenSummaryText {
			return send(ctx, out, ai.Thinking{Text: "\n\n"})
		}
		return nil

	case "response.output_item.added":
		var ia itemAdded
		if err := json.Unmarshal([]byte(data), &ia); err != nil {
			return nil
		}
		if ia.Item.Type == "function_call" {
			state.start(ia.Item.ID, ia.Item.Name, ia.Item.Arguments)
		}
		return nil

	case "response.function_call_arguments.delta":
		var d fnArgsDelta
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			return nil
		}
		state.appendDelta(d.ItemID, d.Delta)
		return nil

	case "response.function_call_arguments.done":
		var d fnArgsDone
		if err := json.Unmarshal([]byte(data), &d); err != nil {
			return nil
		}
		return state.emit(ctx, d.ItemID, d.Arguments, out)

	case "response.output_item.done":
		var id itemDone
		if err := json.Unmarshal([]byte(data), &id); err != nil {
			return nil
		}
		if id.Item.Type == "function_call" {
			return state.emit(ctx, id.Item.ID, id.Item.Arguments, out)
		}
		return nil

	case "response.completed":
		var rc respCompleted
		if err := json.Unmarshal([]byte(data), &rc); err != nil {
			return nil
		}
		if err := state.flushAll(ctx, out); err != nil {
			return err
		}
		if err := send(ctx, out, ai.Usage{
			Input:     rc.Response.Usage.InputTokens,
			Output:    rc.Response.Usage.OutputTokens,
			CacheRead: rc.Response.Usage.InputTokensDetails.CachedTokens,
		}); err != nil {
			return err
		}
		return send(ctx, out, ai.Done{Reason: "stop"})

	case "response.incomplete":
		var ri respIncomplete
		_ = json.Unmarshal([]byte(data), &ri)
		if ri.Response.IncompleteDetails.Reason == "context_length_exceeded" {
			return send(ctx, out, ai.Done{Reason: "context_overflow"})
		}
		return send(ctx, out, ai.Done{Reason: "max_tokens"})

	case "response.failed":
		var rf respFailed
		_ = json.Unmarshal([]byte(data), &rf)
		return send(ctx, out, ai.StreamError{
			Err:       errString(rf.Error.Message),
			Retryable: false,
		})
	}
	return nil
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
