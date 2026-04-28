package log

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_WritesToFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log")
	if err := Init(p); err != nil {
		t.Fatal(err)
	}
	Info("hello", "k", "v")
	Close()

	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "hello") || !strings.Contains(string(b), "v") {
		t.Fatalf("log missing entries: %q", b)
	}
}

func TestRotate_RotatesAtThreshold(t *testing.T) {
	p := filepath.Join(t.TempDir(), "log")
	rotateThreshold = 200
	if err := Init(p); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 50; i++ {
		Info("filler line", "i", i)
	}
	Close()

	rotated := p + ".1"
	if _, err := os.Stat(rotated); err != nil {
		t.Fatalf("expected rotated file at %s: %v", rotated, err)
	}
}
