package agent

import (
	"context"
	"fmt"
	"sort"
	"strconv"
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
	SubagentBlocked   SubagentStatus = "blocked"
	SubagentPending   SubagentStatus = "pending"
	SubagentRunning   SubagentStatus = "running"
	SubagentCompleted SubagentStatus = "completed"
	SubagentFailed    SubagentStatus = "failed"
	SubagentCancelled SubagentStatus = "cancelled"
)

type SubagentTask struct {
	ID           string         `json:"task_id"`
	Name         string         `json:"name"`
	FamiliarName string         `json:"familiar_name,omitempty"`
	AgentType    string         `json:"agent_type"`
	Prompt       string         `json:"-"`
	TimeoutSecs  int            `json:"-"`
	Status       SubagentStatus `json:"status"`
	Dependencies []string       `json:"dependencies,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	StartedAt    *time.Time     `json:"started_at,omitempty"`
	CompletedAt  *time.Time     `json:"completed_at,omitempty"`
	Summary      string         `json:"summary,omitempty"`
	Error        string         `json:"error,omitempty"`
	injected     bool
}

type SubagentEvent struct {
	Task   tools.SubagentTask `json:"task"`
	Status SubagentStatus     `json:"status"`
}

type SubagentToolMode string

const (
	SubagentToolsReadOnly SubagentToolMode = "readonly"
	SubagentToolsFull     SubagentToolMode = "full"
)

type SubagentDefinition struct {
	Name         string
	Description  string
	Model        string
	Timeout      time.Duration
	Tools        SubagentToolMode
	Instructions string
	Path         string
}

type SubagentTypeInfo struct {
	Name        string
	Builtin     bool
	Description string
	Model       string
	Timeout     time.Duration
	Tools       SubagentToolMode
	Path        string
}

type SubagentConfig struct {
	MaxConcurrent      int
	DefaultTimeout     time.Duration
	MaxCompletedRetain int
	Definitions        map[string]SubagentDefinition
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
	cfg.Definitions = normalizeSubagentDefinitions(cfg.Definitions)
	s := &SubagentSupervisor{
		parent:      parent,
		cfg:         cfg,
		sem:         make(chan struct{}, cfg.MaxConcurrent),
		tasks:       map[string]*SubagentTask{},
		cancels:     map[string]context.CancelFunc{},
		subscribers: map[chan SubagentEvent]struct{}{},
	}
	s.restoreFromSession()
	return s
}

func advanceSubagentSeqAtLeast(n uint64) {
	for {
		cur := atomic.LoadUint64(&subagentSeq)
		if cur >= n {
			return
		}
		if atomic.CompareAndSwapUint64(&subagentSeq, cur, n) {
			return
		}
	}
}

func subagentNumericSuffix(id string) (uint64, bool) {
	suffix, ok := strings.CutPrefix(id, "subagent_")
	if !ok || suffix == "" {
		return 0, false
	}
	n, err := strconv.ParseUint(suffix, 10, 64)
	if err != nil {
		return 0, false
	}
	return n, true
}

func (s *SubagentSupervisor) restoreFromSession() {
	if s.parent == nil || s.parent.session == nil {
		return
	}
	restored := s.parent.session.SubagentTasks()
	if len(restored) == 0 {
		return
	}
	now := time.Now()
	var maxRestoredSeq uint64
	for _, wt := range restored {
		if strings.TrimSpace(wt.ID) == "" {
			continue
		}
		if n, ok := subagentNumericSuffix(wt.ID); ok && n > maxRestoredSeq {
			maxRestoredSeq = n
		}
		status := SubagentStatus(wt.Status)
		switch status {
		case SubagentCompleted, SubagentFailed, SubagentCancelled:
		case SubagentBlocked, SubagentPending, SubagentRunning:
			status = SubagentCancelled
			if wt.CompletedAt == nil {
				wt.CompletedAt = &now
			}
			if strings.TrimSpace(wt.Error) == "" {
				wt.Error = "session restored before subagent completed"
			}
		default:
			status = SubagentCancelled
			if wt.CompletedAt == nil {
				wt.CompletedAt = &now
			}
			if strings.TrimSpace(wt.Error) == "" {
				wt.Error = "session restored with unknown subagent status"
			}
		}
		t := &SubagentTask{
			ID:           wt.ID,
			Name:         wt.Name,
			FamiliarName: strings.TrimSpace(wt.FamiliarName),
			AgentType:    wt.AgentType,
			Status:       status,
			Dependencies: append([]string(nil), wt.Dependencies...),
			CreatedAt:    wt.CreatedAt,
			StartedAt:    wt.StartedAt,
			CompletedAt:  wt.CompletedAt,
			Summary:      wt.Summary,
			Error:        wt.Error,
			injected:     status == SubagentCompleted,
		}
		if t.FamiliarName == "" {
			t.FamiliarName = s.nextFamiliarNameLocked()
		}
		s.tasks[t.ID] = t
		s.order = append(s.order, t.ID)
	}
	advanceSubagentSeqAtLeast(maxRestoredSeq)
	s.pruneLocked()
	s.persistLocked()
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
	def, ok := s.subagentDefinition(req.AgentType)
	if !ok {
		return nil, fmt.Errorf("unknown agent_type %q. Available subagent types: %s", req.AgentType, strings.Join(s.availableSubagentTypes(), ", "))
	}
	if req.TimeoutSecs <= 0 && def.Timeout > 0 {
		req.TimeoutSecs = int(def.Timeout / time.Second)
	}

	deps := cleanDependencies(req.Dependencies)
	id := fmt.Sprintf("subagent_%d", atomic.AddUint64(&subagentSeq, 1))
	startNow := true
	t := &SubagentTask{
		ID:           id,
		Name:         strings.TrimSpace(req.Name),
		AgentType:    req.AgentType,
		Prompt:       strings.TrimSpace(req.Prompt),
		TimeoutSecs:  req.TimeoutSecs,
		Status:       SubagentPending,
		Dependencies: deps,
		CreatedAt:    time.Now(),
	}
	req.Dependencies = deps

	s.mu.Lock()
	t.FamiliarName = s.nextFamiliarNameLocked()
	for _, depID := range deps {
		dep := s.tasks[depID]
		if dep == nil {
			s.mu.Unlock()
			return nil, fmt.Errorf("unknown dependency task %q", depID)
		}
		switch dep.Status {
		case SubagentCompleted:
		case SubagentFailed, SubagentCancelled:
			t.Status = SubagentFailed
			now := time.Now()
			t.CompletedAt = &now
			t.Error = fmt.Sprintf("dependency %s is %s", depID, dep.Status)
			startNow = false
		default:
			t.Status = SubagentBlocked
			startNow = false
		}
		if t.Status == SubagentFailed {
			break
		}
	}
	s.tasks[id] = t
	s.order = append(s.order, id)
	s.pruneLocked()
	s.persistLocked()
	s.publishLocked(t)
	s.mu.Unlock()

	done := make(chan struct{})
	if startNow {
		go func() {
			defer close(done)
			s.runTask(ctx, t.ID, req)
		}()
	} else {
		close(done)
	}

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

func (s *SubagentSupervisor) SetDefinitions(defs map[string]SubagentDefinition) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cfg.Definitions = normalizeSubagentDefinitions(defs)
}

func (s *SubagentSupervisor) Types() []SubagentTypeInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SubagentTypeInfo, 0, len(subagentTypes)+len(s.cfg.Definitions))
	for _, name := range subagentTypes {
		out = append(out, SubagentTypeInfo{Name: name, Builtin: true, Tools: SubagentToolsReadOnly})
	}
	names := make([]string, 0, len(s.cfg.Definitions))
	for name := range s.cfg.Definitions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		def := s.cfg.Definitions[name]
		out = append(out, SubagentTypeInfo{
			Name:        def.Name,
			Builtin:     false,
			Description: def.Description,
			Model:       def.Model,
			Timeout:     def.Timeout,
			Tools:       def.Tools,
			Path:        def.Path,
		})
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
	var toStart []string
	s.mu.Lock()
	cancel := s.cancels[id]
	t := s.tasks[id]
	if t == nil {
		s.mu.Unlock()
		return fmt.Errorf("unknown subagent task %q", id)
	}
	if t.Status == SubagentPending || t.Status == SubagentBlocked {
		now := time.Now()
		t.Status = SubagentCancelled
		t.CompletedAt = &now
		s.publishLocked(t)
		toStart = s.resolveBlockedLocked()
		s.persistLocked()
	}
	s.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	s.startReadyTasks(context.Background(), toStart)
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
		s.persistLocked()
		s.publishLocked(t)
	}
	s.mu.Unlock()

	prompt := s.promptWithDependencySummaries(req)
	summary, err := s.runIsolatedAgent(ctx, req, prompt)
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

func (s *SubagentSupervisor) runIsolatedAgent(ctx context.Context, req tools.SpawnSubagentRequest, prompt string) (string, error) {
	def, _ := s.subagentDefinition(req.AgentType)
	model := s.parent.session.Model
	if strings.TrimSpace(def.Model) != "" {
		model = strings.TrimSpace(def.Model)
	}
	childSession := session.New(model)
	childTools := s.parent.tools.CloneReadOnly()
	if def.Tools == SubagentToolsFull {
		childTools = s.parent.tools.CloneForSubagentFull()
	}
	childSystem := s.parent.system
	if childSystem != "" {
		childSystem += "\n\n"
	}
	childSystem += subagentSystemPrompt(req.AgentType, def)
	child := New(s.parent.provider, childTools, childSession, childSystem)
	child.SetReasoningEffort(s.parent.effort)

	msg := ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: prompt}}}
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
	var toStart []string
	s.mu.Lock()
	t := s.tasks[id]
	if t == nil {
		s.mu.Unlock()
		return
	}
	if t.Status == SubagentCancelled && status != SubagentCancelled {
		s.mu.Unlock()
		return
	}
	now := time.Now()
	t.Status = status
	t.CompletedAt = &now
	t.Summary = summary
	t.Error = errMsg
	s.publishLocked(t)
	toStart = s.resolveBlockedLocked()
	s.persistLocked()
	s.mu.Unlock()
	s.startReadyTasks(context.Background(), toStart)
}

func (s *SubagentSupervisor) resolveBlockedLocked() []string {
	var toStart []string
	for _, id := range s.order {
		t := s.tasks[id]
		if t == nil || t.Status != SubagentBlocked {
			continue
		}
		ready := true
		for _, depID := range t.Dependencies {
			dep := s.tasks[depID]
			if dep == nil {
				now := time.Now()
				t.Status = SubagentFailed
				t.CompletedAt = &now
				t.Error = fmt.Sprintf("dependency %s is missing", depID)
				s.persistLocked()
				s.publishLocked(t)
				ready = false
				break
			}
			switch dep.Status {
			case SubagentCompleted:
			case SubagentFailed, SubagentCancelled:
				now := time.Now()
				t.Status = SubagentFailed
				t.CompletedAt = &now
				t.Error = fmt.Sprintf("dependency %s is %s", depID, dep.Status)
				s.persistLocked()
				s.publishLocked(t)
				ready = false
			default:
				ready = false
			}
			if !ready {
				break
			}
		}
		if ready {
			t.Status = SubagentPending
			s.persistLocked()
			s.publishLocked(t)
			toStart = append(toStart, id)
		}
	}
	return toStart
}

func (s *SubagentSupervisor) startReadyTasks(ctx context.Context, ids []string) {
	for _, id := range ids {
		s.mu.Lock()
		t := s.tasks[id]
		if t == nil || t.Status != SubagentPending {
			s.mu.Unlock()
			continue
		}
		req := tools.SpawnSubagentRequest{Name: t.Name, Prompt: t.Prompt, AgentType: t.AgentType, Background: true, Dependencies: append([]string(nil), t.Dependencies...), TimeoutSecs: t.TimeoutSecs}
		s.mu.Unlock()
		go s.runTask(ctx, id, req)
	}
}

func (s *SubagentSupervisor) promptWithDependencySummaries(req tools.SpawnSubagentRequest) string {
	deps := cleanDependencies(req.Dependencies)
	if len(deps) == 0 {
		return req.Prompt
	}
	s.mu.Lock()
	deferred := make([]tools.SubagentTask, 0, len(deps))
	for _, id := range deps {
		if t := s.tasks[id]; t != nil && t.Status == SubagentCompleted && strings.TrimSpace(t.Summary) != "" {
			deferred = append(deferred, toToolSubagentTask(t))
		}
	}
	s.mu.Unlock()
	if len(deferred) == 0 {
		return req.Prompt
	}
	var b strings.Builder
	b.WriteString("Dependency results:\n\n")
	for _, dep := range deferred {
		b.WriteString("[Dependency completed: ")
		b.WriteString(dep.ID)
		if dep.Name != "" {
			b.WriteString(" / ")
			b.WriteString(dep.Name)
		}
		b.WriteString("]\n\n")
		b.WriteString(dep.Summary)
		b.WriteString("\n\n")
	}
	b.WriteString("Delegated task:\n\n")
	b.WriteString(req.Prompt)
	return b.String()
}

func (s *SubagentSupervisor) persistLocked() {
	if s.parent == nil || s.parent.session == nil {
		return
	}
	tasks := make([]session.SubagentTask, 0, len(s.order))
	for _, id := range s.order {
		t := s.tasks[id]
		if t == nil {
			continue
		}
		tasks = append(tasks, session.SubagentTask{
			ID:           t.ID,
			Name:         t.Name,
			FamiliarName: t.FamiliarName,
			AgentType:    t.AgentType,
			Status:       string(t.Status),
			Dependencies: append([]string(nil), t.Dependencies...),
			CreatedAt:    t.CreatedAt,
			StartedAt:    t.StartedAt,
			CompletedAt:  t.CompletedAt,
			Summary:      t.Summary,
			Error:        t.Error,
		})
	}
	s.parent.session.SetSubagents(tasks)
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
	s.persistLocked()
}

func cleanDependencies(deps []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(deps))
	for _, dep := range deps {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			continue
		}
		if _, ok := seen[dep]; ok {
			continue
		}
		seen[dep] = struct{}{}
		out = append(out, dep)
	}
	return out
}

var subagentTypes = []string{"general", "exploration", "validator", "code-explorer", "code-architect", "code-reviewer"}

func BuiltinSubagentTypeSet() map[string]bool {
	out := map[string]bool{}
	for _, typ := range subagentTypes {
		out[typ] = true
	}
	return out
}

func normalizeSubagentType(t string) string {
	t = strings.ToLower(strings.TrimSpace(t))
	if t == "" {
		return "general"
	}
	return t
}

func (s *SubagentSupervisor) subagentDefinition(t string) (SubagentDefinition, bool) {
	t = normalizeSubagentType(t)
	for _, typ := range subagentTypes {
		if t == typ {
			return SubagentDefinition{Name: typ, Tools: SubagentToolsReadOnly}, true
		}
	}
	if s != nil {
		def, ok := s.cfg.Definitions[t]
		return def, ok
	}
	return SubagentDefinition{}, false
}

func (s *SubagentSupervisor) availableSubagentTypes() []string {
	out := append([]string(nil), subagentTypes...)
	if s != nil {
		for name := range s.cfg.Definitions {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func normalizeSubagentDefinitions(in map[string]SubagentDefinition) map[string]SubagentDefinition {
	out := map[string]SubagentDefinition{}
	for name, def := range in {
		if def.Name == "" {
			def.Name = name
		}
		def.Name = normalizeSubagentType(def.Name)
		if def.Name == "" {
			continue
		}
		reserved := false
		for _, typ := range subagentTypes {
			if def.Name == typ {
				reserved = true
				break
			}
		}
		if reserved {
			continue
		}
		if def.Tools == "" {
			def.Tools = SubagentToolsReadOnly
		}
		out[def.Name] = def
	}
	return out
}

var familiarNames = []string{
	"Nyx", "Puck", "Moth", "Ash", "Bramble", "Wisp", "Thistle", "Ember",
	"Sable", "Pip", "Rune", "Fable", "Morrow", "Quill", "Vesper", "Lumen",
}

func (s *SubagentSupervisor) nextFamiliarNameLocked() string {
	used := map[string]struct{}{}
	for _, t := range s.tasks {
		if t != nil && strings.TrimSpace(t.FamiliarName) != "" {
			used[t.FamiliarName] = struct{}{}
		}
	}
	for _, name := range familiarNames {
		if _, ok := used[name]; !ok {
			return name
		}
	}
	for generation := 2; ; generation++ {
		for _, name := range familiarNames {
			candidate := fmt.Sprintf("%s %s", name, romanNumeral(generation))
			if _, ok := used[candidate]; !ok {
				return candidate
			}
		}
	}
}

func romanNumeral(n int) string {
	if n <= 0 || n > 20 {
		return fmt.Sprintf("%d", n)
	}
	vals := []struct {
		value   int
		numeral string
	}{
		{10, "X"}, {9, "IX"}, {5, "V"}, {4, "IV"}, {1, "I"},
	}
	var b strings.Builder
	for _, v := range vals {
		for n >= v.value {
			b.WriteString(v.numeral)
			n -= v.value
		}
	}
	return b.String()
}

func toToolSubagentTask(t *SubagentTask) tools.SubagentTask {
	return tools.SubagentTask{
		ID:           t.ID,
		Name:         t.Name,
		FamiliarName: t.FamiliarName,
		AgentType:    t.AgentType,
		Status:       string(t.Status),
		Dependencies: append([]string(nil), t.Dependencies...),
		CreatedAt:    t.CreatedAt,
		StartedAt:    t.StartedAt,
		CompletedAt:  t.CompletedAt,
		Summary:      t.Summary,
		Error:        t.Error,
	}
}

func subagentSystemPrompt(agentType string, def SubagentDefinition) string {
	base := `You are a rune subagent of type "` + agentType + `".

Work independently on the delegated task using your own isolated context. Keep your scope narrow. Prefer reading and analysis. Do not attempt to modify files or run shell commands unless explicitly available as tools.`

	specialized := ""
	switch agentType {
	case "exploration":
		specialized = `

Exploration focus:
- Discover the relevant files, functions, data flow, and existing tests.
- Use read-only tools to gather evidence before drawing conclusions.
- Do not propose broad rewrites unless the codebase evidence supports them.
- Emphasize concrete implementation touchpoints and open questions.`
	case "validator":
		specialized = `

Validator focus:
- Review the proposed plan for missing files, unsafe assumptions, and sequencing issues.
- Check that the plan includes appropriate tests and validation commands.
- Identify risks, edge cases, and simpler alternatives.
- Do not implement; return approval concerns and suggested revisions.`
	case "code-explorer":
		specialized = `

Code-explorer focus:
- Deeply analyze the existing codebase for the delegated feature area using read-only tools and repository evidence.
- Find entry points such as APIs, UI components, CLI commands, background jobs, tests, and configuration.
- Trace execution paths, data transformations, state changes, dependencies, integrations, side effects, error handling, edge cases, and performance-sensitive paths.
- Map feature boundaries, architecture layers, conventions, abstractions, and similar existing implementations.
- Return concrete file:line references for important evidence and a concise list of essential files the parent agent should read next.
- Do not design or implement the feature unless the delegated task explicitly asks for lightweight implications; keep the focus on discovery.`
	case "code-architect":
		specialized = `

Code-architect focus:
- Design a concrete implementation blueprint grounded in existing codebase patterns, conventions, project guidelines, and similar features.
- Identify module boundaries, abstraction layers, integration points, data flow, state management, error handling, security, performance, compatibility, and test strategy.
- Make clear architectural choices with brief tradeoffs; avoid vague option lists when evidence supports a recommendation.
- Specify every file to create or modify, each component's responsibility, and an ordered build sequence.
- Include validation steps and risks the parent agent should account for before editing.
- Do not edit files; return an actionable architecture plan for the parent agent.`
	case "code-reviewer":
		specialized = `

Code-reviewer focus:
- Review code changes for real bugs, logic errors, security vulnerabilities, broken tests, regressions, and meaningful convention mismatches.
- Prefer unstaged changes from git diff when no narrower review target is provided; also check relevant project guidance such as AGENTS.md or CLAUDE.md when available.
- Report only high-confidence, actionable issues with confidence >= 80. Avoid speculative concerns, style nitpicks, and low-signal suggestions.
- For each issue include severity, confidence score, file:line reference, why it is a problem, and a concrete fix suggestion.
- Group findings by severity. If there are no high-confidence issues, say that the reviewed code meets the requested standards.
- Do not modify files; provide review findings only.`
	}

	custom := ""
	if strings.TrimSpace(def.Instructions) != "" {
		custom = "\n\nCustom subagent instructions:\n" + strings.TrimSpace(def.Instructions)
	}

	return base + specialized + custom + `

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
