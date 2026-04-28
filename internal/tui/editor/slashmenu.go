package editor

import (
	"sort"
	"strings"

	"github.com/sahilm/fuzzy"
)

type SlashMenu struct {
	all    []string
	query  string
	items  []string
	sel    int
	primed bool
}

func NewSlashMenu(cmds []string) *SlashMenu {
	s := &SlashMenu{all: cmds}
	sort.Strings(s.all)
	s.SetQuery("")
	return s
}

func (s *SlashMenu) SetQuery(q string) {
	if s.primed && s.query == q {
		return
	}
	s.primed = true
	s.query = q
	s.sel = 0
	if q == "" {
		s.items = append([]string{}, s.all...)
		return
	}
	matches := fuzzy.Find(q, s.all)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m.Str)
	}
	// Promote exact prefix matches.
	sort.SliceStable(out, func(i, j int) bool {
		ip := strings.HasPrefix(strings.TrimPrefix(out[i], "/"), q)
		jp := strings.HasPrefix(strings.TrimPrefix(out[j], "/"), q)
		if ip != jp {
			return ip
		}
		return false
	})
	s.items = out
}

func (s *SlashMenu) Items() []string { return s.items }
func (s *SlashMenu) Sel() int        { return s.sel }
func (s *SlashMenu) Selected() string {
	if s.sel < 0 || s.sel >= len(s.items) {
		return ""
	}
	return s.items[s.sel]
}
func (s *SlashMenu) Up() {
	if s.sel > 0 {
		s.sel--
	}
}
func (s *SlashMenu) Down() {
	if s.sel < len(s.items)-1 {
		s.sel++
	}
}
