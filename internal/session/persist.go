package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/khang859/rune/internal/ai"
)

type wireSession struct {
	ID        string         `json:"id"`
	Name      string         `json:"name,omitempty"`
	Created   string         `json:"created"`
	Provider  string         `json:"provider,omitempty"`
	Model     string         `json:"model"`
	RootID    string         `json:"root_id"`
	ActiveID  string         `json:"active_id"`
	Nodes     []wireNode     `json:"nodes"`
	Subagents []SubagentTask `json:"subagents,omitempty"`
}

type wireNode struct {
	ID             string     `json:"id"`
	ParentID       string     `json:"parent_id,omitempty"`
	ChildIDs       []string   `json:"children,omitempty"`
	Message        ai.Message `json:"message,omitempty"`
	HasMessage     bool       `json:"has_message"`
	Usage          ai.Usage   `json:"usage,omitempty"`
	Created        string     `json:"created"`
	CompactedCount int        `json:"compacted_count,omitempty"`
}

func (s *Session) Save() error {
	path, w, err := s.snapshotForSave()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	_ = os.Chmod(filepath.Dir(path), 0o700)
	b, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.Write(b); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

func (s *Session) snapshotForSave() (string, wireSession, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.path == "" {
		return "", wireSession{}, fmt.Errorf("session path is empty; set with SetPath or Load")
	}
	w := wireSession{
		ID:        s.ID,
		Name:      s.Name,
		Created:   s.Created.Format("2006-01-02T15:04:05Z07:00"),
		Provider:  normalizeProvider(s.Provider),
		Model:     s.Model,
		RootID:    s.Root.ID,
		ActiveID:  s.Active.ID,
		Subagents: cloneSubagentTasks(s.Subagents),
	}
	walk(s.Root, func(n *Node) {
		wn := wireNode{
			ID:             n.ID,
			Usage:          n.Usage,
			Created:        n.Created.Format("2006-01-02T15:04:05Z07:00"),
			CompactedCount: n.CompactedCount,
		}
		if n.Parent != nil {
			wn.ParentID = n.Parent.ID
		}
		for _, c := range n.Children {
			wn.ChildIDs = append(wn.ChildIDs, c.ID)
		}
		if n != s.Root {
			wn.Message = n.Message
			wn.HasMessage = true
		}
		w.Nodes = append(w.Nodes, wn)
	})
	return s.path, w, nil
}

func Load(path string) (*Session, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var w wireSession
	if err := json.Unmarshal(b, &w); err != nil {
		return nil, err
	}
	nodes := map[string]*Node{}
	for _, wn := range w.Nodes {
		created, _ := time.Parse(time.RFC3339, wn.Created)
		n := &Node{ID: wn.ID, Usage: wn.Usage, Created: created, CompactedCount: wn.CompactedCount}
		if wn.HasMessage {
			n.Message = wn.Message
		}
		nodes[wn.ID] = n
	}
	for _, wn := range w.Nodes {
		n := nodes[wn.ID]
		if wn.ParentID != "" {
			n.Parent = nodes[wn.ParentID]
		}
		for _, cid := range wn.ChildIDs {
			n.Children = append(n.Children, nodes[cid])
		}
	}
	created, _ := time.Parse(time.RFC3339, w.Created)
	return &Session{
		ID:        w.ID,
		Name:      w.Name,
		Created:   created,
		Provider:  normalizeProvider(w.Provider),
		Model:     w.Model,
		Root:      nodes[w.RootID],
		Active:    nodes[w.ActiveID],
		Subagents: cloneSubagentTasks(w.Subagents),
		path:      path,
	}, nil
}

func walk(n *Node, fn func(*Node)) {
	fn(n)
	for _, c := range n.Children {
		walk(c, fn)
	}
}

func normalizeProvider(provider string) string {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "", "codex", "groq", "ollama", "runpod":
		return strings.ToLower(strings.TrimSpace(provider))
	default:
		return "codex"
	}
}
