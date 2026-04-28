package skill

import (
	"os"
	"path/filepath"
	"strings"
)

type Loader struct {
	Roots []string
}

func (l *Loader) Load() ([]Skill, error) {
	by := map[string]Skill{}
	for _, root := range l.Roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if filepath.Ext(e.Name()) != ".md" {
				continue
			}
			p := filepath.Join(root, e.Name())
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			slug := strings.TrimSuffix(e.Name(), ".md")
			by[slug] = Skill{Slug: slug, Path: p, Body: string(b)}
		}
	}
	out := make([]Skill, 0, len(by))
	for _, s := range by {
		out = append(out, s)
	}
	return out, nil
}
