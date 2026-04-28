package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"github.com/khang859/rune/internal/tools"
)

type ServerConfig struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

type Config struct {
	Servers map[string]ServerConfig `json:"servers"`
}

type Manager struct {
	path    string
	clients map[string]*Client
	mu      sync.Mutex
}

func NewManager(path string) *Manager {
	return &Manager{path: path, clients: map[string]*Client{}}
}

func (m *Manager) Start(ctx context.Context, reg *tools.Registry) error {
	b, err := os.ReadFile(m.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var cfg Config
	if err := json.Unmarshal(b, &cfg); err != nil {
		return fmt.Errorf("mcp.json: %w", err)
	}
	for name, sc := range cfg.Servers {
		env := envSlice(sc.Env)
		c, err := Spawn(ctx, name, sc.Command, sc.Args, env)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mcp] failed to spawn %s: %v\n", name, err)
			continue
		}
		if err := c.Initialize(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "[mcp] init %s: %v\n", name, err)
			_ = c.Close()
			continue
		}
		ts, err := c.ListTools(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mcp] list %s: %v\n", name, err)
			_ = c.Close()
			continue
		}
		for _, t := range ts {
			reg.Register(NewTool(c, t))
		}
		m.mu.Lock()
		m.clients[name] = c
		m.mu.Unlock()
	}
	return nil
}

func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.clients {
		_ = c.Close()
	}
	m.clients = map[string]*Client{}
}

func envSlice(m map[string]string) []string {
	if len(m) == 0 {
		return nil
	}
	base := os.Environ()
	out := make([]string, 0, len(base)+len(m))
	out = append(out, base...)
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
