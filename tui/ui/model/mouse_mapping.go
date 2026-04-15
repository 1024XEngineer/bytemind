package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

// viewportPointFromMouse maps terminal mouse coordinates into absolute viewport
// selection coordinates. It prefers bubblezone data first, then falls back to
// layout-derived bounds for terminals with imperfect mouse reports.
func (m *model) viewportPointFromMouse(x, y int) (viewportSelectionPoint, bool) {
	if m == nil {
		return viewportSelectionPoint{}, false
	}
	ensureZoneManager()
	if z := zone.Get(conversationViewportZoneID); z != nil {
		if point, ok := m.viewportPointFromZone(z, x, y); ok {
			return point, true
		}
		if x >= z.StartX-1 && x <= z.EndX+1 {
			for delta := 1; delta <= mouseZoneAutoProbeMaxDelta; delta++ {
				if point, ok := m.viewportPointFromZone(z, x, y-delta); ok {
					return point, true
				}
				if point, ok := m.viewportPointFromZone(z, x, y+delta); ok {
					return point, true
				}
			}
		}
	}

	left, right, top, bottom, ok := m.conversationViewportBounds()
	if !ok {
		return viewportSelectionPoint{}, false
	}
	if renderedTop, found := m.conversationViewportTopFromRenderedView(left, top); found {
		top = renderedTop
		bottom = top + m.viewport.Height - 1
	}
	if x < left-1 || x > right+1 || y < top-1 || y > bottom+1 {
		return viewportSelectionPoint{}, false
	}
	col := clamp(x-left, 0, max(0, m.viewport.Width-1))
	row := clamp(y-top, 0, max(0, m.viewport.Height-1))
	return viewportSelectionPoint{
		Col: col,
		Row: max(0, m.viewport.YOffset) + row,
	}, true
}

func (m model) viewportPointFromZone(z *zone.ZoneInfo, x, y int) (viewportSelectionPoint, bool) {
	col, row := z.Pos(tea.MouseMsg{X: x, Y: y})
	if col < 0 || row < 0 {
		return viewportSelectionPoint{}, false
	}
	return viewportSelectionPoint{
		Col: clamp(col, 0, max(0, m.viewport.Width-1)),
		Row: max(0, m.viewport.YOffset) + clamp(row, 0, max(0, m.viewport.Height-1)),
	}, true
}

// inputPointFromMouse converts mouse coordinates to logical input-editor cells.
func (m model) inputPointFromMouse(x, y int, clampToBounds bool) (viewportSelectionPoint, bool) {
	ensureZoneManager()
	if z := zone.Get(inputEditorZoneID); z != nil {
		if point, ok := m.inputPointFromZone(z, x, y, clampToBounds); ok {
			return point, true
		}
		if x >= z.StartX-1 && x <= z.EndX+1 {
			for delta := 1; delta <= mouseZoneAutoProbeMaxDelta; delta++ {
				if point, ok := m.inputPointFromZone(z, x, y-delta, clampToBounds); ok {
					return point, true
				}
				if point, ok := m.inputPointFromZone(z, x, y+delta, clampToBounds); ok {
					return point, true
				}
			}
		}
	}

	left, right, top, bottom, innerLeft, innerTop, ok := m.inputInnerBounds()
	if !ok {
		return viewportSelectionPoint{}, false
	}
	if !clampToBounds && (x < left || x > right || y < top || y > bottom) {
		return viewportSelectionPoint{}, false
	}
	x = clamp(x, left, right)
	y = clamp(y, top, bottom)
	lines := m.inputSelectionSourceLines("")
	if len(lines) == 0 {
		return viewportSelectionPoint{}, false
	}
	row := clamp(y-innerTop, 0, len(lines)-1)
	lineWidth := xansi.StringWidth(lines[row])
	if lineWidth <= 0 {
		return viewportSelectionPoint{Row: row, Col: 0}, true
	}
	col := clamp(x-innerLeft, 0, lineWidth-1)
	return viewportSelectionPoint{Row: row, Col: col}, true
}

func (m model) inputPointFromZone(z *zone.ZoneInfo, x, y int, clampToBounds bool) (viewportSelectionPoint, bool) {
	col, row := z.Pos(tea.MouseMsg{X: x, Y: y})
	if (col < 0 || row < 0) && clampToBounds {
		clampedX := clamp(x, z.StartX, z.EndX)
		clampedY := clamp(y, z.StartY, z.EndY)
		col, row = z.Pos(tea.MouseMsg{X: clampedX, Y: clampedY})
	}
	if col < 0 || row < 0 {
		return viewportSelectionPoint{}, false
	}
	lines := m.inputSelectionSourceLines("")
	if len(lines) == 0 {
		return viewportSelectionPoint{}, false
	}
	row = clamp(row, 0, len(lines)-1)
	lineWidth := xansi.StringWidth(lines[row])
	if lineWidth <= 0 {
		return viewportSelectionPoint{Row: row, Col: 0}, true
	}
	col = clamp(col, 0, lineWidth-1)
	return viewportSelectionPoint{Row: row, Col: col}, true
}

func (m model) inputInnerBounds() (left, right, top, bottom, innerLeft, innerTop int, ok bool) {
	switch m.screen {
	case screenChat:
		if m.width <= 0 {
			return 0, 0, 0, 0, 0, 0, false
		}
		inputRender := m.inputBorderStyle().Width(m.chatPanelInnerWidth()).Render(m.input.View())
		left = panelStyle.GetHorizontalFrameSize() / 2
		top = panelStyle.GetVerticalFrameSize()/2 + lipgloss.Height(m.renderMainPanel())
		if m.approval != nil {
			top += lipgloss.Height(m.renderApprovalBanner())
		}
		top += m.calculateOverlayHeight(0)
		right = left + max(1, lipgloss.Width(inputRender)) - 1
		bottom = top + max(1, lipgloss.Height(inputRender)) - 1
		innerLeft = left + m.inputBorderStyle().GetHorizontalFrameSize()/2
		innerTop = top + m.inputBorderStyle().GetVerticalFrameSize()/2
		return left, right, top, bottom, innerLeft, innerTop, true
	case screenLanding:
		if m.height <= 0 || m.width <= 0 {
			return 0, 0, 0, 0, 0, 0, false
		}
		box := landingInputStyle.Copy().
			BorderForeground(m.modeAccentColor()).
			Width(m.landingInputShellWidth()).
			Render(m.input.View())
		boxW := max(1, lipgloss.Width(box))
		boxH := max(1, lipgloss.Height(box))
		logoHeight := lipgloss.Height(landingLogoStyle.Render(strings.Join([]string{
			"    ____        __                      _           __",
			"   / __ )__  __/ /____  ____ ___  ____(_)___  ____/ /",
			"  / __  / / / / __/ _ \\/ __ `__ \\/ __/ / __ \\/ __  / ",
			" / /_/ / /_/ / /_/  __/ / / / / / /_/ / / / / /_/ /  ",
			"/_____/\\__, /\\__/\\___/_/ /_/ /_/\\__/_/_/ /_/\\__,_/   ",
			"      /____/                                          ",
		}, "\n")))
		overlayHeight := m.calculateOverlayHeight(1)
		modeTabsHeight := lipgloss.Height(m.renderModeTabs())
		hintHeight := lipgloss.Height(mutedStyle.Render(footerHintText))
		contentHeight := logoHeight + 1 + modeTabsHeight + 1 + overlayHeight + boxH + 1 + hintHeight
		contentTop := max(0, (m.height-contentHeight)/2)
		top = contentTop + logoHeight + 1 + modeTabsHeight + 1 + overlayHeight
		left = max(0, (m.width-boxW)/2)
		right = left + boxW - 1
		bottom = top + boxH - 1
		innerLeft = left + landingInputStyle.GetHorizontalFrameSize()/2
		innerTop = top + landingInputStyle.GetVerticalFrameSize()/2
		return left, right, top, bottom, innerLeft, innerTop, true
	default:
		return 0, 0, 0, 0, 0, 0, false
	}
}

func (m model) calculateOverlayHeight(extraGap int) int {
	switch {
	case m.startupGuide.Active:
		return lipgloss.Height(m.renderStartupGuidePanel()) + extraGap
	case m.promptSearchOpen:
		return lipgloss.Height(m.renderPromptSearchPalette()) + extraGap
	case m.mentionOpen:
		return lipgloss.Height(m.renderMentionPalette()) + extraGap
	case m.commandOpen:
		return lipgloss.Height(m.renderCommandPalette()) + extraGap
	default:
		return 0
	}
}
