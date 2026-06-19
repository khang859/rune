package memory

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Load returns the trimmed contents of an auditable user memory file.
// A missing file is treated as empty memory.
func Load(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(b)), nil
}

// SystemBlock formats the memory file for prompt injection. It never creates
// or modifies memory; writes only happen through explicit user commands.
func SystemBlock(path string) (string, error) {
	text, err := Load(path)
	if err != nil || text == "" {
		return "", err
	}
	return "<user_memory>\n" +
		"The following memories were explicitly saved by the user in " + path + ". Treat them as user preferences/context. Never store secrets here automatically.\n\n" +
		text + "\n" +
		"</user_memory>", nil
}

// Append writes one explicit memory entry to path, creating parent directories
// as needed. The caller is responsible for obtaining explicit user intent.
func Append(path, text string, now time.Time) error {
	text = strings.TrimSpace(text)
	if text == "" {
		return fmt.Errorf("memory text is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = fmt.Fprintf(f, "- %s: %s\n", now.UTC().Format("2006-01-02 15:04"), text)
	return err
}

func Preview(path string, maxLines int) (string, bool, error) {
	text, err := Load(path)
	if err != nil || text == "" {
		return text, false, err
	}
	if maxLines <= 0 {
		return text, false, nil
	}
	lines := strings.Split(text, "\n")
	if len(lines) <= maxLines {
		return text, false, nil
	}
	return strings.Join(lines[:maxLines], "\n"), true, nil
}
