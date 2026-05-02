package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBash_RunsCommand(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"command": "echo hello"})
	res, err := (Bash{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "hello") {
		t.Fatalf("output = %q", res.Output)
	}
	if res.IsError {
		t.Fatal("expected success")
	}
}

func TestBash_NonzeroExitIsErrorButOutputIncluded(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"command": "echo nope; exit 7"})
	res, _ := (Bash{}).Run(context.Background(), args)
	if !res.IsError {
		t.Fatal("expected IsError=true on nonzero exit")
	}
	if !strings.Contains(res.Output, "nope") {
		t.Fatalf("output should include stdout: %q", res.Output)
	}
	if !strings.Contains(res.Output, "7") {
		t.Fatalf("output should mention exit code: %q", res.Output)
	}
}

func TestBash_ContextCancelKillsProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	args, _ := json.Marshal(map[string]any{"command": "sleep 5"})
	done := make(chan struct{})
	var gotErr error
	go func() {
		_, gotErr = (Bash{}).Run(ctx, args)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("bash did not exit on ctx cancel")
	}
	if gotErr != context.Canceled {
		t.Fatalf("expected context.Canceled, got %v", gotErr)
	}
}

// Smoke-tests the wedge that motivated the kill-group fix: bash backgrounds a
// long-lived child that inherits stdout/stderr. The 1500ms assert window is
// below WaitDelay (2s) so the I/O drain must happen via group-kill on Linux.
// (On darwin the kernel/Go combo can return earlier even without group-kill,
// so this test is a smoke test there rather than a tight regression.)
func TestBash_ContextCancelKillsBackgroundedChild(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	args, _ := json.Marshal(map[string]any{"command": "sleep 30 & wait"})
	done := make(chan struct{})
	go func() {
		_, _ = (Bash{}).Run(ctx, args)
		close(done)
	}()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(1500 * time.Millisecond):
		t.Fatal("bash wedged: backgrounded child kept the pipe open after ctx cancel")
	}
}
