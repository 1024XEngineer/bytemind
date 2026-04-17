package tui

import (
	"fmt"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

var enhancedScrollbarTrackStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#2A3F5F")).
	Width(2)

var enhancedScrollbarThumbStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#6CB6FF")).
	Width(2)

var enhancedScrollbarThumbHoverStyle = lipgloss.NewStyle().
	Background(lipgloss.Color("#A78BFA")).
	Width(2)

func NewEnhancedViewport(width, height int) viewport.Model {
	v := viewport.New(width, height)
	v.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#2A3F5F")).
		Background(lipgloss.Color("#1A202C"))
	return v
}

func RenderCustomScrollbar(viewportHeight, contentHeight, scrollTop int) string {
	if contentHeight <= viewportHeight {
		return ""
	}

	thumbHeight := max(2, viewportHeight*viewportHeight/contentHeight)
	thumbPosition := scrollTop * (viewportHeight-thumbHeight) / (contentHeight-viewportHeight)

	track := make([]string, viewportHeight)
	for i := range track {
		if i >= thumbPosition && i < thumbPosition+thumbHeight {
			track[i] = enhancedScrollbarThumbStyle.Render("  ")
			continue
		}
		track[i] = enhancedScrollbarTrackStyle.Render("  ")
	}

	return lipgloss.JoinVertical(lipgloss.Left, track...)
}

func NewSmoothViewport(width, height int) viewport.Model {
	return NewEnhancedViewport(width, height)
}

var scrollPositionStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#718096")).
	Faint(true).
	Background(lipgloss.Color("#2A3F5F")).
	Padding(0, 1).
	Align(lipgloss.Center)

func RenderScrollPosition(current, total int) string {
	if total <= 1 {
		return ""
	}
	return scrollPositionStyle.Render(fmt.Sprintf("%d/%d", current+1, total))
}

var scrollHintStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#6CB6FF")).
	Background(lipgloss.Color("#2A3F5F")).
	Padding(0, 1).
	Bold(true).
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#4A5568"))

func RenderScrollHint(direction string) string {
	switch direction {
	case "up", "down", "top", "bottom":
		return scrollHintStyle.Render(direction)
	default:
		return ""
	}
}
