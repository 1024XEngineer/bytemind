package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m *model) refreshViewport() {
	m.syncViewportSize()
	m.syncTokenUsageBounds()
	chatOffset := m.viewport.YOffset
	keepChatBottom := m.chatAutoFollow || m.viewport.AtBottom()
	conversationContent := m.renderConversation()
	m.viewportContentCache = conversationContent
	m.viewport.SetContent(conversationContent)
	m.copyView.SetContent(m.renderConversationCopy())
	if keepChatBottom {
		m.viewport.GotoBottom()
		m.copyView.GotoBottom()
		m.chatAutoFollow = true
	} else {
		m.viewport.SetYOffset(chatOffset)
		m.copyView.SetYOffset(chatOffset)
	}
	m.syncCopyViewOffset()

	planOffset := m.planView.YOffset
	m.planView.SetContent(m.planPanelContent(max(16, m.planView.Width)))
	m.planView.SetYOffset(planOffset)
}

func (m *model) syncTokenUsageBounds() {
	if m.screen != screenChat || m.width <= 0 || m.height <= 0 {
		m.tokenUsage.SetBounds(0, 0, 0, 0)
		return
	}
	width := max(24, m.chatPanelInnerWidth())
	badge := strings.TrimSpace(m.renderTokenBadge(width))
	if badge == "" {
		m.tokenUsage.SetBounds(0, 0, 0, 0)
		return
	}
	badgeW := lipgloss.Width(badge)
	badgeH := lipgloss.Height(badge)
	x := panelStyle.GetHorizontalFrameSize()/2 + max(0, width-badgeW-1)
	y := panelStyle.GetVerticalFrameSize() / 2
	m.tokenUsage.SetBounds(x, y, badgeW, badgeH)
}

func (m *model) syncLayoutForCurrentScreen() {
	if m.width > 0 {
		if m.screen == screenLanding {
			m.input.SetWidth(m.landingInputContentWidth())
		} else {
			m.input.SetWidth(m.chatInputContentWidth())
		}
	}
	m.syncInputStyle()
	m.syncViewportSize()
}

func (m *model) resize() {
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
}

func (m *model) syncViewportSize() {
	if m.width == 0 || m.height == 0 {
		return
	}
	footerHeight := lipgloss.Height(m.renderFooter())
	bodyHeight := m.height - footerHeight
	if bodyHeight < 6 {
		bodyHeight = 6
	}
	statusHeight := lipgloss.Height(m.renderStatusBar())
	panelInnerHeight := max(4, bodyHeight-panelStyle.GetVerticalFrameSize()-statusHeight-1)
	m.planView.Width = 0
	m.planView.Height = 0
	contentHeight := max(3, panelInnerHeight)
	m.viewport.Width = max(8, m.conversationPanelWidth()-scrollbarWidth)
	m.viewport.Height = contentHeight
	m.copyView.Width = m.viewport.Width
	m.copyView.Height = m.viewport.Height
	m.syncCopyViewOffset()
}

func (m *model) syncCopyViewOffset() {
	if m == nil {
		return
	}
	m.copyView.SetYOffset(m.viewport.YOffset)
}
