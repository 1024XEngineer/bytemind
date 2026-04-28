package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

func (m model) View() string {
	ensureZoneManager()
	if m.width > 0 {
		if m.screen == screenLanding {
			m.input.SetWidth(m.landingInputContentWidth())
			m.syncInputStyle()
		} else {
			m.input.SetWidth(m.chatInputContentWidth())
			m.syncInputStyle()
		}
	}
	base := m.activePage().Render(m)
	rendered := m.viewOverlayComponent().Apply(m, base)
	return zone.Scan(rendered)
}

func (m model) renderMainPanel() string {
	return m.mainPanelComponent().Render(m)
}

func renderMainPanelDefault(m model) string {
	width := max(24, m.chatPanelInnerWidth())
	badge := strings.TrimSpace(m.renderTopRightCluster(width))
	conversation := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.conversationViewportComponent().Render(m),
		m.scrollbarComponent().Render(m, m.viewport.Height, m.viewport.TotalLineCount(), m.viewport.YOffset),
	)
	if badge == "" {
		return lipgloss.JoinVertical(lipgloss.Left, m.statusBarComponent().Render(m, max(24, m.chatPanelInnerWidth())), "", conversation)
	}

	badgeW := lipgloss.Width(badge)
	statusW := max(12, width-badgeW-2)
	status := m.statusBarComponent().Render(m, statusW)
	header := lipgloss.JoinHorizontal(lipgloss.Top, status, "  ", badge)

	parts := []string{header}
	if popup := strings.TrimSpace(m.tokenUsage.PopupView()); popup != "" {
		parts = append(parts, lipgloss.PlaceHorizontal(width, lipgloss.Right, popup))
	}
	parts = append(parts, "", conversation)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) renderLanding() string {
	return m.landingComponent().Render(m)
}

func renderLandingDefault(m model) string {
	ensureZoneManager()
	content := m.renderLandingContent(true)
	return m.renderLandingCanvas(content)
}

func overlayBottomAligned(base, overlay string, width int) string {
	width = max(1, width)
	baseLines := strings.Split(strings.ReplaceAll(base, "\r\n", "\n"), "\n")
	overlayLines := strings.Split(strings.ReplaceAll(overlay, "\r\n", "\n"), "\n")
	if len(baseLines) == 0 || len(overlayLines) == 0 {
		return base
	}
	if len(overlayLines) > len(baseLines) {
		overlayLines = overlayLines[len(overlayLines)-len(baseLines):]
	}
	start := len(baseLines) - len(overlayLines)
	lineStyle := lipgloss.NewStyle().Width(width)
	for i := 0; i < len(overlayLines); i++ {
		baseLines[start+i] = lineStyle.Render(overlayLines[i])
	}
	return strings.Join(baseLines, "\n")
}
