package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/khang859/rune/internal/ai"
)

type wireSession struct {
	ID        string         `json:"id"`
	Name      string         `json:"name,omitempty"`
	Created   string         `json:"created"`
	Updated   string         `json:"updated,omitempty"`
	Provider  string         `json:"provider,omitempty"`
	Model     string         `json:"model"`
	Effort    string         `json:"effort,omitempty"`
	Cwd       string         `json:"cwd,omitempty"`
	RootID    string         `json:"root_id"`
	ActiveID  string         `json:"active_id"`
	Nodes     []wireNode     `json:"nodes"`
	Subagents []SubagentTask `json:"subagents,omitempty"`
	FilesRead []string       `json:"files_read,omitempty"`
}

type wireNode struct {
	ID             string     `json:"id"`
	ParentID       string     `json:"parent_id,omitempty"`
	ChildIDs       []string   `json:"children,omitempty"`
	Message        ai.Message `json:"message,omitempty"`
	HasMessage     bool       `json:"has_message"`
	Usage          ai.Usage   `json:"usage,omitempty"`
	Created        string     `json:"created"`
	DurationMs     int        `json:"duration_ms,omitempty"`
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
	s.mu.Lock()
	s.Updated = time.Now()
	s.mu.Unlock()
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
		Updated:   time.Now().Format("2006-01-02T15:04:05Z07:00"),
		Provider:  normalizeProvider(s.Provider),
		Model:     s.Model,
		Effort:    s.Effort,
		Cwd:       normalizeCwd(s.Cwd),
		RootID:    s.Root.ID,
		ActiveID:  s.Active.ID,
		Subagents: cloneSubagentTasks(s.Subagents),
		FilesRead: append([]string(nil), s.FilesRead...),
	}
	walk(s.Root, func(n *Node) {
		wn := wireNode{
			ID:             n.ID,
			Usage:          n.Usage,
			Created:        n.Created.Format("2006-01-02T15:04:05Z07:00"),
			DurationMs:     n.DurationMs,
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

func LoadByID(dir, id string) (*Session, error) {
	id = strings.TrimSpace(id)
	if !validSessionID(id) {
		return nil, fmt.Errorf("invalid session id %q", id)
	}
	return Load(filepath.Join(dir, id+".json"))
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
	if strings.TrimSpace(w.ID) == "" {
		return nil, fmt.Errorf("malformed session: missing id")
	}
	if strings.TrimSpace(w.RootID) == "" {
		return nil, fmt.Errorf("malformed session %q: missing root id", w.ID)
	}
	if strings.TrimSpace(w.ActiveID) == "" {
		return nil, fmt.Errorf("malformed session %q: missing active id", w.ID)
	}
	nodes := map[string]*Node{}
	for _, wn := range w.Nodes {
		if strings.TrimSpace(wn.ID) == "" {
			return nil, fmt.Errorf("malformed session %q: node missing id", w.ID)
		}
		if nodes[wn.ID] != nil {
			return nil, fmt.Errorf("malformed session %q: duplicate node %q", w.ID, wn.ID)
		}
		created, _ := time.Parse(time.RFC3339, wn.Created)
		n := &Node{ID: wn.ID, Usage: wn.Usage, Created: created, DurationMs: wn.DurationMs, CompactedCount: wn.CompactedCount}
		if wn.HasMessage {
			n.Message = wn.Message
		}
		nodes[wn.ID] = n
	}
	for _, wn := range w.Nodes {
		n := nodes[wn.ID]
		if wn.ParentID != "" {
			parent := nodes[wn.ParentID]
			if parent == nil {
				return nil, fmt.Errorf("malformed session %q: node %q has missing parent %q", w.ID, wn.ID, wn.ParentID)
			}
			n.Parent = parent
		}
		for _, cid := range wn.ChildIDs {
			child := nodes[cid]
			if child == nil {
				return nil, fmt.Errorf("malformed session %q: node %q has missing child %q", w.ID, wn.ID, cid)
			}
			n.Children = append(n.Children, child)
		}
	}
	root := nodes[w.RootID]
	if root == nil {
		return nil, fmt.Errorf("malformed session %q: root node %q not found", w.ID, w.RootID)
	}
	active := nodes[w.ActiveID]
	if active == nil {
		return nil, fmt.Errorf("malformed session %q: active node %q not found", w.ID, w.ActiveID)
	}
	created, _ := time.Parse(time.RFC3339, w.Created)
	updated, _ := time.Parse(time.RFC3339, w.Updated)
	return &Session{
		ID:        w.ID,
		Name:      w.Name,
		Created:   created,
		Updated:   updated,
		Provider:  normalizeProvider(w.Provider),
		Model:     w.Model,
		Effort:    w.Effort,
		Cwd:       normalizeCwd(w.Cwd),
		Root:      root,
		Active:    active,
		Subagents: cloneSubagentTasks(w.Subagents),
		FilesRead: append([]string(nil), w.FilesRead...),
		path:      path,
	}, nil
}

func walk(n *Node, fn func(*Node)) {
	fn(n)
	for _, c := range n.Children {
		walk(c, fn)
	}
}

func validSessionID(id string) bool {
	if id == "" || id == "." || id == ".." || filepath.Base(id) != id {
		return false
	}
	for _, r := range id {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func normalizeProvider(provider string) string {
	p := strings.ToLower(strings.TrimSpace(provider))
	switch p {
	case "", "codex", "groq", "ollama", "runpod", "openrouter":
		return p
	default:
		return "codex"
	}
}

func normalizeCwd(cwd string) string {
	cwd = strings.TrimSpace(cwd)
	if cwd == "" {
		return ""
	}
	if abs, err := filepath.Abs(cwd); err == nil {
		cwd = abs
	}
	return filepath.Clean(cwd)
}
