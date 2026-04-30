package agent

import (
	"context"
	"errors"
	"fmt"
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
	invalidToolAttempt := 0
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
			text             strings.Builder
			calls            []ai.ToolCall
			invalidCallNames []string
			usage            ai.Usage
			doneRsn          string
			done             bool
			overflow         bool
			retry            *retryDirective
			lastStreamErr    error
		)

		// Build a set of tool names actually sent to the model. Anything
		// outside this set is a malformed tool call we must filter out
		// before persisting the assistant message — otherwise the next
		// request includes a tool_use the provider will reject.
		allowed := make(map[string]bool, len(req.Tools))
		for _, t := range req.Tools {
			allowed[t.Name] = true
		}

		for ev := range events {
			switch v := ev.(type) {
			case ai.TextDelta:
				text.WriteString(v.Text)
				out <- AssistantText{Delta: v.Text}
			case ai.Thinking:
				out <- ThinkingText{Delta: v.Text}
			case ai.ToolCall:
				if allowed[v.Name] {
					calls = append(calls, v)
				} else {
					invalidCallNames = append(invalidCallNames, v.Name)
				}
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
					lastStreamErr = v.Err
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
			if retry.nudgeToolGenerationFail {
				invalidToolAttempt++
				if invalidToolAttempt > maxInvalidToolRetries {
					out <- TurnError{Err: fmt.Errorf("model produced unparseable tool calls on %d consecutive attempts; last provider error: %w", invalidToolAttempt, lastStreamErr)}
					return
				}
				out <- InvalidToolCallRecovered{Names: []string{"(unparseable)"}}
				a.session.Append(buildToolGenerationFailedNudge(sortedToolNames(req.Tools)))
				// Tool-gen failures use their own retry budget — don't bump
				// streamAttempt or this would race with the transport
				// retry budget (maxStreamRetries) and surface the raw
				// provider error before invalidToolAttempt is exhausted.
				continue
			}
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

		a.persistAssistant(text.String(), calls, invalidCallNames, usage)
		if len(calls) == 0 && len(invalidCallNames) == 0 {
			out <- TurnDone{Reason: doneRsn}
			return
		}
		if len(calls) > 0 {
			if err := a.runTools(ctx, calls, out); err != nil {
				sendErrOrAbort(ctx, out, err)
				return
			}
		}
		// Reset the consecutive-invalid counter on any turn that produced
		// real work — so an occasional malformed call alongside successful
		// tool calls doesn't burn the budget. The cap exists to stop a
		// model that's stuck producing nothing but malformed calls.
		if len(calls) > 0 {
			invalidToolAttempt = 0
		}
		if len(invalidCallNames) > 0 {
			if len(calls) == 0 {
				invalidToolAttempt++
			}
			if invalidToolAttempt > maxInvalidToolRetries {
				out <- TurnError{Err: fmt.Errorf("model emitted invalid tool calls on %d consecutive turns with no valid progress: %v", invalidToolAttempt, invalidCallNames)}
				return
			}
			out <- InvalidToolCallRecovered{Names: append([]string(nil), invalidCallNames...)}
			a.session.Append(buildInvalidToolNudge(invalidCallNames, sortedToolNames(req.Tools)))
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

func (a *Agent) persistAssistant(text string, calls []ai.ToolCall, invalidNames []string, usage ai.Usage) {
	body := text
	if len(invalidNames) > 0 {
		note := formatInvalidCallsNote(invalidNames)
		if body != "" {
			body += "\n\n" + note
		} else {
			body = note
		}
	}
	var content []ai.ContentBlock
	if body != "" {
		content = append(content, ai.TextBlock{Text: body})
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
