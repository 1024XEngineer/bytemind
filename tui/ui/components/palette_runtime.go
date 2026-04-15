package tui

func (m *model) syncInputOverlays() {
	if m.startupGuide.Active || m.promptSearchOpen {
		return
	}
	m.syncCommandPalette()
	m.syncMentionPalette()
	m.syncInputImageRefs(m.input.Value())
}

func (m model) commandPaletteWidth() int {
	switch m.screen {
	case screenLanding:
		return max(28, m.landingInputShellWidth())
	default:
		return max(32, m.chatPanelInnerWidth())
	}
}
