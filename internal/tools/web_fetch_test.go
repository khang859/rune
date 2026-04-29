package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWebFetchFetchesTextAndStatus(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("missing"))
	}))
	defer ts.Close()
	res, err := (WebFetch{AllowPrivate: true}).Run(context.Background(), mustJSON(map[string]any{"url": ts.URL}))
	if err != nil || res.IsError {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	for _, want := range []string{"Status: 404 Not Found", "missing"} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("output missing %q: %s", want, res.Output)
		}
	}
}

func TestWebFetchRejectsInvalidAndSensitiveArgs(t *testing.T) {
	cases := []string{
		`{"url":"file:///etc/passwd"}`,
		`{"url":"https://example.com","headers":{"Authorization":"x"}}`,
		`{"url":"https://example.com","max_bytes":2000001}`,
	}
	for _, c := range cases {
		res, err := (WebFetch{}).Run(context.Background(), json.RawMessage(c))
		if err != nil {
			t.Fatal(err)
		}
		if !res.IsError {
			t.Fatalf("expected error for %s, got %+v", c, res)
		}
	}
}

func TestWebFetchTruncates(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("abcdef"))
	}))
	defer ts.Close()
	res, err := (WebFetch{AllowPrivate: true}).Run(context.Background(), mustJSON(map[string]any{"url": ts.URL, "max_bytes": 3}))
	if err != nil || res.IsError {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	if !strings.Contains(res.Output, "abc") || !strings.Contains(res.Output, "truncated after 3 bytes") {
		t.Fatalf("not truncated: %s", res.Output)
	}
}

func TestWebFetchBlocksPrivateByDefault(t *testing.T) {
	res, err := (WebFetch{}).Run(context.Background(), json.RawMessage(`{"url":"http://127.0.0.1/"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Output, "private/local") {
		t.Fatalf("expected private block, got %+v", res)
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
