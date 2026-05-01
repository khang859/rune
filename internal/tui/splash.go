package tui

import "github.com/charmbracelet/lipgloss"

const splashWordmark = `██████╗ ██╗   ██╗███╗   ██╗███████╗
██╔══██╗██║   ██║████╗  ██║██╔════╝
██████╔╝██║   ██║██╔██╗ ██║█████╗
██╔══██╗██║   ██║██║╚██╗██║██╔══╝
██║  ██║╚██████╔╝██║ ╚████║███████╗
╚═╝  ╚═╝ ╚═════╝ ╚═╝  ╚═══╝╚══════╝`

const splashTagline = "✦ speak your incantation ✦"

// renderSplash returns a centered fantasy splash for the empty viewport state.
// Returns "" when the area is too small to display the art legibly — the
// viewport then falls back to its natural empty state.
func renderSplash(width, height int, styles Styles, version string) string {
	const minWidth = 38 // wordmark is 35 cols; 3 for breathing room
	const minHeight = 10 // 6 art rows + blank + tagline + version + breathing room
	if width < minWidth || height < minHeight {
		return ""
	}
	art := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")).Render(splashWordmark)
	tag := styles.Info.Render(splashTagline)
	versionLine := styles.Info.Render("rune " + version)
	body := lipgloss.JoinVertical(lipgloss.Center, art, "", tag, versionLine)
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, body)
}
