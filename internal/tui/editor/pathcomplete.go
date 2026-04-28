package editor

import (
	"os"
	"path/filepath"
	"strings"
)

// CompletePath returns a single completion if exactly one entry matches.
// `word` is the current "word" the caller has identified at the cursor.
// `cwd` is the directory to complete relative to.
func CompletePath(word, cwd string) (string, bool) {
	base, prefix := splitWord(word)
	dir := filepath.Join(cwd, base)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var matches []string
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, prefix) {
			continue
		}
		if e.IsDir() {
			n += "/"
		}
		matches = append(matches, n)
	}
	if len(matches) != 1 {
		return "", false
	}
	out := filepath.Join(base, matches[0])
	if strings.HasSuffix(matches[0], "/") && !strings.HasSuffix(out, "/") {
		out += "/"
	}
	return out, true
}

func splitWord(w string) (dir, prefix string) {
	i := strings.LastIndex(w, "/")
	if i < 0 {
		return "", w
	}
	return w[:i], w[i+1:]
}
