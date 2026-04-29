package modal

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/khang859/rune/internal/mcp"
)

type MCPWizardResult struct {
	Name   string
	Config mcp.ServerConfig
}

type mcpWizardStep int

const (
	mcpWizardType mcpWizardStep = iota
	mcpWizardName
	mcpWizardCommand
	mcpWizardArgs
	mcpWizardURL
	mcpWizardHeaders
	mcpWizardReview
)

type MCPWizard struct {
	step       mcpWizardStep
	serverType string
	name       textinput.Model
	command    textinput.Model
	args       textinput.Model
	url        textinput.Model
	headers    textinput.Model
	err        string
}

func NewMCPWizard() Modal {
	name := textinput.New()
	name.Placeholder = "context7"
	name.Prompt = "> "
	name.CharLimit = 128

	command := textinput.New()
	command.Placeholder = "npx"
	command.Prompt = "> "
	command.CharLimit = 512

	args := textinput.New()
	args.Placeholder = "-y @modelcontextprotocol/server-filesystem /path"
	args.Prompt = "> "
	args.CharLimit = 2048

	url := textinput.New()
	url.Placeholder = "https://mcp.context7.com/mcp"
	url.Prompt = "> "
	url.CharLimit = 2048

	headers := textinput.New()
	headers.Placeholder = "CONTEXT7_API_KEY=..."
	headers.Prompt = "> "
	headers.CharLimit = 4096

	return &MCPWizard{
		step:       mcpWizardType,
		serverType: "stdio",
		name:       name,
		command:    command,
		args:       args,
		url:        url,
		headers:    headers,
	}
}

func (m *MCPWizard) Init() tea.Cmd { return nil }

func (m *MCPWizard) Update(msg tea.Msg) (Modal, tea.Cmd) {
	if k, ok := msg.(tea.KeyMsg); ok {
		m.err = ""
		switch k.Type {
		case tea.KeyEsc:
			return m, Cancel()
		case tea.KeyCtrlB:
			m.prev()
			return m, nil
		case tea.KeyTab, tea.KeyUp, tea.KeyDown, tea.KeyLeft, tea.KeyRight:
			if m.step == mcpWizardType {
				m.toggleType()
				return m, nil
			}
		case tea.KeyEnter:
			return m.enter()
		}
	}

	var cmd tea.Cmd
	switch m.step {
	case mcpWizardName:
		m.name, cmd = m.name.Update(msg)
	case mcpWizardCommand:
		m.command, cmd = m.command.Update(msg)
	case mcpWizardArgs:
		m.args, cmd = m.args.Update(msg)
	case mcpWizardURL:
		m.url, cmd = m.url.Update(msg)
	case mcpWizardHeaders:
		m.headers, cmd = m.headers.Update(msg)
	}
	return m, cmd
}

func (m *MCPWizard) enter() (Modal, tea.Cmd) {
	switch m.step {
	case mcpWizardType:
		m.step = mcpWizardName
		return m, m.focusCurrent()
	case mcpWizardName:
		if strings.TrimSpace(m.name.Value()) == "" {
			m.err = "Name is required."
			return m, nil
		}
		if m.serverType == "http" {
			m.step = mcpWizardURL
		} else {
			m.step = mcpWizardCommand
		}
		return m, m.focusCurrent()
	case mcpWizardCommand:
		if strings.TrimSpace(m.command.Value()) == "" {
			m.err = "Command is required."
			return m, nil
		}
		m.step = mcpWizardArgs
		return m, m.focusCurrent()
	case mcpWizardArgs:
		m.step = mcpWizardReview
		return m, m.focusCurrent()
	case mcpWizardURL:
		if strings.TrimSpace(m.url.Value()) == "" {
			m.err = "URL is required."
			return m, nil
		}
		m.step = mcpWizardHeaders
		return m, m.focusCurrent()
	case mcpWizardHeaders:
		if _, err := parseHeaderLines(m.headers.Value()); err != nil {
			m.err = err.Error()
			return m, nil
		}
		m.step = mcpWizardReview
		return m, m.focusCurrent()
	case mcpWizardReview:
		res, err := m.result()
		if err != nil {
			m.err = err.Error()
			return m, nil
		}
		return m, Result(res)
	}
	return m, nil
}

func (m *MCPWizard) prev() {
	m.err = ""
	switch m.step {
	case mcpWizardName:
		m.step = mcpWizardType
	case mcpWizardCommand, mcpWizardURL:
		m.step = mcpWizardName
	case mcpWizardArgs:
		m.step = mcpWizardCommand
	case mcpWizardHeaders:
		m.step = mcpWizardURL
	case mcpWizardReview:
		if m.serverType == "http" {
			m.step = mcpWizardHeaders
		} else {
			m.step = mcpWizardArgs
		}
	}
	_ = m.focusCurrent()
}

func (m *MCPWizard) focusCurrent() tea.Cmd {
	m.name.Blur()
	m.command.Blur()
	m.args.Blur()
	m.url.Blur()
	m.headers.Blur()
	switch m.step {
	case mcpWizardName:
		return m.name.Focus()
	case mcpWizardCommand:
		return m.command.Focus()
	case mcpWizardArgs:
		return m.args.Focus()
	case mcpWizardURL:
		return m.url.Focus()
	case mcpWizardHeaders:
		return m.headers.Focus()
	}
	return nil
}

func (m *MCPWizard) toggleType() {
	if m.serverType == "http" {
		m.serverType = "stdio"
	} else {
		m.serverType = "http"
	}
}

func (m *MCPWizard) result() (MCPWizardResult, error) {
	name := strings.TrimSpace(m.name.Value())
	if name == "" {
		return MCPWizardResult{}, fmt.Errorf("name is required")
	}
	if m.serverType == "http" {
		headers, err := parseHeaderLines(m.headers.Value())
		if err != nil {
			return MCPWizardResult{}, err
		}
		return MCPWizardResult{Name: name, Config: mcp.ServerConfig{Type: "http", URL: strings.TrimSpace(m.url.Value()), Headers: headers}}, nil
	}
	return MCPWizardResult{Name: name, Config: mcp.ServerConfig{Command: strings.TrimSpace(m.command.Value()), Args: splitArgs(m.args.Value())}}, nil
}

func (m *MCPWizard) View(width, height int) string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "Add MCP server\n\n")
	if m.err != "" {
		fmt.Fprintf(&sb, "Error: %s\n\n", m.err)
	}
	switch m.step {
	case mcpWizardType:
		sb.WriteString("Step 1: Server type (↑/↓/Tab toggles, Enter selects)\n")
		writeChoice(&sb, m.serverType == "stdio", "Local command / stdio")
		writeChoice(&sb, m.serverType == "http", "HTTP")
	case mcpWizardName:
		sb.WriteString("Step 2: Name\n")
		sb.WriteString(m.name.View())
	case mcpWizardCommand:
		sb.WriteString("Step 3: Command\n")
		sb.WriteString(m.command.View())
	case mcpWizardArgs:
		sb.WriteString("Step 4: Args (space-separated; quotes supported)\n")
		sb.WriteString(m.args.View())
	case mcpWizardURL:
		sb.WriteString("Step 3: URL\n")
		sb.WriteString(m.url.View())
	case mcpWizardHeaders:
		sb.WriteString("Step 4: Headers (Key=Value, comma-separated; optional)\n")
		sb.WriteString(m.headers.View())
	case mcpWizardReview:
		sb.WriteString("Review\n\n")
		sb.WriteString(m.review())
		sb.WriteString("\nPress Enter to save, Ctrl+B to go back, Esc to cancel.")
	}
	if m.step != mcpWizardReview {
		sb.WriteString("\n\nEnter next • Ctrl+B back • Esc cancel")
	}
	return sb.String()
}

func (m *MCPWizard) review() string {
	res, err := m.result()
	if err != nil {
		return err.Error() + "\n"
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "Name: %s\n", res.Name)
	if res.Config.Type == "http" {
		fmt.Fprintf(&sb, "Type: HTTP\nURL:  %s\n", res.Config.URL)
		if len(res.Config.Headers) > 0 {
			sb.WriteString("Headers:\n")
			keys := make([]string, 0, len(res.Config.Headers))
			for k := range res.Config.Headers {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			for _, k := range keys {
				fmt.Fprintf(&sb, "  %s=%s\n", k, maskSecret(res.Config.Headers[k]))
			}
		}
		return sb.String()
	}
	fmt.Fprintf(&sb, "Type: Local command / stdio\nCommand: %s\n", res.Config.Command)
	if len(res.Config.Args) > 0 {
		fmt.Fprintf(&sb, "Args: %s\n", strings.Join(res.Config.Args, " "))
	}
	return sb.String()
}

func writeChoice(sb *strings.Builder, selected bool, label string) {
	if selected {
		sb.WriteString("  > " + label + "\n")
		return
	}
	sb.WriteString("    " + label + "\n")
}

func parseHeaderLines(s string) (map[string]string, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	headers := map[string]string{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		k, v, ok := strings.Cut(part, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("invalid header %q; want Key=Value", part)
		}
		headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if len(headers) == 0 {
		return nil, nil
	}
	return headers, nil
}

func splitArgs(s string) []string {
	var args []string
	var cur strings.Builder
	var quote rune
	escaped := false
	for _, r := range s {
		if escaped {
			cur.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' {
			if cur.Len() > 0 {
				args = append(args, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		args = append(args, cur.String())
	}
	return args
}

func maskSecret(s string) string {
	if s == "" {
		return ""
	}
	if len(s) <= 4 {
		return strings.Repeat("*", len(s))
	}
	return strings.Repeat("*", len(s)-4) + s[len(s)-4:]
}
