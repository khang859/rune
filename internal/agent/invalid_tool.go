package agent

import (
	"fmt"
	"sort"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

// maxInvalidToolRetries caps how many consecutive turns may emit invalid
// tool calls before the agent surfaces an error. Some models (notably Llama
// on Groq) occasionally produce malformed tool names; without a cap, a stuck
// model could nudge-loop forever.
const maxInvalidToolRetries = 2

func formatInvalidCallsNote(names []string) string {
	return fmt.Sprintf("(attempted invalid tool calls: %s)", strings.Join(names, ", "))
}

func buildInvalidToolNudge(invalidNames, allowedNames []string) ai.Message {
	var b strings.Builder
	b.WriteString("Note: you attempted to call ")
	if len(invalidNames) == 1 {
		b.WriteString("a tool that does not exist: ")
	} else {
		b.WriteString("tools that do not exist: ")
	}
	for i, name := range invalidNames {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte('"')
		b.WriteString(name)
		b.WriteByte('"')
	}
	b.WriteString(".\n\nAvailable tools: ")
	if len(allowedNames) == 0 {
		b.WriteString("(none)")
	} else {
		b.WriteString(strings.Join(allowedNames, ", "))
	}
	b.WriteString(".\n\nPlease retry using only the listed tool names.")
	return ai.Message{
		Role:    ai.RoleUser,
		Content: []ai.ContentBlock{ai.TextBlock{Text: b.String()}},
	}
}

func sortedToolNames(specs []ai.ToolSpec) []string {
	names := make([]string, 0, len(specs))
	for _, s := range specs {
		names = append(names, s.Name)
	}
	sort.Strings(names)
	return names
}

// buildToolGenerationFailedNudge produces a corrective user message for the
// case where the provider rejected a stream because the model emitted output
// that resembled a tool call but couldn't be parsed (Groq's tool_use_failed).
// Unlike buildInvalidToolNudge, no tool name made it through — the whole
// generation failed at the provider's parser. The nudge tells the model what
// to do differently before we retry.
func buildToolGenerationFailedNudge(allowedNames []string) ai.Message {
	var b strings.Builder
	b.WriteString("Note: the previous attempt to call a tool failed because the output could not be parsed as a valid tool call. ")
	b.WriteString("If you intended to use a tool, emit a clean function call matching one of the listed schemas — do not wrap it in prose, code fences, or extra JSON. ")
	b.WriteString("If you did not intend to use a tool, respond with plain text only.\n\nAvailable tools: ")
	if len(allowedNames) == 0 {
		b.WriteString("(none)")
	} else {
		b.WriteString(strings.Join(allowedNames, ", "))
	}
	b.WriteString(".")
	return ai.Message{
		Role:    ai.RoleUser,
		Content: []ai.ContentBlock{ai.TextBlock{Text: b.String()}},
	}
}
