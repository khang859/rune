package agent

import (
	"time"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

type Mode string

const (
	ModeAct  Mode = "act"
	ModePlan Mode = "plan"
)

type Agent struct {
	provider  ai.Provider
	tools     *tools.Registry
	session   *session.Session
	system    string
	effort    string
	mode      Mode
	subagents *SubagentSupervisor
}

func New(p ai.Provider, t *tools.Registry, s *session.Session, systemPrompt string) *Agent {
	return NewWithSubagentConfig(p, t, s, systemPrompt, SubagentConfig{})
}

func NewWithSubagentConfig(p ai.Provider, t *tools.Registry, s *session.Session, systemPrompt string, cfg SubagentConfig) *Agent {
	a := &Agent{
		provider: p,
		tools:    t,
		session:  s,
		system:   systemPrompt,
		effort:   "medium",
		mode:     ModeAct,
	}
	t.SetPermissionMode(tools.PermissionModeAct)
	a.subagents = NewSubagentSupervisor(a, cfg)
	return a
}

func SubagentConfigFromSettings(s config.SubagentSettings) SubagentConfig {
	return SubagentConfig{
		MaxConcurrent:      s.MaxConcurrent,
		DefaultTimeout:     time.Duration(s.DefaultTimeoutSecs) * time.Second,
		MaxCompletedRetain: s.MaxCompletedRetain,
	}
}

func (a *Agent) Provider() ai.Provider          { return a.provider }
func (a *Agent) Tools() *tools.Registry         { return a.tools }
func (a *Agent) System() string                 { return a.system }
func (a *Agent) Subagents() *SubagentSupervisor { return a.subagents }

func (a *Agent) RegisterSubagentTools() {
	tools.RegisterSubagentTools(a.tools, a.subagents)
}

func (a *Agent) RegisterSubagentToolsEnabled(enabled bool) {
	if enabled {
		a.RegisterSubagentTools()
		return
	}
	tools.RegisterSubagentTools(a.tools, tools.DisabledSubagentManager())
}

func (a *Agent) ReasoningEffort() string          { return a.effort }
func (a *Agent) SetReasoningEffort(effort string) { a.effort = effort }

func (a *Agent) Mode() Mode {
	if a.mode == "" {
		return ModeAct
	}
	return a.mode
}

func (a *Agent) SetMode(mode Mode) {
	if mode == "" {
		mode = ModeAct
	}
	a.mode = mode
	if a.tools == nil {
		return
	}
	if mode == ModePlan {
		a.tools.SetPermissionMode(tools.PermissionModePlan)
		return
	}
	a.tools.SetPermissionMode(tools.PermissionModeAct)
}
