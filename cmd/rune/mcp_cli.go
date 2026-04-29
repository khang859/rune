package main

import (
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/khang859/rune/internal/config"
	"github.com/khang859/rune/internal/mcp"
)

func runMCP(args []string, stdout, stderr io.Writer) error {
	if len(args) == 0 {
		printMCPUsage(stderr)
		return errors.New("missing mcp subcommand")
	}

	switch args[0] {
	case "add":
		return runMCPAdd(args[1:])
	case "add-http":
		return runMCPAddHTTP(args[1:])
	case "wizard":
		return runMCPWizard(stdout)
	case "list", "ls":
		return runMCPList(stdout)
	case "remove", "rm":
		return runMCPRemove(args[1:])
	case "path":
		fmt.Fprintln(stdout, config.MCPConfig())
		return nil
	case "validate":
		return runMCPValidate(stdout)
	case "help", "-h", "--help":
		printMCPUsage(stdout)
		return nil
	default:
		printMCPUsage(stderr)
		return fmt.Errorf("unknown mcp subcommand %q", args[0])
	}
}

func printMCPUsage(w io.Writer) {
	fmt.Fprintln(w, "usage:")
	fmt.Fprintln(w, "  rune mcp add <name> [--force] -- <command> [args...]")
	fmt.Fprintln(w, "  rune mcp add-http <name> --url <url> [--header Key=Value] [--force]")
	fmt.Fprintln(w, "  rune mcp wizard")
	fmt.Fprintln(w, "  rune mcp list")
	fmt.Fprintln(w, "  rune mcp remove <name>")
	fmt.Fprintln(w, "  rune mcp path")
	fmt.Fprintln(w, "  rune mcp validate")
}

func runMCPAdd(args []string) error {
	name, force, commandArgs, err := parseMCPAddArgs(args)
	if err != nil {
		return err
	}

	return mcp.AddServer(config.MCPConfig(), name, mcp.ServerConfig{
		Command: commandArgs[0],
		Args:    append([]string(nil), commandArgs[1:]...),
	}, force)
}

func parseMCPAddArgs(args []string) (name string, force bool, commandArgs []string, err error) {
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep == -1 {
		return "", false, nil, errors.New("add requires -- before the server command")
	}
	if sep == len(args)-1 {
		return "", false, nil, errors.New("add requires a server command after --")
	}

	for _, arg := range args[:sep] {
		switch arg {
		case "--force", "-f":
			force = true
		default:
			if strings.HasPrefix(arg, "-") {
				return "", false, nil, fmt.Errorf("unknown add flag %q", arg)
			}
			if name != "" {
				return "", false, nil, fmt.Errorf("unexpected extra server name %q", arg)
			}
			name = arg
		}
	}
	if name == "" {
		return "", false, nil, errors.New("add requires a server name")
	}
	return name, force, args[sep+1:], nil
}

func runMCPAddHTTP(args []string) error {
	name, url, headers, force, err := parseMCPAddHTTPArgs(args)
	if err != nil {
		return err
	}

	return mcp.AddServer(config.MCPConfig(), name, mcp.ServerConfig{
		Type:    "http",
		URL:     url,
		Headers: headers,
	}, force)
}

func parseMCPAddHTTPArgs(args []string) (name, url string, headers map[string]string, force bool, err error) {
	headers = map[string]string{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--force", "-f":
			force = true
		case "--url":
			if i+1 >= len(args) {
				return "", "", nil, false, errors.New("--url requires a value")
			}
			i++
			url = args[i]
		case "--header":
			if i+1 >= len(args) {
				return "", "", nil, false, errors.New("--header requires Key=Value")
			}
			i++
			k, v, ok := strings.Cut(args[i], "=")
			if !ok || k == "" {
				return "", "", nil, false, fmt.Errorf("invalid header %q; want Key=Value", args[i])
			}
			headers[k] = v
		default:
			if strings.HasPrefix(arg, "-") {
				return "", "", nil, false, fmt.Errorf("unknown add-http flag %q", arg)
			}
			if name != "" {
				return "", "", nil, false, fmt.Errorf("unexpected extra server name %q", arg)
			}
			name = arg
		}
	}
	if name == "" {
		return "", "", nil, false, errors.New("add-http requires a server name")
	}
	if url == "" {
		return "", "", nil, false, errors.New("add-http requires --url")
	}
	if len(headers) == 0 {
		headers = nil
	}
	return name, url, headers, force, nil
}

func runMCPList(stdout io.Writer) error {
	cfg, err := loadMCPConfig()
	if err != nil {
		return err
	}
	if len(cfg.Servers) == 0 {
		return nil
	}

	names := make([]string, 0, len(cfg.Servers))
	for name := range cfg.Servers {
		names = append(names, name)
	}
	sort.Strings(names)

	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	for _, name := range names {
		sc := cfg.Servers[name]
		desc := strings.Join(append([]string{sc.Command}, sc.Args...), " ")
		if sc.Type == "http" {
			desc = "http " + sc.URL
		}
		fmt.Fprintf(tw, "%s\t%s\n", name, desc)
	}
	return tw.Flush()
}

func runMCPRemove(args []string) error {
	if len(args) != 1 {
		return errors.New("remove requires exactly one server name")
	}
	name := args[0]
	return mcp.RemoveServer(config.MCPConfig(), name)
}

func runMCPValidate(stdout io.Writer) error {
	cfg, err := loadMCPConfig()
	if err != nil {
		return err
	}
	for name, sc := range cfg.Servers {
		if name == "" {
			return errors.New("server name cannot be empty")
		}
		switch sc.Type {
		case "", "stdio":
			if sc.Command == "" {
				return fmt.Errorf("server %q has empty command", name)
			}
		case "http":
			if sc.URL == "" {
				return fmt.Errorf("server %q has empty url", name)
			}
		default:
			return fmt.Errorf("server %q has unsupported type %q", name, sc.Type)
		}
	}
	fmt.Fprintln(stdout, "mcp config is valid")
	return nil
}

func loadMCPConfig() (mcp.Config, error) {
	return mcp.LoadConfig(config.MCPConfig())
}
