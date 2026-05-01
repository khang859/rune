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
	Provider  string
	Model     string
	Root      *Node
	Active    *Node
	Subagents []SubagentTask
	path      string
}

type Node struct {
	ID       string
	Parent   *Node `json:"-"`
	Children []*Node
	Message  ai.Message
	Usage    ai.Usage
	Created  time.Time
	// CompactedCount > 0 marks this node as a compaction summary that
	// replaced N prior messages along its branch.
	CompactedCount int
}

func New(model string) *Session {
	root := &Node{ID: newID(), Created: time.Now()}
	return &Session{
		ID:      newID(),
		Created: time.Now(),
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
	var msgs []ai.Message
	for n := s.Active; n != nil && n.Parent != nil; n = n.Parent {
		msgs = append([]ai.Message{n.Message}, msgs...)
	}
	s.mu.RUnlock()

	nc := New(model)
	nc.Provider = provider
	nc.Name = name
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

func newID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
