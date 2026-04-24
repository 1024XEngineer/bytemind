package tui

import tea "github.com/charmbracelet/bubbletea"

// pageRenderer renders one top-level screen in the TUI.
type pageRenderer interface {
	Render(m model) string
}

type viewOverlayComponent interface {
	Apply(m model, base string) string
}

type mainPanelComponent interface {
	Render(m model) string
}

type landingComponent interface {
	Render(m model) string
}

type footerComponent interface {
	Render(m model) string
}

type statusBarComponent interface {
	Render(m model, width int) string
}

type scrollbarComponent interface {
	Render(m model, viewHeight, contentHeight, currentOffset int) string
}

type conversationViewportComponent interface {
	Render(m model) string
}

type inputEditorViewComponent interface {
	Render(m model) string
}

// inputComponent owns input-editor update behavior.
type inputComponent interface {
	Update(m model, msg tea.Msg) (model, tea.Cmd)
}
