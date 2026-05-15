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

const (
	bashDefaultMaxBytes = 30_000
	bashHardMaxBytes    = 200_000
)

type Bash struct{}

func (Bash) Spec() ai.ToolSpec {
	return ai.ToolSpec{
		Name:        "bash",
		Description: "Run a shell command. Returns combined stdout+stderr. Nonzero exit is an error result, not a Go error. Output beyond max_bytes is replaced with a head+tail truncation marker — pipe through grep/head/tail or redirect to a file if you need more.",
		Schema: json.RawMessage(`{
            "type":"object",
            "properties":{
                "command":{"type":"string"},
                "max_bytes":{"type":"integer","description":"Cap on captured output bytes (default 30000, max 200000). Excess is dropped from the middle with a marker."}
            },
            "required":["command"]
        }`),
	}
}

func (Bash) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		Command  string `json:"command"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"command": string, "max_bytes"?: int}.`, err), IsError: true}, nil
	}
	if a.MaxBytes < 0 {
		return Result{Output: "max_bytes must be non-negative", IsError: true}, nil
	}
	if a.MaxBytes == 0 {
		a.MaxBytes = bashDefaultMaxBytes
	}
	if a.MaxBytes > bashHardMaxBytes {
		return Result{Output: fmt.Sprintf("max_bytes exceeds hard limit of %d", bashHardMaxBytes), IsError: true}, nil
	}

	cmd := exec.CommandContext(ctx, "bash", "-lc", a.Command)
	applyKillGroup(cmd)
	// Backstop the kill-group: if any descendant survives and keeps the
	// stdout/stderr pipe open, WaitDelay forces the I/O goroutines to abort
	// so cmd.Run returns instead of wedging the agent loop.
	cmd.WaitDelay = 2 * time.Second
	w := newCapWriter(a.MaxBytes)
	cmd.Stdout = w
	cmd.Stderr = w
	err := cmd.Run()
	out := w.String()
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

// capWriter captures up to cap bytes of output as head+tail, dropping the
// middle when total exceeds cap. Head fills first; once full, subsequent
// bytes feed a ring buffer that always holds the most recent tail.
type capWriter struct {
	head     bytes.Buffer
	tail     []byte
	tailW    int   // next write index into tail
	tailSeen int64 // total bytes ever written to tail (clamped to tailCap when reading)
	total    int64
	maxBytes int
	headCap  int
	tailCap  int
}

func newCapWriter(maxBytes int) *capWriter {
	headCap := maxBytes / 2
	tailCap := maxBytes - headCap
	return &capWriter{
		tail:     make([]byte, tailCap),
		maxBytes: maxBytes,
		headCap:  headCap,
		tailCap:  tailCap,
	}
}

func (w *capWriter) Write(p []byte) (int, error) {
	n := len(p)
	w.total += int64(n)
	if room := w.headCap - w.head.Len(); room > 0 {
		take := room
		if take > len(p) {
			take = len(p)
		}
		w.head.Write(p[:take])
		p = p[take:]
	}
	if w.tailCap > 0 {
		for _, b := range p {
			w.tail[w.tailW] = b
			w.tailW++
			if w.tailW == w.tailCap {
				w.tailW = 0
			}
			w.tailSeen++
		}
	}
	return n, nil
}

// tailBytes returns the in-order contents of the tail ring.
func (w *capWriter) tailBytes() []byte {
	if w.tailSeen < int64(w.tailCap) {
		// Ring not yet full; bytes are at [0, tailW).
		return w.tail[:w.tailW]
	}
	// Ring full; oldest byte is at tailW (which equals 0 right at the moment
	// of becoming full).
	out := make([]byte, w.tailCap)
	copy(out, w.tail[w.tailW:])
	copy(out[w.tailCap-w.tailW:], w.tail[:w.tailW])
	return out
}

func (w *capWriter) String() string {
	if w.total <= int64(w.maxBytes) {
		var b bytes.Buffer
		b.Grow(int(w.total))
		b.Write(w.head.Bytes())
		b.Write(w.tailBytes())
		return b.String()
	}
	// total > cap: middle was dropped. Head is full; tail ring holds the
	// last tailCap bytes.
	omitted := w.total - int64(w.maxBytes)
	var b bytes.Buffer
	b.Grow(w.maxBytes + 64)
	b.Write(w.head.Bytes())
	fmt.Fprintf(&b, "\n\n[... truncated %d bytes from middle of output (%d total) ...]\n\n", omitted, w.total)
	b.Write(w.tailBytes())
	return b.String()
}
