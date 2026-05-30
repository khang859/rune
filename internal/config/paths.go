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

func SessionsDir() string  { return filepath.Join(RuneDir(), "sessions") }
func AuthPath() string     { return filepath.Join(RuneDir(), "auth.json") }
func SettingsPath() string { return filepath.Join(RuneDir(), "settings.json") }
func SecretsPath() string  { return filepath.Join(RuneDir(), "secrets.json") }
func SkillsDir() string    { return filepath.Join(RuneDir(), "skills") }
func MCPConfig() string    { return filepath.Join(RuneDir(), "mcp.json") }

// MCPConfigEnvOverride returns the path set in $RUNE_MCP_CONFIG, or "" if unset.
func MCPConfigEnvOverride() string { return os.Getenv("RUNE_MCP_CONFIG") }

// MCPConfigWritePath returns where `rune mcp add`/`remove` should write the MCP
// config: $RUNE_MCP_CONFIG if set, otherwise the global ~/.rune/mcp.json.
func MCPConfigWritePath() string {
	if p := MCPConfigEnvOverride(); p != "" {
		return p
	}
	return MCPConfig()
}
func LogPath() string     { return filepath.Join(RuneDir(), "log") }
func HistoryPath() string { return filepath.Join(RuneDir(), "history") }

// EnsureRuneDir creates the rune dir tree if missing.
func EnsureRuneDir() error {
	for _, dir := range []string{RuneDir(), SessionsDir()} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	return nil
}
