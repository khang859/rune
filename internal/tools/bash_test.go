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

func TestBash_OutputUnderCapNotTruncated(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"command": "printf 'abc'", "max_bytes": 100})
	res, err := (Bash{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.Output != "abc" {
		t.Fatalf("output = %q, want %q", res.Output, "abc")
	}
}

func TestBash_OutputOverCapTruncatedHeadAndTail(t *testing.T) {
	// Generate ~3KB; cap to 200 bytes so head=100, tail=100, middle dropped.
	args, _ := json.Marshal(map[string]any{
		"command":   "printf 'AAAAAAAAAA%.0s' $(seq 1 300)",
		"max_bytes": 200,
	})
	res, err := (Bash{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "[... truncated") {
		t.Fatalf("expected truncation marker, got: %q", res.Output)
	}
	if !strings.HasPrefix(res.Output, "AAAAAAAAAA") {
		t.Fatalf("output should start with head bytes, got: %q", res.Output[:min(50, len(res.Output))])
	}
	if !strings.HasSuffix(res.Output, "AAAAAAAAAA") {
		t.Fatalf("output should end with tail bytes, got tail: %q", res.Output[max(0, len(res.Output)-50):])
	}
}

func TestBash_RejectsMaxBytesAboveHardLimit(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"command": "echo hi", "max_bytes": bashHardMaxBytes + 1})
	res, _ := (Bash{}).Run(context.Background(), args)
	if !res.IsError {
		t.Fatal("expected IsError=true for max_bytes above hard limit")
	}
}

func TestBash_DefaultCapAppliedWhenUnspecified(t *testing.T) {
	// No max_bytes -> default 30000. Generate 60000 bytes so we must truncate.
	args, _ := json.Marshal(map[string]any{
		"command": "head -c 60000 /dev/zero | tr '\\0' 'X'",
	})
	res, err := (Bash{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Output, "[... truncated") {
		t.Fatal("expected default cap to truncate 60KB output")
	}
	if len(res.Output) > bashDefaultMaxBytes+200 {
		t.Fatalf("output should be near cap, got %d bytes", len(res.Output))
	}
}

func TestCapWriter_HandlesExactBoundary(t *testing.T) {
	w := newCapWriter(10) // headCap=5, tailCap=5
	w.Write([]byte("0123456789"))
	if w.String() != "0123456789" {
		t.Fatalf("at exact cap, expected full string, got %q", w.String())
	}
	w2 := newCapWriter(10)
	w2.Write([]byte("0123456789X"))
	got := w2.String()
	if !strings.HasPrefix(got, "01234") || !strings.HasSuffix(got, "6789X") {
		t.Fatalf("over-cap head/tail wrong: %q", got)
	}
	if !strings.Contains(got, "truncated 1 bytes") {
		t.Fatalf("expected truncation marker with 1 byte omitted: %q", got)
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
