package tui

import (
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/lipgloss"
)

const landingInputPlaceholder = "Ask anything, or type / for commands..."

func (m model) landingInputShellWidth() int {
	if m.width <= 0 {
		return 52
	}
	maxFit := max(1, m.width-16)
	preferred := max(landingStableHeroWidth(), (m.width*2)/3)
	maxPreferred := max(104, landingStableHeroWidth())
	if maxFit < 52 {
		return maxFit
	}
	return clamp(min(preferred, maxPreferred), 52, maxFit)
}

func (m model) modeAccentColor() lipgloss.Color {
	if m.screen == screenLanding {
		if m.mode == modePlan {
			return lipgloss.Color(landingPlanAccent)
		}
		return lipgloss.Color(landingBuildAccent)
	}
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
	return max(1, width)
}

func (m model) inputBorderStyle() lipgloss.Style {
	return inputStyle.BorderForeground(m.modeAccentColor())
}

func (m *model) syncInputStyle() {
	m.applyInputThemeForScreen()
	if m.startupGuide.Active {
		m.input.Placeholder = startupGuideInputPlaceholder(m.startupGuide.CurrentField)
	} else if m.screen == screenLanding {
		m.input.Placeholder = landingInputPlaceholder
	} else {
		m.input.Placeholder = "Ask Bytemind to inspect, change, or verify this workspace..."
	}
	m.input.Prompt = ""
	setInputHeightSafe(&m.input, 2)
}

func (m *model) applyInputThemeForScreen() {
	if m == nil {
		return
	}
	if m.screen == screenLanding {
		m.applyLandingInputTheme()
		return
	}
	m.applyDefaultInputTheme()
}

func (m *model) applyDefaultInputTheme() {
	if m == nil {
		return
	}
	focused, blurred := textarea.DefaultStyles()
	m.input.FocusedStyle = focused
	m.input.BlurredStyle = blurred
	m.input.Cursor.Style = textarea.New().Cursor.Style
	_ = m.input.Cursor.SetMode(cursor.CursorBlink)
}

func (m *model) applyLandingInputTheme() {
	if m == nil {
		return
	}
	bg := lipgloss.Color("#020A14")
	text := lipgloss.Color("#E2F1FF")
	placeholder := lipgloss.Color("#6C8FB7")
	cursorLine := bg
	cursorBg := lipgloss.Color("#FFFFFF")
	cursorFg := lipgloss.Color("#000000")
	prompt := lipgloss.Color("#4CB7FF")

	focused, blurred := textarea.DefaultStyles()
	for _, style := range []*textarea.Style{&focused, &blurred} {
		style.Base = style.Base.Background(bg).Foreground(text)
		style.CursorLine = style.CursorLine.Background(cursorLine).Foreground(text)
		style.CursorLineNumber = style.CursorLineNumber.Background(cursorLine).Foreground(text)
		style.EndOfBuffer = style.EndOfBuffer.Background(bg).Foreground(text)
		style.LineNumber = style.LineNumber.Background(bg).Foreground(text)
		style.Placeholder = style.Placeholder.Background(bg).Foreground(placeholder)
		style.Prompt = style.Prompt.Background(bg).Foreground(prompt)
		style.Text = style.Text.Background(bg).Foreground(text)
	}
	m.input.FocusedStyle = focused
	m.input.BlurredStyle = blurred
	m.input.Cursor.Style = m.input.Cursor.Style.Background(cursorBg).Foreground(cursorFg).Bold(true)
	_ = m.input.Cursor.SetMode(cursor.CursorHide)
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
