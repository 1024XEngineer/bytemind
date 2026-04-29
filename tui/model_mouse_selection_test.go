package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestHandleMouseDragSelectionAutoScrollsBeyondViewport(t *testing.T) {
	input := textarea.New()
	input.Focus()

	lines := make([]string, 0, 20)
	for i := 0; i < 20; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	content := strings.Join(lines, "\n")

	m := model{
		screen:               screenChat,
		width:                120,
		height:               30,
		input:                input,
		viewport:             viewport.New(40, 4),
		tokenUsage:           newTokenUsageComponent(),
		viewportContentCache: content,
	}
	m.viewport.SetContent(content)
	m.copyView = viewport.New(40, 4)
	m.copyView.SetContent(content)

	left, _, top, bottom, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatalf("expected viewport bounds")
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      left,
		Y:      top,
	})
	pressed := got.(model)

	got, _ = pressed.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
		X:      left,
		Y:      bottom + 3,
	})
	dragged := got.(model)
	if dragged.viewport.YOffset <= 0 {
		t.Fatalf("expected drag motion outside viewport to auto-scroll down, got offset %d", dragged.viewport.YOffset)
	}

	got, _ = dragged.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      left,
		Y:      bottom + 3,
	})
	released := got.(model)
	if !released.mouseSelectionActive {
		t.Fatalf("expected selection to remain active after auto-scroll drag release")
	}

	selected := released.viewportSelectionText()
	if !strings.Contains(selected, "line-00") || !strings.Contains(selected, "line-06") {
		t.Fatalf("expected selection to span into scrolled rows, got %q", selected)
	}
}

func TestPasteIDAtViewportPointFindsExpandedPasteBodyRows(t *testing.T) {
	m := model{
		viewportContentCache: strings.Join([]string{
			"You",
			"[Paste #1 ~12 lines] [preview]",
			"│ line1",
			"│ line2",
			"... (10 more lines, click again for full, Ctrl+E expand all)",
		}, "\n"),
	}

	for _, row := range []int{1, 2, 3, 4} {
		got := m.pasteIDAtViewportPoint(viewportSelectionPoint{Row: row, Col: 2})
		if got != "1" {
			t.Fatalf("expected row %d to resolve to paste 1, got %q", row, got)
		}
	}
}

func TestMouseSelectionScrollTickAutoScrollsWhileHoldingAtBottomEdge(t *testing.T) {
	input := textarea.New()
	input.Focus()

	lines := make([]string, 0, 24)
	for i := 0; i < 24; i++ {
		lines = append(lines, fmt.Sprintf("line-%02d", i))
	}
	content := strings.Join(lines, "\n")

	m := model{
		screen:               screenChat,
		width:                120,
		height:               30,
		input:                input,
		viewport:             viewport.New(40, 4),
		tokenUsage:           newTokenUsageComponent(),
		viewportContentCache: content,
	}
	m.viewport.SetContent(content)
	m.copyView = viewport.New(40, 4)
	m.copyView.SetContent(content)

	left, _, top, bottom, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatalf("expected viewport bounds")
	}

	got, pressCmd := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      left,
		Y:      top,
	})
	if pressCmd == nil {
		t.Fatalf("expected press to arm selection auto-scroll ticker")
	}
	pressed := got.(model)

	got, _ = pressed.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
		X:      left,
		Y:      bottom,
	})
	dragged := got.(model)
	beforeOffset := dragged.viewport.YOffset
	beforeEnd := dragged.mouseSelectionEnd

	tickedModel, tickCmd := dragged.Update(mouseSelectionScrollTickMsg{ID: dragged.mouseSelectionTickID})
	if tickCmd == nil {
		t.Fatalf("expected selection auto-scroll tick to continue scheduling while dragging")
	}
	scrolled := tickedModel.(model)
	if scrolled.viewport.YOffset <= beforeOffset {
		t.Fatalf("expected held drag at bottom edge to auto-scroll down, before=%d after=%d", beforeOffset, scrolled.viewport.YOffset)
	}
	if scrolled.mouseSelectionEnd.Row <= beforeEnd.Row {
		t.Fatalf("expected selection end row to extend after auto-scroll, before=%d after=%d", beforeEnd.Row, scrolled.mouseSelectionEnd.Row)
	}
}

func TestHandleMousePressInInputDoesNotStartViewportSelection(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.ShowLineNumbers = false
	input.Prompt = ""
	input.SetWidth(40)
	input.SetHeight(3)

	m := model{
		screen:     screenChat,
		width:      100,
		height:     26,
		input:      input,
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
	}
	m.viewport.SetContent(strings.Join([]string{
		"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
	}, "\n"))

	inputY := -1
	for y := 0; y < m.height; y++ {
		if m.mouseOverInput(y) {
			inputY = y
			break
		}
	}
	if inputY < 0 {
		t.Fatalf("expected to find input area")
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      4,
		Y:      inputY,
	})
	updated := got.(model)
	if updated.mouseSelecting {
		t.Fatalf("expected input press to not arm viewport selection")
	}
	if updated.mouseSelectionActive {
		t.Fatalf("expected input press to keep selection inactive")
	}
	if !updated.inputMouseSelecting {
		t.Fatalf("expected input press to arm input selection")
	}
}

func TestHandleMouseDragInInputCanBeCopiedWithCtrlC(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.ShowLineNumbers = false
	input.Prompt = ""
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("hello bytemind")

	writer := &fakeClipboardTextWriter{}
	m := model{
		screen:        screenChat,
		width:         100,
		height:        26,
		input:         input,
		viewport:      viewport.New(60, 10),
		tokenUsage:    newTokenUsageComponent(),
		clipboardText: writer,
	}
	m.viewport.SetContent("a\nb\nc")

	inputY := -1
	for y := 0; y < m.height; y++ {
		if m.mouseOverInput(y) {
			inputY = y
			break
		}
	}
	if inputY < 0 {
		t.Fatalf("expected to find input area")
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      3,
		Y:      inputY,
	})
	pressed := got.(model)
	if !pressed.inputMouseSelecting {
		t.Fatalf("expected input selection to start")
	}

	got, _ = pressed.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
		X:      7,
		Y:      inputY,
	})
	dragged := got.(model)

	got, _ = dragged.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      7,
		Y:      inputY,
	})
	released := got.(model)
	if !released.inputSelectionActive {
		t.Fatalf("expected input selection to remain active after release")
	}

	got, cmd := released.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := got.(model)
	if cmd == nil {
		t.Fatalf("expected ctrl+c copy command for toast scheduling")
	}
	if strings.TrimSpace(writer.last) == "" {
		t.Fatalf("expected copied input selection text, got empty")
	}
	if updated.inputSelectionActive {
		t.Fatalf("expected successful copy to clear input selection")
	}
}

func TestLandingInputSelectionMapsToRenderedInputRow(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.ShowLineNumbers = false
	input.Prompt = ""
	input.SetWidth(40)
	input.SetHeight(2)
	input.SetValue("nihao")

	writer := &fakeClipboardTextWriter{}
	m := model{
		screen:        screenLanding,
		width:         110,
		height:        34,
		input:         input,
		tokenUsage:    newTokenUsageComponent(),
		clipboardText: writer,
	}
	targetRow, startX, dragX := locateLandingInputDragPoints(t, m)

	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      startX,
		Y:      targetRow,
	})
	pressed := got.(model)

	got, _ = pressed.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
		X:      dragX,
		Y:      targetRow,
	})
	dragged := got.(model)

	got, _ = dragged.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      dragX,
		Y:      targetRow,
	})
	released := got.(model)
	if !released.inputSelectionActive {
		t.Fatalf("expected landing input selection to become active")
	}

	got, _ = released.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := got.(model)
	if writer.last != "ni" {
		t.Fatalf("expected landing input copy %q, got %q", "ni", writer.last)
	}
	if updated.inputSelectionActive || updated.inputMouseSelecting {
		t.Fatalf("expected copy to clear landing input selection state")
	}
}

func TestInputPointFromMouseZoneAutoProbeRecoversNearTopMiss(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.ShowLineNumbers = false
	input.Prompt = ""
	input.SetWidth(40)
	input.SetHeight(2)
	input.SetValue("nihao")

	m := model{
		screen:     screenLanding,
		width:      110,
		height:     34,
		input:      input,
		tokenUsage: newTokenUsageComponent(),
	}

	view := m.View()
	lines := strings.Split(strings.ReplaceAll(view, "\r\n", "\n"), "\n")
	targetRow, targetCol := -1, -1
	for row, line := range lines {
		plain := xansi.Strip(line)
		byteCol := strings.Index(plain, "nihao")
		if byteCol >= 0 {
			targetRow = row
			targetCol = xansi.StringWidth(plain[:byteCol])
			break
		}
	}
	if targetRow < 1 || targetCol < 0 {
		t.Fatalf("expected to locate landing input text in rendered view")
	}

	point, ok := m.inputPointFromMouse(targetCol, targetRow-1, false)
	if !ok {
		t.Fatalf("expected zone auto-probe to recover near-top miss for landing input")
	}
	if point.Row != 0 {
		t.Fatalf("expected recovered landing input row 0, got %d", point.Row)
	}
}

func TestLandingInputSelectionIgnoresGlobalMouseYOffset(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.ShowLineNumbers = false
	input.Prompt = ""
	input.SetWidth(40)
	input.SetHeight(2)
	input.SetValue("nihao")

	writer := &fakeClipboardTextWriter{}
	m := model{
		screen:        screenLanding,
		width:         110,
		height:        34,
		input:         input,
		tokenUsage:    newTokenUsageComponent(),
		clipboardText: writer,
		mouseYOffset:  2,
	}
	targetRow, startX, dragX := locateLandingInputDragPoints(t, m)

	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      startX,
		Y:      targetRow,
	})
	pressed := got.(model)
	got, _ = pressed.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		Button: tea.MouseButtonLeft,
		X:      dragX,
		Y:      targetRow,
	})
	dragged := got.(model)
	got, _ = dragged.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      dragX,
		Y:      targetRow,
	})
	released := got.(model)
	if !released.inputSelectionActive {
		t.Fatalf("expected landing input selection to become active")
	}

	got, _ = released.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if writer.last != "ni" {
		t.Fatalf("expected landing input copy %q with global y-offset, got %q", "ni", writer.last)
	}
}

func locateLandingInputDragPoints(t *testing.T, m model) (targetRow, startX, dragX int) {
	t.Helper()
	_ = m.View()

	left, right, top, bottom, _, _, ok := m.inputInnerBounds()
	if !ok {
		t.Fatalf("expected landing input bounds to be available")
	}

	for y := top; y <= bottom; y++ {
		col0 := -1
		col1 := -1
		for x := left; x <= right; x++ {
			point, ok := m.inputPointFromMouse(x, y, false)
			if !ok || point.Row != 0 {
				continue
			}
			if point.Col == 0 && col0 < 0 {
				col0 = x
			}
			if point.Col >= 1 && col1 < 0 {
				col1 = x
			}
		}
		if col0 >= 0 && col1 > col0 {
			return y, col0, col1
		}
	}

	t.Fatalf("expected to locate stable landing input drag points")
	return 0, 0, 0
}

func TestLandingInputBoundsAlignWithLandingCanvasOffset(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.ShowLineNumbers = false
	input.Prompt = ""
	input.SetValue("hello")

	m := model{
		screen:     screenLanding,
		width:      110,
		height:     34,
		input:      input,
		tokenUsage: newTokenUsageComponent(),
	}
	_ = m.View()

	_, _, top, _, _, _, ok := m.inputInnerBounds()
	if !ok {
		t.Fatalf("expected landing input bounds to be available")
	}
	contentTop := m.landingContentTop(m.landingContentHeight())
	wantTop := m.landingInputTop(contentTop)
	if top != wantTop {
		t.Fatalf("expected landing input top %d, got %d", wantTop, top)
	}
	if !m.mouseOverLandingInput(top) {
		t.Fatalf("expected mouseOverLandingInput to include top row %d", top)
	}
	if top > 0 && m.mouseOverLandingInput(top-1) {
		t.Fatalf("expected row above input top to be outside hit area")
	}
}

func TestViewportSelectionTextUsesVisibleViewportLayout(t *testing.T) {
	m := model{
		viewport: viewport.New(32, 6),
		copyView: viewport.New(32, 6),
	}
	m.viewport.SetContent("You\nalpha line")
	m.copyView.SetContent("alpha line\nbeta line")
	m.mouseSelectionStart = viewportSelectionPoint{Row: 1, Col: 0}
	m.mouseSelectionEnd = viewportSelectionPoint{Row: 1, Col: 4}

	got := m.viewportSelectionText()
	if got != "alpha" {
		t.Fatalf("expected viewport-aligned selection %q, got %q", "alpha", got)
	}
}

func TestViewportSelectionTextUsesCellCoordinatesForWideRunes(t *testing.T) {
	m := model{
		viewport: viewport.New(32, 6),
	}
	m.viewport.SetContent("你好世界")
	m.mouseSelectionStart = viewportSelectionPoint{Row: 0, Col: 0}
	m.mouseSelectionEnd = viewportSelectionPoint{Row: 0, Col: 3}

	got := m.viewportSelectionText()
	if got != "你好" {
		t.Fatalf("expected wide-rune selection %q, got %q", "你好", got)
	}
}

func TestViewportSelectionTextUsesANSICellCuts(t *testing.T) {
	styled := lipgloss.NewStyle().Foreground(lipgloss.Color("#6CB6FF")).Render("你好世界")
	m := model{
		viewport: viewport.New(32, 6),
	}
	m.viewport.SetContent(styled)
	m.mouseSelectionStart = viewportSelectionPoint{Row: 0, Col: 0}
	m.mouseSelectionEnd = viewportSelectionPoint{Row: 0, Col: 3}

	got := m.viewportSelectionText()
	if got != "你好" {
		t.Fatalf("expected ANSI-aware wide-rune selection %q, got %q", "你好", got)
	}
}

func TestViewportPointFromMouseMatchesRenderedConversationCoordinates(t *testing.T) {
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
	m.viewport.SetContent("__MAP_TOKEN__\nline two")

	view := m.View()
	lines := strings.Split(strings.ReplaceAll(view, "\r\n", "\n"), "\n")
	targetRow, targetCol := -1, -1
	for row, line := range lines {
		plain := xansi.Strip(line)
		byteCol := strings.Index(plain, "__MAP_TOKEN__")
		if byteCol >= 0 {
			targetRow = row
			targetCol = xansi.StringWidth(plain[:byteCol])
			break
		}
	}
	if targetRow < 0 || targetCol < 0 {
		t.Fatalf("expected to locate token in rendered view")
	}

	point, ok := m.viewportPointFromMouse(targetCol, targetRow)
	if !ok {
		t.Fatalf("expected token screen coordinate to map into viewport")
	}
	if point.Row != 0 || point.Col != 0 {
		t.Fatalf("expected token to map to viewport row=0 col=0, got row=%d col=%d", point.Row, point.Col)
	}
}

func TestViewportPointFromMouseZoneAutoProbeRecoversNearTopMiss(t *testing.T) {
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
	m.viewport.SetContent("__ZONE_AUTO_TOKEN__\nline two")

	view := m.View()
	lines := strings.Split(strings.ReplaceAll(view, "\r\n", "\n"), "\n")
	targetRow, targetCol := -1, -1
	for row, line := range lines {
		plain := xansi.Strip(line)
		byteCol := strings.Index(plain, "__ZONE_AUTO_TOKEN__")
		if byteCol >= 0 {
			targetRow = row
			targetCol = xansi.StringWidth(plain[:byteCol])
			break
		}
	}
	if targetRow < 2 || targetCol < 0 {
		t.Fatalf("expected token row >= 2 and valid col, got row=%d col=%d", targetRow, targetCol)
	}

	point, ok := m.viewportPointFromMouse(targetCol, targetRow-2)
	if !ok {
		t.Fatalf("expected zone auto probe to recover near-top miss")
	}
	if point.Row != 0 || point.Col != 0 {
		t.Fatalf("expected recovered point row=0 col=0, got row=%d col=%d", point.Row, point.Col)
	}
}

func TestViewportPointFromMouseKeepsExactBlankRowFromMouse(t *testing.T) {
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
	m.viewport.SetContent("\n\ntarget line")

	left, _, top, _, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatalf("expected viewport bounds")
	}
	point, ok := m.viewportPointFromMouse(left, top)
	if !ok {
		t.Fatalf("expected mouse point to resolve")
	}
	if point.Row != 0 {
		t.Fatalf("expected blank-row click to keep exact row 0, got %d", point.Row)
	}
}

func TestViewportPointFromMouseKeepsRowWhenClickingTextLineTrailingSpace(t *testing.T) {
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
	m.viewport.SetContent("title\nshort text\n\nnext line")

	left, _, top, _, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatalf("expected viewport bounds")
	}
	// Click the second line at far-right padding area; row should stay at 1.
	point, ok := m.viewportPointFromMouse(left+m.viewport.Width-1, top+1)
	if !ok {
		t.Fatalf("expected mouse point to resolve")
	}
	if point.Row != 1 {
		t.Fatalf("expected trailing-space click to stay on row 1, got %d", point.Row)
	}
}

func TestViewportPointFromMouseKeepsHeadingRowWhenHeadingColumnIsBlank(t *testing.T) {
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
	m.viewport.SetContent("Bytemind\n\n你好，我是 ByteMind，你的交互式 CLI 编码助手。")

	left, _, top, _, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatalf("expected viewport bounds")
	}
	// Click the heading row using a column where heading is blank but body has text.
	// Row mapping should remain stable and not jump to nearby body lines.
	point, ok := m.viewportPointFromMouse(left+24, top)
	if !ok {
		t.Fatalf("expected mouse point to resolve")
	}
	if point.Row != 0 {
		t.Fatalf("expected heading-blank column click to stay on heading row 0, got %d", point.Row)
	}
}

func TestViewportPointFromMouseMatchesAssistantBodyLineInRenderedView(t *testing.T) {
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

	needle := "交互式 CLI 编码助手"

	vpLines := strings.Split(strings.ReplaceAll(m.viewport.View(), "\r\n", "\n"), "\n")
	expectedRow := -1
	for i, line := range vpLines {
		if strings.Contains(xansi.Strip(line), needle) {
			expectedRow = i
			break
		}
	}
	if expectedRow < 0 {
		t.Fatalf("expected to find assistant body needle in viewport")
	}

	fullLines := strings.Split(strings.ReplaceAll(m.View(), "\r\n", "\n"), "\n")
	screenRow := -1
	screenCol := -1
	for i, line := range fullLines {
		plain := xansi.Strip(line)
		byteCol := strings.Index(plain, needle)
		if byteCol >= 0 {
			screenRow = i
			screenCol = xansi.StringWidth(plain[:byteCol])
			break
		}
	}
	if screenRow < 0 {
		t.Fatalf("expected to find assistant body needle in full view")
	}

	point, ok := m.viewportPointFromMouse(screenCol, screenRow)
	if !ok {
		t.Fatalf("expected mouse coordinate to map into viewport")
	}
	if point.Row != expectedRow {
		t.Fatalf("expected body row %d, got %d", expectedRow, point.Row)
	}
}

func TestViewportPointFromMouseMatchesAssistantBodyLineWhileSelectionPreviewActive(t *testing.T) {
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
		mouseSelecting:      true,
		mouseSelectionStart: viewportSelectionPoint{Row: 0, Col: 0},
		mouseSelectionEnd:   viewportSelectionPoint{Row: 1, Col: 6},
	}
	m.resize()
	m.refreshViewport()

	needle := "交互式 CLI 编码助手"

	vpLines := strings.Split(strings.ReplaceAll(m.viewport.View(), "\r\n", "\n"), "\n")
	expectedRow := -1
	for i, line := range vpLines {
		if strings.Contains(xansi.Strip(line), needle) {
			expectedRow = i
			break
		}
	}
	if expectedRow < 0 {
		t.Fatalf("expected to find assistant body needle in viewport")
	}

	fullLines := strings.Split(strings.ReplaceAll(m.View(), "\r\n", "\n"), "\n")
	screenRow := -1
	screenCol := -1
	for i, line := range fullLines {
		plain := xansi.Strip(line)
		byteCol := strings.Index(plain, needle)
		if byteCol >= 0 {
			screenRow = i
			screenCol = xansi.StringWidth(plain[:byteCol])
			break
		}
	}
	if screenRow < 0 {
		t.Fatalf("expected to find assistant body needle in full view")
	}

	point, ok := m.viewportPointFromMouse(screenCol, screenRow)
	if !ok {
		t.Fatalf("expected mouse coordinate to map into viewport")
	}
	if point.Row != expectedRow {
		t.Fatalf("expected body row %d while selecting, got %d", expectedRow, point.Row)
	}
}

func TestViewportPointFromMouseMatchesMarkdownListBodyLineWhileSelectionPreviewActive(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		screen:     screenChat,
		width:      120,
		height:     36,
		input:      input,
		viewport:   viewport.New(70, 16),
		tokenUsage: newTokenUsageComponent(),
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "你好，你是谁？", Status: "final"},
			{Kind: "assistant", Title: assistantLabel, Body: strings.Join([]string{
				"你好，我是 ByteMind，你的交互式 CLI 编码助手。",
				"我可以帮你：",
				"",
				"- 阅读和理解仓库代码",
				"- 实现功能、修复 Bug",
				"- 做代码审查和问题定位",
			}, "\n"), Status: "final"},
		},
		mouseSelecting:      true,
		mouseSelectionStart: viewportSelectionPoint{Row: 0, Col: 0},
		mouseSelectionEnd:   viewportSelectionPoint{Row: 4, Col: 10},
	}
	m.resize()
	m.refreshViewport()

	needle := "阅读和理解仓库代码"

	vpLines := strings.Split(strings.ReplaceAll(m.viewport.View(), "\r\n", "\n"), "\n")
	expectedRow := -1
	for i, line := range vpLines {
		if strings.Contains(xansi.Strip(line), needle) {
			expectedRow = i
			break
		}
	}
	if expectedRow < 0 {
		t.Fatalf("expected to find markdown body needle in viewport")
	}

	fullLines := strings.Split(strings.ReplaceAll(m.View(), "\r\n", "\n"), "\n")
	screenRow := -1
	screenCol := -1
	for i, line := range fullLines {
		plain := xansi.Strip(line)
		byteCol := strings.Index(plain, needle)
		if byteCol >= 0 {
			screenRow = i
			screenCol = xansi.StringWidth(plain[:byteCol])
			break
		}
	}
	if screenRow < 0 {
		t.Fatalf("expected to find markdown body needle in full view")
	}

	point, ok := m.viewportPointFromMouse(screenCol, screenRow)
	if !ok {
		t.Fatalf("expected mouse coordinate to map into viewport")
	}
	if point.Row != expectedRow {
		t.Fatalf("expected markdown body row %d while selecting, got %d", expectedRow, point.Row)
	}
}

func TestRenderConversationViewportShowsHighlightAfterSelection(t *testing.T) {
	m := model{
		viewport:             viewport.New(16, 3),
		copyView:             viewport.New(16, 3),
		mouseSelectionActive: true,
		mouseSelectionStart:  viewportSelectionPoint{Row: 0, Col: 0},
		mouseSelectionEnd:    viewportSelectionPoint{Row: 0, Col: 4},
	}
	m.viewport.SetContent("You\nalpha line\nbeta line")
	m.copyView.SetContent("alpha line\nbeta line")

	got := m.renderConversationViewport()
	if !strings.Contains(got, "You") || !strings.Contains(got, "alpha line") || !strings.Contains(got, "beta line") {
		t.Fatalf("expected selection rendering to preserve visible content, got %q", got)
	}
}

func TestRenderConversationViewportHighlightsWhileDraggingAfterRangeExists(t *testing.T) {
	m := model{
		viewport:            viewport.New(16, 3),
		mouseSelecting:      true,
		mouseSelectionStart: viewportSelectionPoint{Row: 0, Col: 0},
		mouseSelectionEnd:   viewportSelectionPoint{Row: 0, Col: 4},
	}
	m.viewport.SetContent("alpha line\nbeta line")

	got := m.renderConversationViewport()
	if !strings.Contains(got, "alpha line") || !strings.Contains(got, "beta line") {
		t.Fatalf("expected dragging selection rendering to preserve visible content, got %q", got)
	}
}

func TestRenderInputSelectionPreviewHighlightsSelectedCells(t *testing.T) {
	m := model{
		inputSelectionActive: true,
		inputSelectionStart:  viewportSelectionPoint{Row: 0, Col: 0},
		inputSelectionEnd:    viewportSelectionPoint{Row: 0, Col: 1},
	}
	got := m.renderInputSelectionPreview("nihao")
	if xansi.Strip(got) != "nihao" {
		t.Fatalf("expected preview to preserve text, got %q", xansi.Strip(got))
	}
	if !strings.Contains(got, selectionHighlightStyle.Render("ni")) {
		t.Fatalf("expected preview to highlight selected span, got %q", got)
	}
}

func TestInputPointFromMouseClampToBoundsMapsToLineEnd(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.ShowLineNumbers = false
	input.Prompt = ""
	input.SetWidth(40)
	input.SetHeight(2)
	input.SetValue("nihao")

	m := model{
		screen:     screenLanding,
		width:      110,
		height:     34,
		input:      input,
		tokenUsage: newTokenUsageComponent(),
	}
	view := m.View()
	lines := strings.Split(strings.ReplaceAll(view, "\r\n", "\n"), "\n")
	targetRow, targetCol := -1, -1
	for row, line := range lines {
		plain := xansi.Strip(line)
		byteCol := strings.Index(plain, "nihao")
		if byteCol >= 0 {
			targetRow = row
			targetCol = xansi.StringWidth(plain[:byteCol])
			break
		}
	}
	if targetRow < 0 || targetCol < 0 {
		t.Fatalf("expected to locate landing input text in rendered view")
	}

	outsideX := m.width + 50
	if _, ok := m.inputPointFromMouse(outsideX, targetRow, false); ok {
		t.Fatalf("expected clamp=false outside point to be rejected")
	}
	point, ok := m.inputPointFromMouse(outsideX, targetRow, true)
	if !ok {
		t.Fatalf("expected clamp=true outside point to clamp into zone")
	}
	sourceLines := m.inputSelectionSourceLines("")
	if point.Row < 0 || point.Row >= len(sourceLines) {
		t.Fatalf("expected clamped point row to stay within rendered lines, got row=%d", point.Row)
	}
	lineWidth := xansi.StringWidth(sourceLines[point.Row])
	wantCol := max(0, lineWidth-1)
	if point.Row != 0 || point.Col != wantCol {
		t.Fatalf("expected clamped point on first row at rendered line end, got row=%d col=%d wantCol=%d", point.Row, point.Col, wantCol)
	}
}

func TestInputPointFromMouseOutsideHonorsClampFlag(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.ShowLineNumbers = false
	input.Prompt = ""
	input.SetWidth(40)
	input.SetHeight(2)
	input.SetValue("nihao")

	m := model{
		screen:     screenLanding,
		width:      110,
		height:     34,
		input:      input,
		tokenUsage: newTokenUsageComponent(),
	}
	view := m.View()
	lines := strings.Split(strings.ReplaceAll(view, "\r\n", "\n"), "\n")
	targetRow, targetCol := -1, -1
	for row, line := range lines {
		plain := xansi.Strip(line)
		byteCol := strings.Index(plain, "nihao")
		if byteCol >= 0 {
			targetRow = row
			targetCol = xansi.StringWidth(plain[:byteCol])
			break
		}
	}
	if targetRow < 0 || targetCol < 0 {
		t.Fatalf("expected to locate landing input text in rendered view")
	}

	outsideX := m.width + 50
	if _, ok := m.inputPointFromMouse(outsideX, targetRow, false); ok {
		t.Fatalf("expected clamp=false mouse outside to be rejected")
	}
	if _, ok := m.inputPointFromMouse(outsideX, targetRow, true); !ok {
		t.Fatalf("expected clamp=true mouse outside to be accepted")
	}
}

func TestCopyCurrentSelectionEmptyClearsSelectionState(t *testing.T) {
	m := model{
		mouseSelectionActive: true,
		mouseSelectionStart:  viewportSelectionPoint{Row: 0, Col: 0},
		mouseSelectionEnd:    viewportSelectionPoint{Row: 0, Col: 0},
		inputSelectionActive: true,
		inputSelectionStart:  viewportSelectionPoint{Row: 0, Col: 0},
		inputSelectionEnd:    viewportSelectionPoint{Row: 0, Col: 0},
	}
	if cmd := m.copyCurrentSelection(); cmd != nil {
		t.Fatalf("expected empty selection copy to return nil cmd")
	}
	if m.statusNote != "Selection is empty." {
		t.Fatalf("expected empty selection status note, got %q", m.statusNote)
	}
	if m.mouseSelectionActive || m.inputSelectionActive || m.inputMouseSelecting || m.mouseSelecting {
		t.Fatalf("expected empty selection copy to clear selection state")
	}
}

func TestHandleMouseSelectionScrollTickWithoutRangeOnlySchedulesNextTick(t *testing.T) {
	m := model{
		mouseSelecting:       true,
		mouseSelectionTickID: 3,
		mouseSelectionStart:  viewportSelectionPoint{Row: 2, Col: 5},
		mouseSelectionEnd:    viewportSelectionPoint{Row: 2, Col: 5},
		viewport:             viewport.New(40, 4),
	}
	beforeOffset := m.viewport.YOffset
	updatedModel, cmd := m.handleMouseSelectionScrollTick(mouseSelectionScrollTickMsg{ID: 3})
	updated := updatedModel.(model)
	if cmd == nil {
		t.Fatalf("expected tick to schedule the next cycle even without a range")
	}
	if updated.viewport.YOffset != beforeOffset {
		t.Fatalf("expected no autoscroll when selection range is empty, before=%d after=%d", beforeOffset, updated.viewport.YOffset)
	}
}
