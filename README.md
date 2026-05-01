# rune

A minimal terminal coding agent in Go. Inspired by [pi-mono](https://github.com/badlogic/pi-mono).

rune ships with read/write/edit/bash tools, branching sessions with compaction,
markdown skills, and MCP plugin support. It runs against ChatGPT Pro/Plus
subscriptions via OAuth, and Groq via API key.

## Quick start

```bash
curl -fsSL https://raw.githubusercontent.com/khang859/rune/main/install.sh | sh

rune                      # interactive mode
# then use /providers or /settings to choose/configure a provider
# Codex: rune login codex  # opens a browser to auth via your ChatGPT account
# Groq: export GROQ_API_KEY=... && rune --provider groq
# Ollama: ollama pull llama3.2 && rune --provider ollama --model llama3.2
```

The installer downloads the latest GitHub release for macOS or Linux and can be
rerun later to update rune.

If you have Go installed, you can also install or update from source:

```bash
go install github.com/khang859/rune/cmd/rune@latest
```

## Usage

| Mode | How |
|---|---|
| Interactive | `rune` |
| One-shot | `rune --prompt "fix the test in ./foo_test.go"` |
| Headless smoke | `rune --script script.json` |
| Version | `rune --version` |

## Commands

Type `/` in the editor to see all commands. Highlights:

- `/providers` — switch provider, e.g. Codex or Groq
- `/model` — switch model for the active provider
- `/tree` — jump to any point in the session
- `/resume` — pick a previous session
- `/compact` — summarize history
- `/plan` — enter safe planning mode; edits and bash are disabled, and MCP tools require a read-only allowlist
- `/act` — leave planning mode and enable implementation tools
- `/approve` — approve a plan and return to act mode without auto-implementing
- `/cancel-plan` — clear pending plan state while staying in plan mode
- `/skill:<name>` — invoke a markdown skill
- `/skill-creator` — get guided help drafting or improving a rune skill
- `/quit` — exit

See `docs/keybindings.md` for the full key map, `docs/plan-mode.md` for Plan Mode details, and `/hotkeys` for in-app help.

## Customization

- **Skills** — drop a `.md` file into `~/.rune/skills/` or `./.rune/skills/`.
  See `docs/skills.md`.
- **Custom subagents** — drop a `.md` file into `~/.rune/agents/` or `./.rune/agents/`.
  See `docs/subagents.md`.
- **MCP plugins** — configure `~/.rune/mcp.json`. See `docs/mcp.md`.
- **Project context** — rune walks up from the cwd collecting `AGENTS.md`.

## Providers

Codex remains the default provider. Groq can be selected with `--provider groq`, `RUNE_PROVIDER=groq`, or `/providers` in the TUI. Ollama can be selected with `--provider ollama` after pulling a local model, e.g. `ollama pull llama3.2`. Groq keys are read from `RUNE_GROQ_API_KEY`, `GROQ_API_KEY`, or can be stored via `/settings`. See `docs/providers.md`.

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
make all      # vet + fmt + test + build
make test     # tests only
make build    # build binary
```

See `docs/architecture.md`.

## License

MIT.
