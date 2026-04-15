package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestConversationViewportTopFromRenderedPanelMatchesRenderedView(t *testing.T) {
	input := textarea.New()
	input.Focus()

	m := model{
		screen:     screenChat,
		width:      100,
		height:     30,
		input:      input,
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
	}
	m.resize()
	m.viewport.SetContent("__TOP_TOKEN__\nline two")

	vpLines := strings.Split(strings.ReplaceAll(m.viewport.View(), "\r\n", "\n"), "\n")
	tokenRowInViewport := -1
	for i, line := range vpLines {
		if strings.Contains(xansi.Strip(line), "__TOP_TOKEN__") {
			tokenRowInViewport = i
			break
		}
	}
	if tokenRowInViewport < 0 {
		t.Fatalf("expected token row in viewport view")
	}

	fullLines := strings.Split(strings.ReplaceAll(m.View(), "\r\n", "\n"), "\n")
	tokenRowInView := -1
	for i, line := range fullLines {
		if strings.Contains(xansi.Strip(line), "__TOP_TOKEN__") {
			tokenRowInView = i
			break
		}
	}
	if tokenRowInView < 0 {
		t.Fatalf("expected token row in full view")
	}

	left, _, layoutTop, _, ok := m.conversationViewportBoundsByLayout()
	if !ok {
		t.Fatalf("expected viewport layout bounds")
	}
	top, found := m.conversationViewportTopFromRenderedView(left, layoutTop)
	if !found {
		t.Fatalf("expected rendered-panel top to be found")
	}
	expectedTop := tokenRowInView - tokenRowInViewport
	if top != expectedTop {
		t.Fatalf("expected viewport top %d from rendered view, got %d", expectedTop, top)
	}
}

func TestConversationViewportTopFromRenderedViewCorrectsWrongExpectedTop(t *testing.T) {
	input := textarea.New()
	input.Focus()

	m := model{
		screen:     screenChat,
		width:      100,
		height:     30,
		input:      input,
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
	}
	m.resize()
	m.viewport.SetContent("__TOP_SHIFT_TOKEN__\nline two\nline three")

	vpLines := strings.Split(strings.ReplaceAll(m.viewport.View(), "\r\n", "\n"), "\n")
	tokenRowInViewport := -1
	for i, line := range vpLines {
		if strings.Contains(xansi.Strip(line), "__TOP_SHIFT_TOKEN__") {
			tokenRowInViewport = i
			break
		}
	}
	if tokenRowInViewport < 0 {
		t.Fatalf("expected token row in viewport view")
	}

	fullLines := strings.Split(strings.ReplaceAll(m.View(), "\r\n", "\n"), "\n")
	tokenRowInView := -1
	for i, line := range fullLines {
		if strings.Contains(xansi.Strip(line), "__TOP_SHIFT_TOKEN__") {
			tokenRowInView = i
			break
		}
	}
	if tokenRowInView < 0 {
		t.Fatalf("expected token row in full view")
	}
	expectedTop := tokenRowInView - tokenRowInViewport

	left, _, layoutTop, _, ok := m.conversationViewportBoundsByLayout()
	if !ok {
		t.Fatalf("expected viewport layout bounds")
	}
	// Simulate a wrong layout estimate (for example, by 2 lines).
	top, found := m.conversationViewportTopFromRenderedView(left, layoutTop+2)
	if !found {
		t.Fatalf("expected rendered-panel top to be found")
	}
	if top != expectedTop {
		t.Fatalf("expected corrected top %d, got %d", expectedTop, top)
	}
}

func TestConversationViewportTopFromRenderedPanelWithLeadingBlankRows(t *testing.T) {
	input := textarea.New()
	input.Focus()

	m := model{
		screen:     screenChat,
		width:      100,
		height:     30,
		input:      input,
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
	}
	m.resize()
	m.viewport.SetContent("\n\n__TOP_BLANK_TOKEN__\nline two")

	vpLines := strings.Split(strings.ReplaceAll(m.viewport.View(), "\r\n", "\n"), "\n")
	tokenRowInViewport := -1
	for i, line := range vpLines {
		if strings.Contains(xansi.Strip(line), "__TOP_BLANK_TOKEN__") {
			tokenRowInViewport = i
			break
		}
	}
	if tokenRowInViewport < 0 {
		t.Fatalf("expected token row in viewport view")
	}

	fullLines := strings.Split(strings.ReplaceAll(m.View(), "\r\n", "\n"), "\n")
	tokenRowInView := -1
	for i, line := range fullLines {
		if strings.Contains(xansi.Strip(line), "__TOP_BLANK_TOKEN__") {
			tokenRowInView = i
			break
		}
	}
	if tokenRowInView < 0 {
		t.Fatalf("expected token row in full view")
	}

	left, _, layoutTop, _, ok := m.conversationViewportBoundsByLayout()
	if !ok {
		t.Fatalf("expected viewport layout bounds")
	}
	top, found := m.conversationViewportTopFromRenderedView(left, layoutTop)
	if !found {
		t.Fatalf("expected rendered-panel top to be found")
	}
	expectedTop := tokenRowInView - tokenRowInViewport
	if top != expectedTop {
		t.Fatalf("expected viewport top %d with leading blanks, got %d", expectedTop, top)
	}
}

func TestConversationViewportTopFromRenderedViewStableWithSelectionPreview(t *testing.T) {
	input := textarea.New()
	input.Focus()

	m := model{
		screen:     screenChat,
		width:      120,
		height:     34,
		input:      input,
		viewport:   viewport.New(70, 14),
		tokenUsage: newTokenUsageComponent(),
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "你好，你是谁？", Status: "final"},
			{Kind: "assistant", Title: assistantLabel, Body: "你好，我是 ByteMind，你的交互式 CLI 编码助手。", Status: "final"},
		},
	}
	m.resize()
	m.refreshViewport()

	left, _, layoutTop, _, ok := m.conversationViewportBoundsByLayout()
	if !ok {
		t.Fatalf("expected viewport layout bounds")
	}
	baseTop, baseFound := m.conversationViewportTopFromRenderedView(left, layoutTop)
	if !baseFound {
		t.Fatalf("expected base top from rendered view")
	}

	m.mouseSelecting = true
	m.mouseSelectionStart = viewportSelectionPoint{Row: 0, Col: 0}
	m.mouseSelectionEnd = viewportSelectionPoint{Row: 1, Col: 8}

	previewTop, previewFound := m.conversationViewportTopFromRenderedView(left, layoutTop)
	if !previewFound {
		t.Fatalf("expected preview top from rendered view")
	}
	if previewTop != baseTop {
		t.Fatalf("expected stable top with selection preview, base=%d preview=%d", baseTop, previewTop)
	}
}

func TestConversationViewportBoundsStableWhileSelectionPreviewActive(t *testing.T) {
	input := textarea.New()
	input.Focus()

	m := model{
		screen:     screenChat,
		width:      120,
		height:     34,
		input:      input,
		viewport:   viewport.New(70, 14),
		tokenUsage: newTokenUsageComponent(),
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "Hello, who are you?", Status: "final"},
			{Kind: "assistant", Title: assistantLabel, Body: "Hello, I am ByteMind, your interactive CLI coding assistant.", Status: "final"},
		},
		mouseSelecting:      true,
		mouseSelectionStart: viewportSelectionPoint{Row: 0, Col: 0},
		mouseSelectionEnd:   viewportSelectionPoint{Row: 4, Col: 12},
	}
	m.resize()
	m.refreshViewport()

	_, _, topBefore, _, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatalf("expected viewport bounds")
	}

	m.mouseSelectionEnd = viewportSelectionPoint{Row: 6, Col: 16}
	_, _, topAfter, _, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatalf("expected viewport bounds after updating selection")
	}
	if topAfter != topBefore {
		t.Fatalf("expected viewport top stable during selection preview, before=%d after=%d", topBefore, topAfter)
	}
}

func TestConversationViewportBoundsByLayoutMatchesRenderedViewTop(t *testing.T) {
	input := textarea.New()
	input.Focus()

	m := model{
		screen:     screenChat,
		width:      120,
		height:     34,
		input:      input,
		viewport:   viewport.New(70, 14),
		tokenUsage: newTokenUsageComponent(),
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "hello", Status: "final"},
			{Kind: "assistant", Title: assistantLabel, Body: "line-1\nline-2\nline-3\nline-4", Status: "final"},
		},
	}
	m.resize()
	m.refreshViewport()

	left, _, top, _, ok := m.conversationViewportBoundsByLayout()
	if !ok {
		t.Fatalf("expected viewport layout bounds")
	}
	renderedTop, found := m.conversationViewportTopFromRenderedView(left, top)
	if !found {
		t.Fatalf("expected top from rendered view")
	}
	if top != renderedTop {
		t.Fatalf("expected layout top %d to match rendered top %d", top, renderedTop)
	}

	m.mouseSelecting = true
	m.mouseSelectionStart = viewportSelectionPoint{Row: 0, Col: 0}
	m.mouseSelectionEnd = viewportSelectionPoint{Row: 2, Col: 6}

	left, _, top, _, ok = m.conversationViewportBoundsByLayout()
	if !ok {
		t.Fatalf("expected viewport layout bounds with selection preview")
	}
	renderedTop, found = m.conversationViewportTopFromRenderedView(left, top)
	if !found {
		t.Fatalf("expected top from rendered view with selection preview")
	}
	if top != renderedTop {
		t.Fatalf("expected layout top %d to match rendered top %d while selecting", top, renderedTop)
	}
}

func TestRenderFooterOmitsMouseDebugLine(t *testing.T) {
	input := textarea.New()
	input.Focus()

	m := model{
		screen:     screenChat,
		width:      120,
		height:     30,
		input:      input,
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
	}

	footer := m.renderFooter()
	if strings.Contains(footer, "Mouse:") {
		t.Fatalf("expected footer to omit mouse debug line, got %q", footer)
	}
}

func TestConversationViewportBoundsByLayoutStartsAtPanelLeft(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		screen:     screenChat,
		width:      120,
		height:     30,
		input:      input,
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
	}
	m.resize()

	left, right, _, _, ok := m.conversationViewportBoundsByLayout()
	if !ok {
		t.Fatalf("expected viewport bounds")
	}
	panelLeft := panelStyle.GetHorizontalFrameSize() / 2
	if left != panelLeft {
		t.Fatalf("expected left bound %d, got %d", panelLeft, left)
	}
	if right-left+1 != m.viewport.Width {
		t.Fatalf("expected viewport width %d from bounds, got %d", m.viewport.Width, right-left+1)
	}
}
