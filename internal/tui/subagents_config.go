package tui

import (
	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/config"
)

func currentSubagentConfig() agent.SubagentConfig {
	settings, _ := config.LoadSettings(config.SettingsPath())
	return agent.SubagentConfigFromSettings(settings.Subagents)
}

func currentSubagentsEnabled() bool {
	settings, _ := config.LoadSettings(config.SettingsPath())
	return settings.Subagents.EnabledValue()
}
