package tui

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type landingPage struct{}

func (landingPage) Render(m model) string {
	return m.landingComponent().Render(m)
}

type chatPage struct{}

func (chatPage) Render(m model) string {
	chatContent := lipgloss.JoinVertical(
		lipgloss.Left,
		m.mainPanelComponent().Render(m),
		m.footerComponent().Render(m),
	)
	return panelStyle.Width(m.chatPanelWidth()).Render(chatContent)
}

func (m model) activePage() pageRenderer {
	if m.screen == screenChat {
		return chatPage{}
	}
	return landingPage{}
}

type defaultViewOverlayComponent struct{}

func (defaultViewOverlayComponent) Apply(m model, base string) string {
	switch {
	case m.helpOpen:
		return renderModal(m.width, m.height, m.renderHelpModal())
	case m.sessionsOpen:
		return renderModal(m.width, m.height, m.renderSessionsModal())
	case m.skillsOpen:
		return renderModal(m.width, m.height, m.renderSkillsModal())
	default:
		return base
	}
}

func (m model) viewOverlayComponent() viewOverlayComponent {
	return defaultViewOverlayComponent{}
}

type defaultMainPanelComponent struct{}

func (defaultMainPanelComponent) Render(m model) string {
	return renderMainPanelDefault(m)
}

func (m model) mainPanelComponent() mainPanelComponent {
	return defaultMainPanelComponent{}
}

type defaultLandingComponent struct{}

func (defaultLandingComponent) Render(m model) string {
	return renderLandingDefault(m)
}

func (m model) landingComponent() landingComponent {
	return defaultLandingComponent{}
}

type defaultFooterComponent struct{}

func (defaultFooterComponent) Render(m model) string {
	return renderFooterDefault(m)
}

func (m model) footerComponent() footerComponent {
	return defaultFooterComponent{}
}

type defaultStatusBarComponent struct{}

func (defaultStatusBarComponent) Render(m model, width int) string {
	return renderStatusBarWithWidthDefault(m, width)
}

func (m model) statusBarComponent() statusBarComponent {
	return defaultStatusBarComponent{}
}

type defaultScrollbarComponent struct{}

func (defaultScrollbarComponent) Render(m model, viewHeight, contentHeight, currentOffset int) string {
	return renderScrollbarDefault(m, viewHeight, contentHeight, currentOffset)
}

func (m model) scrollbarComponent() scrollbarComponent {
	return defaultScrollbarComponent{}
}

type defaultConversationViewportComponent struct{}

func (defaultConversationViewportComponent) Render(m model) string {
	return renderConversationViewportDefault(m)
}

func (m model) conversationViewportComponent() conversationViewportComponent {
	return defaultConversationViewportComponent{}
}

type defaultInputEditorViewComponent struct{}

func (defaultInputEditorViewComponent) Render(m model) string {
	return renderInputEditorViewDefault(m)
}

func (m model) inputEditorViewComponent() inputEditorViewComponent {
	return defaultInputEditorViewComponent{}
}

type textAreaInputComponent struct{}

func (textAreaInputComponent) Update(m model, msg tea.Msg) (model, tea.Cmd) {
	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.handleInputMutation(before, m.input.Value(), "")
		m.syncInputOverlays()
	}
	return m, cmd
}

func (m model) defaultInputComponent() inputComponent {
	return textAreaInputComponent{}
}
