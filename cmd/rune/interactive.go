package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai/codex"
	"github.com/khang859/rune/internal/ai/oauth"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/mcp"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/skill"
	"github.com/khang859/rune/internal/tools"
	"github.com/khang859/rune/internal/tui"
)

func runInteractive(ctx context.Context, model string) error {
	if err := config.EnsureRuneDir(); err != nil {
		return err
	}
	if model == "" {
		model = os.Getenv("RUNE_CODEX_MODEL")
	}
	if model == "" {
		model = DefaultCodexModel
	}
	endpoint := oauth.CodexResponsesBaseURL + oauth.CodexResponsesPath
	if v := os.Getenv("RUNE_CODEX_ENDPOINT"); v != "" {
		endpoint = v
	}
	tokenURL := oauth.CodexTokenURL
	if v := os.Getenv("RUNE_OAUTH_TOKEN_URL"); v != "" {
		tokenURL = v
	}
	store := oauth.NewStore(config.AuthPath())
	src := &oauth.CodexSource{Store: store, TokenURL: tokenURL}
	if _, err := src.Token(ctx); err != nil {
		return fmt.Errorf("not logged in: %w (run `rune login codex`)", err)
	}
	p := codex.New(endpoint, src)

	sess := session.New(model)
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
	a := agent.New(p, reg, sess, system)
	a.RegisterSubagentTools()

	return tui.Run(a, sess, skills, mgr.Statuses())
}
