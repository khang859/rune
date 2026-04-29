package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
)

type transport interface {
	Call(ctx context.Context, req Request) (Response, error)
	Notify(ctx context.Context, notif Notification) error
	Close() error
}

type Client struct {
	name      string
	transport transport
	nextID    atomic.Int64
}

func Spawn(ctx context.Context, name, bin string, args []string, env []string) (*Client, error) {
	cmd := exec.CommandContext(ctx, bin, args...)
	if len(env) > 0 {
		cmd.Env = append([]string{}, env...)
	}
	in, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	tr := newStdioTransport(cmd, in, out)
	return &Client{name: name, transport: tr}, nil
}

func NewHTTPClient(name, url string, headers map[string]string) (*Client, error) {
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("http server %q missing url", name)
	}
	return &Client{name: name, transport: newHTTPTransport(url, headers)}, nil
}

func (c *Client) Initialize(ctx context.Context) error {
	params, _ := json.Marshal(InitializeParams{
		ProtocolVersion: ProtocolVersion,
		ClientInfo:      map[string]string{"name": "rune", "version": "0.0.0-dev"},
		Capabilities:    map[string]any{},
	})
	if _, err := c.call(ctx, "initialize", params); err != nil {
		return err
	}
	return c.transport.Notify(ctx, Notification{JSONRPC: "2.0", Method: "notifications/initialized"})
}

func (c *Client) ListTools(ctx context.Context) ([]Tool, error) {
	res, err := c.call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}
	var r ToolsListResult
	if err := json.Unmarshal(res, &r); err != nil {
		return nil, err
	}
	return r.Tools, nil
}

func (c *Client) CallTool(ctx context.Context, name string, args json.RawMessage) (ToolsCallResult, error) {
	p, _ := json.Marshal(ToolsCallParams{Name: name, Arguments: args})
	res, err := c.call(ctx, "tools/call", p)
	if err != nil {
		return ToolsCallResult{}, err
	}
	var r ToolsCallResult
	if err := json.Unmarshal(res, &r); err != nil {
		return ToolsCallResult{}, err
	}
	return r, nil
}

func (c *Client) call(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	id := fmt.Sprintf("%d", c.nextID.Add(1))
	rawID, _ := json.Marshal(id)
	req := Request{JSONRPC: "2.0", ID: rawID, Method: method, Params: params}
	resp, err := c.transport.Call(ctx, req)
	if err != nil {
		return nil, err
	}
	if resp.Error != nil {
		return nil, fmt.Errorf("%s: %s", method, resp.Error.Message)
	}
	return resp.Result, nil
}

func (c *Client) Close() error { return c.transport.Close() }

func (c *Client) Name() string { return c.name }

type stdioTransport struct {
	cmd     *exec.Cmd
	in      io.WriteCloser
	out     *bufio.Reader
	mu      sync.Mutex
	pending map[string]chan Response
}

func newStdioTransport(cmd *exec.Cmd, in io.WriteCloser, out io.Reader) *stdioTransport {
	tr := &stdioTransport{
		cmd:     cmd,
		in:      in,
		out:     bufio.NewReader(out),
		pending: map[string]chan Response{},
	}
	go tr.readLoop()
	return tr
}

func (t *stdioTransport) Call(ctx context.Context, req Request) (Response, error) {
	id, err := requestIDString(req.ID)
	if err != nil {
		return Response{}, err
	}
	ch := make(chan Response, 1)
	t.mu.Lock()
	t.pending[id] = ch
	t.mu.Unlock()
	defer func() {
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
	}()

	line, _ := json.Marshal(req)
	line = append(line, '\n')
	if _, err := t.in.Write(line); err != nil {
		return Response{}, err
	}
	select {
	case <-ctx.Done():
		return Response{}, ctx.Err()
	case resp := <-ch:
		return resp, nil
	}
}

func (t *stdioTransport) Notify(ctx context.Context, notif Notification) error {
	line, _ := json.Marshal(notif)
	line = append(line, '\n')
	_, err := t.in.Write(line)
	return err
}

func (t *stdioTransport) readLoop() {
	for {
		line, err := t.out.ReadBytes('\n')
		if err != nil {
			return
		}
		var resp Response
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}
		if len(resp.ID) == 0 {
			continue
		}
		id, err := requestIDString(resp.ID)
		if err != nil {
			continue
		}
		t.mu.Lock()
		ch := t.pending[id]
		t.mu.Unlock()
		if ch != nil {
			ch <- resp
		}
	}
}

func (t *stdioTransport) Close() error {
	_ = t.in.Close()
	if t.cmd.Process != nil {
		return t.cmd.Process.Kill()
	}
	return nil
}

type httpTransport struct {
	url       string
	headers   map[string]string
	client    *http.Client
	sessionID string
	mu        sync.Mutex
}

func newHTTPTransport(url string, headers map[string]string) *httpTransport {
	copied := map[string]string{}
	for k, v := range headers {
		copied[k] = v
	}
	return &httpTransport{url: url, headers: copied, client: http.DefaultClient}
}

func (t *httpTransport) Call(ctx context.Context, rpcReq Request) (Response, error) {
	var resp Response
	if err := t.do(ctx, rpcReq, true, &resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

func (t *httpTransport) Notify(ctx context.Context, notif Notification) error {
	return t.do(ctx, notif, false, nil)
}

func (t *httpTransport) do(ctx context.Context, payload any, expectResponse bool, out *Response) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	for k, v := range t.headers {
		req.Header.Set(k, v)
	}
	t.mu.Lock()
	if t.sessionID != "" {
		req.Header.Set("Mcp-Session-Id", t.sessionID)
	}
	t.mu.Unlock()

	res, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if sid := res.Header.Get("Mcp-Session-Id"); sid != "" {
		t.mu.Lock()
		t.sessionID = sid
		t.mu.Unlock()
	}

	if res.StatusCode == http.StatusAccepted || (!expectResponse && res.StatusCode == http.StatusNoContent) {
		return nil
	}
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(res.Body, 4096))
		return fmt.Errorf("http mcp %s: %s", res.Status, strings.TrimSpace(string(b)))
	}
	if !expectResponse {
		return nil
	}

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return err
	}
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return fmt.Errorf("http mcp empty response")
	}
	if strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") || bytes.HasPrefix(b, []byte("event:")) || bytes.HasPrefix(b, []byte("data:")) {
		b, err = sseJSONData(b)
		if err != nil {
			return err
		}
	}
	return json.Unmarshal(b, out)
}

func (t *httpTransport) Close() error { return nil }

func requestIDString(raw json.RawMessage) (string, error) {
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s, nil
	}
	var n json.Number
	if err := json.Unmarshal(raw, &n); err == nil {
		return n.String(), nil
	}
	return "", fmt.Errorf("invalid json-rpc id %s", string(raw))
}

func sseJSONData(b []byte) ([]byte, error) {
	var data []string
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "data:") {
			part := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			if part == "[DONE]" {
				continue
			}
			data = append(data, part)
		}
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("http mcp event stream missing data")
	}
	return []byte(strings.Join(data, "\n")), nil
}
