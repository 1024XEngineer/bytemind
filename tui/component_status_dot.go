package tui

import (
	"math"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
)

// statusDotState represents the display state of a status dot.
type statusDotState int

const (
	dotHidden  statusDotState = iota // no dot shown
	dotRunning                       // dim theme color, blinking (tool executing)
	dotSuccess                       // green, solid (tool done)
	dotError                         // red, solid (tool failed)
	dotPending                       // dim theme color, solid (queued)
	dotText                          // theme text color, solid (assistant text — static marker)
)

const (
	dotBlinkHalfPeriodMs  = 300 // half of 600ms period → ~1.7Hz
	stagnationCheckMs     = 100 // check stagnation every 100ms
	stagnationThreshold   = 3 * time.Second
	stagnationTransition  = 2 * time.Second
	dotChar               = "●"
	dotWidth              = 2
)

type statusDotTickMsg struct{}
type stagnationTickMsg struct{}

func statusDotTickCmd() tea.Cmd {
	return tea.Tick(time.Duration(dotBlinkHalfPeriodMs)*time.Millisecond, func(time.Time) tea.Msg {
		return statusDotTickMsg{}
	})
}

func stagnationTickCmd() tea.Cmd {
	return tea.Tick(time.Duration(stagnationCheckMs)*time.Millisecond, func(time.Time) tea.Msg {
		return stagnationTickMsg{}
	})
}

// resolveStatusDotState determines the dot state for a chat entry.
// Returns the state and whether a dot should be shown at all.
func (m model) resolveStatusDotState(item chatEntry) (statusDotState, bool) {
	switch item.Kind {
	case "tool":
		switch strings.ToLower(strings.TrimSpace(item.Status)) {
		case "running", "active":
			return dotRunning, true
		case "done", "success":
			return dotSuccess, true
		case "error", "failed", "warn", "warning":
			return dotError, true
		default:
			return dotPending, true
		}
	case "assistant":
		switch strings.ToLower(strings.TrimSpace(item.Status)) {
		case "streaming", "thinking", "final", "thinking_done":
			return dotText, true
		case "error":
			return dotError, true
		case "pending":
			return dotPending, true
		default:
			return dotHidden, false
		}
	default:
		return dotHidden, false
	}
}

// renderStatusDot renders a 2-char right-aligned dot string for a chat entry.
// Returns " ●" (visible) or "  " (hidden) with appropriate color.
func (m model) renderStatusDot(item chatEntry) string {
	state, show := m.resolveStatusDotState(item)
	if !show {
		return dotSpacer()
	}

	visible := m.dotBlinkVisible
	solid := false

	switch state {
	case dotRunning:
		// Blink in normal mode; solid in reduced motion
		if m.reducedMotion {
			solid = true
		}
	case dotSuccess, dotError, dotPending, dotText:
		visible = true
		solid = true
	}

	if !visible {
		return dotSpacer()
	}

	color := m.colorForDotState(state, solid)
	return dotStyle(color).Render(dotChar)
}

// colorForDotState returns the lipgloss color for a given dot state,
// accounting for stagnation transition when in running state.
func (m model) colorForDotState(state statusDotState, solid bool) lipgloss.Color {
	switch state {
	case dotRunning:
		if m.stagnationActive && solid {
			return m.stagnationColor()
		}
		return semanticColors.TextMuted
	case dotSuccess:
		return semanticColors.Success
	case dotError:
		return semanticColors.Danger
	case dotPending:
		return semanticColors.TextMuted
	case dotText:
		return semanticColors.TextBase
	default:
		return semanticColors.TextMuted
	}
}

// stagnationColor returns the current color during stagnation transition.
// Uses eased lerp from theme muted color to danger red.
func (m model) stagnationColor() lipgloss.Color {
	if m.stagnationStart.IsZero() {
		return semanticColors.TextMuted
	}
	elapsed := time.Since(m.stagnationStart)
	if elapsed <= 0 {
		return semanticColors.TextMuted
	}
	if m.reducedMotion {
		// Instant switch in reduced motion mode
		if elapsed >= stagnationThreshold {
			return semanticColors.Danger
		}
		return semanticColors.TextMuted
	}
	if elapsed >= stagnationTransition {
		return semanticColors.Danger
	}
	t := float64(elapsed) / float64(stagnationTransition)
	t = easeOutCubic(t)
	return lerpColorHex(
		colorToHex(semanticColors.TextMuted),
		colorToHex(semanticColors.Danger),
		t,
	)
}

// hasBlinkingDot returns true if any visible chat entry needs a blinking dot.
func (m model) hasBlinkingDot() bool {
	for i := range m.chatItems {
		state, show := m.resolveStatusDotState(m.chatItems[i])
		if show && state == dotRunning {
			return true
		}
	}
	return false
}

// hasActiveDotAnimation returns true if any dot tick or stagnation tick should continue.
func (m model) hasActiveDotAnimation() bool {
	if m.reducedMotion {
		return false
	}
	if m.lastTokenTime.IsZero() {
		return false
	}
	// Continue if there's a run in progress
	if m.busy {
		return true
	}
	// Continue if stagnation transition is still active
	if m.stagnationActive && !m.stagnationStart.IsZero() &&
		time.Since(m.stagnationStart) < stagnationTransition {
		return true
	}
	return false
}

// isReducedMotion checks environment variables for reduced motion preference.
func isReducedMotion() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("BYTEMIND_REDUCED_MOTION"))) {
	case "1", "true", "yes", "on":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(os.Getenv("prefers-reduced-motion"))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// updateStagnation checks stagnation conditions and updates state.
// Returns true if the view needs refresh.
func (m *model) updateStagnation() bool {
	if m.reducedMotion {
		return false
	}
	if m.lastTokenTime.IsZero() {
		return false
	}

	stagnant := time.Since(m.lastTokenTime) >= stagnationThreshold
	hasActiveTool := m.hasActiveToolRunning()

	if stagnant && !hasActiveTool {
		if !m.stagnationActive {
			m.stagnationActive = true
			m.stagnationStart = time.Now()
			return true
		}
		// During transition, refresh for smooth animation
		if time.Since(m.stagnationStart) < stagnationTransition {
			return true
		}
		return false // transition complete, no more changes
	}

	if m.stagnationActive && (!stagnant || hasActiveTool) {
		m.stagnationActive = false
		m.stagnationStart = time.Time{}
		return true
	}

	return false
}

// hasActiveToolRunning returns true if any tool entry is currently running.
func (m model) hasActiveToolRunning() bool {
	for i := range m.chatItems {
		if m.chatItems[i].Kind == "tool" {
			status := strings.ToLower(strings.TrimSpace(m.chatItems[i].Status))
			if status == "running" || status == "active" {
				return true
			}
		}
	}
	return false
}

// resetStagnation resets stagnation tracking on new token/content arrival.
func (m *model) resetStagnation() {
	m.lastTokenTime = time.Now()
	if m.stagnationActive {
		m.stagnationActive = false
		m.stagnationStart = time.Time{}
	}
}

// dotStyle returns a lipgloss style for the dot with the given color.
func dotStyle(color lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(color).
		Width(dotWidth).
		Align(lipgloss.Right)
}

// dotSpacer returns a 2-char blank spacer matching dot width.
func dotSpacer() string {
	return lipgloss.NewStyle().Width(dotWidth).Render("")
}

// --- Color math helpers ---

// lerpColorHex performs RGB linear interpolation between two hex colors.
func lerpColorHex(c1, c2 string, t float64) lipgloss.Color {
	t = clamp01(t)
	r1, g1, b1 := parseHexColor(c1)
	r2, g2, b2 := parseHexColor(c2)
	r := r1 + (r2-r1)*t
	g := g1 + (g2-g1)*t
	b := b1 + (b2-b1)*t
	return lipgloss.Color(hexColor(r, g, b))
}

// easeOutCubic provides fast-start slow-end easing (cubic out).
func easeOutCubic(t float64) float64 {
	t = clamp01(t)
	return 1 - math.Pow(1-t, 3)
}

// parseHexColor parses a hex color string like "#RRGGBB" into 0-255 float64 values.
func parseHexColor(hex string) (r, g, b float64) {
	hex = strings.TrimPrefix(hex, "#")
	if len(hex) != 6 {
		return 0, 0, 0
	}
	var ri, gi, bi uint32
	// Simple hex parsing without fmt.Sscanf for speed
	ri = hexDigit(hex[0])*16 + hexDigit(hex[1])
	gi = hexDigit(hex[2])*16 + hexDigit(hex[3])
	bi = hexDigit(hex[4])*16 + hexDigit(hex[5])
	return float64(ri), float64(gi), float64(bi)
}

func hexDigit(c byte) uint32 {
	switch {
	case c >= '0' && c <= '9':
		return uint32(c - '0')
	case c >= 'a' && c <= 'f':
		return uint32(c - 'a' + 10)
	case c >= 'A' && c <= 'F':
		return uint32(c - 'A' + 10)
	default:
		return 0
	}
}

// hexColor formats RGB values (0-255) as a "#RRGGBB" hex string.
func hexColor(r, g, b float64) string {
	r = clamp0255(r)
	g = clamp0255(g)
	b = clamp0255(b)
	return rgbToHex(byte(r+0.5), byte(g+0.5), byte(b+0.5))
}

func rgbToHex(r, g, b byte) string {
	const hexChars = "0123456789ABCDEF"
	return "#" +
		string([]byte{hexChars[r>>4], hexChars[r&0xF]}) +
		string([]byte{hexChars[g>>4], hexChars[g&0xF]}) +
		string([]byte{hexChars[b>>4], hexChars[b&0xF]})
}

// colorToHex converts a lipgloss.Color to a hex string.
// lipgloss.Color is just a string, may or may not start with #.
func colorToHex(c lipgloss.Color) string {
	s := string(c)
	if strings.HasPrefix(s, "#") && len(s) == 7 {
		return s
	}
	return s
}

func clamp01(t float64) float64 {
	switch {
	case t < 0:
		return 0
	case t > 1:
		return 1
	default:
		return t
	}
}

func clamp0255(v float64) float64 {
	switch {
	case v < 0:
		return 0
	case v > 255:
		return 255
	default:
		return v
	}
}
