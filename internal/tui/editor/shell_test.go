package editor

import (
	"context"
	"strings"
	"testing"
)

func TestRunShell_CapturesOutput(t *testing.T) {
	out, _ := RunShell(context.Background(), "echo hi")
	if !strings.Contains(out, "hi") {
		t.Fatalf("out = %q", out)
	}
}

func TestRunShell_ErrorIncluded(t *testing.T) {
	out, err := RunShell(context.Background(), "exit 5")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(out, "5") && !strings.Contains(err.Error(), "5") {
		t.Fatalf("expected exit code in output, got out=%q err=%v", out, err)
	}
}
