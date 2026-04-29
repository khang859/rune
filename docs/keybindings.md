# Keybindings

| Key | Action |
|---|---|
| Enter | Submit message |
| Shift+Enter | Newline |
| Esc | Cancel turn / close modal / close overlay / exit copy mode |
| Ctrl+C | Quit (twice if interactive editing) |
| Ctrl+L | Open `/model` |
| Tab | Path completion (or accept overlay item) |
| Shift+Tab | Toggle copy mode (releases mouse so terminal can select text) |
| ↑ / ↓ | Navigate overlays / modals |
| `@` | Open file picker |
| `/` | Open command menu |
| `!cmd` | Run shell, send output as message |
| `!!cmd` | Run shell, do not send |
| Ctrl+V | Paste image (where supported) |

## Copy mode

`Shift+Tab` (or `/copy-mode`) surrenders mouse capture so your terminal handles
text selection natively. Drag to highlight, copy with your terminal's normal
shortcut (Cmd+C on macOS, Ctrl+Shift+C on most Linux terminals). Press
`Shift+Tab` or `Esc` again to return to normal mode (wheel-scroll resumes).
