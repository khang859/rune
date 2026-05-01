package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime/debug"

	"github.com/khang859/rune/internal/ai/faux"
	"github.com/khang859/rune/internal/config"
	runelog "github.com/khang859/rune/internal/log"
)

var Version = "0.0.0-dev"

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: rune [--provider codex|groq|ollama] [--model <id>] [--prompt <text>] [--version] | rune login codex | rune mcp <command>")
		flag.PrintDefaults()
	}
	showVersion := flag.Bool("version", false, "print version and exit")
	script := flag.String("script", "", "run a JSON script (headless smoke runner)")
	prompt := flag.String("prompt", "", "run a single turn against the configured provider and exit")
	provider := flag.String("provider", "", "provider id (codex, groq, or ollama; overrides RUNE_PROVIDER and settings)")
	model := flag.String("model", "", "model id (overrides provider-specific env/settings default)")
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

	ctx := context.Background()

	args := flag.Args()
	if len(args) >= 1 && args[0] == "mcp" {
		if err := runMCP(args[1:], os.Stdout, os.Stderr); err != nil {
			runelog.Error("mcp", "err", err.Error())
			fmt.Fprintln(os.Stderr, "mcp error:", err)
			os.Exit(1)
		}
		return
	}
	if len(args) >= 2 && args[0] == "login" {
		if err := runLogin(ctx, args[1]); err != nil {
			runelog.Error("login", "err", err.Error())
			fmt.Fprintln(os.Stderr, "login error:", err)
			os.Exit(1)
		}
		return
	}
	if *script != "" {
		if err := runScript(ctx, *script, os.Stdout, faux.New()); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	if *prompt != "" {
		if err := runPrompt(ctx, *prompt, *provider, *model, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	if err := runInteractive(ctx, *provider, *model, Version); err != nil {
		runelog.Error("interactive", "err", err.Error())
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
