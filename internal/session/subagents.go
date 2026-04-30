package session

import "time"

// SubagentTask is the durable, session-owned metadata for a subagent task.
// It intentionally omits full prompts/transcripts; those may contain large or
// sensitive context and can be added later as artifact references.
type SubagentTask struct {
	ID           string     `json:"task_id"`
	Name         string     `json:"name"`
	FamiliarName string     `json:"familiar_name,omitempty"`
	AgentType    string     `json:"agent_type"`
	Status       string     `json:"status"`
	Dependencies []string   `json:"dependencies,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	CompletedAt  *time.Time `json:"completed_at,omitempty"`
	Summary      string     `json:"summary,omitempty"`
	Error        string     `json:"error,omitempty"`
}

// SetSubagents replaces the persisted subagent task metadata for this session.
func (s *Session) SetSubagents(tasks []SubagentTask) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Subagents = cloneSubagentTasks(tasks)
}

// SubagentTasks returns a copy of the persisted subagent task metadata.
func (s *Session) SubagentTasks() []SubagentTask {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneSubagentTasks(s.Subagents)
}

func cloneSubagentTasks(tasks []SubagentTask) []SubagentTask {
	if len(tasks) == 0 {
		return nil
	}
	out := make([]SubagentTask, len(tasks))
	for i, t := range tasks {
		out[i] = t
		out[i].Dependencies = append([]string(nil), t.Dependencies...)
		if t.StartedAt != nil {
			v := *t.StartedAt
			out[i].StartedAt = &v
		}
		if t.CompletedAt != nil {
			v := *t.CompletedAt
			out[i].CompletedAt = &v
		}
	}
	return out
}
