package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/khang859/rune/internal/ai"
)

const defaultGHMaxBytes = 60000

type GH struct{}

func (GH) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "gh",
		Description: "Run read-only GitHub CLI commands without invoking a shell. Available in Plan Mode. Pass args without the leading 'gh'.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "args":{"type":"array","items":{"type":"string"},"description":"Arguments to pass to gh, excluding the leading gh. Example: [\"pr\",\"view\",\"123\",\"--json\",\"title,body\"]."},
                "max_bytes":{"type":"integer","description":"Maximum output bytes. Defaults to 60000."}
            },
            "required":["args"]
        }`),
	}
}

func (GH) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Args     []string `json:"args"`
		MaxBytes int      `json:"max_bytes"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"args": [string], "max_bytes"?: int}.`, err), IsError: true}, nil
	}
	if a.MaxBytes < 0 {
		return Result{Output: "max_bytes must be non-negative", IsError: true}, nil
	}
	maxBytes := a.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultGHMaxBytes
	}
	if err := validateGHReadonlyArgs(a.Args); err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	out, err := runGHReadonly(ctx, a.Args...)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	if len(out) > maxBytes {
		out = out[:maxBytes] + fmt.Sprintf("\n\n[truncated gh output to %d bytes. Use --json/--jq, --limit, or increase max_bytes to inspect more.]", maxBytes)
	}
	return Result{Output: out}, nil
}

func validateGHReadonlyArgs(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("gh args are required")
	}
	for _, arg := range args {
		if arg == "" {
			return fmt.Errorf("gh args must not contain empty strings")
		}
		switch arg {
		case "--web", "-w", "--browser":
			return fmt.Errorf("gh %s is disabled in read-only mode", arg)
		}
	}

	cmd, subcmd := ghCommandTokens(args)
	if cmd == "" {
		return fmt.Errorf("could not determine gh command from args")
	}
	if cmd == "api" {
		return validateGHAPIReadonly(args)
	}

	allowed := map[string]map[string]bool{
		"issue":    {"list": true, "status": true, "view": true},
		"pr":       {"checks": true, "diff": true, "list": true, "status": true, "view": true},
		"release":  {"list": true, "view": true},
		"repo":     {"list": true, "view": true},
		"run":      {"list": true, "view": true},
		"search":   {"code": true, "commits": true, "issues": true, "prs": true, "repos": true},
		"workflow": {"list": true, "view": true},
	}
	if allowed[cmd][subcmd] {
		return nil
	}
	if subcmd == "" {
		return fmt.Errorf("gh %s is not allowed in read-only mode", cmd)
	}
	return fmt.Errorf("gh %s %s is not allowed in read-only mode", cmd, subcmd)
}

func ghCommandTokens(args []string) (string, string) {
	var toks []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") {
			if ghFlagTakesValue(arg) && !strings.Contains(arg, "=") {
				i++
			}
			continue
		}
		toks = append(toks, arg)
		if len(toks) == 2 {
			break
		}
	}
	cmd := ""
	if len(toks) > 0 {
		cmd = toks[0]
	}
	subcmd := ""
	if len(toks) > 1 {
		subcmd = toks[1]
	}
	return cmd, subcmd
}

func ghFlagTakesValue(arg string) bool {
	switch arg {
	case "-R", "--repo", "--hostname", "--jq", "-q", "--template", "-t", "--limit", "-L", "--method", "-X", "--header", "-H", "--preview", "--cache":
		return true
	}
	if strings.HasPrefix(arg, "-R") && arg != "-R" {
		return false
	}
	if strings.HasPrefix(arg, "-L") && arg != "-L" {
		return false
	}
	if strings.HasPrefix(arg, "-q") && arg != "-q" {
		return false
	}
	if strings.HasPrefix(arg, "-t") && arg != "-t" {
		return false
	}
	if strings.HasPrefix(arg, "-X") && arg != "-X" {
		return false
	}
	return false
}

func validateGHAPIReadonly(args []string) error {
	method := "GET"
	endpoint := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "api" {
			continue
		}
		if arg == "--" {
			break
		}
		if arg == "-f" || arg == "--raw-field" || arg == "-F" || arg == "--field" || arg == "--input" {
			return fmt.Errorf("gh api field/input flags are disabled in read-only mode")
		}
		if strings.HasPrefix(arg, "-f") || strings.HasPrefix(arg, "-F") || strings.HasPrefix(arg, "--raw-field=") || strings.HasPrefix(arg, "--field=") || strings.HasPrefix(arg, "--input=") {
			return fmt.Errorf("gh api field/input flags are disabled in read-only mode")
		}
		if arg == "--method" || arg == "-X" {
			if i+1 >= len(args) {
				return fmt.Errorf("gh api %s requires a method", arg)
			}
			method = strings.ToUpper(args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(arg, "--method=") {
			method = strings.ToUpper(strings.TrimPrefix(arg, "--method="))
			continue
		}
		if strings.HasPrefix(arg, "-X") && arg != "-X" {
			method = strings.ToUpper(strings.TrimPrefix(arg, "-X"))
			continue
		}
		if strings.HasPrefix(arg, "-") {
			if ghFlagTakesValue(arg) && !strings.Contains(arg, "=") {
				i++
			}
			continue
		}
		if endpoint == "" {
			endpoint = arg
		}
	}
	if endpoint == "" {
		return fmt.Errorf("gh api requires an endpoint")
	}
	if endpoint == "graphql" || strings.HasPrefix(endpoint, "graphql?") {
		return fmt.Errorf("gh api graphql is disabled in read-only mode")
	}
	if method != "GET" {
		return fmt.Errorf("gh api method %s is not allowed in read-only mode", method)
	}
	return nil
}

func runGHReadonly(ctx context.Context, args ...string) (string, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return "", fmt.Errorf("gh executable not found")
	}
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Env = append(os.Environ(), "GH_PROMPT_DISABLED=1", "GH_NO_UPDATE_NOTIFIER=1", "NO_COLOR=1", "CLICOLOR=0", "PAGER=cat")
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		out := strings.TrimSpace(buf.String())
		if ctx.Err() != nil {
			if out != "" {
				return "", fmt.Errorf("%s\n(canceled)", out)
			}
			return "", ctx.Err()
		}
		if out != "" {
			return "", fmt.Errorf("gh %s failed: %s", strings.Join(args, " "), out)
		}
		return "", fmt.Errorf("gh %s failed: %v", strings.Join(args, " "), err)
	}
	return buf.String(), nil
}
