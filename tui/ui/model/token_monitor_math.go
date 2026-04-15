package tui

import (
	"fmt"
	"math"
)

func clampFloat(value, low, high float64) float64 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
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
