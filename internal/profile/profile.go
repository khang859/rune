// Package profile loads named Rune worker profiles. A profile resolves a
// model, a set of skills, and a system-prompt body, letting a single Rune
// agent behave as distinct roles (e.g. researcher, writer, reviewer).
package profile

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Profile is a named worker role parsed from a markdown file.
type Profile struct {
	Name         string
	Model        string
	Skills       []string
	Instructions string
	Path         string
}

// Loader discovers profiles from Roots in order; later roots override earlier
// ones by name, so project-local profiles override global ones.
type Loader struct {
	Roots []string
}

// Load reads every *.md profile from each root.
func (l *Loader) Load() (map[string]Profile, error) {
	byName := map[string]Profile{}
	for _, root := range l.Roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, e := range entries {
			if e.IsDir() || filepath.Ext(e.Name()) != ".md" {
				continue
			}
			path := filepath.Join(root, e.Name())
			b, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			fallbackName := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
			p, err := ParseMarkdown(path, fallbackName, string(b))
			if err != nil {
				return nil, err
			}
			byName[p.Name] = p
		}
	}
	return byName, nil
}

// Resolve loads profiles and returns the one matching name, or an error if no
// profile by that name exists.
func (l *Loader) Resolve(name string) (Profile, error) {
	profiles, err := l.Load()
	if err != nil {
		return Profile{}, err
	}
	p, ok := profiles[normalizeName(name)]
	if !ok {
		return Profile{}, fmt.Errorf("unknown profile %q (looked in %s)", name, strings.Join(l.Roots, ", "))
	}
	return p, nil
}

// ParseMarkdown parses a profile file's frontmatter and body.
func ParseMarkdown(path, fallbackName, content string) (Profile, error) {
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		return Profile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	p := Profile{Path: path, Name: normalizeName(fallbackName), Instructions: strings.TrimSpace(body)}
	for key, value := range frontmatter {
		switch key {
		case "name":
			p.Name = normalizeName(value)
		case "model":
			p.Model = value
		case "skills":
			p.Skills = parseList(value)
		}
	}
	if p.Name == "" || !validName(p.Name) {
		return Profile{}, fmt.Errorf("invalid profile name %q in %s", p.Name, path)
	}
	return p, nil
}

func splitFrontmatter(content string) (map[string]string, string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return nil, content, nil
	}
	closing := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closing = i
			break
		}
	}
	if closing == -1 {
		return nil, "", fmt.Errorf("frontmatter is missing closing ---")
	}
	fm := map[string]string{}
	for i, line := range lines[1:closing] {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			return nil, "", fmt.Errorf("invalid frontmatter line %d", i+2)
		}
		key = strings.ToLower(strings.TrimSpace(key))
		fm[key] = stripQuotes(strings.TrimSpace(value))
	}
	return fm, strings.Join(lines[closing+1:], "\n"), nil
}

// parseList parses an inline YAML list "[a, b]" or a comma-separated value.
func parseList(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	out := []string{}
	for _, part := range strings.Split(value, ",") {
		part = stripQuotes(strings.TrimSpace(part))
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

func normalizeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func validName(name string) bool {
	if name == "" {
		return false
	}
	for i, r := range name {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			continue
		}
		if (r == '-' || r == '_') && i > 0 {
			continue
		}
		return false
	}
	return true
}
