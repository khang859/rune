# Keybindings

| Key | Action |
|---|---|
| Enter | Submit message |
| Shift+Enter | Newline |
| Esc | Cancel turn / close modal / close overlay / exit copy mode |
| Ctrl+C | Quit (twice if interactive editing) |
| Ctrl+L | Open `/model` |
| Tab | Path completion (or accept overlay item) |
| Shift+Tab | Cycle normal → Plan Mode → copy mode |
| ↑ / ↓ | Navigate overlays / modals |
| `@` | Open inline file picker |
| `/` | Open command menu |
| `/files` | Open fuzzy file picker with preview |
| `!cmd` | Run shell, send output as message; disabled in Plan Mode |
| `!!cmd` | Run shell, do not send; disabled in Plan Mode |
| Ctrl+V | Paste image (where supported) |

## `/files` picker

| Key | Action |
|---|---|
| Type | Fuzzy-filter project files |
| ↑ / ↓ | Move selection |
| PgUp / PgDown | Move by 10 results |
| Home / End | Jump to first/last result |
| Enter | Insert `@path` into the prompt |
| Ctrl+A | Attach selected image |
| Space | Open selected image in the OS image viewer |
| Ctrl+U | Clear search |
| Esc | Close picker |

The preview pane shows text snippets for text files and stable ANSI thumbnails for images. Press `Space` on an image to open it with the OS image viewer. On WSL, Rune uses `wslpath` + `explorer.exe` so Windows Terminal/ConPTY Sixel limitations do not affect image viewing.

## Plan Mode

Use `/plan` or cycle with `Shift+Tab` to enter Plan Mode. While active, write/edit/bash and shell shortcuts are disabled; the read-only `gh` tool remains available for GitHub inspection. MCP tools are available only when marked read-only or allowlisted with `read_only` / `plan_tools` in MCP config. Use `/approve` to implement the latest plan in a new session, or continue cycling with `Shift+Tab` to leave Plan Mode without approving. See `docs/plan-mode.md`.

## Copy mode

`Shift+Tab` cycles normal → Plan Mode → copy mode → normal. Copy mode (also
available with `/copy-mode`) surrenders mouse capture so your terminal handles
text selection natively. Drag to highlight, copy with your terminal's normal
shortcut (Cmd+C on macOS, Ctrl+Shift+C on most Linux terminals). Press
`Shift+Tab` to continue cycling to normal mode, or `Esc` to exit copy mode
(wheel-scroll resumes).
