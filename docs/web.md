# Web tools

rune can optionally expose web access to the agent.

## Tools

- `web_search` — searches the web and returns ranked results with titles, URLs, and snippets.
- `web_fetch` — fetches one concrete HTTP(S) URL and returns response metadata plus text body.

The expected workflow is search first, fetch second: use `web_search` to discover sources, then `web_fetch` only for URLs returned by search or URLs explicitly provided by the user. Answers that rely on web information should cite source URLs.

## Configuration

Environment variables:

| Variable | Purpose |
|---|---|
| `RUNE_BRAVE_SEARCH_API_KEY` | Brave Search API key. Overrides stored secret. |
| `RUNE_WEB_SEARCH_PROVIDER` | Search provider selector: `auto`, `brave`, `searxng`. |
| `RUNE_SEARXNG_URL` | Reserved for future SearXNG support. |
| `RUNE_WEB_FETCH_ALLOW_PRIVATE` | Allows fetching private/local network URLs when set to `1`, `true`, `yes`, or `on`. |

Persistent settings are stored in `~/.rune/settings.json`. Secrets are stored separately in `~/.rune/secrets.json` with `0600` permissions. API keys are not stored in sessions.

## Brave Search

Set an environment variable:

```sh
export RUNE_BRAVE_SEARCH_API_KEY="..."
```

Or open `/settings`, set web search/provider options, select `brave api key`, and paste the key into the masked prompt.

## Security notes

`web_fetch` only allows `http` and `https`, rejects sensitive headers such as `Authorization` and `Cookie`, caps redirects and response size, and blocks private/local network destinations by default. Enable private/local fetching only when you trust the current task.
