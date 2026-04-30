package ollama

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai"
)

func TestStream_NoAuthRequiredAndStreamsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("auth header = %q, want empty", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.Contains(got, "application/json") {
			t.Fatalf("content-type = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()
	p := New(srv.URL)
	ch, err := p.Stream(context.Background(), ai.Request{Model: "llama3.2"})
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
	p := New(srv.URL)
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

func TestStream_ModelNotFoundMentionsPullCommand(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"message":"model not found"}}`))
	}))
	defer srv.Close()
	p := New(srv.URL)
	p.maxRetries = 0
	ch, err := p.Stream(context.Background(), ai.Request{Model: "qwen3:4b"})
	if err != nil {
		t.Fatal(err)
	}
	var msg string
	for ev := range ch {
		if v, ok := ev.(ai.StreamError); ok {
			msg = v.Err.Error()
		}
	}
	if !strings.Contains(msg, "ollama pull qwen3:4b") {
		t.Fatalf("error = %q", msg)
	}
}

func TestListModelsUsesAPITags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Fatalf("path = %q, want /api/tags", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3.2"},{"name":"qwen3:4b"}]}`))
	}))
	defer srv.Close()
	models, err := ListModels(context.Background(), srv.URL+"/v1/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(models, ",") != "llama3.2,qwen3:4b" {
		t.Fatalf("models = %v", models)
	}
}
