package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/codex"
	"github.com/khang859/rune/internal/ai/oauth"
	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/session"
	"github.com/khang859/rune/internal/tools"
)

func runPrompt(ctx context.Context, text string, w io.Writer) error {
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
	agentsMD := agent.LoadAgentsMD(cwd, home)
	system := defaultSystemPrompt()
	if agentsMD != "" {
		system += "\n\nProject context:\n" + agentsMD
	}
	a := agent.New(p, reg, sess, system)
	msg := ai.Message{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: text}}}
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

func defaultSystemPrompt() string {
	return "You are rune, a coding agent. Use the available tools."
}
