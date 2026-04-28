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
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
	"github.com/khang859/rune/internal/tui"
)

func runInteractive(ctx context.Context) error {
	if err := config.EnsureRuneDir(); err != nil {
		return err
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

	sess := session.New("gpt-5")
	sess.SetPath(filepath.Join(config.SessionsDir(), sess.ID+".json"))

	reg := tools.NewRegistry()
	reg.Register(tools.Read{})
	reg.Register(tools.Write{})
	reg.Register(tools.Edit{})
	reg.Register(tools.Bash{})

	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	system := defaultSystemPrompt() + "\n\n" + agent.LoadAgentsMD(cwd, home)
	a := agent.New(p, reg, sess, system)

	return tui.Run(a, sess)
}
