package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/khang859/rune/internal/ai"
)

type wireSession struct {
	ID       string     `json:"id"`
	Name     string     `json:"name,omitempty"`
	Created  string     `json:"created"`
	Model    string     `json:"model"`
	RootID   string     `json:"root_id"`
	ActiveID string     `json:"active_id"`
	Nodes    []wireNode `json:"nodes"`
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
	s.mu.RLock()
	defer s.mu.RUnlock()

	if s.path == "" {
		return fmt.Errorf("session path is empty; set with SetPath or Load")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	w := wireSession{
		ID:       s.ID,
		Name:     s.Name,
		Created:  s.Created.Format("2006-01-02T15:04:05Z07:00"),
		Model:    s.Model,
		RootID:   s.Root.ID,
		ActiveID: s.Active.ID,
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
	b, err := json.MarshalIndent(w, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	f, err := os.Create(tmp)
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
	return os.Rename(tmp, s.path)
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
		n := &Node{ID: wn.ID, Usage: wn.Usage, CompactedCount: wn.CompactedCount}
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
	return &Session{
		ID:     w.ID,
		Name:   w.Name,
		Model:  w.Model,
		Root:   nodes[w.RootID],
		Active: nodes[w.ActiveID],
		path:   path,
	}, nil
}

func walk(n *Node, fn func(*Node)) {
	fn(n)
	for _, c := range n.Children {
		walk(c, fn)
	}
}
