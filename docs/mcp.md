# MCP plugins

rune speaks the [Model Context Protocol](https://modelcontextprotocol.io/) over
stdio. Configure servers in `~/.rune/mcp.json`. Their tools register alongside
rune's built-ins as `<server>:<tool>`.

## Config

```json
{
  "servers": {
    "filesystem": {
      "command": "npx",
      "args": ["-y", "@modelcontextprotocol/server-filesystem", "/Users/me/work"]
    },
    "sqlite": {
      "command": "uvx",
      "args": ["mcp-server-sqlite", "--db-path", "/tmp/db.sqlite"]
    },
    "context7": {
      "type": "http",
      "url": "https://mcp.context7.com/mcp",
      "plan_tools": ["resolve-library-id", "query-docs"]
    }
  }
}
```

Each server is spawned at rune startup. If a server fails to spawn, rune logs
the error to `~/.rune/log` and continues — its tools are simply unavailable.

## Plan Mode access

MCP tools are denied by default in Plan Mode because rune cannot infer whether an
external tool mutates state. Opt in trusted read-only MCP tools with config
metadata:

```json
{
  "servers": {
    "docs": {
      "type": "http",
      "url": "https://example.com/mcp",
      "read_only": true
    },
    "context7": {
      "type": "http",
      "url": "https://mcp.context7.com/mcp",
      "plan_tools": ["resolve-library-id", "query-docs"]
    }
  }
}
```

- `read_only: true` allows all tools from that server while planning.
- `plan_tools` allows only the listed unprefixed MCP tool names while planning.
- With neither field set, tools from the server are hidden and runtime-denied in Plan Mode.

Only use these fields for tools that cannot mutate files, databases, network
state, issues, tickets, or other external resources.

## Lifecycle

- Servers spawn on `rune` startup.
- Servers terminate when rune exits.
- A crash in one server does not affect other servers or rune itself.

## Tool naming

If `filesystem` exposes a tool named `read_file`, rune surfaces it as
`filesystem:read_file`. The model sees the prefixed name in its tool list.

## Per-tool timeout

Default 60s. Override per server:

```json
{
  "servers": {
    "slow_server": {
      "command": "...",
      "timeout_seconds": 180
    }
  }
}
```

(Not all knobs are wired in v1; see source.)
