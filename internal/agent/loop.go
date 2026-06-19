package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/memory"
	"github.com/khang859/rune/internal/providers"
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
	headlessNudges := 0
	a.requiredToolDone = false
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
		if a.memoryPath != "" {
			if !a.memoryChecked {
				if block, err := memory.SystemBlock(a.memoryPath); err == nil {
					a.CacheMemoryBlock(block)
				} else {
					a.memoryChecked = true
				}
			}
			if block := a.MemoryBlock(); block != "" {
				if sys != "" {
					sys += "\n\n"
				}
				sys += block
			}
		}
		if block := BuildRepoMapBlock(a.session, a.codeIndex, a.repomapEnabled, a.repomapBudget); block != "" {
			if sys != "" {
				sys += "\n\n"
			}
			sys += block
		}
		if len(a.requireTools) > 0 {
			if sys != "" {
				sys += "\n\n"
			}
			sys += RequireToolPrompt(a.requireTools)
		}
		var toolSpecs []ai.ToolSpec
		if providers.ToolUseSupportWithSettings(a.session.Provider, a.session.Model, config.Settings{ModelCapabilities: a.modelCapabilities}) != providers.ToolUnsupported {
			toolSpecs = a.tools.Specs()
		}
		req := ai.Request{
			Model:     a.session.Model,
			System:    sys,
			Messages:  a.session.PathToActive(),
			Tools:     toolSpecs,
			Reasoning: ai.ReasoningConfig{Effort: a.effort},
		}
		turnStart := time.Now()
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
			sawUsage         bool
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
				sawUsage = true
			case ai.StreamError:
				if errors.Is(v.Err, context.Canceled) {
					out <- TurnAborted{}
					return
				}
				if r := classifyRetry(v.Class, streamAttempt); r != nil {
					retry = r
					lastStreamErr = v.Err
					// Discard any remaining events from this stream in the
					// background — don't forward them to the UI so a retry
					// doesn't double up, but don't block the retry path on a
					// provider that errored without closing its channel.
					go drainDiscard(ctx, events)
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
			// A retryable stream error abandons the rest of this stream (now
			// drained in the background); stop reading and fall through to the
			// retry logic below.
			if retry != nil {
				break
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

		if sawUsage {
			out <- TurnUsage{Usage: usage}
		}
		a.persistAssistant(text.String(), calls, invalidCallNames, usage, time.Since(turnStart))
		if len(calls) == 0 && len(invalidCallNames) == 0 {
			if a.subagents != nil {
				if ch, busy := a.subagents.WaitForAnyCompletion(); busy {
					select {
					case <-ch:
						continue
					case <-ctx.Done():
						out <- TurnAborted{}
						return
					}
				}
			}
			// Headless enforcement: the model tried to end its turn with plain
			// text. If a required completion tool hasn't succeeded yet, nudge it
			// to keep going instead of treating silence as "done". Bounded so a
			// model that simply won't comply still terminates (exit 3 upstream).
			if len(a.requireTools) > 0 && !a.requiredToolDone {
				if headlessNudges < maxHeadlessNudges {
					headlessNudges++
					out <- RequiredToolPending{Names: sortedRequireNames(a.requireTools), Attempt: headlessNudges}
					a.session.Append(buildRequireToolNudge(a.requireTools))
					continue
				}
				out <- TurnDone{Reason: ReasonIncompleteRequiredTool}
				return
			}
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
		// Likewise reset the headless-nudge budget: it only caps a model that
		// *repeatedly* stops without completing, not one making real progress.
		if len(calls) > 0 {
			invalidToolAttempt = 0
			headlessNudges = 0
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

func (a *Agent) persistAssistant(text string, calls []ai.ToolCall, invalidNames []string, usage ai.Usage, dur time.Duration) {
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
	a.session.AppendWithUsage(ai.Message{Role: ai.RoleAssistant, Content: content}, usage, int(dur.Milliseconds()))
	a.session.SetEffort(a.effort)
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
		if !res.IsError && a.requireTools[call.Name] {
			a.requiredToolDone = true
		}
		out <- ToolFinished{Call: call, Result: res}
	}
	return nil
}

// drainDiscard consumes any remaining events from an abandoned stream so a
// retry never blocks on a provider that emitted a retryable error without
// closing its channel. It exits on channel close or context cancellation.
// Run it in a goroutine — the retry must not wait for the old stream to drain.
func drainDiscard(ctx context.Context, events <-chan ai.Event) {
	for {
		select {
		case _, ok := <-events:
			if !ok {
				return
			}
		case <-ctx.Done():
			return
		}
	}
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
	return fmt.Errorf("agent panic: %v", r)
}
