package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/khang859/rune/internal/tools"
)

type ServerConfig struct {
	Type      string            `json:"type,omitempty"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	Headers   map[string]string `json:"headers,omitempty"`
	ReadOnly  bool              `json:"read_only,omitempty"`
	PlanTools []string          `json:"plan_tools,omitempty"`
}

type Config struct {
	Servers map[string]ServerConfig `json:"servers"`
}

type Status struct {
	Name        string
	Type        string
	Description string
	Connected   bool
	ToolCount   int
	Tools       []string
	Error       string
}

type Manager struct {
	path     string
	clients  map[string]*Client
	statuses map[string]Status
	mu       sync.Mutex
}

func NewManager(path string) *Manager {
	return &Manager{path: path, clients: map[string]*Client{}, statuses: map[string]Status{}}
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
		status := Status{Name: name, Type: serverType(sc), Description: serverDescription(sc)}
		c, err := m.connect(ctx, name, sc)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mcp] failed to connect %s: %v\n", name, err)
			status.Error = err.Error()
			m.setStatus(status)
			continue
		}
		if err := c.Initialize(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "[mcp] init %s: %v\n", name, err)
			status.Error = err.Error()
			m.setStatus(status)
			_ = c.Close()
			continue
		}
		ts, err := c.ListTools(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[mcp] list %s: %v\n", name, err)
			status.Error = err.Error()
			m.setStatus(status)
			_ = c.Close()
			continue
		}
		status.Connected = true
		status.ToolCount = len(ts)
		status.Tools = make([]string, 0, len(ts))
		planTools := planToolSet(sc.PlanTools)
		for _, t := range ts {
			status.Tools = append(status.Tools, t.Name)
			reg.Register(NewToolWithPlanMode(c, t, sc.ReadOnly || planTools[t.Name]))
		}
		m.mu.Lock()
		m.clients[name] = c
		m.statuses[name] = status
		m.mu.Unlock()
	}
	return nil
}

func (m *Manager) connect(ctx context.Context, name string, sc ServerConfig) (*Client, error) {
	switch sc.Type {
	case "", "stdio":
		env := envSlice(sc.Env)
		return Spawn(ctx, name, sc.Command, sc.Args, env)
	case "http":
		return NewHTTPClient(name, sc.URL, sc.Headers)
	default:
		return nil, fmt.Errorf("unsupported server type %q", sc.Type)
	}
}

func (m *Manager) Statuses() []Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	names := make([]string, 0, len(m.statuses))
	for name := range m.statuses {
		names = append(names, name)
	}
	sort.Strings(names)
	out := make([]Status, 0, len(names))
	for _, name := range names {
		st := m.statuses[name]
		st.Tools = append([]string(nil), st.Tools...)
		sort.Strings(st.Tools)
		out = append(out, st)
	}
	return out
}

func (m *Manager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, c := range m.clients {
		_ = c.Close()
	}
	m.clients = map[string]*Client{}
}

func (m *Manager) setStatus(status Status) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.statuses[status.Name] = status
}

func serverType(sc ServerConfig) string {
	if sc.Type == "" {
		return "stdio"
	}
	return sc.Type
}

func serverDescription(sc ServerConfig) string {
	if sc.Type == "http" {
		return "http " + sc.URL
	}
	return strings.Join(append([]string{sc.Command}, sc.Args...), " ")
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

func planToolSet(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out
}
