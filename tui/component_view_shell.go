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
	logo := landingLogoStyle.Render(strings.Join([]string{
		"    ____        __                      _           __",
		"   / __ )__  __/ /____  ____ ___  ____(_)___  ____/ /",
		"  / __  / / / / __/ _ \\/ __ `__ \\/ __/ / __ \\/ __  / ",
		" / /_/ / /_/ / /_/  __/ / / / / / /_/ / / / / /_/ /  ",
		"/_____/\\__, /\\__/\\___/_/ /_/ /_/\\__/_/_/ /_/\\__,_/   ",
		"      /____/                                          ",
	}, "\n"))
	inputBox := landingInputStyle.Copy().
		BorderForeground(m.modeAccentColor()).
		Width(m.landingInputShellWidth()).
		Render(zone.Mark(inputEditorZoneID, m.inputEditorViewComponent().Render(m)))
	parts := []string{logo, "", m.renderModeTabs(), ""}
	if m.startupGuide.Active {
		parts = append(parts, m.renderStartupGuidePanel(), "")
	} else if m.promptSearchOpen {
		parts = append(parts, m.renderPromptSearchPalette(), "")
	} else if m.mentionOpen {
		parts = append(parts, m.renderMentionPalette(), "")
	} else if m.commandOpen {
		parts = append(parts, m.renderCommandPalette(), "")
	}
	parts = append(parts, inputBox, "", renderFooterShortcutHints())
	content := lipgloss.JoinVertical(lipgloss.Center, parts...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}
