package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

const viewportTopSearchWindow = 12

func (m model) conversationViewportBounds() (left, right, top, bottom int, ok bool) {
	if m.viewport.Width <= 0 || m.viewport.Height <= 0 {
		return 0, 0, 0, 0, false
	}
	return m.conversationViewportBoundsByLayout()
}

// conversationViewportTopFromRenderedView resolves viewport top by matching rendered lines
// in a bounded window around layout-based expectedTop. This keeps per-event mapping cost
// near-constant instead of scanning every line in the whole screen buffer.
// Results are cached and reused until viewport scroll offset or size changes.
func (m *model) conversationViewportTopFromRenderedView(left, expectedTop int) (int, bool) {
	if m == nil || m.viewport.Height <= 0 {
		return 0, false
	}
	if top, found, ok := m.cachedViewportTopLookup(left, expectedTop); ok {
		return top, found
	}
	fullLines := strings.Split(strings.ReplaceAll(m.View(), "\r\n", "\n"), "\n")
	viewportLines := strings.Split(strings.ReplaceAll(m.renderConversationViewport(), "\r\n", "\n"), "\n")
	if len(fullLines) == 0 || len(viewportLines) == 0 || len(fullLines) < len(viewportLines) {
		return m.storeViewportTopLookup(left, expectedTop, 0, false)
	}
	maxOrigin := len(fullLines) - len(viewportLines)
	if maxOrigin < 0 {
		return m.storeViewportTopLookup(left, expectedTop, 0, false)
	}

	bestOrigin := -1
	bestScore := -1
	for _, candidate := range candidateOriginsNear(expectedTop, maxOrigin, viewportTopSearchWindow) {
		score := m.viewportMatchScore(fullLines, viewportLines, left, candidate)
		if score > bestScore {
			bestOrigin = candidate
			bestScore = score
		}
		if score == len(viewportLines) {
			return m.storeViewportTopLookup(left, expectedTop, candidate, true)
		}
	}
	requiredScore := max(1, (len(viewportLines)*8)/10)
	if bestOrigin < 0 || bestScore < requiredScore {
		return m.storeViewportTopLookup(left, expectedTop, 0, false)
	}
	return m.storeViewportTopLookup(left, expectedTop, bestOrigin, true)
}

func (m *model) cachedViewportTopLookup(left, expectedTop int) (int, bool, bool) {
	if m == nil || !m.viewportTopCache.valid {
		return 0, false, false
	}
	cache := m.viewportTopCache
	if cache.left != left || cache.expectedTop != expectedTop {
		return 0, false, false
	}
	if cache.viewportOffset != m.viewport.YOffset || cache.viewportWidth != m.viewport.Width || cache.viewportHeight != m.viewport.Height {
		return 0, false, false
	}
	return cache.top, cache.found, true
}

func (m *model) storeViewportTopLookup(left, expectedTop, top int, found bool) (int, bool) {
	if m == nil {
		return top, found
	}
	m.viewportTopCache = viewportTopLookupCache{
		left:           left,
		expectedTop:    expectedTop,
		viewportWidth:  m.viewport.Width,
		viewportHeight: m.viewport.Height,
		viewportOffset: m.viewport.YOffset,
		top:            top,
		found:          found,
		valid:          true,
	}
	return top, found
}

func candidateOriginsNear(expectedTop, maxOrigin, window int) []int {
	if maxOrigin < 0 {
		return nil
	}
	expectedTop = clamp(expectedTop, 0, maxOrigin)
	out := make([]int, 0, window*2+1)
	seen := make(map[int]struct{}, window*2+1)
	add := func(candidate int) {
		if candidate < 0 || candidate > maxOrigin {
			return
		}
		if _, ok := seen[candidate]; ok {
			return
		}
		seen[candidate] = struct{}{}
		out = append(out, candidate)
	}
	add(expectedTop)
	for delta := 1; delta <= window; delta++ {
		add(expectedTop - delta)
		add(expectedTop + delta)
	}
	return out
}

func (m model) viewportMatchScore(fullLines, viewportLines []string, left, candidate int) int {
	score := 0
	for row := 0; row < len(viewportLines); row++ {
		if m.screenLineMatchesViewportLine(fullLines[candidate+row], left, viewportLines[row]) {
			score++
		}
	}
	return score
}

func (m model) screenLineMatchesViewportLine(screenLine string, left int, viewportLine string) bool {
	segment := xansi.Cut(screenLine, left, left+m.viewport.Width)
	screenText := strings.TrimRight(xansi.Strip(segment), " ")
	viewportText := strings.TrimRight(xansi.Strip(viewportLine), " ")
	return screenText == viewportText
}

func (m model) conversationViewportBoundsByLayout() (left, right, top, bottom int, ok bool) {
	if m.screen != screenChat || m.viewport.Width <= 0 || m.viewport.Height <= 0 {
		return 0, 0, 0, 0, false
	}
	panelTop := panelStyle.GetVerticalFrameSize() / 2
	panelLeft := panelStyle.GetHorizontalFrameSize() / 2
	viewportTop := panelTop + m.conversationViewportOffsetInMainPanel()
	viewportBottom := viewportTop + m.viewport.Height - 1
	viewportLeft := panelLeft
	viewportRight := viewportLeft + m.viewport.Width - 1
	return viewportLeft, viewportRight, viewportTop, viewportBottom, true
}

func (m model) conversationViewportOffsetInMainPanel() int {
	width := max(24, m.chatPanelInnerWidth())
	badge := strings.TrimSpace(m.renderTopRightCluster(width))
	if badge == "" {
		return lipgloss.Height(m.renderStatusBar()) + 1
	}
	badgeW := lipgloss.Width(badge)
	statusW := max(12, width-badgeW-2)
	status := m.renderStatusBarWithWidth(statusW)
	header := lipgloss.JoinHorizontal(lipgloss.Top, status, "  ", badge)
	offset := lipgloss.Height(header)
	if popup := strings.TrimSpace(m.tokenUsage.PopupView()); popup != "" {
		offset += lipgloss.Height(lipgloss.PlaceHorizontal(width, lipgloss.Right, popup))
	}
	return offset + 1
}
