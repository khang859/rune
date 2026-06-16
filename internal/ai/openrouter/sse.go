package openrouter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

func parseSSE(ctx context.Context, r io.Reader, out chan<- ai.Event, model string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	state := newStreamState(model)
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
		if _, err := dispatchData(ctx, data.String(), out, state); err != nil {
			return err
		}
	}
	// Stream ended without an explicit terminator: flush any content still buffered
	// for think-tag detection so it isn't dropped.
	if err := state.flushContent(ctx, out); err != nil {
		return err
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

	// Think-tag handling for models (kimi-family) that sometimes leak their
	// reasoning into the content field instead of the dedicated reasoning field.
	stripThink   bool // model is kimi-family; scan content for leaked think tags
	sawReasoning bool // a reasoning-field delta arrived this turn
	bodyMode     bool // committed to streaming content as assistant text (vs. buffering)
	cbuf         strings.Builder
}

// thinkBufCap bounds how much content we buffer while waiting for a closing think
// tag, so a kimi response that genuinely emits no thinking still streams.
const thinkBufCap = 8192

type pendingCall struct {
	id   string
	name string
	args strings.Builder
}

func newStreamState(model string) *streamState {
	return &streamState{calls: map[int]*pendingCall{}, stripThink: isKimiModel(model)}
}

func isKimiModel(model string) bool {
	return strings.Contains(strings.ToLower(model), "kimi")
}

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
		if err := state.flushContent(ctx, out); err != nil {
			return true, err
		}
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
		return false, send(ctx, out, ai.StreamError{Err: errString(msg), Class: ai.ErrFatal})
	}
	if ch.Usage != nil {
		if err := send(ctx, out, ai.Usage{Input: ch.Usage.PromptTokens, Output: ch.Usage.CompletionTokens, CacheRead: ch.Usage.PromptTokensDetails.CachedTokens}); err != nil {
			return false, err
		}
	}
	for _, c := range ch.Choices {
		// Process reasoning before content so sawReasoning is set when a chunk
		// carries both, and so the content gate sees the correct state.
		thinking := firstNonEmpty(c.Delta.ReasoningContent, c.Delta.Reasoning, c.Delta.ReasoningText)
		if thinking != "" {
			state.sawReasoning = true
			if err := send(ctx, out, ai.Thinking{Text: thinking}); err != nil {
				return false, err
			}
		}
		if c.Delta.Content != "" {
			if err := state.handleContent(ctx, out, c.Delta.Content); err != nil {
				return false, err
			}
		}
		for _, tc := range c.Delta.ToolCalls {
			state.update(tc)
		}
		switch c.FinishReason {
		case "tool_calls", "function_call":
			if err := state.flushContent(ctx, out); err != nil {
				return false, err
			}
			if err := state.flush(ctx, out); err != nil {
				return false, err
			}
			return false, send(ctx, out, ai.Done{Reason: "tool_use"})
		case "stop", "end":
			if err := state.flushContent(ctx, out); err != nil {
				return false, err
			}
			if err := state.flush(ctx, out); err != nil {
				return false, err
			}
			return false, send(ctx, out, ai.Done{Reason: "stop"})
		case "length":
			if err := state.flushContent(ctx, out); err != nil {
				return false, err
			}
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

// handleContent routes a content delta either straight to assistant text or, for
// kimi-family models that have leaked reasoning into content, through think-tag
// detection. The gate is two-fold: only kimi models are scanned, and only until a
// reasoning-field delta proves the provider is separating reasoning correctly.
func (s *streamState) handleContent(ctx context.Context, out chan<- ai.Event, txt string) error {
	if !s.stripThink || s.bodyMode {
		return send(ctx, out, ai.TextDelta{Text: txt})
	}
	// Reasoning already arrived in its own field this turn, so content is the
	// clean answer — stream it directly and stop scanning.
	if s.sawReasoning {
		s.bodyMode = true
		return send(ctx, out, ai.TextDelta{Text: txt})
	}
	// Possible leak path: buffer until we can split reasoning from the answer.
	s.cbuf.WriteString(txt)
	return s.scanContent(ctx, out)
}

// scanContent looks for a closing think tag in the buffered content. Everything
// before it (minus an optional leading <think>) is reasoning; everything after is
// the assistant's answer. This handles both well-formed <think>…</think> and the
// common kimi case where the opening tag never arrives (orphan </think>).
func (s *streamState) scanContent(ctx context.Context, out chan<- ai.Event) error {
	buf := s.cbuf.String()
	if idx, tag := indexCloseThink(buf); idx >= 0 {
		reasoning := stripOpenThink(buf[:idx])
		rest := strings.TrimLeft(buf[idx+len(tag):], " \t\r\n")
		s.cbuf.Reset()
		s.bodyMode = true
		if strings.TrimSpace(reasoning) != "" {
			if err := send(ctx, out, ai.Thinking{Text: reasoning}); err != nil {
				return err
			}
		}
		if rest != "" {
			return send(ctx, out, ai.TextDelta{Text: rest})
		}
		return nil
	}
	// No closing tag yet. If we have buffered a lot without one, this is most
	// likely a genuine answer with no thinking — commit to streaming it as text.
	if s.cbuf.Len() > thinkBufCap {
		return s.flushContent(ctx, out)
	}
	return nil
}

// flushContent emits any buffered content as assistant text. Used at terminal
// points and when the buffer cap is hit. Buffered-but-unconfirmed content is
// treated as a normal answer rather than hidden in a thinking block.
func (s *streamState) flushContent(ctx context.Context, out chan<- ai.Event) error {
	if s.cbuf.Len() == 0 {
		return nil
	}
	txt := s.cbuf.String()
	s.cbuf.Reset()
	s.bodyMode = true
	return send(ctx, out, ai.TextDelta{Text: txt})
}

// indexCloseThink returns the index and matched tag of the earliest closing think
// tag in s, or (-1, "") if none. </think> and </thinking> are distinct (they
// diverge at the 8th byte), so the earliest match wins cleanly.
func indexCloseThink(s string) (int, string) {
	best, tag := -1, ""
	for _, t := range []string{"</think>", "</thinking>"} {
		if i := strings.Index(s, t); i >= 0 && (best < 0 || i < best) {
			best, tag = i, t
		}
	}
	return best, tag
}

// stripOpenThink removes a leading <think>/<thinking> tag (and the whitespace
// before it) if present, leaving the reasoning text.
func stripOpenThink(s string) string {
	trimmed := strings.TrimLeft(s, " \t\r\n")
	for _, t := range []string{"<think>", "<thinking>"} {
		if strings.HasPrefix(trimmed, t) {
			return trimmed[len(t):]
		}
	}
	return s
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
