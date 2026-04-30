package agent

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/khang859/rune/internal/ai"
)

const eventBuffer = 64

func (a *Agent) Run(ctx context.Context, userMsg ai.Message) <-chan Event {
	out := make(chan Event, eventBuffer)
	a.session.Append(userMsg)
	go func() {
		defer close(out)
		defer healOrphans(a.session)
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
	autoCompactRemaining := 1
	streamAttempt := 0
	for {
		a.injectCompletedSubagentSummaries()
		sys := a.system
		if a.Mode() == ModePlan {
			if sys != "" {
				sys += "\n\n"
			}
			sys += PlanModePrompt()
		}
		if sys != "" {
			sys += "\n\n" + RuntimeContext()
		}
		req := ai.Request{
			Model:     a.session.Model,
			System:    sys,
			Messages:  a.session.PathToActive(),
			Tools:     a.tools.Specs(),
			Reasoning: ai.ReasoningConfig{Effort: a.effort},
		}
		events, err := a.provider.Stream(ctx, req)
		if err != nil {
			sendErrOrAbort(ctx, out, err)
			return
		}

		var (
			text     strings.Builder
			calls    []ai.ToolCall
			usage    ai.Usage
			doneRsn  string
			done     bool
			overflow bool
			retry    *retryDirective
		)

		for ev := range events {
			switch v := ev.(type) {
			case ai.TextDelta:
				text.WriteString(v.Text)
				out <- AssistantText{Delta: v.Text}
			case ai.Thinking:
				out <- ThinkingText{Delta: v.Text}
			case ai.ToolCall:
				calls = append(calls, v)
			case ai.Usage:
				usage = v
				out <- TurnUsage{Usage: v}
			case ai.StreamError:
				if errors.Is(v.Err, context.Canceled) {
					out <- TurnAborted{}
					return
				}
				if r := classifyRetry(v.Class, streamAttempt); r != nil {
					retry = r
					// Drop any remaining events on this stream — don't
					// forward them to the UI so a retry doesn't double up.
					for range events {
					}
				} else {
					out <- TurnError{Err: v.Err}
					return
				}
			case ai.Done:
				if v.Reason == "context_overflow" {
					overflow = true
				} else {
					doneRsn = v.Reason
					done = true
				}
			}
		}

		if retry != nil {
			if retry.heal {
				healOrphans(a.session)
			}
			if retry.wait > 0 {
				select {
				case <-time.After(retry.wait):
				case <-ctx.Done():
					out <- TurnAborted{}
					return
				}
			}
			streamAttempt++
			continue
		}
		streamAttempt = 0

		if overflow {
			out <- ContextOverflow{}
			if autoCompactRemaining <= 0 {
				out <- TurnDone{Reason: "context_overflow"}
				return
			}
			autoCompactRemaining--
			if err := a.Compact(ctx, ""); err != nil {
				sendErrOrAbort(ctx, out, err)
				return
			}
			if ctx.Err() != nil {
				out <- TurnAborted{}
				return
			}
			continue
		}

		if !done {
			if ctx.Err() != nil {
				out <- TurnAborted{}
				return
			}
			out <- TurnError{Err: errors.New("stream ended unexpectedly")}
			return
		}

		a.persistAssistant(text.String(), calls, usage)
		if len(calls) == 0 {
			out <- TurnDone{Reason: doneRsn}
			return
		}
		if err := a.runTools(ctx, calls, out); err != nil {
			sendErrOrAbort(ctx, out, err)
			return
		}
	}
}

func (a *Agent) injectCompletedSubagentSummaries() {
	if a.subagents == nil {
		return
	}
	for _, task := range a.subagents.DrainCompletedSummaries() {
		text := "[Subagent completed: " + task.ID + " / " + task.Name + "]\n\n" + task.Summary
		a.session.Append(ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}})
	}
}

func (a *Agent) persistAssistant(text string, calls []ai.ToolCall, usage ai.Usage) {
	var content []ai.ContentBlock
	if text != "" {
		content = append(content, ai.TextBlock{Text: text})
	}
	for _, c := range calls {
		content = append(content, ai.ToolUseBlock(c))
	}
	n := a.session.Append(ai.Message{Role: ai.RoleAssistant, Content: content})
	n.Usage = usage
}

// runTools dispatches each tool call and appends its result to the session.
// On Go-error from a tool, it appends synthetic error results for the failing
// call and every remaining call before returning, so the next request never
// carries orphan tool_use blocks.
func (a *Agent) runTools(ctx context.Context, calls []ai.ToolCall, out chan<- Event) error {
	for i, call := range calls {
		out <- ToolStarted{Call: call}
		res, err := a.tools.Run(ctx, call)
		if err != nil {
			a.session.Append(syntheticToolError(call.ID, "tool runtime error: "+err.Error()))
			for _, rem := range calls[i+1:] {
				a.session.Append(syntheticToolError(rem.ID, "tool execution aborted: prior tool failed"))
			}
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

func sendErrOrAbort(ctx context.Context, out chan<- Event, err error) {
	if errors.Is(err, context.Canceled) || ctx.Err() != nil {
		out <- TurnAborted{}
		return
	}
	out <- TurnError{Err: err}
}

func panicErr(r any) error {
	if e, ok := r.(error); ok {
		return e
	}
	return errors.New("agent panic")
}
