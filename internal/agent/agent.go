package agent

import (
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

type Agent struct {
	provider  ai.Provider
	tools     *tools.Registry
	session   *session.Session
	system    string
	effort    string
	subagents *SubagentSupervisor
}

func New(p ai.Provider, t *tools.Registry, s *session.Session, systemPrompt string) *Agent {
	a := &Agent{
		provider: p,
		tools:    t,
		session:  s,
		system:   systemPrompt,
		effort:   "medium",
	}
	a.subagents = NewSubagentSupervisor(a, SubagentConfig{})
	return a
}

func (a *Agent) Provider() ai.Provider          { return a.provider }
func (a *Agent) Tools() *tools.Registry         { return a.tools }
func (a *Agent) System() string                 { return a.system }
func (a *Agent) Subagents() *SubagentSupervisor { return a.subagents }

func (a *Agent) RegisterSubagentTools() {
	tools.RegisterSubagentTools(a.tools, a.subagents)
}

func (a *Agent) ReasoningEffort() string          { return a.effort }
func (a *Agent) SetReasoningEffort(effort string) { a.effort = effort }
