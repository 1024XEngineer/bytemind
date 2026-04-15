package tui

import "github.com/charmbracelet/lipgloss"

func renderModal(width, height int, modal string) string {
	if width == 0 || height == 0 {
		return modal
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}
