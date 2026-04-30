package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

const (
	defaultListFilesMaxResults   = 500
	defaultSearchFilesMaxResults = 100
	maxSearchFileBytes           = 2 * 1024 * 1024
)

var localExploreIgnoredDirs = map[string]struct{}{
	".git":         {},
	"node_modules": {},
	"vendor":       {},
	"dist":         {},
	"build":        {},
	".next":        {},
	".cache":       {},
	"coverage":     {},
}

type ListFiles struct{}

type SearchFiles struct{}

func (ListFiles) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "list_files",
		Description: "Recursively list local project files without running shell commands. Skips common noisy directories. Supports optional path, glob, and max_results.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "path":{"type":"string","description":"Directory or file to list. Defaults to the current working directory."},
                "glob":{"type":"string","description":"Optional glob filter matched against relative paths and basenames, e.g. \"*.go\" or \"internal/**/*.go\"."},
                "max_results":{"type":"integer","description":"Maximum number of files to return. Defaults to 500."}
            }
        }`),
	}
}

func (ListFiles) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Path       string `json:"path"`
		Glob       string `json:"glob"`
		MaxResults int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"path"?: string, "glob"?: string, "max_results"?: int}.`, err), IsError: true}, nil
	}
	root, err := resolveExplorePath(a.Path)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	maxResults := a.MaxResults
	if maxResults == 0 {
		maxResults = defaultListFilesMaxResults
	}
	if maxResults < 0 {
		return Result{Output: "max_results must be non-negative", IsError: true}, nil
	}
	if maxResults == 0 {
		maxResults = defaultListFilesMaxResults
	}

	info, err := os.Stat(root)
	if err != nil {
		return Result{Output: fmt.Sprintf("couldn't inspect %s: %v", root, err), IsError: true}, nil
	}
	base := root
	if !info.IsDir() {
		base = filepath.Dir(root)
	}
	var files []string
	truncated := false
	add := func(path string) error {
		rel := displayRel(base, path)
		if !matchesExploreGlob(rel, a.Glob) {
			return nil
		}
		if len(files) >= maxResults {
			truncated = true
			return errStopWalk
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	}
	if !info.IsDir() {
		if err := add(root); err != nil && err != errStopWalk {
			return Result{}, err
		}
	} else {
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				if path != root && shouldSkipExploreDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			return add(path)
		})
		if err == errStopWalk {
			err = nil
		}
		if err != nil {
			return Result{Output: fmt.Sprintf("couldn't list %s: %v", root, err), IsError: true}, nil
		}
	}
	if len(files) == 0 {
		return Result{Output: "(no files found)"}, nil
	}
	out := strings.Join(files, "\n")
	if truncated {
		out += fmt.Sprintf("\n\n[showing first %d files. Narrow path/glob or increase max_results to see more.]", maxResults)
	}
	return Result{Output: out}, nil
}

func (SearchFiles) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "search_files",
		Description: "Search local project files for a literal string without running shell commands. Skips common noisy directories and binary/large files. Supports optional path, glob, context_lines, and max_results.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "query":{"type":"string","description":"Literal string to search for."},
                "path":{"type":"string","description":"Directory or file to search. Defaults to the current working directory."},
                "glob":{"type":"string","description":"Optional glob filter matched against relative paths and basenames, e.g. \"*.go\" or \"internal/**/*.go\"."},
                "context_lines":{"type":"integer","description":"Number of lines of context to include before and after each match."},
                "max_results":{"type":"integer","description":"Maximum number of matching lines to return. Defaults to 100."}
            },
            "required":["query"]
        }`),
	}
}

func (SearchFiles) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Query        string `json:"query"`
		Path         string `json:"path"`
		Glob         string `json:"glob"`
		ContextLines int    `json:"context_lines"`
		MaxResults   int    `json:"max_results"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"query": string, "path"?: string, "glob"?: string, "context_lines"?: int, "max_results"?: int}.`, err), IsError: true}, nil
	}
	if a.Query == "" {
		return Result{Output: "query is required", IsError: true}, nil
	}
	if a.ContextLines < 0 || a.MaxResults < 0 {
		return Result{Output: "context_lines and max_results must be non-negative", IsError: true}, nil
	}
	maxResults := a.MaxResults
	if maxResults == 0 {
		maxResults = defaultSearchFilesMaxResults
	}
	root, err := resolveExplorePath(a.Path)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	info, err := os.Stat(root)
	if err != nil {
		return Result{Output: fmt.Sprintf("couldn't inspect %s: %v", root, err), IsError: true}, nil
	}
	base := root
	if !info.IsDir() {
		base = filepath.Dir(root)
	}

	var matches []string
	truncated := false
	searchOne := func(path string) error {
		rel := displayRel(base, path)
		if !matchesExploreGlob(rel, a.Glob) {
			return nil
		}
		fileMatches, err := searchFile(path, filepath.ToSlash(rel), a.Query, a.ContextLines, maxResults-len(matches))
		if err != nil {
			return nil
		}
		matches = append(matches, fileMatches...)
		if len(matches) >= maxResults {
			truncated = true
			return errStopWalk
		}
		return nil
	}
	if !info.IsDir() {
		if err := searchOne(root); err != nil && err != errStopWalk {
			return Result{}, err
		}
	} else {
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if walkErr != nil {
				return nil
			}
			if d.IsDir() {
				if path != root && shouldSkipExploreDir(d.Name()) {
					return filepath.SkipDir
				}
				return nil
			}
			return searchOne(path)
		})
		if err == errStopWalk {
			err = nil
		}
		if err != nil {
			return Result{Output: fmt.Sprintf("couldn't search %s: %v", root, err), IsError: true}, nil
		}
	}
	if len(matches) == 0 {
		return Result{Output: "(no matches)"}, nil
	}
	out := strings.Join(matches, "\n")
	if truncated {
		out += fmt.Sprintf("\n\n[showing first %d matching lines. Narrow path/glob/query or increase max_results to see more.]", maxResults)
	}
	return Result{Output: out}, nil
}

var errStopWalk = fmt.Errorf("stop walk")

func resolveExplorePath(p string) (string, error) {
	if strings.TrimSpace(p) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("couldn't determine current working directory: %v", err)
		}
		return wd, nil
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("couldn't resolve %s: %v", p, err)
	}
	return abs, nil
}

func shouldSkipExploreDir(name string) bool {
	_, ok := localExploreIgnoredDirs[name]
	return ok
}

func displayRel(base, path string) string {
	rel, err := filepath.Rel(base, path)
	if err != nil || rel == "." {
		return filepath.Base(path)
	}
	return rel
}

func matchesExploreGlob(rel, pattern string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return true
	}
	rel = filepath.ToSlash(rel)
	base := filepath.Base(rel)
	pattern = filepath.ToSlash(pattern)
	if ok, _ := filepath.Match(pattern, rel); ok {
		return true
	}
	if ok, _ := filepath.Match(pattern, base); ok {
		return true
	}
	if strings.Contains(pattern, "**/") {
		withoutStarStar := strings.ReplaceAll(pattern, "**/", "")
		if ok, _ := filepath.Match(withoutStarStar, rel); ok {
			return true
		}
		if ok, _ := filepath.Match(withoutStarStar, base); ok {
			return true
		}
	}
	return false
}

func searchFile(path, rel, query string, contextLines, remaining int) ([]string, error) {
	if remaining <= 0 {
		return nil, nil
	}
	info, err := os.Stat(path)
	if err != nil || info.IsDir() || info.Size() > maxSearchFileBytes {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if isLikelyBinary(b) {
		return nil, fmt.Errorf("binary file")
	}
	content := string(b)
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		lines = lines[:len(lines)-1]
	}
	var out []string
	emittedContext := map[int]bool{}
	for i, line := range lines {
		if !strings.Contains(line, query) {
			continue
		}
		start := i - contextLines
		if start < 0 {
			start = 0
		}
		end := i + contextLines
		if end >= len(lines) {
			end = len(lines) - 1
		}
		for j := start; j <= end; j++ {
			if emittedContext[j] && j != i {
				continue
			}
			prefix := fmt.Sprintf("%s:%d:", rel, j+1)
			if j == i {
				out = append(out, prefix+" "+lines[j])
			} else {
				out = append(out, prefix+"-"+lines[j])
				emittedContext[j] = true
			}
			if j == i && len(out) >= remaining {
				return out, nil
			}
		}
		if len(out) >= remaining {
			return out, nil
		}
	}
	return out, nil
}

func isLikelyBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	probe := b
	if len(probe) > 8000 {
		probe = probe[:8000]
	}
	if bytes.IndexByte(probe, 0) >= 0 {
		return true
	}
	s := bufio.NewScanner(bytes.NewReader(probe))
	s.Buffer(make([]byte, 1024), 1024*1024)
	return false && !s.Scan()
}
