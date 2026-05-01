package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/agentdef"
	"github.com/khang859/rune/internal/ai/unavailable"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/mcp"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/skill"
	"github.com/khang859/rune/internal/tools"
	"github.com/khang859/rune/internal/tui"
)

func runInteractive(ctx context.Context, providerOverride, modelOverride, version string) error {
	if err := config.EnsureRuneDir(); err != nil {
		return err
	}
	settings, _ := config.LoadSettings(config.SettingsPath())
	selection, err := buildProvider(ctx, providerOverride, modelOverride)
	if err != nil {
		selection.AI = unavailable.New("no active provider configured")
		if settings.Provider != "" || providerOverride != "" || os.Getenv("RUNE_PROVIDER") != "" {
			return err
		}
	}

	sess := session.New(selection.Model)
	sess.Provider = selection.Provider
	sess.SetPath(filepath.Join(config.SessionsDir(), sess.ID+".json"))

	reg := tools.NewRegistry()
	opts, _, _ := tools.BuiltinOptionsFromSettings(settings)
	tools.RegisterBuiltins(reg, opts)

	mgr := mcp.NewManager(config.MCPConfig())
	if err := mgr.Start(ctx, reg); err != nil {
		fmt.Fprintln(os.Stderr, "[mcp] start failed:", err)
	}
	defer mgr.Shutdown()

	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	skills, _ := (&skill.Loader{
		Roots: []string{
			filepath.Join(home, ".rune", "skills"),
			filepath.Join(cwd, ".rune", "skills"),
		},
	}).Load()
	customAgents, err := (&agentdef.Loader{
		Roots: []string{
			filepath.Join(home, ".rune", "agents"),
			filepath.Join(cwd, ".rune", "agents"),
		},
		Reserved: agent.BuiltinSubagentTypeSet(),
	}).Load()
	if err != nil {
		return err
	}

	system := agent.BasePrompt() + "\n\n" + agent.LoadAgentsMD(cwd, home)
	subagentCfg := agent.SubagentConfigFromSettings(settings.Subagents)
	subagentCfg.Definitions = agent.SubagentDefinitionsFromAgentDefs(customAgents)
	a := agent.NewWithSubagentConfig(selection.AI, reg, sess, system, subagentCfg)
	a.RegisterSubagentToolsEnabled(settings.Subagents.EnabledValue())

	return tui.RunWithProfile(a, sess, selection.ProfileID, skills, mgr.Statuses(), version)
}
