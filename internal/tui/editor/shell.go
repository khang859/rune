package editor

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
)

func RunShell(ctx context.Context, cmd string) (string, error) {
	c := exec.CommandContext(ctx, "bash", "-lc", cmd)
	var buf bytes.Buffer
	c.Stdout = &buf
	c.Stderr = &buf
	err := c.Run()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return buf.String(), fmt.Errorf("exit %d", ee.ExitCode())
		}
		return buf.String(), err
	}
	return buf.String(), nil
}
