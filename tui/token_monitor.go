package tui

import (
	"fmt"
	"math"
	"os"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	tokenMonitorTickInterval = 20 * time.Millisecond
)

type tokenMonitorTickMsg time.Time

type tokenUsageComponent struct {
	used        int
	total       int
	displayUsed float64
	unavailable bool
	input       int
	output      int
	context     int
	inputPrice  float64
	outputPrice float64

	hover        bool
	popup        bool
	popupUntil   time.Time
	bounds       rect
	ringSegments int
	simpleRing   bool
	noBraille    bool
}

type rect struct {
	x int
	y int
	w int
	h int
}

func newTokenUsageComponent() tokenUsageComponent {
	compatRing := runtime.GOOS == "windows" || readEnvFlag("BYTEMIND_TOKEN_MONITOR_COMPAT")
	simpleRing := readEnvFlag("BYTEMIND_TOKEN_MONITOR_SIMPLE")
	noBraille := simpleRing || compatRing || readEnvFlag("BYTEMIND_TOKEN_MONITOR_NO_BRAILLE")
	return tokenUsageComponent{
		total:        5000,
		ringSegments: 8,
		simpleRing:   simpleRing,
		noBraille:    noBraille,
	}
}

func (c *tokenUsageComponent) SetUsage(used, total int) tea.Cmd {
	if total < 0 {
		total = 0
	}
	used = max(0, used)
	c.used = used
	c.total = total
	c.displayUsed = float64(c.used)
	return nil
}

func (c *tokenUsageComponent) SetBreakdown(input, output, context int) {
	c.input = max(0, input)
	c.output = max(0, output)
	c.context = max(0, context)
}

func (c *tokenUsageComponent) SetUnavailable(unavailable bool) {
	c.unavailable = unavailable
}

// SetPrice sets token prices per 1M tokens.
func (c *tokenUsageComponent) SetPrice(inputPrice, outputPrice float64) {
	c.inputPrice = maxFloat(0, inputPrice)
	c.outputPrice = maxFloat(0, outputPrice)
}

func (c *tokenUsageComponent) SetBounds(x, y, width, height int) {
	c.bounds = rect{x: x, y: y, w: width, h: height}
}

func (c tokenUsageComponent) View() string {
	text := c.usageText()
	return c.badgeStyle().Render(text)
}

func (c tokenUsageComponent) CompactView() string {
	return c.badgeStyle().Render(c.compactUsageText())
}

func (c tokenUsageComponent) PopupView() string {
	return ""
}

func (c *tokenUsageComponent) Update(msg tea.Msg) (tea.Cmd, bool) {
	switch msg := msg.(type) {
	case tea.MouseMsg:
		inside := c.contains(msg.X, msg.Y)
		switch msg.Action {
		case tea.MouseActionMotion:
			c.hover = inside
			return nil, inside
		case tea.MouseActionPress:
			if msg.Button == tea.MouseButtonLeft && inside {
				c.hover = true
				return nil, true
			}
			if msg.Button == tea.MouseButtonLeft && !inside && c.popup {
				c.popup = false
				return nil, false
			}
			return nil, inside
		case tea.MouseActionRelease:
			return nil, inside
		default:
			return nil, inside
		}
	case tokenMonitorTickMsg:
		needsMore := false
		if c.total > 0 {
			diff := float64(c.used) - c.displayUsed
			if math.Abs(diff) > 0.5 {
				step := math.Max(2, math.Abs(diff)*0.35)
				if math.Abs(diff) > 120 {
					step = math.Max(step, 10)
				}
				if diff < 0 {
					c.displayUsed -= step
				} else {
					c.displayUsed += step
				}
				c.displayUsed = clampFloat(c.displayUsed, 0, float64(max(c.total, c.used)))
				needsMore = true
			} else {
				c.displayUsed = float64(c.used)
			}
		}
		if c.popup && time.Now().After(c.popupUntil) {
			c.popup = false
		}
		if needsMore || c.popup {
			return c.tickCmd(), false
		}
	}
	return nil, false
}

func (c tokenUsageComponent) tickCmd() tea.Cmd {
	return tea.Tick(tokenMonitorTickInterval, func(t time.Time) tea.Msg {
		return tokenMonitorTickMsg(t)
	})
}

func (c tokenUsageComponent) Layout(containerWidth int) (x, y, w, h int) {
	badgeW := lipgloss.Width(c.View())
	badgeH := lipgloss.Height(c.badgeStyle().Render("x"))
	x = max(0, containerWidth-badgeW-2)
	y = 1
	return x, y, badgeW, badgeH
}

func (c tokenUsageComponent) contains(x, y int) bool {
	if c.bounds.w <= 0 || c.bounds.h <= 0 {
		return false
	}
	return x >= c.bounds.x && x < c.bounds.x+c.bounds.w && y >= c.bounds.y && y < c.bounds.y+c.bounds.h
}

func (c tokenUsageComponent) usageText() string {
	if c.unavailable {
		return "token: unavailable"
	}
	text := "token: " + formatInt(max(0, int(math.Round(c.displayUsed))))
	pad := max(8, len(text))
	if c.hover {
		text = "Used " + text
		pad = max(pad, len(text))
	}
	return lipgloss.NewStyle().Width(pad).Render(text)
}

func (c tokenUsageComponent) compactUsageText() string {
	if c.unavailable {
		return "token: unavailable"
	}
	if c.hover {
		return "used token: " + formatInt(max(0, int(math.Round(c.displayUsed))))
	}
	return "token: " + formatInt(max(0, int(math.Round(c.displayUsed))))
}

func (c tokenUsageComponent) ratio() float64 {
	if c.total <= 0 {
		return 0
	}
	return clampFloat(c.displayUsed/float64(c.total), 0, 1)
}

func (c tokenUsageComponent) ringColor() lipgloss.Color {
	r := c.ratio()
	blue := rgb{0x00, 0x7A, 0xFF}
	orange := rgb{0xFF, 0x95, 0x00}
	red := rgb{0xFF, 0x3B, 0x30}

	switch {
	case r <= 0.80:
		return lipgloss.Color(toHex(blue))
	case r < 0.95:
		t := (r - 0.80) / 0.15
		return lipgloss.Color(toHex(lerpRGB(blue, orange, t)))
	default:
		t := (r - 0.95) / 0.05
		return lipgloss.Color(toHex(lerpRGB(orange, red, t)))
	}
}

func (c tokenUsageComponent) renderRing() string {
	if c.simpleRing {
		return c.renderSimpleBarRing()
	}
	if c.noBraille {
		return c.renderNoBrailleRing()
	}

	segments := clamp(int(math.Round(c.ratio()*float64(c.ringSegments))), 0, c.ringSegments)
	levels := [4]int{}
	for i := 0; i < 4; i++ {
		chunk := segments - i*2
		levels[i] = clamp(chunk, 0, 2)
	}

	active := lipgloss.NewStyle().Foreground(c.ringColor())
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#2E2E2E"))

	topLeft := ringGlyph(levels[0], "\u25dc", "\u2808")
	topRight := ringGlyph(levels[1], "\u25dd", "\u2801")
	bottomRight := ringGlyph(levels[2], "\u25de", "\u2802")
	bottomLeft := ringGlyph(levels[3], "\u25df", "\u2804")

	parts := []struct {
		ch    string
		level int
	}{
		{topLeft, levels[0]},
		{topRight, levels[1]},
		{bottomLeft, levels[3]},
		{bottomRight, levels[2]},
	}
	for i := range parts {
		if parts[i].level > 0 {
			parts[i].ch = active.Render(parts[i].ch)
		} else {
			parts[i].ch = muted.Render(parts[i].ch)
		}
	}

	return parts[0].ch + parts[1].ch + parts[2].ch + parts[3].ch
}

func (c tokenUsageComponent) renderNoBrailleRing() string {
	segments := clamp(int(math.Round(c.ratio()*float64(c.ringSegments))), 0, c.ringSegments)
	levels := [4]int{}
	for i := 0; i < 4; i++ {
		chunk := segments - i*2
		levels[i] = clamp(chunk, 0, 2)
	}

	active := lipgloss.NewStyle().Foreground(c.ringColor())
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#2E2E2E"))

	corner := func(level int, glyph string) string {
		if level > 0 {
			return active.Render(glyph)
		}
		return muted.Render(glyph)
	}

	topLeft := corner(levels[0], "\u25dc")
	topRight := corner(levels[1], "\u25dd")
	bottomLeft := corner(levels[3], "\u25df")
	bottomRight := corner(levels[2], "\u25de")
	top := topLeft + topRight
	bottom := bottomLeft + bottomRight
	return lipgloss.JoinVertical(lipgloss.Left, top, bottom)
}

func (c tokenUsageComponent) renderSimpleBarRing() string {
	segments := clamp(int(math.Round(c.ratio()*4)), 0, 4)
	active := lipgloss.NewStyle().Foreground(c.ringColor())
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("#2E2E2E"))
	parts := make([]string, 0, 4)
	for i := 0; i < 4; i++ {
		if i < segments {
			parts = append(parts, active.Render("\u2588"))
			continue
		}
		parts = append(parts, muted.Render("\u2591"))
	}
	return strings.Join(parts, "")
}

func ringGlyph(level int, full, half string) string {
	switch level {
	case 2:
		return full
	case 1:
		return half
	default:
		return " "
	}
}

func (c tokenUsageComponent) popupText() string {
	return ""
}

func (c tokenUsageComponent) estimatedCost() float64 {
	inputCost := (float64(c.input) / 1_000_000.0) * c.inputPrice
	outputCost := (float64(c.output) / 1_000_000.0) * c.outputPrice
	return inputCost + outputCost
}

func (c tokenUsageComponent) badgeStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		Background(lipgloss.Color("#1A1A1A")).
		Foreground(lipgloss.Color("#E8E8E8")).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#242424"))
}

func (c tokenUsageComponent) popupStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		MarginTop(1).
		Background(lipgloss.Color("#111111")).
		Foreground(lipgloss.Color("#D9D9D9")).
		Padding(0, 1).
		BorderStyle(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#2B2B2B"))
}

func formatInt(v int) string {
	if v < 1000 {
		return fmt.Sprintf("%d", v)
	}
	s := fmt.Sprintf("%d", v)
	n := len(s)
	out := make([]byte, 0, n+n/3)
	for i := 0; i < n; i++ {
		if i > 0 && (n-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	return string(out)
}

func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

type rgb struct {
	r int
	g int
	b int
}

func lerpRGB(a, b rgb, t float64) rgb {
	t = clampFloat(t, 0, 1)
	return rgb{
		r: int(math.Round(float64(a.r) + (float64(b.r)-float64(a.r))*t)),
		g: int(math.Round(float64(a.g) + (float64(b.g)-float64(a.g))*t)),
		b: int(math.Round(float64(a.b) + (float64(b.b)-float64(a.b))*t)),
	}
}

func toHex(c rgb) string {
	return fmt.Sprintf("#%02X%02X%02X", clamp(c.r, 0, 255), clamp(c.g, 0, 255), clamp(c.b, 0, 255))
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func readEnvFlag(key string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
