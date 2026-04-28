package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/khang859/rune/internal/ai/faux"
)

const version = "0.0.0-dev"

func main() {
	script := flag.String("script", "", "run a JSON script (headless smoke runner)")
	flag.Parse()

	if *script != "" {
		if err := runScript(context.Background(), *script, os.Stdout, faux.New()); err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		return
	}
	fmt.Println("rune", version)
}
