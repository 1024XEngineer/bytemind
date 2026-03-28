package tui

import (
	"strings"
	"testing"
	"time"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func TestHandleMouseScrollsViewport(t *testing.T) {
	m := model{
		screen: screenChat,
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 5)
			vp.SetContent(strings.Join([]string{
				"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
			}, "\n"))
			return vp
		}(),
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	updated := got.(model)
	if updated.viewport.YOffset == 0 {
		t.Fatalf("expected viewport to scroll down, got offset %d", updated.viewport.YOffset)
	}
}

func TestHandleMouseWheelUpScrollsViewport(t *testing.T) {
	m := model{
		screen: screenChat,
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 5)
			vp.SetContent(strings.Join([]string{
				"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
			}, "\n"))
			vp.LineDown(4)
			return vp
		}(),
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
	})
	updated := got.(model)
	if updated.viewport.YOffset >= m.viewport.YOffset {
		t.Fatalf("expected viewport to scroll up, got offset %d", updated.viewport.YOffset)
	}
}

func TestHandleMouseEnablesViewportMouseForwarding(t *testing.T) {
	m := model{
		screen: screenChat,
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 5)
			vp.SetContent(strings.Join([]string{
				"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
			}, "\n"))
			vp.MouseWheelEnabled = false
			vp.MouseWheelDelta = 0
			return vp
		}(),
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelDown,
		Action: tea.MouseActionPress,
	})
	updated := got.(model)
	if !updated.viewport.MouseWheelEnabled {
		t.Fatalf("expected mouse wheel support to be enabled for viewport updates")
	}
	if updated.viewport.MouseWheelDelta != scrollStep {
		t.Fatalf("expected viewport wheel delta %d, got %d", scrollStep, updated.viewport.MouseWheelDelta)
	}
	if updated.viewport.YOffset == 0 {
		t.Fatalf("expected mouse wheel to scroll viewport")
	}
}

func TestHandleMouseWheelScrollsInputWhenPointerIsOverInput(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("line1\nline2\nline3\nline4\nline5\nline6")
	input.CursorEnd()

	m := model{
		screen:    screenChat,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		width:     100,
		height:    24,
		input:     input,
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 5)
			vp.SetContent(strings.Join([]string{
				"1", "2", "3", "4", "5", "6", "7", "8", "9", "10",
			}, "\n"))
			vp.LineDown(2)
			return vp
		}(),
	}

	beforeLine := m.input.Line()
	beforeOffset := m.viewport.YOffset
	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
		Y:      20,
	})
	updated := got.(model)

	if updated.input.Line() >= beforeLine {
		t.Fatalf("expected input cursor to move up, got line %d -> %d", beforeLine, updated.input.Line())
	}
	if updated.viewport.YOffset != beforeOffset {
		t.Fatalf("expected conversation viewport to stay put, got offset %d -> %d", beforeOffset, updated.viewport.YOffset)
	}
}

func TestHandleMouseWheelScrollsLandingInputWhenPointerIsOverInput(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("line1\nline2\nline3\nline4\nline5\nline6")
	input.CursorEnd()

	m := model{
		screen: screenLanding,
		width:  100,
		height: 32,
		input:  input,
	}

	beforeLine := m.input.Line()
	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
		Y:      18,
	})
	updated := got.(model)

	if updated.input.Line() >= beforeLine {
		t.Fatalf("expected landing input cursor to move up, got line %d -> %d", beforeLine, updated.input.Line())
	}
}

func TestWindowSizeMsgUpdatesViewportDimensions(t *testing.T) {
	input := textarea.New()
	m := model{
		screen:    screenChat,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		input:     input,
	}

	got, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 36})
	updated := got.(model)

	if updated.viewport.Width <= 0 {
		t.Fatalf("expected viewport width to be updated, got %d", updated.viewport.Width)
	}
	if updated.viewport.Height <= 0 {
		t.Fatalf("expected viewport height to be updated, got %d", updated.viewport.Height)
	}
}

func TestSubmitPromptRecomputesInputWidthWhenEnteringChat(t *testing.T) {
	input := textarea.New()
	input.Focus()

	m := model{
		screen:    screenLanding,
		width:     120,
		height:    36,
		input:     input,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
	}
	m.syncLayoutForCurrentScreen()
	beforeWidth := lipgloss.Width(m.input.View())

	got, _ := m.submitPrompt("hello from landing")
	updated := got.(model)
	afterWidth := lipgloss.Width(updated.input.View())

	if updated.screen != screenChat {
		t.Fatalf("expected submit prompt to switch to chat screen")
	}
	if afterWidth <= beforeWidth {
		t.Fatalf("expected chat input width to expand after screen switch, got %d -> %d", beforeWidth, afterWidth)
	}
}

func TestChatViewOmitsRedundantChrome(t *testing.T) {
	input := textarea.New()
	m := model{
		screen:    screenChat,
		width:     120,
		height:    36,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		input:     input,
	}

	m.syncLayoutForCurrentScreen()
	view := m.View()

	for _, unwanted := range []string{
		"Conversation",
		"Bytemind TUI",
		"? help",
	} {
		if strings.Contains(view, unwanted) {
			t.Fatalf("did not expect chat view to contain %q", unwanted)
		}
	}
	for _, wanted := range []string{
		"/ commands",
		"Ctrl+L sessions",
		"Mouse wheel scroll",
		"Ctrl+C quit",
	} {
		if !strings.Contains(view, wanted) {
			t.Fatalf("expected chat view to contain %q", wanted)
		}
	}
	if strings.Contains(view, "PgUp/PgDn") {
		t.Fatalf("did not expect chat view to advertise PgUp/PgDn anymore")
	}
	if m.viewport.Height <= 20 {
		t.Fatalf("expected viewport height to stay roomy after removing header/footer text, got %d", m.viewport.Height)
	}
}

func TestRefreshViewportKeepsLatestMessagesVisible(t *testing.T) {
	input := textarea.New()
	m := model{
		screen:    screenChat,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		width:     100,
		height:    24,
		input:     input,
		chatItems: make([]chatEntry, 0, 12),
	}
	for i := 0; i < 12; i++ {
		m.chatItems = append(m.chatItems, chatEntry{
			Kind:   "user",
			Title:  "You",
			Body:   strings.Repeat("message ", 8),
			Status: "final",
		})
	}

	m.refreshViewport()

	if m.viewport.YOffset == 0 {
		t.Fatalf("expected viewport to follow latest content")
	}
}

func TestEnterSubmitsPrompt(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("ship this prompt")
	input.CursorEnd()

	m := model{
		screen:         screenChat,
		input:          input,
		workspace:      "E:\\bytemind",
		sess:           session.New("E:\\bytemind"),
		streamingIndex: -1,
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.chatItems) < 2 {
		t.Fatalf("expected enter to submit prompt, got %d chat items", len(updated.chatItems))
	}
	if updated.chatItems[0].Body != "ship this prompt" {
		t.Fatalf("expected submitted user prompt to match input, got %q", updated.chatItems[0].Body)
	}
}

func TestImmediateEnterAfterPasteDoesNotSubmit(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("first line")
	input.CursorEnd()

	m := model{
		screen:      screenChat,
		input:       input,
		workspace:   "E:\\bytemind",
		sess:        session.New("E:\\bytemind"),
		lastPasteAt: time.Now(),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.chatItems) != 0 {
		t.Fatalf("expected immediate enter after paste not to submit")
	}
	if updated.input.Value() != "first line\n" {
		t.Fatalf("expected pasted enter to stay inside input, got %q", updated.input.Value())
	}
}

func TestEnterSubmitsAfterPasteGuardExpires(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("first line\nsecond line")
	input.CursorEnd()

	m := model{
		screen:      screenChat,
		input:       input,
		workspace:   "E:\\bytemind",
		sess:        session.New("E:\\bytemind"),
		lastPasteAt: time.Now().Add(-time.Second),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.chatItems) < 2 {
		t.Fatalf("expected enter to submit after paste guard expires, got %d chat items", len(updated.chatItems))
	}
}

func TestHelpTextOnlyMentionsSupportedEntryPoints(t *testing.T) {
	text := model{}.helpText()

	for _, unwanted := range []string{
		"scripts\\install.ps1",
		"aicoding chat",
		"aicoding run",
		"/plan",
	} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("help text should not mention %q", unwanted)
		}
	}

	for _, wanted := range []string{
		"go run ./cmd/bytemind chat",
		"go run ./cmd/bytemind run -prompt",
		"/quit",
		"/new",
	} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("help text should mention %q", wanted)
		}
	}
}

func TestRenderFooterOnlyShowsInputRegion(t *testing.T) {
	input := textarea.New()
	m := model{
		width: 120,
		input: input,
	}

	footer := m.renderFooter()
	for _, unwanted := range []string{
		"Up/Down history",
		"Ctrl+Up/Down scroll",
		"? help",
		"Enter send",
		"Ctrl+N new session",
	} {
		if strings.Contains(footer, unwanted) {
			t.Fatalf("footer should not advertise %q", unwanted)
		}
	}
	for _, wanted := range []string{
		"/ commands",
		"Ctrl+L sessions",
		"Mouse wheel scroll",
		"Ctrl+C quit",
	} {
		if !strings.Contains(footer, wanted) {
			t.Fatalf("footer should advertise %q", wanted)
		}
	}
	if strings.Contains(footer, "PgUp/PgDn") {
		t.Fatalf("footer should not advertise PgUp/PgDn anymore")
	}
}

func TestCommandPaletteListsQuitCommand(t *testing.T) {
	found := false
	for _, item := range commandItems {
		if item.Name == "/quit" && item.Kind == "command" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected command palette to include /quit")
	}
}

func TestCommandPaletteDoesNotListExitAlias(t *testing.T) {
	for _, item := range commandItems {
		if item.Name == "/exit" {
			t.Fatalf("did not expect command palette to include /exit")
		}
	}
}

func TestCommandPaletteDoesNotListPlanCommands(t *testing.T) {
	for _, item := range commandItems {
		if strings.HasPrefix(item.Name, "/plan") || item.Group == "plan" {
			t.Fatalf("did not expect command palette to include plan item %+v", item)
		}
	}
}

func TestSlashOpensCommandPaletteWithPrefilledSlash(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		screen: screenChat,
		input:  input,
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	updated := got.(model)

	if !updated.commandOpen {
		t.Fatalf("expected slash to open command palette")
	}
	if updated.input.Value() != "/" {
		t.Fatalf("expected main input to start with '/', got %q", updated.input.Value())
	}
}

func TestFilteredCommandsShowsRootSelectorGroups(t *testing.T) {
	input := textarea.New()
	input.SetValue("/")
	m := model{input: input}

	items := m.filteredCommands()
	usages := make([]string, 0, len(items))
	for _, item := range items {
		usages = append(usages, item.Usage)
	}

	for _, want := range []string{"/help", "/new", "/quit"} {
		if !containsString(usages, want) {
			t.Fatalf("expected root selector to contain %q, got %v", want, usages)
		}
	}
	for _, unwanted := range []string{"/session", "/sessions [limit]", "/resume <id>", "/plan", "/plan add <step>"} {
		if containsString(usages, unwanted) {
			t.Fatalf("did not expect root selector to contain %q", unwanted)
		}
	}
}

func TestCommandPaletteFiltersAsUserTypes(t *testing.T) {
	input := textarea.New()
	input.SetValue("/h")
	m := model{input: input}

	items := m.filteredCommands()
	if len(items) != 1 || items[0].Name != "/help" {
		t.Fatalf("expected /h to only show /help, got %+v", items)
	}
}

func TestEscapeClosesCommandPalette(t *testing.T) {
	input := textarea.New()
	input.SetValue("/h")
	m := model{
		screen:      screenChat,
		commandOpen: true,
		input:       input,
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := got.(model)

	if updated.commandOpen {
		t.Fatalf("expected esc to close command palette")
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected main input to reset after esc, got %q", updated.input.Value())
	}
}

func TestCommandPaletteEnterOnQuitReturnsQuitCmd(t *testing.T) {
	input := textarea.New()
	input.SetValue("/quit")
	m := model{
		screen:      screenChat,
		commandOpen: true,
		input:       input,
	}
	m.syncCommandPalette()

	_, cmd := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatalf("expected /quit from command palette to return a quit command")
	}
}

func TestViewRendersCommandPaletteAsOverlaySection(t *testing.T) {
	input := textarea.New()
	input.SetValue("/")
	m := model{
		screen:      screenChat,
		width:       100,
		height:      30,
		input:       input,
		commandOpen: true,
		sess:        session.New("E:\\bytemind"),
		workspace:   "E:\\bytemind",
		cfg: config.Config{
			Provider:       config.ProviderConfig{Type: "openai-compatible", Model: "deepseek-chat"},
			ApprovalPolicy: "on-request",
			MaxIterations:  32,
		},
	}
	m.syncCommandPalette()

	view := m.View()
	if !strings.Contains(view, "/help") {
		t.Fatalf("expected slash command overlay to render, got %q", view)
	}
	if strings.Contains(view, "Conversation") {
		t.Fatalf("did not expect redundant conversation header in chat view")
	}
}

func TestLandingViewRendersCommandPaletteAboveInput(t *testing.T) {
	input := textarea.New()
	input.SetValue("/h")
	m := model{
		screen:      screenLanding,
		width:       100,
		height:      30,
		input:       input,
		commandOpen: true,
	}
	m.syncCommandPalette()

	view := m.View()
	if !strings.Contains(view, "Bytemind Chat") {
		t.Fatalf("expected landing view to remain visible, got %q", view)
	}
	if !strings.Contains(view, "/help") {
		t.Fatalf("expected landing slash menu to render, got %q", view)
	}
}

func TestCommandPaletteUsesCompactThreeRowList(t *testing.T) {
	input := textarea.New()
	input.SetValue("/")
	m := model{
		screen:      screenChat,
		width:       100,
		height:      30,
		input:       input,
		commandOpen: true,
	}

	m.syncCommandPalette()

	if len(m.visibleCommandItemsPage()) != 3 {
		t.Fatalf("expected command palette list height 3, got %d", len(m.visibleCommandItemsPage()))
	}
}

func TestCommandPaletteSupportsPageNavigation(t *testing.T) {
	original := commandItems
	commandItems = []commandItem{
		{Name: "/a", Usage: "/a", Description: "a"},
		{Name: "/b", Usage: "/b", Description: "b"},
		{Name: "/c", Usage: "/c", Description: "c"},
		{Name: "/d", Usage: "/d", Description: "d"},
		{Name: "/e", Usage: "/e", Description: "e"},
	}
	defer func() { commandItems = original }()

	m := model{
		commandOpen: true,
		input: func() textarea.Model {
			input := textarea.New()
			input.SetValue("/")
			return input
		}(),
	}
	m.syncCommandPalette()

	afterDown, _ := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyPgDown})
	downModel := afterDown.(model)
	if downModel.commandCursor != 3 {
		t.Fatalf("expected pgdown to move to next command page, got cursor %d", downModel.commandCursor)
	}
	page := downModel.visibleCommandItemsPage()
	if len(page) == 0 || page[0].Name != "/d" {
		t.Fatalf("expected second page to start with /d, got %+v", page)
	}

	afterUp, _ := downModel.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyPgUp})
	upModel := afterUp.(model)
	if upModel.commandCursor != 0 {
		t.Fatalf("expected pgup to move back to first command page, got cursor %d", upModel.commandCursor)
	}
}

func TestRenderCommandPaletteDoesNotCorruptChineseDescriptions(t *testing.T) {
	input := textarea.New()
	input.SetValue("/")
	m := model{
		screen:      screenChat,
		width:       80,
		input:       input,
		commandOpen: true,
	}
	m.syncCommandPalette()

	got := m.renderCommandPalette()
	if strings.Contains(got, string('\uFFFD')) {
		t.Fatalf("expected command palette not to contain replacement glyphs, got %q", got)
	}
	for _, want := range []string{"/help", "/new", "/quit"} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected command palette to contain %q, got %q", want, got)
		}
	}
}

func TestAssistantChatBubbleUsesFullAvailableWidth(t *testing.T) {
	width := 80
	assistantWidth := chatBubbleWidth(chatEntry{Kind: "assistant"}, width)
	if assistantWidth != width {
		t.Fatalf("expected assistant bubble width %d, got %d", width, assistantWidth)
	}

	userWidth := chatBubbleWidth(chatEntry{Kind: "user"}, width)
	if userWidth != width {
		t.Fatalf("expected user bubble width %d, got %d", width, userWidth)
	}
}

func TestRenderChatRowFitsViewportWidth(t *testing.T) {
	row := renderChatRow(chatEntry{
		Kind:   "user",
		Title:  "You",
		Body:   "Please describe the relationship between tui, session, agent, and tools in several paragraphs so we can inspect wrapping behavior.",
		Status: "final",
	}, 80)

	if lipgloss.Width(row) > 80 {
		t.Fatalf("expected rendered row to fit viewport width, got %d", lipgloss.Width(row))
	}
	if !strings.Contains(row, "Please describe the relationship") {
		t.Fatalf("expected rendered row to contain the full user message")
	}
}

func TestRenderConversationPreservesFullUserText(t *testing.T) {
	m := model{
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 10)
			vp.Width = 40
			return vp
		}(),
		chatItems: []chatEntry{
			{
				Kind:   "user",
				Title:  "You",
				Body:   "Please describe the relationship between tui, session, agent, and tools in several detailed sections.",
				Status: "final",
			},
		},
	}

	got := m.renderConversation()
	flat := strings.Join(strings.Fields(got), "")
	for _, want := range []string{"Pleasedescribetherelationship", "session,agent,andtools", "severaldetailedsections"} {
		if !strings.Contains(flat, want) {
			t.Fatalf("expected conversation to preserve %q, got %q", want, got)
		}
	}
}

func TestRenderConversationIncludesToolEntries(t *testing.T) {
	m := model{
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(60, 10)
			vp.Width = 60
			return vp
		}(),
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "check repo", Status: "final"},
			{Kind: "tool", Title: "Tool | read_file", Body: "Read internal/tui/model.go lines 1-20", Status: "done"},
		},
	}

	got := m.renderConversation()
	if !strings.Contains(got, "Tool | read_file") {
		t.Fatalf("expected conversation to show tool entry, got %q", got)
	}
	if !strings.Contains(got, "Read internal/tui/model.go lines 1-20") {
		t.Fatalf("expected conversation to show tool summary, got %q", got)
	}
}

func TestHandleAgentEventShowsToolProgressInChat(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "what project is this", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "thinking", Status: "thinking"},
		},
		streamingIndex: 1,
	}

	m.handleAgentEvent(agent.Event{
		Type:          agent.EventToolCallStarted,
		ToolName:      "read_file",
		ToolArguments: `{"path":"internal/tui/model.go"}`,
	})
	if len(m.chatItems) != 3 {
		t.Fatalf("expected tool start to keep assistant step then append tool call, got %d items", len(m.chatItems))
	}
	if m.chatItems[1].Kind != "assistant" || m.chatItems[1].Title != thinkingLabel || m.chatItems[1].Status != "thinking" || !strings.Contains(m.chatItems[1].Body, "read_file") {
		t.Fatalf("expected assistant step before tool call, got %+v", m.chatItems[1])
	}
	if m.chatItems[2].Kind != "tool" || m.chatItems[2].Status != "running" || !strings.Contains(m.chatItems[2].Title, "Tool Call | read_file") {
		t.Fatalf("expected running tool call chat item, got %+v", m.chatItems[2])
	}

	m.handleAgentEvent(agent.Event{
		Type:       agent.EventToolCallCompleted,
		ToolName:   "read_file",
		ToolResult: `{"path":"internal/tui/model.go","start_line":1,"end_line":20}`,
	})
	if len(m.chatItems) != 4 {
		t.Fatalf("expected completed tool to append tool result, got %d", len(m.chatItems))
	}
	if m.chatItems[2].Status != "running" {
		t.Fatalf("expected tool call entry to remain running history, got %q", m.chatItems[2].Status)
	}
	if m.chatItems[3].Kind != "tool" || !strings.Contains(m.chatItems[3].Title, "Tool Result | read_file") {
		t.Fatalf("expected tool result entry after tool call, got %+v", m.chatItems[3])
	}
	if m.chatItems[3].Status != "done" {
		t.Fatalf("expected completed tool result status to be done, got %q", m.chatItems[3].Status)
	}
	if !strings.Contains(m.chatItems[3].Body, "Read internal/tui/model.go lines 1-20") {
		t.Fatalf("expected completed tool summary in result item, got %q", m.chatItems[3].Body)
	}
}

func TestToolStartKeepsStreamedAssistantTurn(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "what project is this", Status: "final"},
			{Kind: "assistant", Title: assistantLabel, Body: "let me inspect the repo structure first", Status: "streaming"},
		},
		streamingIndex: 1,
	}

	m.handleAgentEvent(agent.Event{
		Type:          agent.EventToolCallStarted,
		ToolName:      "list_files",
		ToolArguments: `{"path":"."}`,
	})

	if len(m.chatItems) != 3 {
		t.Fatalf("expected tool start to append only tool call after streamed assistant turn, got %d items", len(m.chatItems))
	}
	if m.chatItems[1].Body != "let me inspect the repo structure first" || m.chatItems[1].Status != "thinking" || m.chatItems[1].Title != thinkingLabel {
		t.Fatalf("expected streamed assistant turn to be preserved and finalized, got %+v", m.chatItems[1])
	}
	if !strings.Contains(m.chatItems[2].Title, "Tool Call | list_files") {
		t.Fatalf("expected tool call entry, got %+v", m.chatItems[2])
	}
}

func TestFinishAssistantMessageAppendsFinalCardAfterThinking(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "what project is this", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "let me inspect the repo structure first", Status: "thinking"},
		},
		streamingIndex: 1,
	}

	m.finishAssistantMessage("This is a Go TUI project.")

	if len(m.chatItems) != 3 {
		t.Fatalf("expected final answer to be appended after thinking, got %d items", len(m.chatItems))
	}
	if m.chatItems[1].Title != thinkingLabel || m.chatItems[1].Status != "thinking" {
		t.Fatalf("expected thinking card to remain visible, got %+v", m.chatItems[1])
	}
	if m.chatItems[2].Title != assistantLabel || m.chatItems[2].Status != "final" || m.chatItems[2].Body != "This is a Go TUI project." {
		t.Fatalf("expected final assistant card after thinking, got %+v", m.chatItems[2])
	}
}

func TestApprovalBannerRendersAboveInput(t *testing.T) {
	input := textarea.New()
	m := model{
		width: 120,
		input: input,
		approval: &approvalPrompt{
			Command: "go test ./internal/tui",
			Reason:  "run tests",
		},
	}

	footer := m.renderFooter()
	for _, want := range []string{
		"go test ./internal/tui",
		"run tests",
		"Y / Enter",
		"N / Esc",
	} {
		if !strings.Contains(footer, want) {
			t.Fatalf("expected approval banner to contain %q", want)
		}
	}
	if strings.Contains(footer, "Approval Request") {
		t.Fatalf("did not expect old centered approval modal title in footer")
	}
}

func TestFormatChatBodyPreservesExplicitBlankLines(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "first paragraph\n\nsecond paragraph",
	}

	got := formatChatBody(item, 80)
	if !strings.Contains(got, "first paragraph\n\nsecond paragraph") {
		t.Fatalf("expected explicit blank line to be preserved, got %q", got)
	}
}

func TestFormatChatBodyWrapsLongUserText(t *testing.T) {
	item := chatEntry{
		Kind: "user",
		Body: "Please describe the relationship between tui, session, agent, and tools so I can inspect how long user text wraps in the chat body.",
	}

	got := formatChatBody(item, 16)
	if !strings.Contains(got, "\n") {
		t.Fatalf("expected long user text to wrap, got %q", got)
	}
	flat := strings.Join(strings.Fields(got), "")
	if flat != strings.Join(strings.Fields(item.Body), "") {
		t.Fatalf("expected wrapped user text to preserve all content, got %q", got)
	}
}

func TestFormatChatBodySeparatesParagraphAndList(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "Explanation\n- first\n- second",
	}

	got := formatChatBody(item, 80)
	if !strings.Contains(got, "Explanation\n\n- first") {
		t.Fatalf("expected list to be separated from paragraph, got %q", got)
	}
}

func TestFormatChatBodyRendersMarkdownHeadingWithoutHashes(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "# Heading\nBody",
	}

	got := formatChatBody(item, 80)
	if strings.Contains(got, "# Heading") {
		t.Fatalf("expected heading marker to be stripped, got %q", got)
	}
	if !strings.Contains(got, "Heading") {
		t.Fatalf("expected heading text to remain, got %q", got)
	}
}

func TestFormatChatBodyRendersCodeBlockWithoutFences(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "```go\nfmt.Println(\"hi\")\n```",
	}

	got := formatChatBody(item, 80)
	if strings.Contains(got, "```") {
		t.Fatalf("expected code fences to be removed, got %q", got)
	}
	if !strings.Contains(got, "fmt.Println(\"hi\")") {
		t.Fatalf("expected code contents to remain, got %q", got)
	}
}

func TestFinishAssistantMessageDoesNotAppendDuplicateCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{
				Kind:   "assistant",
				Title:  assistantLabel,
				Body:   "same answer",
				Status: "streaming",
			},
		},
		streamingIndex: -1,
	}

	m.finishAssistantMessage("same answer")

	if len(m.chatItems) != 1 {
		t.Fatalf("expected no duplicate assistant card, got %d items", len(m.chatItems))
	}
	if m.chatItems[0].Status != "final" {
		t.Fatalf("expected assistant card to be marked final, got %q", m.chatItems[0].Status)
	}
}

func containsString(items []string, target string) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
