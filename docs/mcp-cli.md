# MCP CLI management

## Status

Proposed feature.

## Summary

Add first-class CLI commands for managing MCP servers in rune, so users do not
have to manually edit `~/.rune/mcp.json`.

Today MCP servers are configured by writing JSON directly to:

```bash
~/.rune/mcp.json
```

This works, but it is inconvenient for common tasks like adding, listing, and
removing servers. rune should provide a small CLI interface for these workflows.

## Proposed commands

### Add a server

```bash
rune mcp add <name> -- <command> [args...]
```

Example:

```bash
rune mcp add filesystem -- npx -y @modelcontextprotocol/server-filesystem /Users/khangnguyen/Development
```

This should create or update `~/.rune/mcp.json` with:

```json
{
  "servers": {
    "filesystem": {
      "command": "npx",
      "args": [
        "-y",
        "@modelcontextprotocol/server-filesystem",
        "/Users/khangnguyen/Development"
      ]
    }
  }
}
```

If the config file does not exist, rune should create it. If the server name
already exists, rune should either prompt before replacing it or require an
explicit overwrite flag.

Possible overwrite form:

```bash
rune mcp add filesystem --force -- npx -y @modelcontextprotocol/server-filesystem /tmp/work
```

### List servers

```bash
rune mcp list
```

Example output:

```text
filesystem  npx -y @modelcontextprotocol/server-filesystem /Users/khangnguyen/Development
sqlite      uvx mcp-server-sqlite --db-path /tmp/db.sqlite
```

This should read from `~/.rune/mcp.json` and print configured servers.

### Remove a server

```bash
rune mcp remove <name>
```

Example:

```bash
rune mcp remove filesystem
```

This should remove the named server from `~/.rune/mcp.json`.

### Show config path

Optional convenience command:

```bash
rune mcp path
```

Example output:

```text
/Users/khangnguyen/.rune/mcp.json
```

### Validate config

Optional command:

```bash
rune mcp validate
```

This should parse `~/.rune/mcp.json` and report whether the config is valid.
It does not need to spawn servers initially, but a later version could support a
health check.

## Desired behavior

- Preserve existing MCP config when adding or removing one server.
- Create `~/.rune/` and `~/.rune/mcp.json` if needed.
- Produce clear errors for malformed JSON.
- Avoid silently overwriting existing server entries.
- Keep the config compatible with the existing MCP manager.
- Continue supporting manual edits to `~/.rune/mcp.json`.

## Initial scope

Minimum useful version:

```bash
rune mcp add <name> -- <command> [args...]
rune mcp list
rune mcp remove <name>
```

Nice-to-have later:

```bash
rune mcp path
rune mcp validate
rune mcp add --timeout-seconds <seconds> <name> -- <command> [args...]
```
