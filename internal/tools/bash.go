package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"time"

	"github.com/khang859/rune/internal/ai"
)

type Bash struct{}

func (Bash) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "bash",
		Description: "Run a shell command. Returns combined stdout+stderr. Nonzero exit is an error result, not a Go error.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{"command":{"type":"string"}},
            "required":["command"]
        }`),
	}
}

func (Bash) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"command": string}.`, err), IsError: true}, nil
	}
	cmd := exec.CommandContext(ctx, "bash", "-lc", a.Command)
	applyKillGroup(cmd)
	// Backstop the kill-group: if any descendant survives and keeps the
	// stdout/stderr pipe open, WaitDelay forces the I/O goroutines to abort
	// so cmd.Run returns instead of wedging the agent loop.
	cmd.WaitDelay = 2 * time.Second
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	out := buf.String()
	if err != nil {
		// Surface cancellation as a Go error so runTools short-circuits the
		// remaining tool batch and the agent loop emits TurnAborted instead
		// of churning through queued calls.
		if ctxErr := ctx.Err(); ctxErr != nil {
			return Result{Output: out + "\n(canceled)", IsError: true}, ctxErr
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return Result{Output: fmt.Sprintf("%s\n(exit %d)", out, ee.ExitCode()), IsError: true}, nil
		}
		return Result{Output: out + "\n" + err.Error(), IsError: true}, nil
	}
	return Result{Output: out}, nil
}
