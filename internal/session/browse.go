// internal/session/browse.go
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/khang859/rune/internal/ai"
)

type Summary struct {
	ID           string
	Name         string
	Preview      string
	Created      time.Time
	Updated      time.Time
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
		info, err := os.Stat(p)
		if err != nil {
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
			Preview:      activePathPreview(w),
			Created:      ts,
			Updated:      info.ModTime(),
			Path:         p,
			MessageCount: msgCount,
			Model:        w.Model,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		left := out[i].Updated
		right := out[j].Updated
		if left.IsZero() {
			left = out[i].Created
		}
		if right.IsZero() {
			right = out[j].Created
		}
		return left.After(right)
	})
	return out, nil
}

func activePathPreview(w wireSession) string {
	nodes := make(map[string]wireNode, len(w.Nodes))
	for _, n := range w.Nodes {
		nodes[n.ID] = n
	}
	var path []wireNode
	for id := w.ActiveID; id != ""; {
		n, ok := nodes[id]
		if !ok {
			break
		}
		path = append([]wireNode{n}, path...)
		id = n.ParentID
	}
	for _, n := range path {
		if n.HasMessage && n.Message.Role == ai.RoleUser {
			return messagePreview(n.Message)
		}
	}
	return ""
}

func messagePreview(msg ai.Message) string {
	var parts []string
	for _, c := range msg.Content {
		if t, ok := c.(ai.TextBlock); ok {
			text := strings.Join(strings.Fields(t.Text), " ")
			if text != "" {
				parts = append(parts, text)
			}
		}
	}
	return truncateRunes(strings.Join(parts, " "), 72)
}

func truncateRunes(s string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}
