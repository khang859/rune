package config

import (
	"os"
	"path/filepath"
)

// RuneDir returns ~/.rune (or $RUNE_DIR if set).
func RuneDir() string {
	if d := os.Getenv("RUNE_DIR"); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ".rune"
	}
	return filepath.Join(home, ".rune")
}

func SessionsDir() string { return filepath.Join(RuneDir(), "sessions") }
func AuthPath() string    { return filepath.Join(RuneDir(), "auth.json") }
func SkillsDir() string   { return filepath.Join(RuneDir(), "skills") }
func MCPConfig() string   { return filepath.Join(RuneDir(), "mcp.json") }
func LogPath() string     { return filepath.Join(RuneDir(), "log") }

// EnsureRuneDir creates the rune dir tree if missing.
func EnsureRuneDir() error {
	return os.MkdirAll(SessionsDir(), 0o755)
}
