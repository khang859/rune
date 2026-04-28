# Providers

## Codex (ChatGPT Pro/Plus)

The only built-in provider in v1.

### Login

```bash
rune login codex
```

Opens your default browser to OpenAI's auth page. After approval, the local
callback at `http://localhost:1455/auth/callback` exchanges the code for an
access + refresh token. Tokens are stored in `~/.rune/auth.json` (chmod 0600).

### Refresh

rune refreshes the access token automatically when it has less than 5 minutes
remaining. Concurrent rune processes coordinate via a file lock on
`~/.rune/auth.json.lock`, so a single refresh applies to both.

### Models

- `gpt-5` (default)
- `gpt-5-codex`
- `gpt-5.1-codex-mini`

Switch with `/model` or Ctrl+L.

### Reasoning effort

`/settings` exposes `minimal` / `low` / `medium` / `high`. Higher = more
thinking, more cost-equivalent (subscription is unmetered, but slower).

### Troubleshooting

| Symptom | Fix |
|---|---|
| `not logged in` | `rune login codex` |
| `login expired, run /login` | `rune login codex` (refresh token revoked) |
| `429` | rune retries automatically (3 attempts, exp backoff) |
| `context_length_exceeded` | rune auto-compacts and retries once |

### Env overrides (testing)

- `RUNE_CODEX_ENDPOINT` — override the Responses URL.
- `RUNE_OAUTH_TOKEN_URL` — override the token endpoint.
- `RUNE_DIR` — override `~/.rune`.
