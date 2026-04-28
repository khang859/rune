//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"os"
)

func main() {
	in := bufio.NewReader(os.Stdin)
	enc := json.NewEncoder(os.Stdout)
	for {
		line, err := in.ReadString('\n')
		if err != nil {
			return
		}
		var req struct {
			JSONRPC string          `json:"jsonrpc"`
			ID      json.RawMessage `json:"id,omitempty"`
			Method  string          `json:"method"`
			Params  json.RawMessage `json:"params,omitempty"`
		}
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			continue
		}
		switch req.Method {
		case "initialize":
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(req.ID),
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"serverInfo":      map[string]string{"name": "stub", "version": "0.0.1"},
					"capabilities":    map[string]any{"tools": map[string]any{}},
				},
			})
		case "tools/list":
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(req.ID),
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "echo",
							"description": "echoes back its argument",
							"inputSchema": map[string]any{
								"type":       "object",
								"properties": map[string]any{"text": map[string]any{"type": "string"}},
								"required":   []string{"text"},
							},
						},
					},
				},
			})
		case "tools/call":
			var p struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			}
			_ = json.Unmarshal(req.Params, &p)
			var args struct {
				Text string `json:"text"`
			}
			_ = json.Unmarshal(p.Arguments, &args)
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(req.ID),
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": "echo: " + args.Text}},
				},
			})
		case "notifications/initialized":
			// no-op
		default:
			_ = enc.Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      json.RawMessage(req.ID),
				"error":   map[string]any{"code": -32601, "message": "method not found: " + req.Method},
			})
		}
	}
}
