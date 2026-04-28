// internal/session/browse.go
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type Summary struct {
	ID           string
	Name         string
	Created      time.Time
	Path         string
	MessageCount int
	Model        string
}

func ListSessions(dir string) ([]Summary, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Summary
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		p := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		var w wireSession
		if err := json.Unmarshal(b, &w); err != nil {
			continue
		}
		ts, _ := time.Parse(time.RFC3339, w.Created)
		msgCount := 0
		for _, n := range w.Nodes {
			if n.HasMessage {
				msgCount++
			}
		}
		out = append(out, Summary{
			ID:           w.ID,
			Name:         w.Name,
			Created:      ts,
			Path:         p,
			MessageCount: msgCount,
			Model:        w.Model,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Created.After(out[j].Created) })
	return out, nil
}
