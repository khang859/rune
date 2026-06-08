package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/agentdef"
	"github.com/khang859/rune/internal/ai/unavailable"
	"github.com/khang859/rune/internal/codeindex"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/mcp"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/skill"
	"github.com/khang859/rune/internal/tools"
	"github.com/khang859/rune/internal/tui"
)

func runInteractive(ctx context.Context, providerOverride, modelOverride, profileName, version string) error {
	if err := config.EnsureRuneDir(); err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	prof, err := loadProfile(profileName, cwd, home)
	if err != nil {
		return err
	}
	settings, _ := config.LoadSettings(config.SettingsPath())
	selection, err := buildProvider(ctx, providerOverride, profileModel(modelOverride, prof))
	var startupNotice string
	if err != nil {
		// Never trap the user at the CLI: drop into the TUI with an unavailable
		// provider and a recovery banner so they can /login or /providers from
		// inside, regardless of why the provider failed to build.
		startupNotice = startupRecoveryNotice(selection.Provider, err)
		selection.AI = unavailable.New(startupNotice)
	}

	sess := session.New(selection.Model)
	sess.Provider = selection.Provider
	sess.Cwd = cwd
	sess.SetPath(filepath.Join(config.SessionsDir(), sess.ID+".json"))

	reg := tools.NewRegistry()
	opts, _, _ := tools.BuiltinOptionsFromSettings(settings)
	opts.OnRead = sess.RecordFileRead
	tools.RegisterBuiltins(reg, opts)

	mcpCfg, err := resolveMCPConfig()
	if err != nil {
		fmt.Fprintln(os.Stderr, "[mcp] config load failed:", err)
	}
	mgr := mcp.NewManager(mcpCfg)
	if err := mgr.Start(ctx, reg); err != nil {
		fmt.Fprintln(os.Stderr, "[mcp] start failed:", err)
	}
	defer mgr.Shutdown()

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
	system, skills = prependProfile(system, prof, skills, false, os.Stderr)
	subagentCfg := agent.SubagentConfigFromSettings(settings.Subagents)
	subagentCfg.Definitions = agent.SubagentDefinitionsFromAgentDefs(customAgents)
	a := agent.NewWithSubagentConfig(selection.AI, reg, sess, system, subagentCfg)
	a.SetModelCapabilities(settings.ModelCapabilities)
	a.RegisterSubagentToolsEnabled(settings.Subagents.EnabledValue())
	a.SetRepoMapEnabled(settings.RepoMap.Enabled || settings.RepoMap.MaxTokens == 0)
	budget := settings.RepoMap.MaxTokens
	if budget == 0 {
		budget = 2000
	}
	a.SetRepoMapBudget(budget)
	go func() {
		idx, err := codeindex.BuildCached(ctx, codeindex.BuildOptions{Root: cwd})
		if err == nil {
			a.SetCodeIndex(idx)
		}
	}()

	return tui.RunWithProfile(a, sess, selection.ProfileID, skills, mgr.Statuses(), version, prof, startupNotice)
}
