package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/mcp"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/skill"
	"github.com/khang859/rune/internal/tools"
	"github.com/khang859/rune/internal/tui"
)

func runInteractive(ctx context.Context, providerOverride, modelOverride string) error {
	if err := config.EnsureRuneDir(); err != nil {
		return err
	}
	selection, err := buildProvider(ctx, providerOverride, modelOverride)
	if err != nil {
		return err
	}

	sess := session.New(selection.Model)
	sess.Provider = selection.Provider
	sess.SetPath(filepath.Join(config.SessionsDir(), sess.ID+".json"))

	settings, _ := config.LoadSettings(config.SettingsPath())
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

	system := agent.BasePrompt() + "\n\n" + agent.LoadAgentsMD(cwd, home)
	a := agent.NewWithSubagentConfig(selection.AI, reg, sess, system, agent.SubagentConfigFromSettings(settings.Subagents))
	a.RegisterSubagentToolsEnabled(settings.Subagents.EnabledValue())

	return tui.Run(a, sess, skills, mgr.Statuses())
}
