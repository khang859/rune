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

func TestWebFetchSanitizesHTML(t *testing.T) {
	body := `<!doctype html><html><head><title>Hello</title>` +
		`<script>var x=1;alert('pwn')</script>` +
		`<style>body{color:red}</style></head>` +
		`<body><h1>Heading</h1>` +
		`<p>Visible paragraph.</p>` +
		`<noscript>js disabled</noscript>` +
		`<script>more junk</script>` +
		`<a href="https://example.com/foo">click here</a>` +
		`</body></html>`
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	}))
	defer ts.Close()
	res, err := (WebFetch{AllowPrivate: true}).Run(context.Background(), mustJSON(map[string]any{"url": ts.URL}))
	if err != nil || res.IsError {
		t.Fatalf("Run = %+v, %v", res, err)
	}
	for _, want := range []string{"[html sanitized", "Title: Hello", "Heading", "Visible paragraph.", "click here (https://example.com/foo)"} {
		if !strings.Contains(res.Output, want) {
			t.Fatalf("output missing %q: %s", want, res.Output)
		}
	}
	for _, bad := range []string{"alert('pwn')", "color:red", "var x=1", "more junk", "js disabled"} {
		if strings.Contains(res.Output, bad) {
			t.Fatalf("output should not contain %q: %s", bad, res.Output)
		}
	}
}

func TestWebFetchBlocksRedirectToNonHTTPScheme(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "file:///etc/passwd")
		w.WriteHeader(http.StatusFound)
	}))
	defer ts.Close()
	res, err := (WebFetch{AllowPrivate: true}).Run(context.Background(), mustJSON(map[string]any{"url": ts.URL}))
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError || !strings.Contains(res.Output, "unsupported redirect scheme") {
		t.Fatalf("expected redirect scheme block, got %+v", res)
	}
}

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
