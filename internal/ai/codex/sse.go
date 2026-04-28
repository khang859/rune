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
				if err := dispatchEvent(ctx, eventName, dataBuf.String(), out); err != nil {
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
		if err := dispatchEvent(ctx, eventName, dataBuf.String(), out); err != nil {
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

func dispatchEvent(ctx context.Context, name, data string, out chan<- ai.Event) error {
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

	case "response.output_item.added":
		var ia itemAdded
		if err := json.Unmarshal([]byte(data), &ia); err != nil {
			return nil
		}
		if ia.Item.Type == "function_call" {
			return send(ctx, out, ai.ToolCall{
				ID:   ia.Item.ID,
				Name: ia.Item.Name,
				Args: json.RawMessage(ia.Item.Arguments),
			})
		}
		return nil

	case "response.completed":
		var rc respCompleted
		if err := json.Unmarshal([]byte(data), &rc); err != nil {
			return nil
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
