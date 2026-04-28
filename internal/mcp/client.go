package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"sync/atomic"
)

type Client struct {
	name    string
	cmd     *exec.Cmd
	in      io.WriteCloser
	out     *bufio.Reader
	enc     *json.Encoder
	nextID  atomic.Int64
	mu      sync.Mutex
	pending map[string]chan Response
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
	c := &Client{
		name:    name,
		cmd:     cmd,
		in:      in,
		out:     bufio.NewReader(out),
		enc:     json.NewEncoder(in),
		pending: map[string]chan Response{},
	}
	go c.readLoop()
	return c, nil
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
	notif, _ := json.Marshal(Notification{JSONRPC: "2.0", Method: "notifications/initialized"})
	notif = append(notif, '\n')
	_, err := c.in.Write(notif)
	return err
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
	ch := make(chan Response, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
	}()
	line, _ := json.Marshal(req)
	line = append(line, '\n')
	if _, err := c.in.Write(line); err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		if resp.Error != nil {
			return nil, fmt.Errorf("%s: %s", method, resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (c *Client) readLoop() {
	for {
		line, err := c.out.ReadBytes('\n')
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
		var id string
		_ = json.Unmarshal(resp.ID, &id)
		c.mu.Lock()
		ch := c.pending[id]
		c.mu.Unlock()
		if ch != nil {
			ch <- resp
		}
	}
}

func (c *Client) Close() error {
	_ = c.in.Close()
	if c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}

func (c *Client) Name() string { return c.name }
