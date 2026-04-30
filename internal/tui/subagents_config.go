package tui

import (
	"os"
	"path/filepath"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/agentdef"
	"github.com/khang859/rune/internal/config"
)

func currentSubagentConfig() agent.SubagentConfig {
	settings, _ := config.LoadSettings(config.SettingsPath())
	cfg := agent.SubagentConfigFromSettings(settings.Subagents)
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	defs, err := (&agentdef.Loader{
		Roots: []string{
			filepath.Join(home, ".rune", "agents"),
			filepath.Join(cwd, ".rune", "agents"),
		},
		Reserved: agent.BuiltinSubagentTypeSet(),
	}).Load()
	if err == nil {
		cfg.Definitions = agent.SubagentDefinitionsFromAgentDefs(defs)
	}
	return cfg
}

func currentSubagentsEnabled() bool {
	settings, _ := config.LoadSettings(config.SettingsPath())
	return settings.Subagents.EnabledValue()
}
