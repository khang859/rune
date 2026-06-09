package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/khang859/rune/internal/agent"
	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/config"
	runelog "github.com/khang859/rune/internal/log"
)

var Version = "0.0.0-dev"

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: rune [--provider codex|groq|ollama|runpod|openrouter] [--model <id>] [--profile <name>] [--prompt <text>] [--resume <session-id>] [--version] | rune login [provider] | rune mcp <command>")
		flag.PrintDefaults()
	}
	showVersion := flag.Bool("version", false, "print version and exit")
	script := flag.String("script", "", "run a JSON script (headless smoke runner)")
	prompt := flag.String("prompt", "", "run a single turn against the configured provider and exit")
	resume := flag.String("resume", "", "resume a saved session by id (interactive, or one headless turn with --prompt)")
	provider := flag.String("provider", "", "provider id (codex, groq, ollama, runpod, or openrouter; overrides RUNE_PROVIDER and settings)")
	model := flag.String("model", "", "model id (overrides provider-specific env/settings default)")
	profileName := flag.String("profile", "", "named worker profile (~/.rune/profiles/<name>.md) applying a model, skills, and persona")
	requireTool := flag.String("require-tool", "", "headless: comma-separated tools the agent must call before ending its turn; nudges it to continue otherwise, exits 3 if it never does")
	flag.Parse()

	if *showVersion {
		fmt.Println("rune", Version)
		return
	}

	if err := config.EnsureRuneDir(); err == nil {
		_ = runelog.Init(config.LogPath())
		defer runelog.Close()
	}
	defer func() {
		if r := recover(); r != nil {
			runelog.Error("panic", "value", fmt.Sprint(r), "stack", string(debug.Stack()))
			fmt.Fprintln(os.Stderr, "rune crashed; details in", config.LogPath())
			os.Exit(2)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	args := flag.Args()
	if len(args) >= 1 && args[0] == "mcp" {
		if err := runMCP(args[1:], os.Stdout, os.Stderr); err != nil {
			runelog.Error("mcp", "err", err.Error())
			fmt.Fprintln(os.Stderr, "mcp error:", err)
			os.Exit(1)
		}
		return
	}
	if len(args) >= 1 && args[0] == "login" {
		var loginErr error
		if len(args) >= 2 {
			loginErr = runLogin(ctx, args[1])
		} else {
			loginErr = runLoginInteractive(ctx, os.Stdin, os.Stdout)
		}
		if loginErr != nil {
			runelog.Error("login", "err", loginErr.Error())
			fmt.Fprintln(os.Stderr, "login error:", loginErr)
			os.Exit(1)
		}
		return
	}
	if *resume != "" && *script != "" {
		fmt.Fprintln(os.Stderr, "error: --resume cannot be combined with --script")
		os.Exit(1)
	}
	if *script != "" {
		if err := runScript(ctx, *script, os.Stdout, faux.New()); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	if *prompt != "" {
		err := runPrompt(ctx, *prompt, *provider, *model, *profileName, *requireTool, *resume, os.Stdout)
		if errors.Is(err, agent.ErrIncompleteRequiredTool) {
			// Distinct from a generic error (exit 1) or crash (exit 2): the run
			// finished cleanly but the model never called a required completion
			// tool. An orchestrator uses this to route to review instead of retry.
			fmt.Fprintln(os.Stderr, "incomplete:", err)
			os.Exit(3)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	if err := runInteractiveWithOptions(ctx, interactiveOptions{
		ProviderOverride: *provider,
		ModelOverride:    *model,
		ProfileName:      *profileName,
		Version:          Version,
		ResumeID:         *resume,
	}); err != nil {
		runelog.Error("interactive", "err", err.Error())
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
