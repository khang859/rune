package session

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"

	"github.com/khang859/rune/internal/ai"
)

type Session struct {
	mu sync.RWMutex

	ID        string
	Name      string
	Created   time.Time
	Updated   time.Time
	Provider  string
	Model     string
	Effort    string
	Cwd       string
	Root      *Node
	Active    *Node
	Subagents []SubagentTask
	FilesRead []string
	path      string
}

type Node struct {
	ID       string
	Parent   *Node `json:"-"`
	Children []*Node
	Message  ai.Message
	Usage    ai.Usage
	Created  time.Time
	// DurationMs is the wall-clock time of the turn that produced this node
	// (request start to stream end), in milliseconds. 0 for non-assistant nodes.
	DurationMs int
	// CompactedCount > 0 marks this node as a compaction summary that
	// replaced N prior messages along its branch.
	CompactedCount int
}

func New(model string) *Session {
	now := time.Now()
	root := &Node{ID: newID(), Created: now}
	return &Session{
		ID:      newID(),
		Created: now,
		Updated: now,
		Model:   model,
		Root:    root,
		Active:  root,
	}
}

// SetPath assigns the file path used by Save. Callers in cmd/rune use this
// to place sessions under ~/.rune/sessions; tests use it to point at a temp dir.
func (s *Session) SetPath(p string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.path = p
}

func (s *Session) Path() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.path
}

func (s *Session) Append(msg ai.Message) *Node {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.appendLocked(msg)
}

// AppendWithUsage appends msg and records its token usage and turn duration in
// the same lock acquisition. Callers must not set Usage on the returned node
// themselves: once Append returns, the node is reachable by concurrent tree
// walkers (e.g. Save), so any later unlocked write races with them.
func (s *Session) AppendWithUsage(msg ai.Message, usage ai.Usage, durationMs int) *Node {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := s.appendLocked(msg)
	n.Usage = usage
	n.DurationMs = durationMs
	return n
}

// SetEffort records the reasoning-effort level used for the most recent turn.
func (s *Session) SetEffort(effort string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Effort = effort
}

func (s *Session) appendLocked(msg ai.Message) *Node {
	n := &Node{
		ID:      newID(),
		Parent:  s.Active,
		Message: msg,
		Created: time.Now(),
	}
	s.Active.Children = append(s.Active.Children, n)
	s.Active = n
	return n
}

func (s *Session) Fork(target *Node) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Active = target
}

func (s *Session) Clone() *Session {
	s.mu.RLock()
	provider := s.Provider
	model := s.Model
	name := s.Name
	cwd := s.Cwd
	var msgs []ai.Message
	for n := s.Active; n != nil && n.Parent != nil; n = n.Parent {
		msgs = append([]ai.Message{n.Message}, msgs...)
	}
	s.mu.RUnlock()

	nc := New(model)
	nc.Provider = provider
	nc.Name = name
	nc.Cwd = cwd
	// Copy the active path: walk up to root, reverse, replay Append.
	for _, m := range msgs {
		nc.Append(m)
	}
	return nc
}

// PathToActive returns the messages from the first child of root down to Active (excluding root).
func (s *Session) PathToActive() []ai.Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.pathToActiveLocked()
}

func (s *Session) pathToActiveLocked() []ai.Message {
	var msgs []ai.Message
	for n := s.Active; n != nil && n.Parent != nil; n = n.Parent {
		msgs = append([]ai.Message{n.Message}, msgs...)
	}
	return msgs
}

// PathToActiveNodes returns the nodes from the first child of root down to
// Active (excluding root). Use when callers need per-node metadata (e.g.
// CompactedCount) that PathToActive's []ai.Message strips away.
func (s *Session) PathToActiveNodes() []*Node {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var nodes []*Node
	for n := s.Active; n != nil && n.Parent != nil; n = n.Parent {
		nodes = append([]*Node{n}, nodes...)
	}
	return nodes
}

// UsageStats aggregates token, latency, and turn metadata across the active
// branch (root → Active), plus subagent token totals for the whole session.
type UsageStats struct {
	Provider       string
	Model          string
	Effort         string
	Created        time.Time
	Updated        time.Time
	Turns          int
	Input          int
	Output         int
	CacheRead      int
	DurationMs     int
	SubagentCount  int
	SubagentInput  int
	SubagentOutput int
}

// UsageStats computes per-session usage metadata over the active branch.
func (s *Session) UsageStats() UsageStats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	st := UsageStats{
		Provider: s.Provider,
		Model:    s.Model,
		Effort:   s.Effort,
		Created:  s.Created,
		Updated:  s.Updated,
	}
	for n := s.Active; n != nil && n.Parent != nil; n = n.Parent {
		if n.Usage.Input == 0 && n.Usage.Output == 0 && n.Usage.CacheRead == 0 {
			continue
		}
		st.Turns++
		st.Input += n.Usage.Input
		st.Output += n.Usage.Output
		st.CacheRead += n.Usage.CacheRead
		st.DurationMs += n.DurationMs
	}
	for _, t := range s.Subagents {
		st.SubagentCount++
		st.SubagentInput += t.InputTokens
		st.SubagentOutput += t.OutputTokens
	}
	return st
}

const maxFilesRead = 50

// RecordFileRead prepends path to FilesRead, deduping and capping at 50.
// Called by tools/read.go on every successful read.
func (s *Session) RecordFileRead(path string) {
	if path == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []string{path}
	for _, p := range s.FilesRead {
		if p == path {
			continue
		}
		out = append(out, p)
		if len(out) >= maxFilesRead {
			break
		}
	}
	s.FilesRead = out
}

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
