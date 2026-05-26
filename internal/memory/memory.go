package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/khang859/rune/internal/config"
)

const DefaultMaxBytes = 25_000

type Store struct {
	CWD      string
	MaxBytes int
}

func NewStore(cwd string, maxBytes int) *Store {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return &Store{CWD: cwd, MaxBytes: maxBytes}
}

func (s *Store) ProjectRoot() string {
	cwd := strings.TrimSpace(s.CWD)
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	abs, err := filepath.Abs(cwd)
	if err == nil {
		cwd = abs
	}
	cur := cwd
	for {
		if st, err := os.Stat(filepath.Join(cur, ".git")); err == nil && st != nil {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return cwd
		}
		cur = parent
	}
}

func (s *Store) ProjectID() string {
	sum := sha256.Sum256([]byte(s.ProjectRoot()))
	return hex.EncodeToString(sum[:])[:16]
}

func (s *Store) Dir() string {
	return filepath.Join(config.RuneDir(), "projects", s.ProjectID(), "memory")
}

func (s *Store) Path() string {
	return filepath.Join(s.Dir(), "MEMORY.md")
}

func (s *Store) Load() (string, error) {
	b, err := os.ReadFile(s.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	if s.MaxBytes > 0 && len(b) > s.MaxBytes {
		b = b[:s.MaxBytes]
	}
	return strings.TrimSpace(string(b)), nil
}

func (s *Store) Save(content string) error {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	content = RedactSecrets(content)
	if err := os.MkdirAll(s.Dir(), 0o700); err != nil {
		return err
	}
	_ = os.Chmod(s.Dir(), 0o700)
	path := s.Path()
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := f.WriteString(content + "\n"); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	_ = os.Chmod(path, 0o600)
	return nil
}

func FormatBlock(content string) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	return "<auto_memory>\n" + strings.Join([]string{
		"The following memories were automatically saved by Rune for this project.",
		"Use them only when relevant. Current user instructions, repository evidence, and tool results override memory.",
		"",
		content,
	}, "\n") + "\n</auto_memory>"
}

func CleanExtractorOutput(raw string, maxBytes int) (string, bool, error) {
	out := strings.TrimSpace(raw)
	if out == "" || strings.EqualFold(out, "NO_CHANGE") {
		return "", false, nil
	}
	out = stripCodeFence(out)
	out = RedactSecrets(out)
	if unsafeMemory(out) {
		return "", false, fmt.Errorf("memory update rejected by safety filter")
	}
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	if len(out) > maxBytes {
		out = strings.TrimSpace(out[:maxBytes])
	}
	if strings.TrimSpace(out) == "" {
		return "", false, nil
	}
	return out, true, nil
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "```") {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) < 2 || !strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return s
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n"))
}

func unsafeMemory(s string) bool {
	lower := strings.ToLower(s)
	phrases := []string{
		"ignore previous instructions",
		"ignore prior instructions",
		"disregard previous instructions",
		"disable safety",
		"exfiltrate",
		"send secrets",
		"leak secrets",
	}
	for _, phrase := range phrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?is)-----BEGIN [A-Z ]*PRIVATE KEY-----.*?-----END [A-Z ]*PRIVATE KEY-----`),
	regexp.MustCompile(`(?i)\bsk-[A-Za-z0-9_-]{20,}\b`),
	regexp.MustCompile(`(?i)\bgithub_pat_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`(?i)\bgh[pousr]_[A-Za-z0-9_]{20,}\b`),
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	regexp.MustCompile(`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`),
	regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\s*[:=]\s*["']?[^\s"']+`),
}

func RedactSecrets(s string) string {
	out := s
	for _, re := range secretPatterns[:6] {
		out = re.ReplaceAllString(out, "<redacted secret>")
	}
	out = secretPatterns[6].ReplaceAllString(out, "$1=<redacted>")
	return out
}
