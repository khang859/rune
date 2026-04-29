# Web tools implementation plan

This document captures the planned implementation for adding web access to rune.

## Goals

Add first-class web tools that let the agent find and inspect current external information without guessing URLs.

Planned tools:

- `web_search` — search the web and return ranked results with titles, URLs, and snippets.
- `web_fetch` — fetch a specific HTTP(S) URL and return response metadata plus body text.

The expected agent workflow is:

1. Use `web_search` when it needs to discover relevant web pages.
2. Use `web_fetch` only for URLs returned by search results or URLs explicitly provided by the user.
3. Do not invent or guess URLs.
4. Cite source URLs when relying on web information.

## Non-goals for v1

- Full browser automation.
- JavaScript rendering.
- Search-engine scraping.
- POST requests or authenticated arbitrary web requests.
- Storing API keys in normal settings files.
- Mandatory web access; search should be optional/configured.

## Tool design

### `web_search`

Purpose: discover relevant pages before fetching.

Initial schema:

```json
{
  "query": "string",
  "limit": 5
}
```

Behavior:

- `query` is required.
- `limit` defaults to `5`.
- `limit` is capped at `10`.
- Tool is only registered when web search is enabled and a provider is configured.
- Search errors return `tools.Result{IsError: true}` with a concise message.
- Search results should be formatted as plain text for the model.

Example output:

```text
Search results for: "Go net/http client timeout best practices"

1. net/http package - Go Packages
   URL: https://pkg.go.dev/net/http
   Snippet: Package http provides HTTP client and server implementations...

2. The complete guide to Go net/http timeouts
   URL: https://blog.cloudflare.com/the-complete-guide-to-golang-net-http-timeouts/
   Snippet: Timeouts are important for clients and servers...
```

### `web_fetch`

Purpose: fetch one concrete URL.

Initial schema:

```json
{
  "url": "https://example.com",
  "headers": {
    "Accept": "text/html"
  },
  "max_bytes": 200000
}
```

Behavior:

- `url` is required.
- Only `http` and `https` URLs are allowed.
- Default max body size: `200000` bytes.
- Hard max body size: `2000000` bytes.
- Default timeout: around `15s`.
- Redirects are allowed but capped, e.g. 10 redirects.
- Large responses are truncated with a footer.
- Obvious binary responses are not dumped into context; return metadata and an omitted-body message.
- HTTP statuses like `404` should generally not be Go/runtime errors. Return the status and body with `IsError: false` unless the request itself failed.

Example output:

```text
URL: https://example.com/
Status: 200 OK
Content-Type: text/html; charset=utf-8
Content-Length: 1256

<!doctype html>
<html>
...
```

Truncation footer:

```text
[truncated after 200000 bytes. Re-run web_fetch with a smaller target or higher max_bytes up to 2000000.]
```

## Search providers

Add a small provider abstraction, either under `internal/search` or initially under `internal/tools`.

Recommended interface:

```go
type Result struct {
    Title   string
    URL     string
    Snippet string
}

type Provider interface {
    Search(ctx context.Context, query string, limit int) ([]Result, error)
}
```

### v1 provider

Start with Brave Search API.

Environment variable:

```sh
RUNE_BRAVE_SEARCH_API_KEY=...
```

Optional provider selector:

```sh
RUNE_WEB_SEARCH_PROVIDER=brave
```

### Future provider

Support SearXNG for self-hosted users:

```sh
RUNE_WEB_SEARCH_PROVIDER=searxng
RUNE_SEARXNG_URL=https://search.example.com
```

Provider resolution should support:

- `auto` — choose Brave if a Brave key is configured, otherwise SearXNG if configured, otherwise disabled.
- `brave` — require Brave configuration.
- `searxng` — require SearXNG configuration.

## Configuration

### Environment variables

Document these variables:

| Variable | Purpose | Required |
|---|---|---|
| `RUNE_BRAVE_SEARCH_API_KEY` | Brave Search API key. Enables Brave-backed `web_search`. | For Brave search |
| `RUNE_WEB_SEARCH_PROVIDER` | Search provider selector: `auto`, `brave`, `searxng`. | Optional |
| `RUNE_SEARXNG_URL` | SearXNG instance base URL. | For SearXNG |
| `RUNE_WEB_FETCH_ALLOW_PRIVATE` | Allows fetching private/local network URLs. | Optional; default off if private blocking is implemented |

### Persistent settings

Add a settings file:

```text
~/.rune/settings.json
```

Add path helper:

```go
func SettingsPath() string { return filepath.Join(RuneDir(), "settings.json") }
```

Suggested normalized settings shape:

```json
{
  "reasoning_effort": "medium",
  "icon_mode": "unicode",
  "activity_mode": "arcane",
  "web": {
    "fetch_enabled": true,
    "fetch_allow_private": false,
    "search_enabled": "auto",
    "search_provider": "auto"
  }
}
```

`search_enabled` should support:

- `auto` — enable if provider config is available.
- `off` — do not register `web_search`.
- `on` — try to enable and prompt/show guidance if config is missing.

Suggested precedence:

1. `/settings` runtime changes.
2. `~/.rune/settings.json`.
3. Environment variables.
4. Defaults.

For secrets, environment variables should still override stored secrets.

## Secret storage

Do not store API keys in `settings.json`.

Add a separate local secrets file:

```text
~/.rune/secrets.json
```

Add path helper:

```go
func SecretsPath() string { return filepath.Join(RuneDir(), "secrets.json") }
```

Suggested shape:

```json
{
  "brave_search_api_key": "..."
}
```

Requirements:

- File permissions should be `0600`.
- Missing file means no secrets configured.
- Writes should be atomic where practical.
- Never store secret values in sessions.
- Never print secret values in errors, logs, tool output, or UI messages.
- Environment variable `RUNE_BRAVE_SEARCH_API_KEY` takes precedence over stored secret.

Suggested API:

```go
type Secrets struct {
    BraveSearchAPIKey string `json:"brave_search_api_key,omitempty"`
}

type SecretStore struct {
    path string
}

func NewSecretStore(path string) *SecretStore
func (s *SecretStore) Load() (Secrets, error)
func (s *SecretStore) Save(Secrets) error
func (s *SecretStore) BraveSearchAPIKey() (string, error)
func (s *SecretStore) SetBraveSearchAPIKey(key string) error
func (s *SecretStore) DeleteBraveSearchAPIKey() error
```

## API key normalization and validation

All API key inputs are untrusted and should be sanitized before saving or use.

Apply normalization/validation to keys from:

- `/settings` secret popup.
- `~/.rune/secrets.json`.
- `RUNE_BRAVE_SEARCH_API_KEY`.
- Brave provider construction.

### Normalization

Support common paste formats:

```text
BSAxxxxxxxx
"BSAxxxxxxxx"
'BSAxxxxxxxx'
export RUNE_BRAVE_SEARCH_API_KEY="BSAxxxxxxxx"
RUNE_BRAVE_SEARCH_API_KEY=BSAxxxxxxxx
```

Suggested helper:

```go
func NormalizeBraveAPIKeyInput(raw string) string {
    s := strings.TrimSpace(raw)

    for _, prefix := range []string{
        "export RUNE_BRAVE_SEARCH_API_KEY=",
        "RUNE_BRAVE_SEARCH_API_KEY=",
    } {
        if strings.HasPrefix(s, prefix) {
            s = strings.TrimSpace(strings.TrimPrefix(s, prefix))
            break
        }
    }

    if len(s) >= 2 {
        first := s[0]
        last := s[len(s)-1]
        if (first == '"' && last == '"') || (first == '\'' && last == '\'') {
            s = s[1 : len(s)-1]
        }
    }

    return strings.TrimSpace(s)
}
```

Do not evaluate shell syntax. Only extract simple assignment forms.

### Validation

Suggested rules:

```go
func ValidateBraveAPIKey(key string) error {
    switch {
    case key == "":
        return errors.New("empty")
    case len(key) < 20:
        return errors.New("too short")
    case len(key) > 512:
        return errors.New("too long")
    case strings.ContainsAny(key, " \t\r\n"):
        return errors.New("contains whitespace")
    case strings.ContainsAny(key, "<>{}[]()"):
        return errors.New("contains unexpected characters")
    }
    return nil
}
```

Validation should catch obvious mistakes while avoiding overly strict assumptions about Brave key format.

If validation fails:

- Do not save the key.
- Do not overwrite an existing valid stored key.
- Show a sanitized error message, e.g. `invalid Brave Search API key: contains whitespace`.
- Never echo the actual key.

## `/settings` integration

Current settings modal supports enum rows for:

- thinking effort
- icon mode
- activity indicator

Extend it with web settings:

```text
web fetch: on/off
fetch private urls: off/on
web search: auto/off/on
search provider: auto/brave/searxng
brave api key: missing — Enter to set
```

If the key exists:

```text
brave api key: configured — Enter to replace
```

Optional future behavior:

```text
brave api key: configured — Enter to replace, Backspace to delete
```

### Modal row model

The current row model assumes all rows are cycling enums:

```go
type settingsRow struct {
    label   string
    options []string
    value   int
}
```

Extend this to support action/status rows:

```go
type settingsRowKind int

const (
    settingsRowEnum settingsRowKind = iota
    settingsRowAction
    settingsRowStatus
)

type settingsRow struct {
    kind    settingsRowKind
    label   string
    options []string
    value   int
    action  string
}
```

On Enter:

- enum rows apply settings as today.
- action rows return a settings action result.

Suggested action result:

```go
type SettingsAction struct {
    Action   string
    Settings Settings
}
```

### Secret input popup

Add a masked input modal, e.g.:

```text
internal/tui/modal/secret_input.go
```

Requirements:

- Accept pasted input.
- Mask the displayed value.
- Enter submits.
- Esc cancels.
- Ctrl+U clears input if easy.
- Does not render the secret in `View`.
- Returns a typed result carrying the secret value to root handling.

Potential implementation can use `bubbles/textinput` with password echo mode:

```go
ti.EchoMode = textinput.EchoPassword
ti.EchoCharacter = '•'
```

### First-run setup flow

When a user enables Brave search from `/settings` and no key is configured:

1. User opens `/settings`.
2. User sets:

   ```text
   web search: on
   search provider: brave
   ```

3. User applies settings or selects `brave api key` action row.
4. rune opens the Brave API key popup.
5. User pastes key.
6. rune normalizes and validates key.
7. If valid, rune stores it in `~/.rune/secrets.json` with `0600` permissions.
8. rune registers `web_search` for the next turn.
9. rune shows:

   ```text
   (saved Brave Search API key; web_search enabled)
   ```

If canceled:

```text
(web_search not enabled: Brave Search API key missing)
```

If invalid:

```text
invalid Brave Search API key: contains whitespace
```

Keep the popup open with the current input, or clear and let the user retry. Prefer keeping it open with the error visible if straightforward.

## Tool registration

Built-in tools are currently registered manually in several command modes. Add a helper to avoid repeating registration.

Suggested API:

```go
type BuiltinOptions struct {
    WebFetchEnabled      bool
    WebFetchAllowPrivate bool
    SearchProvider       search.Provider
}

func RegisterBuiltins(r *Registry, opts BuiltinOptions) {
    r.Register(Read{})
    r.Register(Write{})
    r.Register(Edit{})
    r.Register(Bash{})

    if opts.WebFetchEnabled {
        r.Register(WebFetch{AllowPrivate: opts.WebFetchAllowPrivate})
    }
    if opts.SearchProvider != nil {
        r.Register(WebSearch{Provider: opts.SearchProvider})
    }
}
```

Update these call sites:

- `cmd/rune/interactive.go`
- `cmd/rune/prompt.go`
- `cmd/rune/script.go`

### Dynamic reconfiguration

Because the agent reads `a.tools.Specs()` on each turn, settings changes can affect the next turn if the registry is updated.

Add registry helpers:

```go
func (r *Registry) Unregister(name string) { delete(r.tools, name) }
func (r *Registry) Has(name string) bool { _, ok := r.tools[name]; return ok }
```

On `/settings` apply:

- Save settings.
- Re-resolve search provider.
- Register or unregister `web_fetch`.
- Register or unregister `web_search`.
- Show a helpful message if web search is enabled but provider config is missing.

## Security

### `web_fetch`

Recommended v1 protections:

- Allow only `http` and `https`.
- Do not send browser cookies or credentials.
- Reject sensitive user-supplied headers:
  - `Authorization`
  - `Cookie`
  - `Proxy-Authorization`
- Consider blocking private/local network destinations by default:
  - `localhost`
  - `127.0.0.0/8`
  - `::1`
  - RFC1918 private ranges
  - link-local addresses
  - cloud metadata address `169.254.169.254`
- Allow private/local fetch only if enabled by `/settings` or `RUNE_WEB_FETCH_ALLOW_PRIVATE=1`.

### Secrets

- Never include API keys in session history.
- Never include API keys in tool results.
- Never echo API keys in modal views.
- Never log API keys.
- Use sanitized error messages.
- Store secrets with `0600` permissions.

## System prompt update

Add web behavior guidance to the base/system prompt:

```text
For current or unknown web information, use web_search first to discover relevant sources, then use web_fetch only on search results or URLs explicitly provided by the user. Do not guess URLs. Cite source URLs when relying on web information.
```

Add/update tests if prompt content is asserted.

## Documentation updates

### README

Add a short section:

```md
## Web tools

rune can optionally expose web tools to the agent:

- `web_search` — search the web for relevant pages.
- `web_fetch` — fetch a specific HTTP(S) URL.

`web_fetch` is available when enabled in settings. `web_search` requires a configured search provider.

For Brave Search, set:

```sh
export RUNE_BRAVE_SEARCH_API_KEY="..."
```

Or configure it interactively with `/settings` and paste the key into the popup.

See `docs/web.md` for details and security notes.
```

### `docs/web.md`

Add user-facing documentation covering:

- `web_search` and `web_fetch` schemas.
- Search-first, fetch-second workflow.
- Brave env setup.
- Interactive `/settings` setup.
- Stored secrets path and permissions.
- SearXNG setup if included.
- Private/local fetch behavior.
- Env var table.

### `docs/architecture.md`

Update the architecture overview to mention web tools/search providers, for example:

```text
internal/tools   built-in tool implementations + Registry
internal/search  web search providers, e.g. Brave/SearXNG
```

## Tests

### Tool tests

`internal/tools/web_fetch_test.go`:

- Fetches text response via `httptest`.
- Rejects invalid JSON args.
- Rejects missing/invalid URL.
- Rejects unsupported scheme.
- Truncates large body.
- Honors `max_bytes`.
- Rejects `max_bytes` above hard cap.
- Returns non-2xx status metadata without Go error.
- Handles timeout/cancel.
- Handles redirect limit.
- Blocks/rejects sensitive headers.
- Blocks private/local addresses if that behavior is implemented.

`internal/tools/web_search_test.go`:

- Validates args.
- Applies default/capped limit.
- Formats search results.
- Converts provider errors to `IsError: true`.

### Search/provider tests

- Brave request shape.
- Brave response parsing.
- HTTP error handling.
- Timeout/cancel behavior.
- Provider resolver behavior for `auto`, `brave`, and `searxng`.
- Missing/invalid key handling.

### Secret store tests

- Missing secrets file returns empty secrets.
- Save/load roundtrip.
- File permissions are `0600`.
- Whitespace and shell-style pasted key normalization.
- Invalid key rejected.
- Failed save does not overwrite existing valid key.
- Deleting key works.
- Env var takes precedence over stored key.

### Key normalization/validation tests

Cases:

```go
" key\n"                                  -> "key"
"\"key\""                              -> "key"
"'key'"                                -> "key"
"export RUNE_BRAVE_SEARCH_API_KEY=key" -> "key"
"RUNE_BRAVE_SEARCH_API_KEY='key'"      -> "key"
```

Validation rejects:

- Empty value.
- Too short value.
- Internal whitespace.
- Multiline input.
- Extremely long value.
- Unexpected bracket/script-like characters.

### Modal tests

`internal/tui/modal/settings_test.go`:

- View shows new web rows.
- Cycling web rows works.
- Enter on Brave API key action row returns expected action.
- Existing settings rows still work.

Secret input modal tests:

- Typed/pasted value is accepted.
- `View` masks input and does not include raw secret.
- Enter returns secret value.
- Esc cancels.
- Optional clear shortcut works.

### Root/TUI tests

- `/settings` opens with web settings.
- Applying settings updates reasoning/icon/activity as before.
- Applying settings toggles `web_fetch` registration.
- Applying settings registers `web_search` when provider resolves.
- Missing Brave key opens secret popup or shows guidance.
- Saving key from popup registers `web_search`.
- Canceling key popup leaves search disabled if no key exists.
- Env key avoids popup.
- Stored key avoids popup.

## Suggested implementation phases

### Phase 1: infrastructure

- Add `settings.json` load/save.
- Add `secrets.json` store.
- Add key normalization/validation.
- Add tests for config and secrets.

### Phase 2: tools

- Add `web_fetch`.
- Add search provider interface.
- Add Brave provider.
- Add `web_search`.
- Add tool tests.

### Phase 3: registration/config

- Add `tools.RegisterBuiltins`.
- Replace manual registration in interactive/prompt/script modes.
- Add registry `Unregister`/`Has`.
- Add provider resolver.
- Wire env/settings/secrets into registration.

### Phase 4: TUI settings UX

- Extend `/settings` rows.
- Add masked secret input popup.
- Save settings from TUI.
- Save Brave key from popup.
- Reconfigure web tools after settings apply.

### Phase 5: docs and prompt

- Add `docs/web.md`.
- Update README.
- Update `docs/architecture.md`.
- Add system prompt guidance and tests.

### Phase 6: optional enhancements

- Add SearXNG provider.
- Add validation request for Brave key.
- Add delete/replace secret actions.
- Add HTML-to-text extraction.
- Add `HEAD` support for `web_fetch`.
- Add OS keychain integration.
