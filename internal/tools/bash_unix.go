//go:build !windows

package tools

import (
	"errors"
	"os/exec"
	"syscall"
)

// applyKillGroup makes ctx-cancel kill bash AND every descendant. The default
// exec.CommandContext only SIGKILLs the bash process itself; descendants
// (e.g. test binaries spawned by `bash -lc 'go test ./...'`) keep the
// inherited stdout pipe open, blocking cmd.Wait and wedging the agent loop.
func applyKillGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative PID delivers the signal to every process in the group.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == nil || errors.Is(err, syscall.ESRCH) {
			return nil
		}
		return cmd.Process.Kill()
	}
}
