package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

// Enhanced input component with better visual feedback
var enhancedInputStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#4A5568")).
	Background(lipgloss.Color("#1A202C")).
	Padding(1, 2).
	MarginTop(1).
	MarginBottom(1)

var enhancedInputFocusStyle = lipgloss.NewStyle().
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#6CB6FF")).
	Background(lipgloss.Color("#1A202C")).
	Padding(1, 2).
	MarginTop(1).
	MarginBottom(1)

var placeholderStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#718096")).
	Faint(true)

var cursorStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#6CB6FF")).
	Bold(true)

// Enhanced input with placeholder and better visual feedback
func NewEnhancedTextInput() textinput.Model {
	ti := textinput.New()
	ti.Placeholder = "Enter your message..."
	ti.PlaceholderStyle = placeholderStyle
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#E2E8F0"))
	ti.Cursor.Style = cursorStyle
	ti.Focus()
	return ti
}

// Enhanced textarea with better styling
func NewEnhancedTextArea() textarea.Model {
	ta := textarea.New()
	ta.Placeholder = "Enter your message (multi-line supported)..."
	ta.Cursor.Style = cursorStyle
	ta.Focus()
	return ta
}

// Render enhanced input with focus state
func RenderEnhancedInput(input textinput.Model, focused bool) string {
	var style lipgloss.Style
	if focused {
		style = enhancedInputFocusStyle
	} else {
		style = enhancedInputStyle
	}
	return style.Render(input.View())
}

// Render enhanced textarea with focus state
func RenderEnhancedTextArea(textarea textarea.Model, focused bool) string {
	var style lipgloss.Style
	if focused {
		style = enhancedInputFocusStyle
	} else {
		style = enhancedInputStyle
	}
	return style.Render(textarea.View())
}

// Input suggestion styles
var suggestionStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#6CB6FF")).
	Background(lipgloss.Color("#2D3748")).
	Padding(0, 1).
	MarginRight(1).
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#4A5568"))

var selectedSuggestionStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FFFFFF")).
	Background(lipgloss.Color("#6CB6FF")).
	Bold(true).
	Padding(0, 1).
	MarginRight(1).
	Border(lipgloss.RoundedBorder()).
	BorderForeground(lipgloss.Color("#4A5568"))

// Render input suggestions
func RenderInputSuggestions(suggestions []string, selectedIndex int) string {
	if len(suggestions) == 0 {
		return ""
	}

	var parts []string
	for i, suggestion := range suggestions {
		if i == selectedIndex {
			parts = append(parts, selectedSuggestionStyle.Render(suggestion))
		} else {
			parts = append(parts, suggestionStyle.Render(suggestion))
		}
	}

	return lipgloss.NewStyle().
		MarginTop(1).
		Render(lipgloss.JoinHorizontal(lipgloss.Left, parts...))
}

// Character count style
var charCountStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#718096")).
	Faint(true).
	Align(lipgloss.Right).
	MarginTop(1)

var charCountWarningStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#F56565")).
	Bold(true).
	Align(lipgloss.Right).
	MarginTop(1)

// Render character count
func RenderCharacterCount(current, max int) string {
	count := strings.Builder{}
	count.WriteString(strconv.Itoa(current))
	count.WriteString("/")
	count.WriteString(strconv.Itoa(max))

	style := charCountStyle
	if current > max {
		style = charCountWarningStyle
	}

	return style.Render(count.String())
}

// Input mode indicator
var modeIndicatorStyle = lipgloss.NewStyle().
	Foreground(lipgloss.Color("#FFFFFF")).
	Bold(true).
	Padding(0, 2).
	MarginRight(1)

var chatModeStyle = modeIndicatorStyle.Copy().
	Background(lipgloss.Color("#48BB78"))

var commandModeStyle = modeIndicatorStyle.Copy().
	Background(lipgloss.Color("#ED8936"))

// Render input mode indicator
func RenderModeIndicator(mode string) string {
	switch mode {
	case "chat":
		return chatModeStyle.Render("CHAT")
	case "command":
		return commandModeStyle.Render("CMD")
	default:
		return modeIndicatorStyle.Background(lipgloss.Color("#718096")).Render(strings.ToUpper(mode))
	}
}
