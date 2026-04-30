package groq

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

func parseSSE(ctx context.Context, r io.Reader, out chan<- ai.Event) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	state := newStreamState()
	var data strings.Builder
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		line := scanner.Text()
		if line == "" {
			if data.Len() > 0 {
				if done, err := dispatchData(ctx, data.String(), out, state); done || err != nil {
					return err
				}
			}
			data.Reset()
			continue
		}
		if strings.HasPrefix(line, ":") || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "event:") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if data.Len() > 0 {
		_, err := dispatchData(ctx, data.String(), out, state)
		if err != nil {
			return err
		}
	}
	return scanner.Err()
}

type chunk struct {
	Choices []choice  `json:"choices"`
	Usage   *usage    `json:"usage"`
	Error   *apiError `json:"error"`
}

type choice struct {
	Delta        delta  `json:"delta"`
	FinishReason string `json:"finish_reason"`
}

type delta struct {
	Content          string          `json:"content"`
	ReasoningContent string          `json:"reasoning_content"`
	Reasoning        string          `json:"reasoning"`
	ReasoningText    string          `json:"reasoning_text"`
	ToolCalls        []deltaToolCall `json:"tool_calls"`
}

type deltaToolCall struct {
	Index    int               `json:"index"`
	ID       string            `json:"id"`
	Type     string            `json:"type"`
	Function deltaToolFunction `json:"function"`
}

type deltaToolFunction struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type usage struct {
	PromptTokens        int `json:"prompt_tokens"`
	CompletionTokens    int `json:"completion_tokens"`
	PromptTokensDetails struct {
		CachedTokens int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

type apiError struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    any    `json:"code"`
}

type streamState struct {
	calls map[int]*pendingCall
	order []int
}

type pendingCall struct {
	id   string
	name string
	args strings.Builder
}

func newStreamState() *streamState { return &streamState{calls: map[int]*pendingCall{}} }

func (s *streamState) update(tc deltaToolCall) {
	idx := tc.Index
	pc := s.calls[idx]
	if pc == nil {
		pc = &pendingCall{}
		s.calls[idx] = pc
		s.order = append(s.order, idx)
	}
	if tc.ID != "" {
		pc.id = tc.ID
	}
	if tc.Function.Name != "" {
		pc.name = tc.Function.Name
	}
	if tc.Function.Arguments != "" {
		pc.args.WriteString(tc.Function.Arguments)
	}
}

func (s *streamState) flush(ctx context.Context, out chan<- ai.Event) error {
	ids := append([]int(nil), s.order...)
	for _, idx := range ids {
		pc := s.calls[idx]
		if pc == nil || pc.name == "" {
			continue
		}
		id := pc.id
		if id == "" {
			id = fmt.Sprintf("call_%d", idx)
		}
		args := pc.args.String()
		if args == "" {
			args = "{}"
		}
		if err := send(ctx, out, ai.ToolCall{ID: id, Name: pc.name, Args: json.RawMessage(args)}); err != nil {
			return err
		}
		delete(s.calls, idx)
	}
	s.order = nil
	return nil
}

func dispatchData(ctx context.Context, data string, out chan<- ai.Event, state *streamState) (bool, error) {
	data = strings.TrimSpace(data)
	if data == "" {
		return false, nil
	}
	if data == "[DONE]" {
		if err := state.flush(ctx, out); err != nil {
			return true, err
		}
		return true, send(ctx, out, ai.Done{Reason: "stop"})
	}
	var ch chunk
	if err := json.Unmarshal([]byte(data), &ch); err != nil {
		return false, nil
	}
	if ch.Error != nil {
		msg := ch.Error.Message
		if msg == "" {
			msg = "stream error"
		}
		code := ""
		switch t := ch.Error.Code.(type) {
		case string:
			code = t
		case float64:
			code = fmt.Sprintf("%v", t)
		}
		class := ai.ErrFatal
		if (errorDetails{Message: msg, Code: code}).isToolGenerationFailed() {
			class = ai.ErrToolGenerationFailed
		}
		return false, send(ctx, out, ai.StreamError{Err: errString(msg), Class: class})
	}
	if ch.Usage != nil {
		if err := send(ctx, out, ai.Usage{Input: ch.Usage.PromptTokens, Output: ch.Usage.CompletionTokens, CacheRead: ch.Usage.PromptTokensDetails.CachedTokens}); err != nil {
			return false, err
		}
	}
	for _, c := range ch.Choices {
		if c.Delta.Content != "" {
			if err := send(ctx, out, ai.TextDelta{Text: c.Delta.Content}); err != nil {
				return false, err
			}
		}
		thinking := firstNonEmpty(c.Delta.ReasoningContent, c.Delta.Reasoning, c.Delta.ReasoningText)
		if thinking != "" {
			if err := send(ctx, out, ai.Thinking{Text: thinking}); err != nil {
				return false, err
			}
		}
		for _, tc := range c.Delta.ToolCalls {
			state.update(tc)
		}
		switch c.FinishReason {
		case "tool_calls", "function_call":
			if err := state.flush(ctx, out); err != nil {
				return false, err
			}
			return false, send(ctx, out, ai.Done{Reason: "tool_use"})
		case "stop", "end":
			if err := state.flush(ctx, out); err != nil {
				return false, err
			}
			return false, send(ctx, out, ai.Done{Reason: "stop"})
		case "length":
			if err := state.flush(ctx, out); err != nil {
				return false, err
			}
			return false, send(ctx, out, ai.Done{Reason: "max_tokens"})
		case "content_filter":
			return false, send(ctx, out, ai.StreamError{Err: errString("provider finish_reason: content_filter"), Class: ai.ErrFatal})
		}
	}
	return false, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
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
