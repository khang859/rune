package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRead_File(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "foo.txt")
	if err := os.WriteFile(p, []byte("hello\nworld\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	args, _ := json.Marshal(map[string]any{"path": p})
	res, err := (Read{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if res.Output != "hello\nworld\n" {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestRead_Missing(t *testing.T) {
	args, _ := json.Marshal(map[string]any{"path": "/does/not/exist"})
	res, err := (Read{}).Run(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected go error: %v", err)
	}
	if !res.IsError {
		t.Fatalf("expected IsError=true, got %#v", res)
	}
	if !strings.Contains(res.Output, "/does/not/exist") || !strings.Contains(res.Output, "ls") {
		t.Fatalf("output should guide recovery: %q", res.Output)
	}
}

func TestRead_BadArgs(t *testing.T) {
	res, err := (Read{}).Run(context.Background(), json.RawMessage(`not-json`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true")
	}
	if !strings.Contains(res.Output, `"path"`) {
		t.Fatalf("output should echo expected schema: %q", res.Output)
	}
}

func writeNumberedLines(t *testing.T, n int) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "big.txt")
	var sb strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, "line%d\n", i)
	}
	if err := os.WriteFile(p, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRead_DefaultLimitTruncates(t *testing.T) {
	p := writeNumberedLines(t, 500)
	args, _ := json.Marshal(map[string]any{"path": p})
	res, err := (Read{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "line1\n") || !strings.Contains(res.Output, "line200") {
		t.Fatalf("expected first 200 lines; got: %q", res.Output)
	}
	if strings.Contains(res.Output, "line201") {
		t.Fatalf("should not include line beyond default limit")
	}
	if !strings.Contains(res.Output, "showing lines 1-200 of 500") {
		t.Fatalf("missing truncation footer: %q", res.Output)
	}
	if !strings.Contains(res.Output, "offset=201") {
		t.Fatalf("footer should suggest next offset: %q", res.Output)
	}
	if !strings.Contains(res.Output, "read_all") {
		t.Fatalf("footer should mention read_all: %q", res.Output)
	}
}

func TestRead_OffsetAndLimit(t *testing.T) {
	p := writeNumberedLines(t, 500)
	args, _ := json.Marshal(map[string]any{"path": p, "offset": 100, "limit": 5})
	res, err := (Read{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	want := "line100\nline101\nline102\nline103\nline104"
	if !strings.HasPrefix(res.Output, want) {
		t.Fatalf("output prefix = %q", res.Output)
	}
	if !strings.Contains(res.Output, "showing lines 100-104 of 500") {
		t.Fatalf("missing footer: %q", res.Output)
	}
}

func TestRead_OffsetReachesEnd(t *testing.T) {
	p := writeNumberedLines(t, 10)
	args, _ := json.Marshal(map[string]any{"path": p, "offset": 9, "limit": 200})
	res, err := (Read{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if res.Output != "line9\nline10\n" {
		t.Fatalf("output = %q", res.Output)
	}
}

func TestRead_OffsetOutOfRange(t *testing.T) {
	p := writeNumberedLines(t, 10)
	args, _ := json.Marshal(map[string]any{"path": p, "offset": 11})
	res, err := (Read{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true")
	}
	if !strings.Contains(res.Output, "10 lines") {
		t.Fatalf("error should mention file length: %q", res.Output)
	}
}

func TestRead_ReadAllIgnoresLimit(t *testing.T) {
	p := writeNumberedLines(t, 500)
	args, _ := json.Marshal(map[string]any{"path": p, "read_all": true})
	res, err := (Read{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("unexpected error: %s", res.Output)
	}
	if !strings.Contains(res.Output, "line500\n") {
		t.Fatalf("read_all should return whole file: %q", res.Output[len(res.Output)-50:])
	}
	if strings.Contains(res.Output, "showing lines") {
		t.Fatalf("read_all should not emit truncation footer")
	}
}

func TestRead_NegativeArgs(t *testing.T) {
	p := writeNumberedLines(t, 10)
	args, _ := json.Marshal(map[string]any{"path": p, "offset": -1})
	res, err := (Read{}).Run(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatal("expected IsError=true for negative offset")
	}
}
