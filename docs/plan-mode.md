# Plan Mode

Plan Mode is an interactive safety mode for planning before implementation.

Enter it with `/plan`. While active, rune adds stronger planning instructions to the agent and enforces tool permissions in code:

- `write`, `edit`, and `bash` are hidden from the model and denied at runtime.
- MCP/external tools are denied by default.
- Read-only tools such as `read`, `web_search`, and `web_fetch` remain available when configured.
- Subagents remain available to the main agent for read-only exploration; child subagents cannot recursively spawn subagents.
- Shell shortcuts (`!cmd` and `!!cmd`) are blocked in the TUI.

Plan Mode does not automatically implement after approval. Use `/approve` to return to Act Mode, then send a follow-up instruction such as “go ahead”.

## Commands

- `/plan` — enter Plan Mode.
- `/act` — leave Plan Mode and enable implementation tools.
- `/approve` — approve the current plan, leave Plan Mode, and wait for your next message.
- `/cancel-plan` — clear pending plan approval state while staying in Plan Mode.

The footer shows `plan` while Plan Mode is active.
