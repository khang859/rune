package agent

import (
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

type Agent struct {
	provider ai.Provider
	tools    *tools.Registry
	session  *session.Session
	system   string
}

func New(p ai.Provider, t *tools.Registry, s *session.Session, systemPrompt string) *Agent {
	return &Agent{provider: p, tools: t, session: s, system: systemPrompt}
}

func (a *Agent) Provider() ai.Provider  { return a.provider }
func (a *Agent) Tools() *tools.Registry { return a.tools }
func (a *Agent) System() string         { return a.system }
