// internal/agent/compact.go
package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

const compactSystemPrompt = `You are rune's session memory compactor for a terminal coding agent.

Compress the provided conversation history into a structured continuation summary that lets the next assistant continue work without re-reading unnecessary history. Optimize for task continuity and technical accuracy, not human readability.

Output only the summary. Do not include preamble, apologies, hidden reasoning, XML tags, or commentary about summarization.

Important:
- The summary may replace prior raw messages in a live coding session.
- If the history contains an earlier compacted summary, merge it into the new summary instead of treating it as a normal assistant message.
- Preserve exact technical identifiers: file paths, function/type names, commands, package names, URLs, issue IDs, error messages, test names, config keys, and user-provided constraints.
- Redact secrets and credentials. Keep placeholders like <redacted API key> when relevant.
- Prefer specific facts over vague statements.
- Recent messages are more important than older messages, but do not drop decisions, user corrections, or failed approaches that prevent repeated work.
- Do not invent facts. If something is uncertain, mark it as uncertain.

Use this format:

## Goal and user constraints
- Original user goal and any later refinements.
- User preferences, explicit constraints, approvals/denials, and "do not do X" corrections.

## Current state
- What has been completed.
- What is currently in progress.
- Where the session should resume.

## Files and artifacts
For each relevant file/artifact, include status when known:
- path: read/created/modified/deleted/planned; what changed or was learned.
Include important symbols, APIs, structs, functions, tests, docs, or config entries.

## Commands, tools, and results
- Important commands run and their outcomes.
- Test/lint/build results, including exact failing tests or error snippets.
- Tool/subagent/web results that matter for continuing the task.

## Decisions and rationale
- Decisions made, alternatives considered, and why rejected.
- Architectural or implementation rationale that should not be rediscovered.

## Errors, blockers, and risks
- Exact error messages when relevant.
- Failed approaches and why they failed.
- Open risks, unknowns, or assumptions.

## Pending next steps
- Concrete remaining tasks in recommended order.
- Anything awaiting user approval or clarification.

## Key references
- Important URLs, docs, snippets, IDs, dates, values, or external references needed later.

Keep the summary compact but complete. Target 800-1200 tokens; exceed that only when needed to preserve critical coding context.`

// compactToolResultMaxBytes caps each tool_result's Output before sending the
// history to the summarizer, so a giant captured tool output doesn't push
// /compact itself past the model's input window. Picked low enough that a
// dozen oversized results still leave room for a useful summary.
const compactToolResultMaxBytes = 8_000

func (a *Agent) Compact(ctx context.Context, userInstructions string) error {
	return a.session.Compact(ctx, userInstructions, func(ctx context.Context, history []ai.Message, instr string) (string, error) {
		sys := compactSystemPrompt
		if instr != "" {
			sys += "\n\nUser instructions: " + instr
		}
		// Build a single Request that summarizes `history`. Shrink oversized
		// tool results so compact can succeed even when raw history is what
		// caused the overflow that triggered this call.
		req := ai.Request{
			Model:    a.session.Model,
			System:   sys,
			Messages: shrinkHistoryForCompact(history, compactToolResultMaxBytes),
		}
		ch, err := a.provider.Stream(ctx, req)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for ev := range ch {
			switch v := ev.(type) {
			case ai.TextDelta:
				b.WriteString(v.Text)
			case ai.StreamError:
				return "", v.Err
			case ai.Done:
				if v.Reason == "context_overflow" {
					return "", fmt.Errorf("compact failed: context overflow")
				}
				return strings.TrimSpace(b.String()), nil
			}
		}
		return strings.TrimSpace(b.String()), nil
	})
}

// shrinkHistoryForCompact returns a copy of history where any ToolResultBlock
// whose Output exceeds maxBytes is replaced with a head+tail truncated copy.
// The session is not mutated. When maxBytes <= 0, history is returned as-is.
func shrinkHistoryForCompact(history []ai.Message, maxBytes int) []ai.Message {
	if maxBytes <= 0 {
		return history
	}
	out := make([]ai.Message, len(history))
	for i, m := range history {
		out[i] = m
		if len(m.Content) == 0 {
			continue
		}
		var newContent []ai.ContentBlock
		for j, c := range m.Content {
			tr, ok := c.(ai.ToolResultBlock)
			if !ok || len(tr.Output) <= maxBytes {
				if newContent != nil {
					newContent = append(newContent, c)
				}
				continue
			}
			if newContent == nil {
				newContent = make([]ai.ContentBlock, 0, len(m.Content))
				newContent = append(newContent, m.Content[:j]...)
			}
			tr.Output = truncateMiddle(tr.Output, maxBytes)
			newContent = append(newContent, tr)
		}
		if newContent != nil {
			out[i].Content = newContent
		}
	}
	return out
}

func truncateMiddle(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	headLen := maxBytes / 2
	tailLen := maxBytes - headLen
	omitted := len(s) - maxBytes
	return fmt.Sprintf("%s\n\n[... truncated %d bytes from middle (%d total) ...]\n\n%s",
		s[:headLen], omitted, len(s), s[len(s)-tailLen:])
}
