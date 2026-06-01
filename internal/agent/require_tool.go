package agent

import (
	"errors"
	"sort"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

// maxHeadlessNudges caps how many consecutive times the agent may end its turn
// with plain text (no completion tool) before we give up and exit. The counter
// resets whenever the model makes real progress (calls a tool), so this only
// trips on a model that repeatedly refuses to finish — the canonical headless
// "ends the turn with a question" failure mode.
const maxHeadlessNudges = 6

// ReasonIncompleteRequiredTool is the TurnDone reason emitted when the agent
// exhausted its nudge budget without calling a required completion tool.
const ReasonIncompleteRequiredTool = "incomplete_required_tool"

// ErrIncompleteRequiredTool is returned by a headless run (--require-tool) that
// ended without the model calling one of the required tools. Callers map it to
// a distinct process exit code so an orchestrator can tell "agent went quiet"
// apart from a real crash.
var ErrIncompleteRequiredTool = errors.New("agent ended turn without calling a required completion tool")

// RequireToolPrompt is the system-prompt block injected when --require-tool is
// set. It overrides the base prompt's interactive "ask one question / wait for
// approval" guidance: a headless worker has no human to answer, so it must
// decide, proceed, and end only by calling a completion tool.
func RequireToolPrompt(required map[string]bool) string {
	list := strings.Join(sortedRequireNames(required), ", ")
	return strings.Join([]string{
		"<headless-execution>",
		"You are running non-interactively as an autonomous worker. No human will read intermediate questions or answer them, and there is no one to approve plans or clarify requirements.",
		"",
		"- Keep working until the task is fully resolved. Do not stop to ask questions, request approval, or confirm assumptions — choose the most reasonable course of action, proceed, and record any assumptions in your final summary.",
		"- Treat tool errors and missing prerequisites as problems to solve or work around, not reasons to stop and hand back control.",
		"- You MUST end your turn by calling one of these tools: " + list + ".",
		"- Ending your turn with plain text instead of calling one of those tools is a failure. Never yield control without calling one of them.",
		"</headless-execution>",
	}, "\n")
}

// buildRequireToolNudge is appended as a user message when the model ends its
// turn without calling a required tool, then the loop continues.
func buildRequireToolNudge(required map[string]bool) ai.Message {
	list := strings.Join(sortedRequireNames(required), ", ")
	text := "You ended your turn without calling a required completion tool. " +
		"You are non-interactive — no human will answer questions or approve plans, so do not ask for clarification or confirmation; make the most reasonable decision and continue. " +
		"When the task is resolved (or genuinely cannot proceed), you MUST call one of: " + list + ". " +
		"Continue now and finish by calling one of those tools."
	return ai.Message{
		Role:    ai.RoleUser,
		Content: []ai.ContentBlock{ai.TextBlock{Text: text}},
	}
}

func sortedRequireNames(required map[string]bool) []string {
	names := make([]string, 0, len(required))
	for n := range required {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// ParseRequireTools splits a comma-separated --require-tool value into a set,
// trimming whitespace and dropping empties. Returns nil when no names remain.
func ParseRequireTools(csv string) map[string]bool {
	out := map[string]bool{}
	for _, part := range strings.Split(csv, ",") {
		if name := strings.TrimSpace(part); name != "" {
			out[name] = true
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
