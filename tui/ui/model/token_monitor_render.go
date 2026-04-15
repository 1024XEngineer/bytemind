package tui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

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

	topLeft := ringGlyph(levels[0], "◜", "⠈")
	topRight := ringGlyph(levels[1], "◝", "⠁")
	bottomRight := ringGlyph(levels[2], "◞", "⠂")
	bottomLeft := ringGlyph(levels[3], "◟", "⠄")

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

	topLeft := corner(levels[0], "◜")
	topRight := corner(levels[1], "◝")
	bottomLeft := corner(levels[3], "◟")
	bottomRight := corner(levels[2], "◞")
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
			parts = append(parts, active.Render("█"))
			continue
		}
		parts = append(parts, muted.Render("░"))
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
