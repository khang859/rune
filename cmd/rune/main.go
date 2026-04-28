package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/khang859/rune/internal/ai/faux"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(os.Stderr, "usage: rune [--script <file>] [--prompt <text>] | rune login codex")
		flag.PrintDefaults()
	}
	script := flag.String("script", "", "run a JSON script (headless smoke runner)")
	prompt := flag.String("prompt", "", "run a single turn against the configured provider and exit")
	model := flag.String("model", "", "Codex model id (overrides RUNE_CODEX_MODEL; default gpt-5.5)")
	flag.Parse()

	ctx := context.Background()

	args := flag.Args()
	if len(args) >= 2 && args[0] == "login" {
		if err := runLogin(ctx, args[1]); err != nil {
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
	// default: interactive
	if err := runInteractive(ctx, *model); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
