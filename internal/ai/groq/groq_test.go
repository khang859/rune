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
