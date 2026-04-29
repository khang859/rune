package agent

import (
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/session"
)

// healOrphans appends a synthetic error tool_result for any tool_use along
// the active path that lacks one. Without this, a provider that requires
// every function_call to have a function_call_output (e.g., the OpenAI
// Responses API) will reject every subsequent request. Idempotent.
func healOrphans(s *session.Session) {
	if s == nil {
		return
	}
	var orphanIDs []string
	resulted := map[string]bool{}
	for _, m := range s.PathToActive() {
		for _, c := range m.Content {
			switch v := c.(type) {
			case ai.ToolUseBlock:
				orphanIDs = append(orphanIDs, v.ID)
			case ai.ToolResultBlock:
				resulted[v.ToolCallID] = true
			}
		}
	}
	for _, id := range orphanIDs {
		if resulted[id] {
			continue
		}
		s.Append(syntheticToolError(id, "tool execution aborted: turn ended before completion"))
		resulted[id] = true
	}
}

func syntheticToolError(callID, msg string) ai.Message {
	return ai.Message{
		Role:       ai.RoleToolResult,
		ToolCallID: callID,
		Content: []ai.ContentBlock{ai.ToolResultBlock{
			ToolCallID: callID,
			Output:     "(" + msg + ")",
			IsError:    true,
		}},
	}
}
