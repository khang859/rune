# rune

A minimal terminal coding agent in Go. Inspired by [pi-mono](https://github.com/badlogic/pi-mono).

rune ships with read/write/edit/bash tools, local file search, code indexing and
repo maps, read-only git/GitHub inspection, branching sessions with compaction,
markdown skills, built-in specialist subagents, optional web tools, and MCP
plugin support. It runs against ChatGPT Pro/Plus subscriptions via OAuth, Groq,
Ollama, Runpod, and OpenRouter.

## Quick start

```bash
curl -fsSL https://raw.githubusercontent.com/khang859/rune/main/install.sh | sh
```

## Setup and login

Start rune, then use `/providers` or `/settings` to choose and configure a provider:

```bash
rune
```

Provider setup examples:

```bash
# Codex: opens a browser to auth via your ChatGPT account
rune login codex

# Groq
export GROQ_API_KEY="..."
rune --provider groq

# Ollama
ollama pull llama3.2
rune --provider ollama --model llama3.2

# Runpod
export RUNPOD_API_KEY="..."
rune --provider runpod --model openai/gpt-oss-120b

# OpenRouter
export OPENROUTER_API_KEY="..."
rune --provider openrouter --model anthropic/claude-sonnet-4.5
```

API keys can also be stored with `/settings`.

The installer downloads the latest GitHub release for macOS or Linux and can be
rerun later to update rune. Set `RUNE_INSTALL_DIR` to install somewhere other
than `~/.local/bin`.

If you have Go 1.26+ installed, you can also install or update from source:

```bash
go install github.com/khang859/rune/cmd/rune@latest
```

Linux source builds may require X11 development headers, such as `libx11-dev` on
Debian/Ubuntu.

## Usage

| Mode | How |
|---|---|
| Interactive | `rune` |
| One-shot | `rune --prompt "fix the test in ./foo_test.go"` |
| With attachments | `rune --prompt "summarize @paper.pdf"` |
| Headless smoke | `rune --script script.json` |
| Version | `rune --version` |

## Attachments

Reference files with `@path` in prompts or press `@` / use `/files` in the TUI to
pick files. rune can inline text files and attach supported images and PDFs. See
`docs/file-attachments-and-pdfs.md` and `docs/keybindings.md`.

## Commands

Type `/` in the editor to see all commands. Highlights:

- `/providers` — switch provider, e.g. Codex, Groq, Ollama, Runpod, or OpenRouter
- `/login` — authenticate a provider such as Codex
- `/settings` — configure providers, API keys, web tools, subagents, and UI options
- `/model` — switch model for the active provider
- `/thinking` — toggle thinking display; Ctrl+T also toggles it
- `/tree` — jump to any point in the session
- `/resume` — pick a previous session
- `/new`, `/clear`, `/fork`, `/clone` — manage sessions and branches
- `/compact` — summarize history
- `/plan` — enter safe planning mode; edits and bash are disabled, and MCP tools require a read-only allowlist
- `/approve` — approve a plan and implement it in a new session
- `/cancel-plan` — clear pending plan state while staying in plan mode
- `/skill:<name>` — invoke a markdown skill
- `/skill-creator` — get guided help drafting or improving a rune skill
- `/feature-dev` — start a guided feature-development workflow with specialist subagents
- `/subagents` — list built-in and custom subagents
- `/mcp` — configure MCP servers
- `/mcp-status` — inspect MCP server/tool status
- `/git-status` — show repository status
- `/files` — pick files to attach
- `/copy`, `/copy-mode` — copy messages from the transcript
- `/repomap` — show/toggle the always-on code repo map (status, on, off, budget N)
- `/reload` — reload custom agents and skills
- `/quit` — exit

See `docs/keybindings.md` for the full key map, `docs/plan-mode.md` for Plan Mode details, `docs/feature-dev.md` for the feature workflow, and `/hotkeys` for in-app help.

## Customization

- **Skills** — drop a `.md` file into `~/.rune/skills/` or `./.rune/skills/`.
  See `docs/skills.md`.
- **Subagents** — rune includes built-in specialist subagents and supports custom
  Markdown agent definitions in `~/.rune/agents/` or `./.rune/agents/`.
  See `docs/subagents.md`.
- **MCP plugins** — configure MCP servers with `/mcp`, the `rune mcp` CLI, or
  `~/.rune/mcp.json`. See `docs/mcp.md`.
- **Project context** — rune walks up from the cwd collecting `AGENTS.md`.

### MCP CLI examples

```bash
rune mcp add filesystem -- npx -y @modelcontextprotocol/server-filesystem "$PWD"
rune mcp add-http context7 --url https://mcp.context7.com/mcp
rune mcp list
```

## Providers

Select a provider with `--provider`, `RUNE_PROVIDER`, or `/providers` in the TUI.
Provider-specific models can be selected with `--model`, `/model`, or settings.
See `docs/providers.md`.

- **Codex** — authenticate with `rune login codex` or `/login`.
- **Groq** — set `RUNE_GROQ_API_KEY` or `GROQ_API_KEY`, or store the key with `/settings`.
- **Ollama** — run a local Ollama server and pull a model, e.g. `ollama pull llama3.2`.
  Optional API keys are read from `RUNE_OLLAMA_API_KEY` or `OLLAMA_API_KEY`.
- **Runpod** — set `RUNE_RUNPOD_API_KEY` or `RUNPOD_API_KEY`, or store the key with `/settings`.
- **OpenRouter** — set `RUNE_OPENROUTER_API_KEY` or `OPENROUTER_API_KEY`, or store the key with `/settings`.

## Environment variables

Common user-facing environment variables:

| Variable | Purpose |
|---|---|
| `RUNE_INSTALL_DIR` | Override the installer target directory. |
| `RUNE_DIR` | Override the rune config/data directory. |
| `RUNE_PROVIDER` | Select the active provider. |
| `RUNE_PROVIDER_PROFILE` | Select a saved provider profile. |
| `RUNE_CODEX_MODEL`, `RUNE_GROQ_MODEL`, `RUNE_OLLAMA_MODEL`, `RUNE_RUNPOD_MODEL`, `RUNE_OPENROUTER_MODEL` | Override provider model defaults. |
| `RUNE_OPENROUTER_PROVIDER` | Route OpenRouter requests to a specific provider slug (sets `provider.order`). |
| `RUNE_CODEX_ENDPOINT`, `RUNE_GROQ_ENDPOINT`, `RUNE_OLLAMA_ENDPOINT`, `RUNE_RUNPOD_ENDPOINT`, `RUNE_OPENROUTER_ENDPOINT` | Override provider endpoints. |
| `RUNE_OAUTH_AUTHORIZE_URL`, `RUNE_OAUTH_TOKEN_URL` | Override Codex OAuth endpoints. |
| `RUNE_GROQ_API_KEY`, `GROQ_API_KEY` | Groq API key. |
| `RUNE_OLLAMA_API_KEY`, `OLLAMA_API_KEY` | Ollama API key, if your endpoint requires one. |
| `RUNE_RUNPOD_API_KEY`, `RUNPOD_API_KEY` | Runpod API key. |
| `RUNE_OPENROUTER_API_KEY`, `OPENROUTER_API_KEY` | OpenRouter API key. |
| `RUNE_WEB_SEARCH_PROVIDER` | Select the web search provider, e.g. `brave` or `tavily`. |
| `RUNE_BRAVE_SEARCH_API_KEY` | Brave Search API key. |
| `RUNE_TAVILY_API_KEY` | Tavily API key. |
| `RUNE_WEB_FETCH_ALLOW_PRIVATE` | Allow `web_fetch` to reach private network addresses. |
| `RUNE_ICONS` | Choose icon mode: `auto`, `nerd`, `unicode`, or `ascii`. |

## Web tools

rune can optionally expose web tools to the agent:

- `web_search` — search the web for relevant pages.
- `web_fetch` — fetch a specific HTTP(S) URL.

`web_fetch` is available when enabled in settings. `web_search` requires a configured search provider. For Brave Search or Tavily, set:

```sh
export RUNE_BRAVE_SEARCH_API_KEY="..."
# or
export RUNE_TAVILY_API_KEY="tvly-..."
export RUNE_WEB_SEARCH_PROVIDER="tavily"
```

Or configure it interactively with `/settings` and paste the key into the masked popup. See `docs/web.md` for details and security notes.

## Development

```bash
make all      # vet + fmt check + race tests + build
make test     # race tests
make lint     # staticcheck
make build    # build binary
```

See `docs/architecture.md`.

## License

MIT.
