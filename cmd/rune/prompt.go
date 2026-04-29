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

// DefaultCodexModel is the default model id when --model and RUNE_CODEX_MODEL are unset.
const DefaultCodexModel = "gpt-5.5"

func runPrompt(ctx context.Context, text, model string, w io.Writer) error {
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

	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	agentsMD := agent.LoadAgentsMD(cwd, home)
	system := agent.BasePrompt()
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
