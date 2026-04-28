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
  Tab             path completion
  @               file picker
  /               command menu
  !cmd            run shell, send output
  !!cmd           run shell, do not send

(any key to close)`
}
