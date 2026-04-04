package tui

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/mention"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/tools"

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
	inputY := -1
	for y := 0; y < m.height; y++ {
		if m.mouseOverInput(y) {
			inputY = y
			break
		}
	}
	if inputY < 0 {
		t.Fatalf("expected to find chat input region")
	}
	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
		Y:      inputY,
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
	inputY := -1
	for y := 0; y < m.height; y++ {
		if m.mouseOverInput(y) {
			inputY = y
			break
		}
	}
	if inputY < 0 {
		t.Fatalf("expected to find landing input region")
	}
	got, _ := m.handleMouse(tea.MouseMsg{
		Button: tea.MouseButtonWheelUp,
		Action: tea.MouseActionPress,
		Y:      inputY,
	})
	updated := got.(model)

	if updated.input.Line() >= beforeLine {
		t.Fatalf("expected landing input cursor to move up, got line %d -> %d", beforeLine, updated.input.Line())
	}
}

func TestPlanModeDoesNotShowDetailedPlanPanel(t *testing.T) {
	input := textarea.New()
	m := model{
		screen:    screenChat,
		width:     140,
		height:    24,
		input:     input,
		viewport:  viewport.New(0, 0),
		planView:  viewport.New(0, 0),
		mode:      modePlan,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		plan: planpkg.State{
			Phase: planpkg.PhaseReady,
			Goal:  "Create a plan",
			Steps: []planpkg.Step{
				{Title: "Step 1", Status: planpkg.StepInProgress},
				{Title: "Step 2", Status: planpkg.StepPending},
			},
		},
	}

	m.refreshViewport()

	if m.hasPlanPanel() {
		t.Fatalf("expected detailed plan panel to stay hidden in plan mode")
	}
	for y := 0; y < m.height; y++ {
		for x := 0; x < m.width; x++ {
			if m.mouseOverPlan(x, y) {
				t.Fatalf("did not expect a mouse-active plan panel region in plan mode")
			}
		}
	}
}

func TestCtrlLFromLandingOpensSessions(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	m := model{
		screen:       screenLanding,
		sessionLimit: defaultSessionLimit,
		store:        store,
	}

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlL})
	updated := got.(model)

	if !updated.sessionsOpen {
		t.Fatalf("expected ctrl+l on landing screen to open sessions")
	}
	if cmd == nil {
		t.Fatalf("expected ctrl+l on landing screen to trigger session loading")
	}
}

func TestCtrlGOpensAndClosesHelp(t *testing.T) {
	m := model{}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlG})
	opened := got.(model)
	if !opened.helpOpen {
		t.Fatalf("expected ctrl+g to open help")
	}

	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyCtrlG})
	closed := got.(model)
	if closed.helpOpen {
		t.Fatalf("expected ctrl+g to close help")
	}
}

func TestTabTogglesBetweenBuildAndPlanModes(t *testing.T) {
	m := model{
		mode: modeBuild,
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	updated := got.(model)
	if updated.mode != modePlan {
		t.Fatalf("expected tab to switch to plan mode, got %q", updated.mode)
	}

	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	updated = got.(model)
	if updated.mode != modeBuild {
		t.Fatalf("expected second tab to switch back to build mode, got %q", updated.mode)
	}
}

func TestRefreshViewportPreservesManualScrollOffset(t *testing.T) {
	input := textarea.New()
	m := model{
		screen:    screenChat,
		width:     100,
		height:    24,
		input:     input,
		viewport:  viewport.New(0, 0),
		planView:  viewport.New(0, 0),
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
	}
	for i := 0; i < 20; i++ {
		m.chatItems = append(m.chatItems, chatEntry{
			Kind:   "assistant",
			Title:  "Bytemind",
			Body:   strings.Repeat("message ", 12),
			Status: "final",
		})
	}

	m.chatAutoFollow = true
	m.refreshViewport()
	m.viewport.LineUp(5)
	m.chatAutoFollow = false
	beforeOffset := m.viewport.YOffset
	m.chatItems = append(m.chatItems, chatEntry{
		Kind:   "assistant",
		Title:  "Bytemind",
		Body:   "new content should not force the viewport to jump",
		Status: "final",
	})

	m.refreshViewport()

	if m.viewport.YOffset != beforeOffset {
		t.Fatalf("expected manual scroll offset %d to be preserved, got %d", beforeOffset, m.viewport.YOffset)
	}
}

func TestContinueExecutionInputPreparesPlanAndSubmitsPrompt(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("\u7ee7\u7eed\u6267\u884c")
	input.CursorEnd()
	m := model{
		screen:    screenChat,
		width:     100,
		height:    24,
		input:     input,
		viewport:  viewport.New(0, 0),
		planView:  viewport.New(0, 0),
		mode:      modePlan,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		plan: planpkg.State{
			Goal:       "Finish plan mode",
			Phase:      planpkg.PhaseReady,
			NextAction: "Start: Implement continuation",
			Steps: []planpkg.Step{
				{Title: "Implement continuation", Status: planpkg.StepPending},
				{Title: "Verify workflow", Status: planpkg.StepPending},
			},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if updated.mode != modeBuild {
		t.Fatalf("expected continue execution to switch to build mode, got %q", updated.mode)
	}
	if updated.plan.Phase != planpkg.PhaseExecuting {
		t.Fatalf("expected plan phase to become executing, got %q", updated.plan.Phase)
	}
	if len(updated.chatItems) < 1 {
		t.Fatalf("expected continue execution to submit a prompt")
	}
	if updated.chatItems[0].Body != "\u7ee7\u7eed\u6267\u884c" {
		t.Fatalf("expected original continue input to be appended, got %q", updated.chatItems[0].Body)
	}
	if updated.plan.Steps[0].Status != planpkg.StepInProgress {
		t.Fatalf("expected first pending step to become in progress, got %q", updated.plan.Steps[0].Status)
	}
}

func TestIsContinueExecutionInputSupportsPlanAlias(t *testing.T) {
	for _, input := range []string{"continue plan", "继续做"} {
		if !isContinueExecutionInput(input) {
			t.Fatalf("expected %q to be treated as continue input", input)
		}
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
		"tab agents",
		"/ commands",
		"Ctrl+L sessions",
		"Ctrl+C quit",
		"Build",
		"Plan",
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

	if len(updated.chatItems) < 1 {
		t.Fatalf("expected enter to submit prompt, got %d chat items", len(updated.chatItems))
	}
	if updated.chatItems[0].Body != "ship this prompt" {
		t.Fatalf("expected submitted user prompt to match input, got %q", updated.chatItems[0].Body)
	}
}

func TestAltEnterInsertsNewlineWithoutSubmitting(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("first line")
	input.CursorEnd()

	m := model{
		screen:    screenChat,
		input:     input,
		workspace: "E:\\bytemind",
		sess:      session.New("E:\\bytemind"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter, Alt: true})
	updated := got.(model)

	if len(updated.chatItems) != 0 {
		t.Fatalf("expected alt+enter not to submit prompt")
	}
	if updated.input.Value() != "first line\n" {
		t.Fatalf("expected alt+enter to insert newline, got %q", updated.input.Value())
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

	if len(updated.chatItems) < 1 {
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
		"/skill use",
		"/skill show",
	} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("help text should not mention %q", unwanted)
		}
	}

	for _, wanted := range []string{
		"go run ./cmd/bytemind chat",
		"go run ./cmd/bytemind run -prompt",
		"/session",
		"/quit",
		"/new",
		"Ctrl+G",
		"continue execution",
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
		"tab agents",
		"/ commands",
		"Ctrl+L sessions",
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

func TestRenderFooterInfoLineCombinesModeAndHints(t *testing.T) {
	input := textarea.New()
	m := model{
		width: 160,
		input: input,
		cfg: config.Config{
			Provider: config.ProviderConfig{Model: "deepseek-chat"},
		},
	}

	footer := m.renderFooter()
	lines := strings.Split(footer, "\n")
	infoLine := ""
	for _, line := range lines {
		if strings.Contains(line, "tab agents") {
			infoLine = line
			break
		}
	}
	if infoLine == "" {
		t.Fatalf("expected footer to contain a quick-hint info line")
	}
	for _, want := range []string{"Build", "Plan", "deepseek-chat", "tab agents"} {
		if !strings.Contains(infoLine, want) {
			t.Fatalf("expected combined info line to contain %q, got %q", want, infoLine)
		}
	}
}

func TestRenderStatusBarShowsCurrentRuntimeState(t *testing.T) {
	m := model{
		width:          200,
		mode:           modeBuild,
		phase:          "thinking",
		chatAutoFollow: false,
		cfg: config.Config{
			Provider: config.ProviderConfig{Model: "deepseek-chat"},
		},
		sess: &session.Session{ID: "1234567890abcdef"},
		plan: planpkg.State{
			Phase: planpkg.PhaseExecuting,
			Steps: []planpkg.Step{
				{Title: "Implement plan resumption", Status: planpkg.StepInProgress},
			},
		},
	}

	bar := m.renderStatusBar()
	for _, want := range []string{
		"Mode: BUILD",
		"Phase: executing",
		"Session: 1234567890ab",
		"Step: Implement plan resumption",
		"Follow: manual",
		"Model: deepseek-chat",
	} {
		if !strings.Contains(bar, want) {
			t.Fatalf("expected status bar to contain %q", want)
		}
	}
}

func TestSyncInputStyleUsesSingleLineSearchField(t *testing.T) {
	input := textarea.New()
	m := model{
		screen: screenChat,
		input:  input,
	}

	m.syncInputStyle()

	if m.input.Prompt != "" {
		t.Fatalf("expected empty prompt, got %q", m.input.Prompt)
	}
	if m.input.Placeholder != "Ask Bytemind to inspect, change, or verify this workspace..." {
		t.Fatalf("unexpected placeholder: %q", m.input.Placeholder)
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

	for _, want := range []string{"/help", "/session", "/new", "/quit"} {
		if !containsString(usages, want) {
			t.Fatalf("expected root selector to contain %q, got %v", want, usages)
		}
	}
	for _, unwanted := range []string{"/sessions [limit]", "/resume <id>", "/plan", "/plan add <step>"} {
		if containsString(usages, unwanted) {
			t.Fatalf("did not expect root selector to contain %q", unwanted)
		}
	}
}

func TestHandleSlashSessionOpensSessionsModal(t *testing.T) {
	m := model{}

	if err := m.handleSlashCommand("/session"); err != nil {
		t.Fatalf("expected /session to succeed, got %v", err)
	}
	if !m.sessionsOpen {
		t.Fatalf("expected /session to open sessions modal")
	}
}

func TestHandleSlashSkillsListsDiscoveredSkills(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "skill.json"), []byte(`{
  "name":"review",
  "description":"review skill"
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
		},
		Store:    store,
		Registry: tools.DefaultRegistry(),
	})

	m := model{
		runner:    runner,
		store:     store,
		sess:      sess,
		workspace: workspace,
		screen:    screenChat,
	}
	if err := m.handleSlashCommand("/skills"); err != nil {
		t.Fatalf("expected /skills to succeed, got %v", err)
	}
	if len(m.chatItems) < 2 {
		t.Fatalf("expected /skills command exchange in chat, got %#v", m.chatItems)
	}
	if !strings.Contains(m.chatItems[len(m.chatItems)-1].Body, "review") {
		t.Fatalf("expected skills output to contain review, got %q", m.chatItems[len(m.chatItems)-1].Body)
	}
}

func TestHandleSlashSkillActivateAndClear(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "bug-investigation"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "bug-investigation", "skill.json"), []byte(`{
  "name":"bug-investigation",
  "description":"bug skill",
  "entry":{"slash":"/bug-investigation"},
  "tools":{"policy":"allowlist","items":["read_file"]}
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "skill.json"), []byte(`{
  "name":"review",
  "description":"review skill",
  "tools":{"policy":"allowlist","items":["read_file"]}
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}
	runner := agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
		},
		Store:    store,
		Registry: tools.DefaultRegistry(),
	})

	m := model{
		runner:    runner,
		store:     store,
		sess:      sess,
		workspace: workspace,
		screen:    screenChat,
	}
	if err := m.handleSlashCommand("/bug-investigation"); err != nil {
		t.Fatalf("expected /bug-investigation to succeed, got %v", err)
	}
	if m.sess.ActiveSkill == nil || m.sess.ActiveSkill.Name != "bug-investigation" {
		t.Fatalf("expected bug-investigation active before switch, got %#v", m.sess.ActiveSkill)
	}
	if err := m.handleSlashCommand("/review severity=high"); err != nil {
		t.Fatalf("expected /review to succeed, got %v", err)
	}
	if m.sess.ActiveSkill == nil || m.sess.ActiveSkill.Name != "review" {
		t.Fatalf("expected active skill to be set, got %#v", m.sess.ActiveSkill)
	}
	if got := m.sess.ActiveSkill.Args["severity"]; got != "high" {
		t.Fatalf("expected skill arg severity=high, got %q", got)
	}
	if err := m.handleSlashCommand("/skill clear"); err != nil {
		t.Fatalf("expected /skill clear to succeed, got %v", err)
	}
	if m.sess.ActiveSkill != nil {
		t.Fatalf("expected active skill to be cleared, got %#v", m.sess.ActiveSkill)
	}
}

func TestFilteredCommandsIncludeSkillSlashCommands(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "skill.json"), []byte(`{
  "name":"review",
  "description":"review skill",
  "entry":{"slash":"/review"}
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
		},
		Store:    store,
		Registry: tools.DefaultRegistry(),
	})

	input := textarea.New()
	input.SetValue("/re")
	m := model{
		runner:    runner,
		store:     store,
		sess:      sess,
		workspace: workspace,
		input:     input,
	}

	items := m.filteredCommands()
	found := false
	for _, item := range items {
		if item.Name == "/review" && item.Kind == "skill" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /review skill command in filtered commands, got %+v", items)
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
	if !strings.Contains(view, "Build") || !strings.Contains(view, "Plan") {
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

func TestAtOpensMentionPaletteWithPrefilledToken(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		screen: screenChat,
		input:  input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "internal/tui/model.go", BaseName: "model.go"},
		}, 0, false),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("@")})
	updated := got.(model)

	if !updated.mentionOpen {
		t.Fatalf("expected @ to open mention palette")
	}
	if updated.input.Value() != "@" {
		t.Fatalf("expected main input to keep @ token, got %q", updated.input.Value())
	}
	if len(updated.mentionResults) == 0 {
		t.Fatalf("expected mention palette to return candidates")
	}
}

func TestMentionPaletteFiltersAsUserTypes(t *testing.T) {
	input := textarea.New()
	input.SetValue("@mod")
	m := model{
		input: input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "internal/tui/model.go", BaseName: "model.go"},
			{Path: "README.md", BaseName: "README.md"},
		}, 0, false),
	}

	m.syncInputOverlays()

	if !m.mentionOpen {
		t.Fatalf("expected mention palette to stay open for @mod")
	}
	if len(m.mentionResults) != 1 || m.mentionResults[0].Path != "internal/tui/model.go" {
		t.Fatalf("expected @mod to only match internal/tui/model.go, got %+v", m.mentionResults)
	}
}

func TestMentionPaletteEnterInsertsMentionInsteadOfSubmitting(t *testing.T) {
	input := textarea.New()
	input.SetValue("@mod")
	m := model{
		screen: screenLanding,
		input:  input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "internal/tui/model.go", BaseName: "model.go"},
			{Path: "README.md", BaseName: "README.md"},
		}, 0, false),
	}
	m.syncInputOverlays()

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if cmd != nil {
		t.Fatalf("expected Enter on mention selection to avoid submit command")
	}
	if updated.input.Value() != "@internal/tui/model.go " {
		t.Fatalf("expected mention selection to rewrite input, got %q", updated.input.Value())
	}
	if updated.mentionOpen {
		t.Fatalf("expected mention palette to close after inserting a file")
	}
	if len(updated.chatItems) != 0 {
		t.Fatalf("expected mention insertion to avoid sending message")
	}
	if updated.mentionRecent["internal/tui/model.go"] <= 0 {
		t.Fatalf("expected selected mention to be recorded as recent")
	}
}

func TestMentionPaletteEscClosesWithoutResettingInput(t *testing.T) {
	input := textarea.New()
	input.SetValue("@mod")
	m := model{
		screen: screenChat,
		input:  input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "internal/tui/model.go", BaseName: "model.go"},
		}, 0, false),
	}
	m.syncInputOverlays()

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	updated := got.(model)

	if updated.mentionOpen {
		t.Fatalf("expected Esc to close mention palette")
	}
	if updated.input.Value() != "@mod" {
		t.Fatalf("expected Esc to keep typed mention token, got %q", updated.input.Value())
	}
}

func TestMentionPaletteEnterWithoutCandidatesFallsBackToSubmit(t *testing.T) {
	input := textarea.New()
	input.SetValue("@unknown")
	m := model{
		screen: screenLanding,
		input:  input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "README.md", BaseName: "README.md"},
		}, 0, false),
	}
	m.syncInputOverlays()
	if !m.mentionOpen {
		t.Fatalf("expected mention palette to open for unmatched query")
	}
	if len(m.mentionResults) != 0 {
		t.Fatalf("expected no candidates for @unknown, got %+v", m.mentionResults)
	}

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if cmd == nil {
		t.Fatalf("expected Enter with no mention candidates to submit prompt")
	}
	if updated.screen != screenChat {
		t.Fatalf("expected fallback Enter flow to switch to chat screen")
	}
	if updated.mentionOpen {
		t.Fatalf("expected mention palette to close during fallback submit")
	}
}

func TestMentionPaletteTabInsertsMentionWithoutTogglingMode(t *testing.T) {
	input := textarea.New()
	input.SetValue("@mod")
	m := model{
		screen: screenChat,
		mode:   modeBuild,
		input:  input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "internal/tui/model.go", BaseName: "model.go", TypeTag: "go"},
		}, 0, false),
	}
	m.syncInputOverlays()
	if !m.mentionOpen {
		t.Fatalf("expected mention palette to open")
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyTab})
	updated := got.(model)

	if updated.mode != modeBuild {
		t.Fatalf("expected tab in mention palette not to toggle mode, got %q", updated.mode)
	}
	if updated.input.Value() != "@internal/tui/model.go " {
		t.Fatalf("expected Tab to insert mention, got %q", updated.input.Value())
	}
}

func TestMentionPaletteRecentSelectionRanksFirstOnEmptyQuery(t *testing.T) {
	input := textarea.New()
	input.SetValue("@")
	m := model{
		screen: screenChat,
		input:  input,
		mentionIndex: mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
			{Path: "alpha.go", BaseName: "alpha.go", TypeTag: "go"},
			{Path: "beta.go", BaseName: "beta.go", TypeTag: "go"},
		}, 0, false),
		mentionRecent: map[string]int{"beta.go": 99},
	}
	m.syncInputOverlays()
	if !m.mentionOpen {
		t.Fatalf("expected mention palette for empty query")
	}
	if len(m.mentionResults) < 2 {
		t.Fatalf("expected at least two mention results")
	}
	if m.mentionResults[0].Path != "beta.go" {
		t.Fatalf("expected recent file beta.go first, got %q", m.mentionResults[0].Path)
	}
}

func TestRenderMentionPaletteShowsTruncatedMeta(t *testing.T) {
	index := mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
		{Path: "a.go", BaseName: "a.go", TypeTag: "go"},
		{Path: "b.go", BaseName: "b.go", TypeTag: "go"},
	}, 2, true)

	m := model{
		screen:      screenChat,
		width:       100,
		mentionOpen: true,
		mentionResults: []mention.Candidate{
			{Path: "a.go", BaseName: "a.go", TypeTag: "go"},
			{Path: "b.go", BaseName: "b.go", TypeTag: "go"},
		},
		mentionIndex: index,
	}

	view := m.renderMentionPalette()
	if !strings.Contains(view, "indexed first 2 files") {
		t.Fatalf("expected mention palette to show truncation hint, got %q", view)
	}
}

func TestCommandPaletteAllowsTypingJKWhenOpen(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("/")
	m := model{
		screen:        screenChat,
		commandOpen:   true,
		commandCursor: 1,
		input:         input,
	}

	afterK, _ := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	kModel := afterK.(model)
	if kModel.input.Value() != "/k" {
		t.Fatalf("expected k to be inserted into slash input, got %q", kModel.input.Value())
	}

	afterJ, _ := kModel.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	jModel := afterJ.(model)
	if jModel.input.Value() != "/kj" {
		t.Fatalf("expected j to be inserted into slash input, got %q", jModel.input.Value())
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
	for _, want := range []string{"/help", "/session", "/new"} {
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
	if m.chatItems[1].Kind != "assistant" || m.chatItems[1].Title != thinkingLabel || m.chatItems[1].Status != "thinking" || strings.TrimSpace(m.chatItems[1].Body) == "" {
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

func TestHandleAgentEventTracksRunLifecyclePhases(t *testing.T) {
	m := model{
		busy:         true,
		llmConnected: true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "inspect tui", Status: "final"},
			{Kind: "assistant", Title: thinkingLabel, Body: "thinking", Status: "thinking"},
		},
		streamingIndex: 1,
	}

	m.handleAgentEvent(agent.Event{
		Type:    agent.EventAssistantDelta,
		Content: "Inspecting the TUI flow...",
	})
	if m.phase != "responding" || m.statusNote != "LLM is responding..." {
		t.Fatalf("expected assistant delta to move UI into responding phase, got phase=%q note=%q", m.phase, m.statusNote)
	}
	if m.chatItems[1].Status != "streaming" || !strings.Contains(m.chatItems[1].Body, "Inspecting the TUI flow") {
		t.Fatalf("expected streaming assistant card after delta, got %+v", m.chatItems[1])
	}

	m.handleAgentEvent(agent.Event{
		Type:          agent.EventToolCallStarted,
		ToolName:      "read_file",
		ToolArguments: `{"path":"internal/tui/model.go","start_line":1,"end_line":5}`,
	})
	if m.phase != "tool" || m.statusNote != "Running tool: read_file" {
		t.Fatalf("expected tool start to move UI into tool phase, got phase=%q note=%q", m.phase, m.statusNote)
	}

	m.handleAgentEvent(agent.Event{
		Type:       agent.EventToolCallCompleted,
		ToolName:   "read_file",
		ToolResult: `{"path":"internal/tui/model.go","start_line":1,"end_line":5}`,
	})
	if m.phase != "thinking" {
		t.Fatalf("expected completed tool to return UI to thinking phase, got %q", m.phase)
	}
	if !strings.Contains(m.statusNote, "Read internal/tui/model.go lines 1-5") {
		t.Fatalf("expected tool result summary in status note, got %q", m.statusNote)
	}

	m.handleAgentEvent(agent.Event{
		Type:    agent.EventRunFinished,
		Content: "Done.",
	})
	if m.phase != "idle" || m.statusNote != "Run finished." {
		t.Fatalf("expected run finished event to return UI to idle, got phase=%q note=%q", m.phase, m.statusNote)
	}
}

func TestToolStartKeepsStreamedAssistantReasoning(t *testing.T) {
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
	if !strings.Contains(m.chatItems[1].Body, "inspect the repo structure first") || m.chatItems[1].Status != "thinking" || m.chatItems[1].Title != thinkingLabel {
		t.Fatalf("expected streamed assistant turn to preserve reasoning content, got %+v", m.chatItems[1])
	}
	if !strings.Contains(m.chatItems[2].Title, "Tool Call | list_files") {
		t.Fatalf("expected tool call entry, got %+v", m.chatItems[2])
	}
}

func TestToolStartWithoutAssistantDeltaDoesNotInjectThinkingCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "list files", Status: "final"},
		},
		streamingIndex: -1,
	}

	m.handleAgentEvent(agent.Event{
		Type:          agent.EventToolCallStarted,
		ToolName:      "list_files",
		ToolArguments: `{"path":"."}`,
	})

	if len(m.chatItems) != 2 {
		t.Fatalf("expected only tool call entry to be appended, got %d items", len(m.chatItems))
	}
	if m.chatItems[1].Kind != "tool" || !strings.Contains(m.chatItems[1].Title, "Tool Call | list_files") {
		t.Fatalf("expected tool call entry, got %+v", m.chatItems[1])
	}
}

func TestToolStartWithGenericToolIntentDoesNotShowThinkingCard(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "list files", Status: "final"},
			{Kind: "assistant", Title: assistantLabel, Body: "I will call `list_files` to inspect the relevant context first.", Status: "streaming"},
		},
		streamingIndex: 1,
	}

	m.handleAgentEvent(agent.Event{
		Type:          agent.EventToolCallStarted,
		ToolName:      "list_files",
		ToolArguments: `{"path":"."}`,
	})

	if len(m.chatItems) != 2 {
		t.Fatalf("expected generic tool-intent placeholder to be removed, got %d items", len(m.chatItems))
	}
	if m.chatItems[1].Kind != "tool" || !strings.Contains(m.chatItems[1].Title, "Tool Call | list_files") {
		t.Fatalf("expected tool call entry after removing placeholder, got %+v", m.chatItems[1])
	}
}

func TestAssistantDeltaPlanningTextRendersAsThinking(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "请检查项目", Status: "final"},
		},
		streamingIndex: -1,
	}

	m.handleAgentEvent(agent.Event{
		Type:    agent.EventAssistantDelta,
		Content: "我会先了解项目结构和配置，然后检查代码组织和依赖关系，最后通过构建和测试来验证功能。",
	})

	if len(m.chatItems) != 2 {
		t.Fatalf("expected assistant delta to append one assistant item, got %d", len(m.chatItems))
	}
	if m.chatItems[1].Title != thinkingLabel || m.chatItems[1].Status != "thinking" {
		t.Fatalf("expected planning delta to render as thinking, got %+v", m.chatItems[1])
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

func TestRenderFooterShowsActiveSkillBanner(t *testing.T) {
	input := textarea.New()
	m := model{
		width: 120,
		input: input,
		sess: &session.Session{
			ActiveSkill: &session.ActiveSkill{
				Name: "review",
				Args: map[string]string{"severity": "high"},
			},
		},
	}

	footer := m.renderFooter()
	if !strings.Contains(footer, "Active skill: review") {
		t.Fatalf("expected footer to show active skill banner, got %q", footer)
	}
	if !strings.Contains(footer, "severity=high") {
		t.Fatalf("expected footer to show active skill args, got %q", footer)
	}
}

func TestUpdateApprovalRequestMsgSetsApprovalPhase(t *testing.T) {
	reply := make(chan approvalDecision, 1)
	m := model{async: make(chan tea.Msg, 1)}

	got, cmd := m.Update(approvalRequestMsg{
		Request: tools.ApprovalRequest{
			Command: "go test ./internal/tui",
			Reason:  "run focused tests",
		},
		Reply: reply,
	})
	updated := got.(model)

	if cmd == nil {
		t.Fatalf("expected approval request to keep waiting for async events")
	}
	if updated.approval == nil {
		t.Fatalf("expected approval prompt to be stored on the model")
	}
	if updated.approval.Command != "go test ./internal/tui" || updated.approval.Reason != "run focused tests" {
		t.Fatalf("expected approval prompt contents to be preserved, got %+v", updated.approval)
	}
	if updated.phase != "approval" || updated.statusNote != "Approval required." {
		t.Fatalf("expected approval request to switch UI into approval state, got phase=%q note=%q", updated.phase, updated.statusNote)
	}
}

func TestApprovalKeysTransitionStateAndSendDecision(t *testing.T) {
	t.Run("approve", func(t *testing.T) {
		reply := make(chan approvalDecision, 1)
		m := model{
			approval: &approvalPrompt{
				Command: "go test ./internal/tui",
				Reason:  "run focused tests",
				Reply:   reply,
			},
			phase: "approval",
		}

		got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
		updated := got.(model)

		if updated.approval != nil {
			t.Fatalf("expected approval prompt to clear after approval")
		}
		if updated.phase != "tool" || updated.statusNote != "Shell command approved." {
			t.Fatalf("expected approval to move UI into tool phase, got phase=%q note=%q", updated.phase, updated.statusNote)
		}

		select {
		case decision := <-reply:
			if !decision.Approved {
				t.Fatalf("expected approval decision to be true")
			}
		default:
			t.Fatalf("expected approval decision to be sent")
		}
	})

	t.Run("reject", func(t *testing.T) {
		reply := make(chan approvalDecision, 1)
		m := model{
			approval: &approvalPrompt{
				Command: "go test ./internal/tui",
				Reason:  "run focused tests",
				Reply:   reply,
			},
			phase: "approval",
		}

		got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
		updated := got.(model)

		if updated.approval != nil {
			t.Fatalf("expected approval prompt to clear after rejection")
		}
		if updated.phase != "thinking" || updated.statusNote != "Shell command rejected." {
			t.Fatalf("expected rejection to return UI to thinking phase, got phase=%q note=%q", updated.phase, updated.statusNote)
		}

		select {
		case decision := <-reply:
			if decision.Approved {
				t.Fatalf("expected rejection decision to be false")
			}
		default:
			t.Fatalf("expected rejection decision to be sent")
		}
	})
}

func TestUpdateRunFinishedMsgResetsBusyState(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		m := model{
			async:          make(chan tea.Msg, 1),
			busy:           true,
			streamingIndex: 3,
			statusNote:     "Running...",
			phase:          "responding",
			llmConnected:   true,
		}

		got, cmd := m.Update(runFinishedMsg{})
		updated := got.(model)

		if cmd == nil {
			t.Fatalf("expected run finished to schedule follow-up async/session work")
		}
		if updated.busy {
			t.Fatalf("expected run finished to clear busy state")
		}
		if updated.streamingIndex != -1 {
			t.Fatalf("expected run finished to clear streaming index, got %d", updated.streamingIndex)
		}
		if updated.phase != "idle" || updated.statusNote != "Ready." {
			t.Fatalf("expected successful run to return to idle, got phase=%q note=%q", updated.phase, updated.statusNote)
		}
		if !updated.llmConnected {
			t.Fatalf("expected successful run to keep llmConnected=true")
		}
	})

	t.Run("error", func(t *testing.T) {
		m := model{
			async:          make(chan tea.Msg, 1),
			busy:           true,
			streamingIndex: 1,
			statusNote:     "Running...",
			phase:          "responding",
			llmConnected:   true,
			chatItems: []chatEntry{
				{Kind: "user", Title: "You", Body: "inspect repo", Status: "final"},
				{Kind: "assistant", Title: thinkingLabel, Body: "thinking", Status: "thinking"},
			},
		}

		got, _ := m.Update(runFinishedMsg{Err: errors.New("provider unavailable")})
		updated := got.(model)

		if updated.busy {
			t.Fatalf("expected failed run to clear busy state")
		}
		if updated.phase != "error" || !strings.Contains(updated.statusNote, "provider unavailable") {
			t.Fatalf("expected failed run to switch UI into error state, got phase=%q note=%q", updated.phase, updated.statusNote)
		}
		if updated.llmConnected {
			t.Fatalf("expected failed run to mark llmConnected=false")
		}
		last := updated.chatItems[len(updated.chatItems)-1]
		if last.Status != "error" || !strings.Contains(last.Body, "provider unavailable") {
			t.Fatalf("expected latest assistant card to show failure, got %+v", last)
		}
	})
}

func TestUpdateSessionsLoadedMsgUpdatesAndClampsSessions(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		m := model{
			sessionCursor: 3,
			sessions: []session.Summary{
				{ID: "old-1"},
				{ID: "old-2"},
				{ID: "old-3"},
				{ID: "old-4"},
			},
		}

		got, _ := m.Update(sessionsLoadedMsg{
			Summaries: []session.Summary{
				{ID: "new-1"},
				{ID: "new-2"},
			},
		})
		updated := got.(model)

		if len(updated.sessions) != 2 {
			t.Fatalf("expected sessions list to be replaced, got %d entries", len(updated.sessions))
		}
		if updated.sessionCursor != 1 {
			t.Fatalf("expected session cursor to clamp to last available entry, got %d", updated.sessionCursor)
		}
	})

	t.Run("error", func(t *testing.T) {
		m := model{
			sessionCursor: 1,
			sessions: []session.Summary{
				{ID: "keep-1"},
				{ID: "keep-2"},
			},
		}

		got, _ := m.Update(sessionsLoadedMsg{
			Err: errors.New("store unavailable"),
		})
		updated := got.(model)

		if len(updated.sessions) != 2 || updated.sessions[0].ID != "keep-1" || updated.sessions[1].ID != "keep-2" {
			t.Fatalf("expected session list to stay unchanged on load error, got %+v", updated.sessions)
		}
		if updated.sessionCursor != 1 {
			t.Fatalf("expected session cursor to remain unchanged on load error, got %d", updated.sessionCursor)
		}
	})
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
