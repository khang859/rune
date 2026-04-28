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
		fmt.Fprintln(os.Stderr, "usage: rune [--script <file>] [--prompt <text>] [--version] | rune login codex")
		flag.PrintDefaults()
	}
	showVersion := flag.Bool("version", false, "print version and exit")
	script := flag.String("script", "", "run a JSON script (headless smoke runner)")
	prompt := flag.String("prompt", "", "run a single turn against the configured provider and exit")
	model := flag.String("model", "", "Codex model id (overrides RUNE_CODEX_MODEL; default gpt-5.5)")
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
		if err := runPrompt(ctx, *prompt, *model, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	if err := runInteractive(ctx, *model); err != nil {
		runelog.Error("interactive", "err", err.Error())
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
