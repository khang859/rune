package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/agentdef"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/attachments"
	"github.com/khang859/rune/internal/codeindex"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/mcp"
	"github.com/khang859/rune/internal/providers"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/skill"
	"github.com/khang859/rune/internal/tools"
)

func runPrompt(ctx context.Context, text, providerOverride, modelOverride, profileName, requireTools string, w io.Writer) error {
	if err := config.EnsureRuneDir(); err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	prof, err := loadProfile(profileName, cwd, home)
	if err != nil {
		return err
	}
	selection, err := buildProvider(ctx, providerOverride, profileModel(modelOverride, prof))
	if err != nil {
		// Headless can't recover interactively, so make the failure actionable:
		// name both the re-login and the switch-provider paths.
		if selection.Provider == providers.Codex {
			return fmt.Errorf("%w\n  re-login:  rune login codex\n  or switch: rune --provider <groq|ollama|runpod|openrouter> --prompt ...", err)
		}
		return fmt.Errorf("%w\n  fix it:    rune login   (interactive provider chooser)\n  or switch: rune --provider <id> --prompt ...", err)
	}
	sess := session.New(selection.Model)
	sess.Provider = selection.Provider
	sess.Cwd = cwd
	sess.SetPath(filepath.Join(config.SessionsDir(), sess.ID+".json"))

	settings, _ := config.LoadSettings(config.SettingsPath())
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
	agentsMD := agent.LoadAgentsMD(cwd, home)
	system := agent.BasePrompt()
	if agentsMD != "" {
		system += "\n\nProject context:\n" + agentsMD
	}
	skills, _ := (&skill.Loader{
		Roots: []string{
			filepath.Join(home, ".rune", "skills"),
			filepath.Join(cwd, ".rune", "skills"),
		},
	}).Load()
	system, _ = prependProfile(system, prof, skills, true, w)
	subagentCfg := agent.SubagentConfigFromSettings(settings.Subagents)
	subagentCfg.Definitions = agent.SubagentDefinitionsFromAgentDefs(customAgents)
	a := agent.NewWithSubagentConfig(selection.AI, reg, sess, system, subagentCfg)
	a.SetRequireTools(agent.ParseRequireTools(requireTools))
	a.SetModelCapabilities(settings.ModelCapabilities)
	a.RegisterSubagentToolsEnabled(settings.Subagents.EnabledValue())
	a.SetRepoMapEnabled(settings.RepoMap.Enabled || settings.RepoMap.MaxTokens == 0)
	budget := settings.RepoMap.MaxTokens
	if budget == 0 {
		budget = 2000
	}
	a.SetRepoMapBudget(budget)
	if idx, err := codeindex.BuildCached(ctx, codeindex.BuildOptions{Root: cwd}); err == nil {
		a.SetCodeIndex(idx)
	}
	resolved := attachments.ResolveUserInput(text, attachments.Options{CWD: cwd, Provider: selection.Provider, Model: selection.Model})
	if summary := promptAttachmentSummary(resolved.Attached); summary != "" {
		fmt.Fprintln(w, summary)
	}
	for _, warning := range resolved.Warnings {
		fmt.Fprintf(w, "(%s)\n", warning)
	}
	content := []ai.ContentBlock{ai.TextBlock{Text: resolved.Text}}
	content = append(content, resolved.Attachments...)
	msg := ai.Message{Role: ai.RoleUser, Content: content}
	incomplete := false
	for ev := range a.Run(ctx, msg) {
		switch v := ev.(type) {
		case agent.AssistantText:
			fmt.Fprint(w, v.Delta)
		case agent.ToolStarted:
			fmt.Fprintf(w, "\n[tool: %s]", v.Call.Name)
		case agent.ToolFinished:
			fmt.Fprintf(w, "\n[done: %d bytes]", len(v.Result.Output))
		case agent.RequiredToolPending:
			fmt.Fprintf(w, "\n[persist: must call %v before ending (attempt %d)]", v.Names, v.Attempt)
		case agent.TurnError:
			fmt.Fprintf(w, "\n[error: %v]", v.Err)
		case agent.TurnDone:
			if v.Reason == agent.ReasonIncompleteRequiredTool {
				incomplete = true
				fmt.Fprintf(w, "\n[incomplete: ended without calling a required completion tool]")
			}
		}
	}
	fmt.Fprintln(w)
	if err := sess.Save(); err != nil {
		return err
	}
	if incomplete {
		return agent.ErrIncompleteRequiredTool
	}
	return nil
}
