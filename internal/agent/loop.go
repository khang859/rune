package agent

import (
	"context"
	"errors"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

const eventBuffer = 64

func (a *Agent) Run(ctx context.Context, userMsg ai.Message) <-chan Event {
	out := make(chan Event, eventBuffer)
	a.session.Append(userMsg)
	go func() {
		defer close(out)
		defer func() {
			if r := recover(); r != nil {
				out <- TurnError{Err: panicErr(r)}
			}
		}()
		a.runTurn(ctx, out)
	}()
	return out
}

func (a *Agent) runTurn(ctx context.Context, out chan<- Event) {
	for {
		req := ai.Request{
			Model:    a.session.Model,
			System:   a.system,
			Messages: a.session.PathToActive(),
			Tools:    a.tools.Specs(),
		}
		events, err := a.provider.Stream(ctx, req)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				out <- TurnAborted{}
			} else {
				out <- TurnError{Err: err}
			}
			return
		}

		var (
			assistantText strings.Builder
			toolCalls     []ai.ToolCall
			usage         ai.Usage
		)

		for ev := range events {
			switch v := ev.(type) {
			case ai.TextDelta:
				assistantText.WriteString(v.Text)
				out <- AssistantText{Delta: v.Text}
			case ai.Thinking:
				out <- ThinkingText{Delta: v.Text}
			case ai.ToolCall:
				toolCalls = append(toolCalls, v)
			case ai.Usage:
				usage = v
				out <- TurnUsage{Usage: v}
			case ai.StreamError:
				if errors.Is(v.Err, context.Canceled) {
					out <- TurnAborted{}
				} else {
					out <- TurnError{Err: v.Err}
				}
				return
			case ai.Done:
				if v.Reason == "context_overflow" {
					out <- ContextOverflow{}
					// For now: end the turn. Auto-compact lands in Plan 05.
					out <- TurnDone{Reason: "context_overflow"}
					return
				}
				a.persistAssistant(assistantText.String(), toolCalls, usage)
				if len(toolCalls) == 0 {
					out <- TurnDone{Reason: v.Reason}
					return
				}
				if err := a.runTools(ctx, toolCalls, out); err != nil {
					if errors.Is(err, context.Canceled) {
						out <- TurnAborted{}
						return
					}
					out <- TurnError{Err: err}
					return
				}
				// continue outer loop for next provider call
			}
		}
		if ctx.Err() != nil {
			out <- TurnAborted{}
			return
		}
	}
}

func (a *Agent) persistAssistant(text string, calls []ai.ToolCall, usage ai.Usage) {
	var content []ai.ContentBlock
	if text != "" {
		content = append(content, ai.TextBlock{Text: text})
	}
	for _, c := range calls {
		content = append(content, ai.ToolUseBlock{ID: c.ID, Name: c.Name, Args: c.Args})
	}
	n := a.session.Append(ai.Message{Role: ai.RoleAssistant, Content: content})
	n.Usage = usage
}

func (a *Agent) runTools(ctx context.Context, calls []ai.ToolCall, out chan<- Event) error {
	for _, call := range calls {
		out <- ToolStarted{Call: call}
		res, err := a.tools.Run(ctx, call)
		if err != nil {
			return err
		}
		a.session.Append(ai.Message{
			Role:       ai.RoleToolResult,
			ToolCallID: call.ID,
			Content:    []ai.ContentBlock{ai.ToolResultBlock{ToolCallID: call.ID, Output: res.Output, IsError: res.IsError}},
		})
		out <- ToolFinished{Call: call, Result: res}
	}
	return nil
}

func panicErr(r any) error {
	if e, ok := r.(error); ok {
		return e
	}
	return errors.New("agent panic")
}
