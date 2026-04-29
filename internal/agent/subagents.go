package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

type SubagentStatus string

const (
	SubagentPending   SubagentStatus = "pending"
	SubagentRunning   SubagentStatus = "running"
	SubagentCompleted SubagentStatus = "completed"
	SubagentFailed    SubagentStatus = "failed"
	SubagentCancelled SubagentStatus = "cancelled"
)

type SubagentTask struct {
	ID          string         `json:"task_id"`
	Name        string         `json:"name"`
	AgentType   string         `json:"agent_type"`
	Prompt      string         `json:"-"`
	Status      SubagentStatus `json:"status"`
	CreatedAt   time.Time      `json:"created_at"`
	StartedAt   *time.Time     `json:"started_at,omitempty"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Summary     string         `json:"summary,omitempty"`
	Error       string         `json:"error,omitempty"`
	injected    bool
}

type SubagentEvent struct {
	Task   tools.SubagentTask `json:"task"`
	Status SubagentStatus     `json:"status"`
}

type SubagentConfig struct {
	MaxConcurrent      int
	DefaultTimeout     time.Duration
	MaxCompletedRetain int
}

type SubagentSupervisor struct {
	parent *Agent
	cfg    SubagentConfig
	sem    chan struct{}

	mu          sync.Mutex
	tasks       map[string]*SubagentTask
	order       []string
	cancels     map[string]context.CancelFunc
	subscribers map[chan SubagentEvent]struct{}
}

var subagentSeq uint64

func NewSubagentSupervisor(parent *Agent, cfg SubagentConfig) *SubagentSupervisor {
	if cfg.MaxConcurrent <= 0 {
		cfg.MaxConcurrent = 4
	}
	if cfg.DefaultTimeout <= 0 {
		cfg.DefaultTimeout = 10 * time.Minute
	}
	if cfg.MaxCompletedRetain <= 0 {
		cfg.MaxCompletedRetain = 100
	}
	return &SubagentSupervisor{
		parent:      parent,
		cfg:         cfg,
		sem:         make(chan struct{}, cfg.MaxConcurrent),
		tasks:       map[string]*SubagentTask{},
		cancels:     map[string]context.CancelFunc{},
		subscribers: map[chan SubagentEvent]struct{}{},
	}
}

func (s *SubagentSupervisor) Subscribe(ctx context.Context) <-chan SubagentEvent {
	ch := make(chan SubagentEvent, 16)
	s.mu.Lock()
	for _, id := range s.order {
		if t := s.tasks[id]; t != nil {
			ch <- SubagentEvent{Task: toToolSubagentTask(t), Status: t.Status}
		}
	}
	s.subscribers[ch] = struct{}{}
	s.mu.Unlock()

	go func() {
		<-ctx.Done()
		s.mu.Lock()
		delete(s.subscribers, ch)
		close(ch)
		s.mu.Unlock()
	}()
	return ch
}

func (s *SubagentSupervisor) Spawn(ctx context.Context, req tools.SpawnSubagentRequest) (*tools.SubagentTask, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, fmt.Errorf("name is required")
	}
	if strings.TrimSpace(req.Prompt) == "" {
		return nil, fmt.Errorf("prompt is required")
	}
	req.AgentType = normalizeSubagentType(req.AgentType)
	if req.AgentType != "general" {
		return nil, fmt.Errorf("unknown agent_type %q. Available subagent types: general", req.AgentType)
	}

	id := fmt.Sprintf("subagent_%d", atomic.AddUint64(&subagentSeq, 1))
	t := &SubagentTask{
		ID:        id,
		Name:      strings.TrimSpace(req.Name),
		AgentType: req.AgentType,
		Prompt:    strings.TrimSpace(req.Prompt),
		Status:    SubagentPending,
		CreatedAt: time.Now(),
	}

	s.mu.Lock()
	s.tasks[id] = t
	s.order = append(s.order, id)
	s.pruneLocked()
	s.publishLocked(t)
	s.mu.Unlock()

	done := make(chan struct{})
	go func() {
		defer close(done)
		s.runTask(ctx, t.ID, req)
	}()

	if !req.Background {
		select {
		case <-done:
		case <-ctx.Done():
			_ = s.Cancel(t.ID)
			<-done
		}
	}
	return s.Get(t.ID), nil
}

func (s *SubagentSupervisor) List() []tools.SubagentTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]tools.SubagentTask, 0, len(s.order))
	for _, id := range s.order {
		if t := s.tasks[id]; t != nil {
			out = append(out, toToolSubagentTask(t))
		}
	}
	return out
}

func (s *SubagentSupervisor) Get(id string) *tools.SubagentTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tasks[id]
	if t == nil {
		return nil
	}
	cp := toToolSubagentTask(t)
	return &cp
}

func (s *SubagentSupervisor) DrainCompletedSummaries() []tools.SubagentTask {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []tools.SubagentTask
	for _, id := range s.order {
		t := s.tasks[id]
		if t == nil || t.injected || t.Status != SubagentCompleted || strings.TrimSpace(t.Summary) == "" {
			continue
		}
		t.injected = true
		out = append(out, toToolSubagentTask(t))
	}
	return out
}

func (s *SubagentSupervisor) Cancel(id string) error {
	s.mu.Lock()
	cancel := s.cancels[id]
	t := s.tasks[id]
	if t == nil {
		s.mu.Unlock()
		return fmt.Errorf("unknown subagent task %q", id)
	}
	if t.Status == SubagentPending {
		now := time.Now()
		t.Status = SubagentCancelled
		t.CompletedAt = &now
		s.publishLocked(t)
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return nil
}

func (s *SubagentSupervisor) runTask(parentCtx context.Context, id string, req tools.SpawnSubagentRequest) {
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-parentCtx.Done():
		s.finish(id, SubagentCancelled, "", parentCtx.Err().Error())
		return
	}

	timeout := s.cfg.DefaultTimeout
	if req.TimeoutSecs > 0 {
		requested := time.Duration(req.TimeoutSecs) * time.Second
		if requested < timeout {
			timeout = requested
		}
	}
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	s.mu.Lock()
	if t := s.tasks[id]; t != nil {
		if t.Status == SubagentCancelled {
			s.mu.Unlock()
			return
		}
		now := time.Now()
		t.Status = SubagentRunning
		t.StartedAt = &now
		s.cancels[id] = cancel
		s.publishLocked(t)
	}
	s.mu.Unlock()

	summary, err := s.runIsolatedAgent(ctx, req)
	s.mu.Lock()
	delete(s.cancels, id)
	s.mu.Unlock()
	if err != nil {
		if ctx.Err() != nil {
			s.finish(id, SubagentCancelled, summary, ctx.Err().Error())
			return
		}
		s.finish(id, SubagentFailed, summary, err.Error())
		return
	}
	s.finish(id, SubagentCompleted, summary, "")
}

func (s *SubagentSupervisor) runIsolatedAgent(ctx context.Context, req tools.SpawnSubagentRequest) (string, error) {
	childSession := session.New(s.parent.session.Model)
	childTools := s.parent.tools.CloneReadOnly()
	childSystem := s.parent.system
	if childSystem != "" {
		childSystem += "\n\n"
	}
	childSystem += subagentSystemPrompt(req.AgentType)
	child := New(s.parent.provider, childTools, childSession, childSystem)
	child.SetReasoningEffort(s.parent.effort)

	msg := ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: req.Prompt}}}
	var b strings.Builder
	for ev := range child.Run(ctx, msg) {
		switch v := ev.(type) {
		case AssistantText:
			b.WriteString(v.Delta)
		case TurnError:
			return strings.TrimSpace(b.String()), v.Err
		case TurnAborted:
			return strings.TrimSpace(b.String()), context.Canceled
		}
	}
	return strings.TrimSpace(b.String()), nil
}

func (s *SubagentSupervisor) finish(id string, status SubagentStatus, summary, errMsg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := s.tasks[id]
	if t == nil {
		return
	}
	if t.Status == SubagentCancelled && status != SubagentCancelled {
		return
	}
	now := time.Now()
	t.Status = status
	t.CompletedAt = &now
	t.Summary = summary
	t.Error = errMsg
	s.publishLocked(t)
}

func (s *SubagentSupervisor) publishLocked(t *SubagentTask) {
	ev := SubagentEvent{Task: toToolSubagentTask(t), Status: t.Status}
	for ch := range s.subscribers {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (s *SubagentSupervisor) pruneLocked() {
	if len(s.order) <= s.cfg.MaxCompletedRetain {
		return
	}
	keep := s.order[:0]
	removed := 0
	for _, id := range s.order {
		if len(s.order)-removed <= s.cfg.MaxCompletedRetain {
			keep = append(keep, id)
			continue
		}
		t := s.tasks[id]
		if t != nil && (t.Status == SubagentCompleted || t.Status == SubagentFailed || t.Status == SubagentCancelled) {
			delete(s.tasks, id)
			removed++
			continue
		}
		keep = append(keep, id)
	}
	s.order = keep
}

func normalizeSubagentType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if t == "" {
		return "general"
	}
	return t
}

func toToolSubagentTask(t *SubagentTask) tools.SubagentTask {
	return tools.SubagentTask{
		ID:          t.ID,
		Name:        t.Name,
		AgentType:   t.AgentType,
		Status:      string(t.Status),
		CreatedAt:   t.CreatedAt,
		StartedAt:   t.StartedAt,
		CompletedAt: t.CompletedAt,
		Summary:     t.Summary,
		Error:       t.Error,
	}
}

func subagentSystemPrompt(agentType string) string {
	return `You are a rune subagent of type "` + agentType + `".

Work independently on the delegated task using your own isolated context. Keep your scope narrow. Prefer reading and analysis. Do not attempt to modify files or run shell commands unless explicitly available as tools.

Return a concise structured final answer using this format:

## Summary
...

## Findings
- ...

## Files inspected
- ...

## Risks
- ...

## Recommended next steps
- ...`
}
