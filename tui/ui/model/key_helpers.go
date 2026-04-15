package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func normalizeKeyName(key string) string {
	key = strings.ToLower(strings.TrimSpace(key))
	replacer := strings.NewReplacer(" ", "", "-", "", "_", "")
	return replacer.Replace(key)
}

func isInputNewlineKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyCtrlJ || normalizeKeyName(msg.String()) == "ctrl+j" {
		return true
	}
	if msg.Type == tea.KeyEnter && msg.Alt {
		return true
	}
	key := normalizeKeyName(msg.String())
	return key == "shift+enter" || key == "shift+return"
}

func isCtrlVPasteKey(msg tea.KeyMsg) bool {
	if msg.Type == tea.KeyCtrlV {
		return true
	}
	if len(msg.Runes) == 1 && msg.Runes[0] == []rune(ctrlVMarkerRune)[0] {
		return true
	}
	return normalizeKeyName(msg.String()) == "ctrl+v"
}

func inputMutationSource(msg tea.KeyMsg) string {
	source := strings.TrimSpace(msg.String())
	if !msg.Paste {
		return source
	}
	if source == "" {
		return "paste"
	}
	return source + ":paste"
}

func isClipboardNoImageNote(note string) bool {
	return strings.Contains(strings.ToLower(strings.TrimSpace(note)), "clipboard has no image")
}

func isPageUpKey(msg tea.KeyMsg) bool {
	key := normalizeKeyName(msg.String())
	return msg.Type == tea.KeyPgUp || key == "pgup" || key == "pageup" || key == "prior"
}

func isPageDownKey(msg tea.KeyMsg) bool {
	key := normalizeKeyName(msg.String())
	return msg.Type == tea.KeyPgDown || key == "pgdn" || key == "pgdown" || key == "pagedown" || key == "next"
}

func (m *model) scrollInput(delta int) {
	switch {
	case delta < 0:
		for i := 0; i < -delta; i++ {
			m.input.CursorUp()
		}
	case delta > 0:
		for i := 0; i < delta; i++ {
			m.input.CursorDown()
		}
	}
}

func (m model) mouseOverInput(y int) bool {
	ensureZoneManager()
	switch m.screen {
	case screenLanding:
		return m.mouseOverLandingInput(y)
	case screenChat:
		return m.mouseOverChatInput(y)
	default:
		return false
	}
}

func (m model) mouseOverPlan(x, y int) bool {
	return false
}

func (m model) mouseOverChatInput(y int) bool {
	if m.width <= 0 {
		return false
	}
	footerTop := panelStyle.GetVerticalFrameSize()/2 + lipgloss.Height(m.renderMainPanel())
	inputHeight := lipgloss.Height(
		m.inputBorderStyle().
			Width(m.chatPanelInnerWidth()).
			Render(m.input.View()),
	)
	inputTop := footerTop
	if m.approval != nil {
		inputTop += lipgloss.Height(m.renderApprovalBanner())
	}
	if m.startupGuide.Active {
		inputTop += lipgloss.Height(m.renderStartupGuidePanel())
	} else if m.promptSearchOpen {
		inputTop += lipgloss.Height(m.renderPromptSearchPalette())
	} else if m.mentionOpen {
		inputTop += lipgloss.Height(m.renderMentionPalette())
	} else if m.commandOpen {
		inputTop += lipgloss.Height(m.renderCommandPalette())
	}
	inputBottom := inputTop + max(1, inputHeight) - 1
	return y >= inputTop && y <= inputBottom
}

func (m model) mouseOverLandingInput(y int) bool {
	if m.height <= 0 {
		return false
	}
	logoHeight := lipgloss.Height(landingLogoStyle.Render(strings.Join([]string{
		"    ____        __                      _           __",
		"   / __ )__  __/ /____  ____ ___  ____(_)___  ____/ /",
		"  / __  / / / / __/ _ \\/ __ `__ \\/ __/ / __ \\/ __  / ",
		" / /_/ / /_/ / /_/  __/ / / / / / /_/ / / / / /_/ /  ",
		"/_____/\\__, /\\__/\\___/_/ /_/ /_/\\__/_/_/ /_/\\__,_/   ",
		"      /____/                                          ",
	}, "\n")))
	modeTabsHeight := lipgloss.Height(m.renderModeTabs())
	overlayHeight := 0
	if m.startupGuide.Active {
		overlayHeight = lipgloss.Height(m.renderStartupGuidePanel()) + 1
	} else if m.promptSearchOpen {
		overlayHeight = lipgloss.Height(m.renderPromptSearchPalette()) + 1
	} else if m.mentionOpen {
		overlayHeight = lipgloss.Height(m.renderMentionPalette()) + 1
	} else if m.commandOpen {
		overlayHeight = lipgloss.Height(m.renderCommandPalette()) + 1
	}
	inputHeight := lipgloss.Height(
		landingInputStyle.Copy().
			BorderForeground(m.modeAccentColor()).
			Width(m.landingInputShellWidth()).
			Render(m.input.View()),
	)
	hintHeight := lipgloss.Height(renderFooterShortcutHints())
	contentHeight := logoHeight + 1 + modeTabsHeight + 1 + overlayHeight + inputHeight + 1 + hintHeight
	contentTop := max(0, (m.height-contentHeight)/2)
	inputTop := contentTop + logoHeight + 1 + modeTabsHeight + 1 + overlayHeight
	inputBottom := inputTop + max(1, inputHeight) - 1
	return y >= inputTop && y <= inputBottom
}
