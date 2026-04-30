package agentdef

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	ToolsReadOnly = "readonly"
	ToolsFull     = "full"
)

type Definition struct {
	Name         string
	Description  string
	Model        string
	TimeoutSecs  int
	Tools        string
	Instructions string
	Path         string
}

type Loader struct {
	Roots    []string
	Reserved map[string]bool
}

func (l *Loader) Load() (map[string]Definition, error) {
	byName := map[string]Definition{}
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
			def, err := ParseMarkdown(path, fallbackName, string(b))
			if err != nil {
				return nil, err
			}
			if l.Reserved[strings.ToLower(def.Name)] {
				return nil, fmt.Errorf("custom subagent %q in %s uses a reserved built-in name", def.Name, path)
			}
			byName[def.Name] = def
		}
	}
	return byName, nil
}

func ParseMarkdown(path, fallbackName, content string) (Definition, error) {
	frontmatter, body, err := splitFrontmatter(content)
	if err != nil {
		return Definition{}, fmt.Errorf("parse %s: %w", path, err)
	}
	def := Definition{Path: path, Name: normalizeName(fallbackName), Tools: ToolsReadOnly, Instructions: strings.TrimSpace(body)}
	for key, value := range frontmatter {
		switch key {
		case "name":
			def.Name = normalizeName(value)
		case "description":
			def.Description = value
		case "model":
			def.Model = value
		case "timeout_secs":
			if value == "" {
				return Definition{}, fmt.Errorf("timeout_secs is empty")
			}
			n, err := strconv.Atoi(value)
			if err != nil || n <= 0 {
				return Definition{}, fmt.Errorf("timeout_secs must be a positive integer")
			}
			def.TimeoutSecs = n
		case "tools":
			tools := strings.ToLower(strings.TrimSpace(value))
			switch tools {
			case "", ToolsReadOnly:
				def.Tools = ToolsReadOnly
			case ToolsFull:
				def.Tools = ToolsFull
			default:
				return Definition{}, fmt.Errorf("tools must be %q or %q", ToolsReadOnly, ToolsFull)
			}
		}
	}
	if def.Name == "" || !validName(def.Name) {
		return Definition{}, fmt.Errorf("invalid agent name %q", def.Name)
	}
	return def, nil
}

func splitFrontmatter(content string) (map[string]string, string, error) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") && strings.TrimSpace(content) != "---" {
		return nil, content, nil
	}
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
		value = stripQuotes(strings.TrimSpace(value))
		fm[key] = value
	}
	return fm, strings.Join(lines[closing+1:], "\n"), nil
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
