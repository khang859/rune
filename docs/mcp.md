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
    }
  }
}
```

Each server is spawned at rune startup. If a server fails to spawn, rune logs
the error to `~/.rune/log` and continues — its tools are simply unavailable.

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
