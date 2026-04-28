package codex

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/khang859/rune/internal/ai"
	"github.com/khang859/rune/internal/ai/oauth"
)

type stubAuth struct{ tok oauth.Credentials }

func (s *stubAuth) Token(ctx context.Context) (string, error) { return s.tok.AccessToken, nil }
func (s *stubAuth) Refresh(ctx context.Context) error         { return nil }

func TestStream_TextResponse(t *testing.T) {
	fixture, _ := os.ReadFile("testdata/stream_text_only.sse")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer AT" {
			t.Fatalf("auth = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		_, _ = io.Copy(w, strings.NewReader(string(fixture)))
	}))
	defer srv.Close()

	p := New(srv.URL+"/codex/responses", &stubAuth{tok: oauth.Credentials{AccessToken: "AT"}})
	ch, err := p.Stream(context.Background(), ai.Request{
		Model:    "gpt-5",
		Messages: []ai.Message{{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextBlock{Text: "hi"}}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	deadline := time.After(2 * time.Second)
	var sb strings.Builder
	var sawDone bool
loop:
	for {
		select {
		case e, ok := <-ch:
			if !ok {
				break loop
			}
			switch v := e.(type) {
			case ai.TextDelta:
				sb.WriteString(v.Text)
			case ai.Done:
				sawDone = true
				_ = v
			}
		case <-deadline:
			t.Fatal("stream did not finish")
		}
	}
	if sb.String() != "hello world" {
		t.Fatalf("text = %q", sb.String())
	}
	if !sawDone {
		t.Fatal("missing Done")
	}
}

func TestStream_NonStreamingErrorIsRetryable(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("rate limited"))
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"status\":\"completed\",\"usage\":{}}}\n\n"))
	}))
	defer srv.Close()

	p := New(srv.URL+"/codex/responses", &stubAuth{tok: oauth.Credentials{AccessToken: "AT"}})
	p.retryBaseDelay = 1 * time.Millisecond

	ch, err := p.Stream(context.Background(), ai.Request{Model: "gpt-5"})
	if err != nil {
		t.Fatal(err)
	}
	for range ch {
	}
	if hits != 3 {
		t.Fatalf("expected 3 hits after retries, got %d", hits)
	}
}
