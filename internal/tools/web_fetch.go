package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/khang859/rune/internal/ai"
)

const defaultWebFetchMaxBytes int64 = 200000
const hardWebFetchMaxBytes int64 = 2000000

type WebFetch struct {
	AllowPrivate bool
	Client       *http.Client
}

func (w WebFetch) Spec() ai.ToolSpec {
	return ai.ToolSpec{Name: "web_fetch", Description: "Fetch a specific HTTP(S) URL and return response metadata plus body text. Use only on URLs from web_search results or URLs explicitly provided by the user.", Schema: json.RawMessage(`{"type":"object","properties":{"url":{"type":"string"},"headers":{"type":"object","additionalProperties":{"type":"string"}},"max_bytes":{"type":"integer","default":200000}},"required":["url"]}`)}
}
func (w WebFetch) Run(ctx context.Context, args json.RawMessage) (Result, error) {
	var a struct {
		URL      string            `json:"url"`
		Headers  map[string]string `json:"headers"`
		MaxBytes int64             `json:"max_bytes"`
	}
	if err := json.Unmarshal(args, &a); err != nil {
		return Result{Output: fmt.Sprintf(`invalid args: %v. Expected JSON: {"url": string, "headers"?: object, "max_bytes"?: number}.`, err), IsError: true}, nil
	}
	if strings.TrimSpace(a.URL) == "" {
		return Result{Output: "url is required", IsError: true}, nil
	}
	u, err := url.Parse(a.URL)
	if err != nil || u.Host == "" {
		return Result{Output: "invalid URL", IsError: true}, nil
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Result{Output: "unsupported URL scheme: only http and https are allowed", IsError: true}, nil
	}
	if a.MaxBytes <= 0 {
		a.MaxBytes = defaultWebFetchMaxBytes
	}
	if a.MaxBytes > hardWebFetchMaxBytes {
		return Result{Output: fmt.Sprintf("max_bytes exceeds hard limit of %d", hardWebFetchMaxBytes), IsError: true}, nil
	}
	for k := range a.Headers {
		if sensitiveHeader(k) {
			return Result{Output: fmt.Sprintf("header %q is not allowed", http.CanonicalHeaderKey(k)), IsError: true}, nil
		}
	}
	if !w.AllowPrivate {
		if err := rejectPrivateHost(ctx, u.Hostname()); err != nil {
			return Result{Output: err.Error(), IsError: true}, nil
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Result{Output: err.Error(), IsError: true}, nil
	}
	req.Header.Set("User-Agent", "rune-web-fetch/1.0")
	for k, v := range a.Headers {
		req.Header.Set(k, v)
	}
	c := w.Client
	if c == nil {
		c = &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			return nil
		}}
	}
	resp, err := c.Do(req)
	if err != nil {
		return Result{Output: fmt.Sprintf("request failed: %v", err), IsError: true}, nil
	}
	defer resp.Body.Close()
	ct := resp.Header.Get("Content-Type")
	var b strings.Builder
	fmt.Fprintf(&b, "URL: %s\nStatus: %s\nContent-Type: %s\n", resp.Request.URL.String(), resp.Status, ct)
	if resp.ContentLength >= 0 {
		fmt.Fprintf(&b, "Content-Length: %d\n", resp.ContentLength)
	}
	b.WriteByte('\n')
	if isBinaryContentType(ct) {
		b.WriteString("[body omitted: response appears to be binary]\n")
		return Result{Output: b.String()}, nil
	}
	lr := io.LimitReader(resp.Body, a.MaxBytes+1)
	data, err := io.ReadAll(lr)
	if err != nil {
		return Result{Output: fmt.Sprintf("read failed: %v", err), IsError: true}, nil
	}
	truncated := int64(len(data)) > a.MaxBytes
	if truncated {
		data = data[:a.MaxBytes]
	}
	if looksBinary(data) {
		b.WriteString("[body omitted: response appears to be binary]\n")
		return Result{Output: b.String()}, nil
	}
	b.Write(data)
	if truncated {
		fmt.Fprintf(&b, "\n[truncated after %d bytes. Re-run web_fetch with a smaller target or higher max_bytes up to %d.]\n", a.MaxBytes, hardWebFetchMaxBytes)
	}
	return Result{Output: b.String()}, nil
}
func sensitiveHeader(k string) bool {
	switch http.CanonicalHeaderKey(k) {
	case "Authorization", "Cookie", "Proxy-Authorization":
		return true
	}
	return false
}
func isBinaryContentType(ct string) bool {
	mt, _, _ := mime.ParseMediaType(ct)
	if mt == "" {
		return false
	}
	if strings.HasPrefix(mt, "text/") {
		return false
	}
	switch mt {
	case "application/json", "application/xml", "application/xhtml+xml", "application/javascript", "application/x-javascript":
		return false
	}
	return true
}
func looksBinary(b []byte) bool {
	if len(b) == 0 {
		return false
	}
	return bytes.IndexByte(b, 0) >= 0
}
func rejectPrivateHost(ctx context.Context, host string) error {
	if strings.EqualFold(host, "localhost") {
		return fmt.Errorf("private/local URLs are blocked by default")
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolve host: %v", err)
	}
	for _, ipa := range ips {
		ip := ipa.IP
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.Equal(net.ParseIP("169.254.169.254")) {
			return fmt.Errorf("private/local URLs are blocked by default")
		}
	}
	return nil
}
