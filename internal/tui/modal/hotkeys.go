// internal/tui/modal/hotkeys.go
package modal

import tea "github.com/charmbracelet/bubbletea"

type Hotkeys struct{}

func NewHotkeys() Modal { return Hotkeys{} }

func (Hotkeys) Init() tea.Cmd { return nil }

func (h Hotkeys) Update(msg tea.Msg) (Modal, tea.Cmd) {
	if _, ok := msg.(tea.KeyMsg); ok {
		return h, Cancel()
	}
	return h, nil
}

func (Hotkeys) View(width, height int) string {
	return `Hotkeys:
  Enter           submit
  Shift+Enter     newline
  Esc             cancel turn / close modal
  Ctrl+C ×2       quit
  Ctrl+L          /model
  Ctrl+T          toggle thinking
  Ctrl+R          toggle tool results
  Tab             path completion
  @               file picker
  /               command menu
  /mcp-status     show MCP connection status
  /git-status     view git status and diffs
  /plan           enter Plan Mode (edits/bash disabled)
  /act            leave Plan Mode
  /approve        approve plan, return to Act Mode
  /cancel-plan    clear pending plan state
  !cmd            run shell, send output (disabled in Plan Mode)
  !!cmd           run shell, do not send (disabled in Plan Mode)

(any key to close)`
}
