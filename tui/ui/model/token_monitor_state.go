package tui

import (
	"math"
	"os"
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

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

func (c *tokenUsageComponent) SetPrice(inputPrice, outputPrice float64) {
	c.inputPrice = maxFloat(0, inputPrice)
	c.outputPrice = maxFloat(0, outputPrice)
}

func (c *tokenUsageComponent) SetBounds(x, y, width, height int) {
	c.bounds = rect{x: x, y: y, w: width, h: height}
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

func readEnvFlag(key string) bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch value {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
