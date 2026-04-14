package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
)

func (m model) landingInputShellWidth() int {
	return min(72, max(36, m.width/2))
}

func (m model) modeAccentColor() lipgloss.Color {
	if m.mode == modePlan {
		return colorThinking
	}
	return colorAccent
}

func (m model) chatInputContentWidth() int {
	width := m.chatPanelInnerWidth() - m.inputBorderStyle().GetHorizontalFrameSize()
	return max(18, width)
}

func (m model) landingInputContentWidth() int {
	width := m.landingInputShellWidth() - landingInputStyle.GetHorizontalFrameSize()
	return max(24, width)
}

func (m model) inputBorderStyle() lipgloss.Style {
	return inputStyle.BorderForeground(m.modeAccentColor())
}

func (m *model) syncInputStyle() {
	if m.startupGuide.Active {
		m.input.Placeholder = startupGuideInputPlaceholder(m.startupGuide.CurrentField)
	} else {
		m.input.Placeholder = "Ask Bytemind to inspect, change, or verify this workspace..."
	}
	m.input.Prompt = ""
	setInputHeightSafe(&m.input, 2)
}

func setInputHeightSafe(input *textarea.Model, height int) {
	if input == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	input.SetHeight(height)
}

func startupGuideInputHint(field string) string {
	switch strings.TrimSpace(field) {
	case startupFieldType:
		return "Enter provider and press Enter."
	case startupFieldBaseURL:
		return "Enter base_url and press Enter."
	case startupFieldModel:
		return "Enter model and press Enter."
	case startupFieldAPIKey:
		return "Paste API key and press Enter to verify."
	default:
		return "Input value then press Enter."
	}
}

func startupGuideInputPlaceholder(field string) string {
	switch strings.TrimSpace(field) {
	case startupFieldType:
		return "Step 1/4: provider (openai-compatible or anthropic)"
	case startupFieldBaseURL:
		return "Step 2/4: base_url (example: https://api.deepseek.com)"
	case startupFieldModel:
		return "Step 3/4: model (example: deepseek-chat)"
	case startupFieldAPIKey:
		return "Step 4/4: API key"
	default:
		return "Input provider setup value"
	}
}
