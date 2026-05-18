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
		// Should not advertise SSE on the native endpoint.
		if got := r.Header.Get("Accept"); strings.Contains(got, "text/event-stream") {
			t.Fatalf("accept = %q, should not be SSE", got)
		}
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":"hi"},"done":true,"done_reason":"stop"}` + "\n"))
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

func TestStream_SendsAuthWhenAPIKeyConfigured(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer OK" {
			t.Fatalf("auth header = %q, want bearer", got)
		}
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":""},"done":true}` + "\n"))
	}))
	defer srv.Close()
	p := New(srv.URL, "OK")
	ch, err := p.Stream(context.Background(), ai.Request{Model: "llama3.2"})
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
}

func TestStream_RetriesRateLimit(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limited"}`))
			return
		}
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":""},"done":true}` + "\n"))
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
		_, _ = w.Write([]byte(`{"error":"model not found"}`))
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

func TestNewWithOptions_RewritesLegacyEndpointAtRequestTime(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"no trailing slash", "/v1/chat/completions"},
		{"with trailing slash", "/v1/chat/completions/"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var gotPath string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotPath = r.URL.Path
				_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":""},"done":true}` + "\n"))
			}))
			defer srv.Close()

			p := NewWithOptions(Options{Endpoint: srv.URL + tc.in})
			ch, err := p.Stream(context.Background(), ai.Request{Model: "m"})
			if err != nil {
				t.Fatal(err)
			}
			for range ch {
			}
			if gotPath != "/api/chat" {
				t.Fatalf("rewritten path = %q, want /api/chat", gotPath)
			}
		})
	}
}

func TestNewWithOptions_PassesNumCtxAndThinkInPayload(t *testing.T) {
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		body = buf
		_, _ = w.Write([]byte(`{"message":{"role":"assistant","content":""},"done":true}` + "\n"))
	}))
	defer srv.Close()

	p := NewWithOptions(Options{Endpoint: srv.URL, NumCtx: 32768, Think: true})
	ch, err := p.Stream(context.Background(), ai.Request{Model: "m", Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hi"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
	s := string(body)
	for _, want := range []string{`"think":true`, `"num_ctx":32768`} {
		if !strings.Contains(s, want) {
			t.Fatalf("payload missing %q:\n%s", want, s)
		}
	}
}

func TestStream_IdleTimeoutFailsFastWithActionableError(t *testing.T) {
	// Server accepts the request, writes headers, then goes silent — mimicking
	// Ollama when the prompt exceeds num_ctx (connection stays open, never
	// emits a token). The provider should hit its idle timeout, surface a
	// fatal error mentioning num_ctx, and NOT retry the request.
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		<-r.Context().Done()
	}))
	defer srv.Close()

	p := NewWithOptions(Options{Endpoint: srv.URL, NumCtx: 16384})
	p.streamIdleTimeout = 50 * time.Millisecond
	p.retryBaseDelay = time.Millisecond

	ch, err := p.Stream(context.Background(), ai.Request{Model: "qwen3:4b"})
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
	if streamErr.Class != ai.ErrFatal {
		t.Fatalf("class = %v, want ErrFatal so neither provider nor agent retries", streamErr.Class)
	}
	msg := streamErr.Err.Error()
	for _, want := range []string{"stalled", "num_ctx=16384"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q missing %q", msg, want)
		}
	}
	if hits != 1 {
		t.Fatalf("hits = %d, want 1 (no retries on idle timeout)", hits)
	}
}

func TestListModelsUsesAPITags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			t.Fatalf("path = %q, want /api/tags", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer OK" {
			t.Fatalf("auth header = %q, want bearer", got)
		}
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3.2"},{"name":"qwen3:4b"}]}`))
	}))
	defer srv.Close()
	// Pass the legacy URL on purpose — ListModels should still resolve /api/tags.
	models, err := ListModels(context.Background(), srv.URL+"/v1/chat/completions", "OK")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(models, ",") != "llama3.2,qwen3:4b" {
		t.Fatalf("models = %v", models)
	}
}
