package codeindex

import (
	"bufio"
	"os"
	"path"
	"path/filepath"
	"strings"
)

type ignoreMatcher struct {
	rules []ignoreRule
}

type ignoreRule struct {
	pattern  string
	negated  bool
	dirOnly  bool
	anchored bool
	hasSlash bool
}

func loadIgnoreMatcher(root string) ignoreMatcher {
	var m ignoreMatcher
	for _, name := range []string{".gitignore", ".runeignore"} {
		m.load(filepath.Join(root, name))
	}
	return m
}

func (m *ignoreMatcher) load(file string) {
	f, err := os.Open(file)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if rule, ok := parseIgnoreRule(scanner.Text()); ok {
			m.rules = append(m.rules, rule)
		}
	}
}

func parseIgnoreRule(line string) (ignoreRule, bool) {
	line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
	if line == "" {
		return ignoreRule{}, false
	}
	if strings.HasPrefix(line, `\#`) || strings.HasPrefix(line, `\!`) {
		line = line[1:]
	} else if strings.HasPrefix(line, "#") {
		return ignoreRule{}, false
	}

	var r ignoreRule
	if strings.HasPrefix(line, "!") {
		r.negated = true
		line = strings.TrimSpace(line[1:])
	}
	line = filepath.ToSlash(line)
	if strings.HasSuffix(line, "/") {
		r.dirOnly = true
		line = strings.TrimRight(line, "/")
	}
	if strings.HasPrefix(line, "/") {
		r.anchored = true
		line = strings.TrimLeft(line, "/")
	}
	line = strings.TrimPrefix(line, "./")
	if line == "" {
		return ignoreRule{}, false
	}
	r.pattern = line
	r.hasSlash = strings.Contains(line, "/")
	return r, true
}

func (m ignoreMatcher) ignored(rel string, isDir bool) bool {
	rel = filepath.ToSlash(rel)
	if rel == "" || rel == "." {
		return false
	}

	ignored := false
	for _, rule := range m.rules {
		if rule.matches(rel, isDir) {
			ignored = !rule.negated
		}
	}
	return ignored
}

func (r ignoreRule) matches(rel string, isDir bool) bool {
	if r.dirOnly && !isDir {
		return false
	}

	if r.anchored || r.hasSlash {
		if matchIgnoreGlob(r.pattern, rel) {
			return true
		}
		return r.dirOnly && strings.HasPrefix(rel, r.pattern+"/")
	}

	for _, part := range strings.Split(rel, "/") {
		if matchIgnoreGlob(r.pattern, part) {
			return true
		}
	}
	return false
}

func matchIgnoreGlob(pattern, target string) bool {
	if ok, _ := path.Match(pattern, target); ok {
		return true
	}
	if !strings.Contains(pattern, "**") {
		return false
	}

	variants := []string{
		strings.ReplaceAll(pattern, "**/", ""),
		strings.ReplaceAll(pattern, "/**", ""),
		strings.ReplaceAll(pattern, "**", "*"),
	}
	for _, variant := range variants {
		if variant == pattern || variant == "" {
			continue
		}
		if ok, _ := path.Match(variant, target); ok {
			return true
		}
	}
	return false
}
