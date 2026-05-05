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
	"golang.org/x/net/html"
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
		c = w.buildClient()
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
	if isHTMLContentType(ct) {
		text := extractReadableHTML(data)
		b.WriteString("[html sanitized: scripts, styles, and markup stripped]\n\n")
		b.WriteString(text)
		if truncated {
			fmt.Fprintf(&b, "\n[truncated after %d bytes of raw HTML. Re-run web_fetch with a higher max_bytes up to %d.]\n", a.MaxBytes, hardWebFetchMaxBytes)
		}
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
func isHTMLContentType(ct string) bool {
	mt, _, _ := mime.ParseMediaType(ct)
	switch mt {
	case "text/html", "application/xhtml+xml":
		return true
	}
	return false
}

var skipHTMLElements = map[string]bool{
	"script": true, "style": true, "noscript": true, "svg": true,
	"iframe": true, "template": true, "canvas": true, "math": true,
}

func extractReadableHTML(data []byte) string {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return string(data)
	}
	var title string
	var buf strings.Builder
	var walk func(n *html.Node, inBody bool)
	walk = func(n *html.Node, inBody bool) {
		switch n.Type {
		case html.CommentNode:
			return
		case html.ElementNode:
			if skipHTMLElements[n.Data] {
				return
			}
			if n.Data == "title" && n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				title = strings.TrimSpace(n.FirstChild.Data)
				return
			}
			if n.Data == "body" {
				inBody = true
			}
			if inBody && n.Data == "a" {
				var linkText strings.Builder
				collectText(n, &linkText)
				txt := strings.TrimSpace(collapseWS(linkText.String()))
				href := attr(n, "href")
				if txt != "" {
					buf.WriteString(txt)
					if strings.HasPrefix(href, "http://") || strings.HasPrefix(href, "https://") {
						buf.WriteString(" (")
						buf.WriteString(href)
						buf.WriteString(")")
					}
					buf.WriteByte(' ')
				}
				return
			}
			if inBody && isBlockElement(n.Data) {
				buf.WriteByte('\n')
			}
		case html.TextNode:
			if inBody {
				t := collapseWS(n.Data)
				if t != "" {
					buf.WriteString(t)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, inBody)
		}
		if n.Type == html.ElementNode && inBody && isBlockElement(n.Data) {
			buf.WriteByte('\n')
		}
	}
	walk(doc, false)
	out := normalizeBlankLines(buf.String())
	if title != "" {
		return "Title: " + title + "\n\n" + out
	}
	return out
}

func collectText(n *html.Node, b *strings.Builder) {
	if n.Type == html.TextNode {
		b.WriteString(n.Data)
		return
	}
	if n.Type == html.ElementNode && skipHTMLElements[n.Data] {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectText(c, b)
	}
}

func attr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func isBlockElement(tag string) bool {
	switch tag {
	case "p", "div", "br", "hr", "li", "ul", "ol", "tr", "table",
		"section", "article", "header", "footer", "nav", "aside", "main",
		"h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "figure":
		return true
	}
	return false
}

func collapseWS(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	prevSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !prevSpace {
				b.WriteByte(' ')
				prevSpace = true
			}
			continue
		}
		b.WriteRune(r)
		prevSpace = false
	}
	return b.String()
}

func normalizeBlankLines(s string) string {
	lines := strings.Split(s, "\n")
	var out []string
	blanks := 0
	for _, ln := range lines {
		ln = strings.TrimSpace(ln)
		if ln == "" {
			blanks++
			if blanks <= 1 {
				out = append(out, "")
			}
			continue
		}
		blanks = 0
		out = append(out, ln)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
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
func isBlockedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified() ||
		ip.Equal(net.ParseIP("169.254.169.254"))
}

func (w WebFetch) buildClient() *http.Client {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	allowPrivate := w.AllowPrivate
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			if !allowPrivate && strings.EqualFold(host, "localhost") {
				return nil, fmt.Errorf("private/local addresses are blocked by default")
			}
			ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolve host: %v", err)
			}
			var lastErr error
			for _, ipa := range ips {
				if !allowPrivate && isBlockedIP(ipa.IP) {
					lastErr = fmt.Errorf("private/local addresses are blocked by default")
					continue
				}
				ipStr := ipa.IP.String()
				if ipa.Zone != "" {
					ipStr = ipStr + "%" + ipa.Zone
				}
				conn, derr := dialer.DialContext(ctx, network, net.JoinHostPort(ipStr, port))
				if derr == nil {
					return conn, nil
				}
				lastErr = derr
			}
			if lastErr == nil {
				lastErr = fmt.Errorf("no addresses for %s", host)
			}
			return nil, lastErr
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("stopped after 10 redirects")
			}
			if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
				return fmt.Errorf("unsupported redirect scheme: %s", req.URL.Scheme)
			}
			return nil
		},
	}
}
