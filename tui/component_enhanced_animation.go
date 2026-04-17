package tui

import (
	"time"

	"github.com/charmbracelet/bubbles/spinner"
)

var enhancedThinkingSpinner = spinner.Model{
	Spinner: spinner.Spinner{
		Frames: []string{"-", "\\", "|", "/"},
		FPS:    80 * time.Millisecond,
	},
}

func NewEnhancedThinkingSpinner(_ string) spinner.Model {
	return enhancedThinkingSpinner
}

func RenderEnhancedThinkingText(sp spinner.Model) string {
	return thinkingIndicatorStyle.Render(sp.View() + " thinking")
}

func RenderCompletedThinkingText() string {
	return thinkingDoneBodyStyle.Render("thinking")
}
