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
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

func runPrompt(ctx context.Context, text, providerOverride, modelOverride string, w io.Writer) error {
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

	cwd, _ := os.Getwd()
	sess.Cwd = cwd
	home, _ := os.UserHomeDir()
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
	subagentCfg := agent.SubagentConfigFromSettings(settings.Subagents)
	subagentCfg.Definitions = agent.SubagentDefinitionsFromAgentDefs(customAgents)
	a := agent.NewWithSubagentConfig(selection.AI, reg, sess, system, subagentCfg)
	a.RegisterSubagentToolsEnabled(settings.Subagents.EnabledValue())
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
	for ev := range a.Run(ctx, msg) {
		switch v := ev.(type) {
		case agent.AssistantText:
			fmt.Fprint(w, v.Delta)
		case agent.ToolStarted:
			fmt.Fprintf(w, "\n[tool: %s]", v.Call.Name)
		case agent.ToolFinished:
			fmt.Fprintf(w, "\n[done: %d bytes]", len(v.Result.Output))
		case agent.TurnError:
			fmt.Fprintf(w, "\n[error: %v]", v.Err)
		}
	}
	fmt.Fprintln(w)
	return sess.Save()
}
