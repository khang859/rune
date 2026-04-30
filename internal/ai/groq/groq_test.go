package groq

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai"
)

func TestStream_SendsAuthAndStreamsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer GK" {
			t.Fatalf("auth = %q", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("content-type = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()
	p := New(srv.URL, "GK")
	ch, err := p.Stream(context.Background(), ai.Request{Model: "llama-3.3-70b-versatile"})
	if err != nil {
		t.Fatal(err)
	}
	var text string
	for ev := range ch {
		if v, ok := ev.(ai.TextDelta); ok {
			text += v.Text
		}
	}
	if text != "hi" {
		t.Fatalf("text = %q", text)
	}
}

func TestStream_RetriesRateLimit(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()
	p := New(srv.URL, "GK")
	p.retryBaseDelay = time.Millisecond
	ch, err := p.Stream(context.Background(), ai.Request{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
	if hits != 3 {
		t.Fatalf("hits = %d, want 3", hits)
	}
}

func TestStream_MissingKey(t *testing.T) {
	_, err := New("http://example.invalid", "").Stream(context.Background(), ai.Request{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "Groq API key") {
		t.Fatalf("err = %v", err)
	}
}

// TestStream_ToolUseFailedClassifies verifies that Groq's "tool_use_failed"
// 400 (returned when the model emits something Groq's tool-call parser
// can't extract) surfaces as ai.ErrToolGenerationFailed so the agent loop
// can heal+retry instead of dying. Format documented at
// https://console.groq.com/docs/errors and confirmed in practice via
// `error.code == "tool_use_failed"` plus the `failed_generation` field.
func TestStream_ToolUseFailedClassifies(t *testing.T) {
	body := `{"error":{"message":"Failed to call a function. Please adjust your prompt. See 'failed_generation' for more details.","type":"invalid_request_error","code":"tool_use_failed","failed_generation":"<<bash>>{\"command\":\"git log\"}"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := New(srv.URL, "GK")
	p.maxRetries = 0 // ErrToolGenerationFailed is not transport-retryable; surface immediately
	ch, err := p.Stream(context.Background(), ai.Request{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	var streamErr *ai.StreamError
	for ev := range ch {
		if v, ok := ev.(ai.StreamError); ok {
			streamErr = &v
		}
	}
	if streamErr == nil {
		t.Fatal("expected StreamError")
	}
	if streamErr.Class != ai.ErrToolGenerationFailed {
		t.Fatalf("class = %v, want ErrToolGenerationFailed", streamErr.Class)
	}
}

// TestStream_ToolUseFailedFallsBackToMessageMatch verifies that even if Groq
// changes the `code` field, the substring fallback on the message still
// classifies the error correctly. This is defense in depth.
func TestStream_ToolUseFailedFallsBackToMessageMatch(t *testing.T) {
	body := `{"error":{"message":"Failed to call a function. Please adjust your prompt. See 'failed_generation' for more details.","type":"invalid_request_error"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := New(srv.URL, "GK")
	p.maxRetries = 0
	ch, _ := p.Stream(context.Background(), ai.Request{Model: "m"})
	var class ai.ErrorClass = -1
	for ev := range ch {
		if v, ok := ev.(ai.StreamError); ok {
			class = v.Class
		}
	}
	if class != ai.ErrToolGenerationFailed {
		t.Fatalf("class = %v, want ErrToolGenerationFailed", class)
	}
}

// TestStream_GenericBadRequestStaysFatal verifies that other 400s (not the
// tool_use_failed shape) still classify as ErrFatal — we don't want every
// 400 to trigger the recovery path.
func TestStream_GenericBadRequestStaysFatal(t *testing.T) {
	body := `{"error":{"message":"some other 400 reason","type":"invalid_request_error","code":"some_other_code"}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	p := New(srv.URL, "GK")
	p.maxRetries = 0
	ch, _ := p.Stream(context.Background(), ai.Request{Model: "m"})
	var class ai.ErrorClass = -1
	for ev := range ch {
		if v, ok := ev.(ai.StreamError); ok {
			class = v.Class
		}
	}
	if class != ai.ErrFatal {
		t.Fatalf("class = %v, want ErrFatal", class)
	}
}
