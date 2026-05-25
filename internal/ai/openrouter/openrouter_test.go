package openrouter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai"
)

func TestStreamSendsHeadersAndStreamsText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer sk-or-test-key" {
			t.Fatalf("auth = %q", got)
		}
		if got := r.Header.Get("X-OpenRouter-Title"); got != "rune" {
			t.Fatalf("X-OpenRouter-Title = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("accept = %q", got)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if got := body["model"]; got != "anthropic/claude-sonnet-4.5" {
			t.Fatalf("model = %v", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"},\"finish_reason\":\"stop\"}]}\n\n"))
	}))
	defer srv.Close()

	p := New(srv.URL, "sk-or-test-key")
	ch, err := p.Stream(context.Background(), ai.Request{Model: "anthropic/claude-sonnet-4.5"})
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

func TestStreamMissingKey(t *testing.T) {
	_, err := New("http://example.invalid", "").Stream(context.Background(), ai.Request{Model: "m"})
	if err == nil || !strings.Contains(err.Error(), "openrouter API key") {
		t.Fatalf("err = %v", err)
	}
}

func TestStreamRetriesRateLimit(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer srv.Close()
	p := New(srv.URL, "sk-or-test-key")
	p.retryBaseDelay = time.Millisecond
	ch, err := p.Stream(context.Background(), ai.Request{Model: "m"})
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
	if hits != 2 {
		t.Fatalf("hits = %d, want 2", hits)
	}
}

func TestStreamServerErrorClassified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"error":{"message":"bad gateway","type":"server_error"}}`))
	}))
	defer srv.Close()
	p := New(srv.URL, "sk-or-test-key")
	p.maxRetries = 0
	ch, _ := p.Stream(context.Background(), ai.Request{Model: "m"})
	var class ai.ErrorClass = -1
	for ev := range ch {
		if v, ok := ev.(ai.StreamError); ok {
			class = v.Class
		}
	}
	if class != ai.ErrServer {
		t.Fatalf("class = %v, want ErrServer", class)
	}
}

func TestNormalizeEndpoint(t *testing.T) {
	cases := map[string]string{
		"":                             DefaultEndpoint,
		"https://openrouter.ai/api/v1": "https://openrouter.ai/api/v1/chat/completions",
		"https://openrouter.ai/api/v1/chat/completions": "https://openrouter.ai/api/v1/chat/completions",
		"https://example.test/custom/":                  "https://example.test/custom/chat/completions",
	}
	for in, want := range cases {
		if got := NormalizeEndpoint(in); got != want {
			t.Fatalf("NormalizeEndpoint(%q) = %q, want %q", in, got, want)
		}
	}
}
