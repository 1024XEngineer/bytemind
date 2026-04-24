package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"
)

func (m model) renderTopRightCluster(width int) string {
	parts := make([]string, 0, 2)
	if toast := strings.TrimSpace(m.selectionToast); toast != "" {
		parts = append(parts, selectionToastStyle.Render(toast))
	}
	if badge := strings.TrimSpace(m.renderTokenBadge(width)); badge != "" {
		parts = append(parts, badge)
	}
	return strings.Join(parts, "  ")
}

func (m model) renderTokenBadge(width int) string {
	return m.tokenUsage.View()
}

func (m model) renderScrollbar(viewHeight, contentHeight, currentOffset int) string {
	return m.scrollbarComponent().Render(m, viewHeight, contentHeight, currentOffset)
}

func renderScrollbarDefault(m model, viewHeight, contentHeight, currentOffset int) string {
	thumbTop, thumbHeight, _, visible := m.scrollbarLayout(viewHeight, contentHeight, currentOffset)
	if !visible {
		return ""
	}
	trackStyle := scrollbarTrackStyle.Copy().Background(lipgloss.Color("#1B1D22"))
	thumbStyle := scrollbarThumbIdleStyle.Copy().Background(lipgloss.Color("#C2C7CF"))
	if m.draggingScrollbar {
		thumbStyle = scrollbarThumbActiveStyle.Copy().Background(lipgloss.Color("#E5E7EB"))
	}
	lines := make([]string, 0, viewHeight)
	for row := 0; row < viewHeight; row++ {
		if row >= thumbTop && row < thumbTop+thumbHeight {
			lines = append(lines, thumbStyle.Render(" "))
			continue
		}
		lines = append(lines, trackStyle.Render(" "))
	}
	return strings.Join(lines, "\n")
}

func (m model) scrollbarLayout(viewHeight, contentHeight, currentOffset int) (thumbTop, thumbHeight, maxOffset int, visible bool) {
	if viewHeight <= 0 {
		return 0, 0, 0, false
	}
	if contentHeight <= 0 {
		contentHeight = viewHeight
	}
	maxOffset = max(0, contentHeight-viewHeight)
	if maxOffset == 0 {
		return 0, viewHeight, 0, true
	}

	thumbHeight = roundedScaledDivision(viewHeight, viewHeight, contentHeight)
	thumbHeight = clamp(thumbHeight, 1, viewHeight)

	trackRange := max(0, viewHeight-thumbHeight)
	if trackRange == 0 {
		return 0, thumbHeight, maxOffset, true
	}
	offset := clamp(currentOffset, 0, maxOffset)
	thumbTop = roundedScaledDivision(offset, trackRange, maxOffset)
	thumbTop = clamp(thumbTop, 0, trackRange)
	return thumbTop, thumbHeight, maxOffset, true
}

func (m model) scrollbarTrackBounds() (x, top, bottom int, ok bool) {
	if m.screen != screenChat || m.viewport.Width <= 0 || m.viewport.Height <= 0 {
		return 0, 0, 0, false
	}
	left, right, top, bottom, ok := m.conversationViewportBoundsByLayout()
	if !ok {
		return 0, 0, 0, false
	}
	return right + 1, top, bottom, left >= 0
}

func (m *model) dragScrollbarTo(mouseY int) {
	_, trackTop, _, ok := m.scrollbarTrackBounds()
	if !ok {
		return
	}
	_, thumbHeight, maxOffset, visible := m.scrollbarLayout(m.viewport.Height, m.viewport.TotalLineCount(), m.viewport.YOffset)
	if !visible || maxOffset == 0 {
		return
	}
	trackRange := max(0, m.viewport.Height-thumbHeight)
	if trackRange == 0 {
		m.viewport.SetYOffset(0)
		m.syncCopyViewOffset()
		return
	}
	desiredTop := mouseY - trackTop - m.scrollbarDragOffset
	desiredTop = clamp(desiredTop, 0, trackRange)
	offset := roundedScaledDivision(desiredTop, maxOffset, trackRange)
	m.viewport.SetYOffset(clamp(offset, 0, maxOffset))
	m.syncCopyViewOffset()
}

func roundedScaledDivision(value, scale, denominator int) int {
	if denominator <= 0 || value <= 0 || scale <= 0 {
		return 0
	}
	// Use int64 math to avoid overflow when terminal dimensions are large.
	numerator := int64(value)*int64(scale) + int64(denominator)/2
	result := numerator / int64(denominator)
	maxInt := int64(^uint(0) >> 1)
	if result > maxInt {
		return int(maxInt)
	}
	return int(result)
}

func stripANSIText(value string) string {
	return xansi.Strip(value)
}

func (m model) renderStatusBar() string {
	return m.statusBarComponent().Render(m, max(24, m.chatPanelInnerWidth()))
}

func (m model) renderStatusBarWithWidth(width int) string {
	return m.statusBarComponent().Render(m, width)
}

func renderStatusBarWithWidthDefault(m model, width int) string {
	stepTitle := currentOrNextStepTitle(m.plan)
	if stepTitle == "" {
		stepTitle = "-"
	}
	left := strings.Join([]string{
		"Mode: " + strings.ToUpper(string(m.mode)),
		"Phase: " + m.currentPhaseLabel(),
		"Step: " + stepTitle,
		"Skill: " + m.currentSkillLabel(),
	}, "  |  ")
	right := strings.Join([]string{
		fmt.Sprintf("%d msgs", len(m.chatItems)),
		"Session: " + m.currentSessionLabel(),
		"Follow: " + m.autoFollowLabel(),
		"Model: " + m.currentModelLabel(),
	}, "  |  ")

	line := m.renderTopInfoLine(left, right, width)
	return statusBarStyle.Width(width).Render(line)
}

func (m model) renderTopInfoLine(left, right string, width int) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if width <= 0 {
		return strings.TrimSpace(left + " | " + right)
	}

	leftW := runewidth.StringWidth(left)
	rightW := runewidth.StringWidth(right)
	if leftW+rightW+2 > width {
		return compact(left+"  |  "+right, width)
	}
	gap := width - leftW - rightW
	return left + strings.Repeat(" ", max(2, gap)) + right
}
