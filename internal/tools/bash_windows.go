//go:build windows

package tools

import "os/exec"

// applyKillGroup is a no-op on Windows; native process-tree termination
// requires JobObject + TerminateJobObject, which is not yet wired up.
func applyKillGroup(cmd *exec.Cmd) {}
