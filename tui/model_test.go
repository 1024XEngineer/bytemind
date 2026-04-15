package tui

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/history"
	"bytemind/internal/llm"
	"bytemind/internal/mention"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/tools"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type fakeClipboardTextWriter struct {
	last       string
	err        error
	waitForCtx bool
}

func (f *fakeClipboardTextWriter) WriteText(ctx context.Context, text string) error {
	if f.waitForCtx {
		<-ctx.Done()
		return ctx.Err()
	}
	if f.err != nil {
		return f.err
	}
	f.last = text
	return nil
}

type compactCommandTestClient struct {
	replies  []llm.Message
	requests []llm.ChatRequest
	index    int
}

func (c *compactCommandTestClient) CreateMessage(_ context.Context, req llm.ChatRequest) (llm.Message, error) {
	c.requests = append(c.requests, req)
	if len(c.replies) == 0 {
		return llm.Message{}, nil
	}
	if c.index >= len(c.replies) {
		return c.replies[len(c.replies)-1], nil
	}
	reply := c.replies[c.index]
	c.index++
	return reply, nil
}

func (c *compactCommandTestClient) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	reply, err := c.CreateMessage(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	if onDelta != nil && strings.TrimSpace(reply.Content) != "" {
		onDelta(reply.Content)
	}
	return reply, nil
}

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

func TestNormalizeMouseMsgAppliesYOffset(t *testing.T) {
	m := model{mouseYOffset: 2}
	msg := tea.MouseMsg{X: 10, Y: 8}
	got := m.normalizeMouseMsg(msg)
	if got.X != 10 || got.Y != 10 {
		t.Fatalf("expected normalized mouse msg to keep X and shift Y by offset, got %+v", got)
	}
}

func TestResolveMouseYOffsetFromEnv(t *testing.T) {
	t.Setenv("BYTEMIND_MOUSE_Y_OFFSET", "2")
	if got := resolveMouseYOffset(); got != 2 {
		t.Fatalf("expected env-configured y offset 2, got %d", got)
	}

	t.Setenv("BYTEMIND_MOUSE_Y_OFFSET", "99")
	if got := resolveMouseYOffset(); got != 10 {
		t.Fatalf("expected y offset to clamp to 10, got %d", got)
	}
}

func TestResolveMouseYOffsetDefaultIsZero(t *testing.T) {
	t.Setenv("BYTEMIND_MOUSE_Y_OFFSET", "")
	if got := resolveMouseYOffset(); got != 0 {
		t.Fatalf("expected default y offset 0, got %d", got)
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

func TestHandleMouseDragSelectionArmsCopyableSelection(t *testing.T) {
	writer := &fakeClipboardTextWriter{}
	input := textarea.New()
	input.Focus()

	m := model{
		screen:        screenChat,
		width:         120,
		height:        28,
		input:         input,
		viewport:      viewport.New(60, 10),
		tokenUsage:    newTokenUsageComponent(),
		clipboardText: writer,
	}
	m.viewport.SetContent("alpha line\nbeta line\ngamma line")

	left, _, top, _, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatal("expected conversation viewport bounds to be available")
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
		X:      left + 4,
		Y:      top,
	})
	moved := got.(model)

	got, _ = moved.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      left + 4,
		Y:      top,
	})
	released := got.(model)

	if writer.last != "" {
		t.Fatalf("expected drag release not to copy before ctrl+c, got %q", writer.last)
	}
	if !released.mouseSelectionActive {
		t.Fatalf("expected drag release to keep an active selection")
	}
	if !strings.Contains(released.statusNote, "Press Ctrl+C to copy") {
		t.Fatalf("expected copy hint after drag selection, got %q", released.statusNote)
	}
}

func TestHandleMouseReleaseAtDifferentPointArmsSelectionWithoutMotion(t *testing.T) {
	writer := &fakeClipboardTextWriter{}
	input := textarea.New()
	input.Focus()

	m := model{
		screen:        screenChat,
		width:         120,
		height:        28,
		input:         input,
		viewport:      viewport.New(60, 10),
		tokenUsage:    newTokenUsageComponent(),
		clipboardText: writer,
	}
	m.viewport.SetContent("alpha line\nbeta line")

	left, _, top, _, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatal("expected conversation viewport bounds to be available")
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      left,
		Y:      top,
	})
	pressed := got.(model)

	got, _ = pressed.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      left + 4,
		Y:      top,
	})
	released := got.(model)

	if writer.last != "" {
		t.Fatalf("expected release-with-range not to copy before ctrl+c, got %q", writer.last)
	}
	if !released.mouseSelectionActive {
		t.Fatalf("expected release at different point to keep an active selection")
	}
	if !strings.Contains(released.statusNote, "Press Ctrl+C to copy") {
		t.Fatalf("expected copy hint after selection, got %q", released.statusNote)
	}
}

func TestHandleMouseSingleClickStartsSelectionWithoutCopy(t *testing.T) {
	writer := &fakeClipboardTextWriter{}
	input := textarea.New()
	input.Focus()

	m := model{
		screen:        screenChat,
		width:         120,
		height:        28,
		input:         input,
		viewport:      viewport.New(60, 10),
		tokenUsage:    newTokenUsageComponent(),
		clipboardText: writer,
	}
	m.viewport.SetContent("alpha line\nbeta line")

	left, _, top, _, ok := m.conversationViewportBounds()
	if !ok {
		t.Fatal("expected conversation viewport bounds to be available")
	}

	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      left + 2,
		Y:      top,
	})
	pressed := got.(model)

	got, _ = pressed.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      left + 2,
		Y:      top,
	})
	released := got.(model)

	if writer.last != "" {
		t.Fatalf("expected click without drag not to copy text, got %q", writer.last)
	}
	if released.mouseSelecting {
		t.Fatalf("expected click without drag to leave selection mode")
	}
	if released.mouseSelectionActive {
		t.Fatalf("expected click without drag not to keep an active selection")
	}
}

func TestCtrlCCopiesActiveSelectionAndShowsToast(t *testing.T) {
	writer := &fakeClipboardTextWriter{}
	input := textarea.New()
	input.Focus()

	m := model{
		screen:               screenChat,
		width:                120,
		height:               28,
		input:                input,
		viewport:             viewport.New(60, 10),
		tokenUsage:           newTokenUsageComponent(),
		clipboardText:        writer,
		mouseSelectionActive: true,
		mouseSelectionStart:  viewportSelectionPoint{Row: 0, Col: 0},
		mouseSelectionEnd:    viewportSelectionPoint{Row: 0, Col: 4},
	}
	m.viewport.SetContent("alpha line\nbeta line")

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := got.(model)

	if writer.last != "alpha" {
		t.Fatalf("expected ctrl+c copied selection %q, got %q", "alpha", writer.last)
	}
	if updated.mouseSelectionActive {
		t.Fatalf("expected successful copy to clear active selection")
	}
	if updated.selectionToast != "Copied selection" {
		t.Fatalf("expected copy toast, got %q", updated.selectionToast)
	}
	if cmd == nil {
		t.Fatalf("expected ctrl+c copy to schedule toast expiry")
	}
}

func TestCtrlCCopyFailureKeepsSelectionAndSetsStatus(t *testing.T) {
	writer := &fakeClipboardTextWriter{err: errors.New("clipboard write failed")}
	input := textarea.New()
	input.Focus()

	m := model{
		screen:               screenChat,
		width:                120,
		height:               28,
		input:                input,
		viewport:             viewport.New(60, 10),
		tokenUsage:           newTokenUsageComponent(),
		clipboardText:        writer,
		mouseSelectionActive: true,
		mouseSelectionStart:  viewportSelectionPoint{Row: 0, Col: 0},
		mouseSelectionEnd:    viewportSelectionPoint{Row: 0, Col: 2},
	}
	m.viewport.SetContent("alpha line\nbeta line")

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	released := got.(model)

	if !strings.Contains(released.statusNote, "clipboard write failed") {
		t.Fatalf("expected copy error in status note, got %q", released.statusNote)
	}
	if !released.mouseSelectionActive {
		t.Fatalf("expected failed copy to keep active selection")
	}
}

func TestCtrlCCopyTimeoutKeepsSelectionAndSetsTimeoutStatus(t *testing.T) {
	writer := &fakeClipboardTextWriter{waitForCtx: true}
	input := textarea.New()
	input.Focus()

	previousTimeout := clipboardWriteTimeout
	clipboardWriteTimeout = 5 * time.Millisecond
	defer func() { clipboardWriteTimeout = previousTimeout }()

	m := model{
		screen:               screenChat,
		width:                120,
		height:               28,
		input:                input,
		viewport:             viewport.New(60, 10),
		tokenUsage:           newTokenUsageComponent(),
		clipboardText:        writer,
		mouseSelectionActive: true,
		mouseSelectionStart:  viewportSelectionPoint{Row: 0, Col: 0},
		mouseSelectionEnd:    viewportSelectionPoint{Row: 0, Col: 2},
	}
	m.viewport.SetContent("alpha line\nbeta line")

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	updated := got.(model)
	if !strings.Contains(updated.statusNote, "timed out") {
		t.Fatalf("expected timeout status note, got %q", updated.statusNote)
	}
	if !updated.mouseSelectionActive {
		t.Fatalf("expected timeout to keep active selection")
	}
}

func TestRenderConversationCopyUsesPlainMessageText(t *testing.T) {
	m := model{
		width:  120,
		height: 28,
		viewport: func() viewport.Model {
			vp := viewport.New(60, 10)
			return vp
		}(),
		chatItems: []chatEntry{
			{Kind: "assistant", Title: assistantLabel, Body: "line one\nline two", Status: "final"},
		},
	}

	got := m.renderConversationCopy()
	if strings.Contains(got, "\u2502") || strings.Contains(got, "\u2503") {
		t.Fatalf("expected copy conversation without card borders, got %q", got)
	}
	if !strings.Contains(got, "line one") || !strings.Contains(got, "line two") {
		t.Fatalf("expected copy conversation to contain message body, got %q", got)
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

func TestWrapPlainTextPrefersWordBoundariesForEnglish(t *testing.T) {
	text := "Risks - this section should keep words intact"
	got := wrapPlainText(text, 8)
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		if strings.Contains(line, "Ris") && !strings.Contains(line, "Risks") {
			t.Fatalf("expected not to split 'Risks' across lines, got %q", got)
		}
		if strings.Contains(line, "Act") && !strings.Contains(line, "Action") {
			t.Fatalf("expected not to split words abruptly, got %q", got)
		}
	}
}

func TestRenderMainPanelShowsTokenUsageBadge(t *testing.T) {
	m := model{
		screen:     screenChat,
		width:      120,
		height:     28,
		input:      textarea.New(),
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
	}
	m.viewport.SetContent(strings.Repeat("line\n", 40))
	m.tokenUsage.displayUsed = 1234
	_ = m.tokenUsage.SetUsage(1234, 5000)

	panel := m.renderMainPanel()
	if !strings.Contains(panel, "token: 1,234") {
		t.Fatalf("expected token usage badge text in main panel, got %q", panel)
	}
}

func TestHandleMouseHoverTokenUsageConsumesEvent(t *testing.T) {
	m := model{
		screen:     screenChat,
		width:      120,
		height:     28,
		input:      textarea.New(),
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
	}
	m.viewport.SetContent(strings.Repeat("line\n", 60))
	_ = m.tokenUsage.SetUsage(1500, 5000)
	m.refreshViewport()

	x := m.tokenUsage.bounds.x + max(0, m.tokenUsage.bounds.w/2)
	y := m.tokenUsage.bounds.y
	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		X:      x,
		Y:      y,
	})
	updated := got.(model)
	if !updated.tokenUsage.hover {
		t.Fatalf("expected hover state to activate over token badge")
	}
}

func TestHandleAgentEventUsageUpdatedAccumulatesRealTokens(t *testing.T) {
	m := model{
		tokenUsage:  newTokenUsageComponent(),
		tokenBudget: 5000,
	}

	m.handleAgentEvent(agent.Event{
		Type: agent.EventUsageUpdated,
		Usage: llm.Usage{
			InputTokens:   120,
			OutputTokens:  40,
			ContextTokens: 30,
			TotalTokens:   190,
		},
	})

	if m.tokenUsedTotal != 190 {
		t.Fatalf("expected cumulative used tokens 190, got %d", m.tokenUsedTotal)
	}
	if m.tokenInput != 120 || m.tokenOutput != 40 || m.tokenContext != 30 {
		t.Fatalf("unexpected token breakdown input=%d output=%d context=%d", m.tokenInput, m.tokenOutput, m.tokenContext)
	}
	if m.tokenUsage.used != 190 {
		t.Fatalf("expected token component used value 190, got %d", m.tokenUsage.used)
	}
}

func TestAssistantDeltaDoesNotChangeUsageWithoutOfficialUsage(t *testing.T) {
	m := model{
		tokenUsage:  newTokenUsageComponent(),
		tokenBudget: 5000,
	}

	m.handleAgentEvent(agent.Event{Type: agent.EventRunStarted})
	m.handleAgentEvent(agent.Event{
		Type:    agent.EventAssistantDelta,
		Content: "This streamed delta should not change usage counters.",
	})

	if m.tokenUsedTotal != 0 || m.tokenOutput != 0 {
		t.Fatalf("expected no provisional usage without official usage, used=%d output=%d", m.tokenUsedTotal, m.tokenOutput)
	}

	m.handleAgentEvent(agent.Event{
		Type: agent.EventUsageUpdated,
		Usage: llm.Usage{
			InputTokens:   20,
			OutputTokens:  7,
			ContextTokens: 3,
			TotalTokens:   30,
		},
	})

	if m.tokenUsedTotal != 30 {
		t.Fatalf("expected total tokens to follow official total 30, got %d", m.tokenUsedTotal)
	}
	if m.tokenInput != 20 || m.tokenOutput != 7 || m.tokenContext != 3 {
		t.Fatalf("expected official breakdown after calibration, got input=%d output=%d context=%d", m.tokenInput, m.tokenOutput, m.tokenContext)
	}
}

func TestApplyUsageFallsBackToBreakdownWhenTotalIsZero(t *testing.T) {
	m := model{
		tokenUsage:  newTokenUsageComponent(),
		tokenBudget: 5000,
	}

	m.handleAgentEvent(agent.Event{
		Type: agent.EventUsageUpdated,
		Usage: llm.Usage{
			InputTokens:   11,
			OutputTokens:  5,
			ContextTokens: 4,
			TotalTokens:   0,
		},
	})

	if m.tokenUsedTotal != 20 {
		t.Fatalf("expected fallback sum of usage breakdown (20), got %d", m.tokenUsedTotal)
	}
}

func TestFetchRemoteTokenUsageCmdReturnsErrorMsgWhenConfigMissing(t *testing.T) {
	m := model{cfg: config.Config{}}
	cmd := m.fetchRemoteTokenUsageCmd()
	if cmd == nil {
		t.Fatalf("expected remote usage command")
	}
	msg := cmd()
	pulled, ok := msg.(tokenUsagePulledMsg)
	if !ok {
		t.Fatalf("expected tokenUsagePulledMsg, got %T", msg)
	}
	if pulled.Err == nil || !strings.Contains(pulled.Err.Error(), "missing base url or api key") {
		t.Fatalf("expected missing config error, got %v", pulled.Err)
	}
}

func TestFetchRemoteTokenUsageCmdReturnsUsageMsgOnSuccess(t *testing.T) {
	orig := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = orig })

	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		body := `{"data":[{"results":[{"input_tokens":12,"output_tokens":8,"input_cached_tokens":3}]}]}`
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(body)),
			Header:     make(http.Header),
		}, nil
	})

	m := model{
		cfg: config.Config{
			Provider: config.ProviderConfig{
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "test-key",
			},
		},
	}

	cmd := m.fetchRemoteTokenUsageCmd()
	msg := cmd()
	pulled, ok := msg.(tokenUsagePulledMsg)
	if !ok {
		t.Fatalf("expected tokenUsagePulledMsg, got %T", msg)
	}
	if pulled.Err != nil {
		t.Fatalf("expected successful usage pull message, got %v", pulled.Err)
	}
	if pulled.Used != 23 || pulled.Input != 12 || pulled.Output != 8 || pulled.Context != 3 {
		t.Fatalf("unexpected pulled usage payload: %+v", pulled)
	}
}

func TestUpdateTokenUsagePulledMsgIgnoredForSessionOnly(t *testing.T) {
	m := model{
		tokenUsage:     newTokenUsageComponent(),
		tokenBudget:    5000,
		tokenUsedTotal: 100,
		tokenInput:     60,
		tokenOutput:    20,
		tokenContext:   5,
	}

	got, _ := m.Update(tokenUsagePulledMsg{
		Used:    90,
		Input:   40,
		Output:  30,
		Context: 10,
	})
	updated := got.(model)
	if updated.tokenUsedTotal != 100 || updated.tokenInput != 60 || updated.tokenOutput != 20 || updated.tokenContext != 5 {
		t.Fatalf("expected remote usage pull to be ignored, got used=%d input=%d output=%d context=%d", updated.tokenUsedTotal, updated.tokenInput, updated.tokenOutput, updated.tokenContext)
	}

	got, _ = updated.Update(tokenUsagePulledMsg{Err: errors.New("boom")})
	still := got.(model)
	if still.tokenUsedTotal != updated.tokenUsedTotal || still.tokenInput != updated.tokenInput || still.tokenOutput != updated.tokenOutput || still.tokenContext != updated.tokenContext {
		t.Fatalf("expected error usage message to leave counters unchanged, got %+v", still)
	}
}

func TestAccumulateTokenUsageFallbackAndClamp(t *testing.T) {
	m := model{}
	m.accumulateTokenUsage([]llm.Message{
		{},
		{Usage: &llm.Usage{InputTokens: 10, OutputTokens: 4, ContextTokens: 1, TotalTokens: 0}},
		{Usage: &llm.Usage{InputTokens: -5, OutputTokens: 8, ContextTokens: 0, TotalTokens: -1}},
		{Usage: &llm.Usage{InputTokens: 1, OutputTokens: 1, ContextTokens: 1, TotalTokens: 20}},
	})

	if m.tokenUsedTotal != 38 {
		t.Fatalf("expected used total 38, got %d", m.tokenUsedTotal)
	}
	if m.tokenInput != 11 || m.tokenOutput != 13 || m.tokenContext != 2 {
		t.Fatalf("unexpected breakdown input=%d output=%d context=%d", m.tokenInput, m.tokenOutput, m.tokenContext)
	}
}

func TestRestoreTokenUsageFromSessionUsesCurrentSessionOnly(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatalf("failed to create session store: %v", err)
	}

	workspace := t.TempDir()
	current := session.New(workspace)
	current.Messages = []llm.Message{
		{Role: "assistant", Parts: []llm.Part{{Type: llm.PartText, Text: &llm.TextPart{Value: "ok"}}}, Usage: &llm.Usage{InputTokens: 30, OutputTokens: 20, ContextTokens: 10, TotalTokens: 60}},
	}
	other := session.New(workspace)
	other.Messages = []llm.Message{
		{Role: "assistant", Parts: []llm.Part{{Type: llm.PartText, Text: &llm.TextPart{Value: "ok"}}}, Usage: &llm.Usage{InputTokens: 200, OutputTokens: 100, ContextTokens: 50, TotalTokens: 350}},
	}
	if err := store.Save(current); err != nil {
		t.Fatalf("failed to save current session: %v", err)
	}
	if err := store.Save(other); err != nil {
		t.Fatalf("failed to save other session: %v", err)
	}

	m := model{
		store:     store,
		workspace: workspace,
	}
	m.restoreTokenUsageFromSession(current)

	if m.tokenUsedTotal != 60 {
		t.Fatalf("expected current session total 60, got %d", m.tokenUsedTotal)
	}
	if m.tokenInput != 30 || m.tokenOutput != 20 || m.tokenContext != 10 {
		t.Fatalf("unexpected breakdown input=%d output=%d context=%d", m.tokenInput, m.tokenOutput, m.tokenContext)
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

func TestCtrlFOpensPromptSearchAndFiltersEntries(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		input:               input,
		promptHistoryLoaded: true,
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "fix tui layout spacing"},
			{Prompt: "add model test case"},
			{Prompt: "review runner error handling"},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	opened := got.(model)
	if !opened.promptSearchOpen {
		t.Fatalf("expected ctrl+f to open prompt search")
	}
	if len(opened.promptSearchMatches) != 3 {
		t.Fatalf("expected 3 prompt matches, got %d", len(opened.promptSearchMatches))
	}

	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("test")})
	filtered := got.(model)
	if filtered.promptSearchQuery != "test" {
		t.Fatalf("expected query to become test, got %q", filtered.promptSearchQuery)
	}
	if len(filtered.promptSearchMatches) != 1 {
		t.Fatalf("expected one filtered prompt, got %d", len(filtered.promptSearchMatches))
	}
	if !strings.Contains(filtered.promptSearchMatches[0].Prompt, "test case") {
		t.Fatalf("unexpected filtered prompt: %+v", filtered.promptSearchMatches[0])
	}
}

func TestCtrlFWhilePromptSearchOpenMovesSelection(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		input:               input,
		promptHistoryLoaded: true,
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "first prompt"},
			{Prompt: "second prompt"},
			{Prompt: "third prompt"},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	opened := got.(model)
	if opened.promptSearchCursor != 0 {
		t.Fatalf("expected initial cursor 0, got %d", opened.promptSearchCursor)
	}

	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	moved := got.(model)
	if moved.promptSearchCursor != 1 {
		t.Fatalf("expected ctrl+f to move cursor to 1, got %d", moved.promptSearchCursor)
	}
}

func TestPromptSearchEnterRestoresSelectedPrompt(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("draft")
	m := model{
		input:               input,
		promptHistoryLoaded: true,
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "first prompt"},
			{Prompt: "second prompt"},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	opened := got.(model)
	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	down := got.(model)

	got, _ = down.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	applied := got.(model)
	if applied.promptSearchOpen {
		t.Fatalf("expected prompt search to close after enter")
	}
	if applied.input.Value() != "first prompt" {
		t.Fatalf("expected selected prompt to be restored, got %q", applied.input.Value())
	}
}

func TestPromptSearchEscRestoresOriginalInput(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("work in progress")
	m := model{
		input:               input,
		promptHistoryLoaded: true,
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "old prompt"},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	opened := got.(model)
	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("old")})
	filtered := got.(model)
	got, _ = filtered.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	closed := got.(model)

	if closed.promptSearchOpen {
		t.Fatalf("expected prompt search to close on esc")
	}
	if closed.input.Value() != "work in progress" {
		t.Fatalf("expected original input to be restored, got %q", closed.input.Value())
	}
}

func TestCtrlHDoesNotOpenPromptSearch(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		input:               input,
		promptHistoryLoaded: true,
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "first prompt"},
			{Prompt: "second prompt"},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlH})
	updated := got.(model)
	if updated.promptSearchOpen {
		t.Fatalf("expected ctrl+h to have no prompt search binding")
	}
}

func TestPromptSearchQuerySupportsWorkspaceAndSessionFilters(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		input:               input,
		promptHistoryLoaded: true,
		promptHistoryEntries: []history.PromptEntry{
			{Prompt: "fix test", Workspace: "repo-a", SessionID: "sess-alpha"},
			{Prompt: "fix test", Workspace: "repo-b", SessionID: "sess-beta"},
			{Prompt: "add docs", Workspace: "repo-a", SessionID: "sess-alpha"},
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	opened := got.(model)
	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fix ws:repo-a sid:alpha")})
	filtered := got.(model)

	if len(filtered.promptSearchMatches) != 1 {
		t.Fatalf("expected one filtered match, got %d", len(filtered.promptSearchMatches))
	}
	match := filtered.promptSearchMatches[0]
	if match.Workspace != "repo-a" || match.SessionID != "sess-alpha" {
		t.Fatalf("unexpected filtered match: %+v", match)
	}
}

func TestPromptSearchPanelSupportsPageNavigation(t *testing.T) {
	input := textarea.New()
	input.Focus()
	entries := make([]history.PromptEntry, 0, 12)
	for i := 0; i < 12; i++ {
		entries = append(entries, history.PromptEntry{Prompt: "prompt " + string(rune('a'+i))})
	}
	m := model{
		input:                input,
		promptHistoryLoaded:  true,
		promptHistoryEntries: entries,
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlF})
	opened := got.(model)
	if opened.promptSearchCursor != 0 {
		t.Fatalf("expected cursor at 0, got %d", opened.promptSearchCursor)
	}

	got, _ = opened.handleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	paged := got.(model)
	if paged.promptSearchCursor != promptSearchPageSize {
		t.Fatalf("expected pgdown to move cursor to %d, got %d", promptSearchPageSize, paged.promptSearchCursor)
	}

	got, _ = paged.handleKey(tea.KeyMsg{Type: tea.KeyPgUp})
	back := got.(model)
	if back.promptSearchCursor != 0 {
		t.Fatalf("expected pgup to move cursor back to 0, got %d", back.promptSearchCursor)
	}
}

func TestStartupGuideSequentialFlowAdvancesAndClearsInput(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4"
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	input := textarea.New()
	input.Focus()
	input.SetValue("openai-compatible")
	m := model{
		input: input,
		cfg: config.Config{
			Provider: config.ProviderConfig{
				Type:  "openai-compatible",
				Model: "gpt-5.4",
			},
		},
		startupGuide: StartupGuide{
			Active:       true,
			Status:       "Bytemind needs a working API key before chat can start.",
			ConfigPath:   configPath,
			CurrentField: startupFieldType,
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if !updated.startupGuide.Active {
		t.Fatalf("expected startup guide to remain active before api key")
	}
	if updated.startupGuide.CurrentField != startupFieldBaseURL {
		t.Fatalf("expected next step base_url, got %q", updated.startupGuide.CurrentField)
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected input to be cleared after step submit, got %q", updated.input.Value())
	}

	updated.input.SetValue("https://api.deepseek.com")
	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated = got.(model)
	if updated.cfg.Provider.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("expected base_url update, got %q", updated.cfg.Provider.BaseURL)
	}
	if updated.startupGuide.CurrentField != startupFieldModel {
		t.Fatalf("expected next step model, got %q", updated.startupGuide.CurrentField)
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected input to be cleared after base_url submit, got %q", updated.input.Value())
	}

	updated.input.SetValue("deepseek-chat")
	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated = got.(model)
	if updated.cfg.Provider.Model != "deepseek-chat" {
		t.Fatalf("expected model update, got %q", updated.cfg.Provider.Model)
	}
	if updated.startupGuide.CurrentField != startupFieldAPIKey {
		t.Fatalf("expected next step api_key, got %q", updated.startupGuide.CurrentField)
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected input to be cleared after model submit, got %q", updated.input.Value())
	}
}

func TestStartupGuideAcceptsValidKeyAndDisablesGuide(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/models" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{"provider":{"type":"openai-compatible","base_url":"`+server.URL+`","model":"gpt-5.4"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	input := textarea.New()
	input.Focus()
	input.SetValue("test-key")
	m := model{
		input: input,
		cfg: config.Config{
			Provider: config.ProviderConfig{
				Type:      "openai-compatible",
				BaseURL:   server.URL,
				Model:     "gpt-5.4",
				APIKeyEnv: "BYTEMIND_API_KEY",
			},
		},
		startupGuide: StartupGuide{
			Active:       true,
			Status:       "Bytemind needs a working API key before chat can start.",
			ConfigPath:   configPath,
			CurrentField: startupFieldAPIKey,
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if updated.startupGuide.Active {
		t.Fatalf("expected startup guide to be disabled after valid key")
	}
	if !strings.Contains(updated.statusNote, "Provider configured and verified") {
		t.Fatalf("unexpected status after setup: %q", updated.statusNote)
	}

	written, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), `"api_key": "test-key"`) {
		t.Fatalf("expected config file to store api key, got %q", string(written))
	}
}

func TestStartupGuideSupportsModelAndBaseURLInput(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(configPath, []byte(`{
  "provider": {
    "type": "openai-compatible",
    "base_url": "https://api.openai.com/v1",
    "model": "gpt-5.4"
  }
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	input := textarea.New()
	input.Focus()
	input.SetValue("model=deepseek-chat")
	m := model{
		input: input,
		cfg: config.Config{
			Provider: config.ProviderConfig{
				Type:      "openai-compatible",
				BaseURL:   "https://api.openai.com/v1",
				Model:     "gpt-5.4",
				APIKeyEnv: "BYTEMIND_API_KEY",
			},
		},
		startupGuide: StartupGuide{
			Active:       true,
			Status:       "Bytemind needs a working API key before chat can start.",
			ConfigPath:   configPath,
			CurrentField: startupFieldType,
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if !updated.startupGuide.Active {
		t.Fatalf("expected startup guide to remain active before key verification")
	}
	if updated.cfg.Provider.Model != "deepseek-chat" {
		t.Fatalf("expected model to update in memory, got %q", updated.cfg.Provider.Model)
	}
	if updated.startupGuide.CurrentField != startupFieldAPIKey {
		t.Fatalf("expected explicit model input to move to api_key step, got %q", updated.startupGuide.CurrentField)
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected input to be cleared after model update, got %q", updated.input.Value())
	}

	updated.input.SetValue("base_url=https://api.deepseek.com")
	got, _ = updated.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated = got.(model)
	if updated.cfg.Provider.BaseURL != "https://api.deepseek.com" {
		t.Fatalf("expected base url to update in memory, got %q", updated.cfg.Provider.BaseURL)
	}
	if updated.startupGuide.CurrentField != startupFieldModel {
		t.Fatalf("expected explicit base_url input to move to model step, got %q", updated.startupGuide.CurrentField)
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected input to be cleared after base_url update, got %q", updated.input.Value())
	}

	raw, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"model": "deepseek-chat"`) {
		t.Fatalf("expected model to be persisted, got %q", string(raw))
	}
	if !strings.Contains(string(raw), `"base_url": "https://api.deepseek.com"`) {
		t.Fatalf("expected base_url to be persisted, got %q", string(raw))
	}
}

func TestStartupGuideStillAllowsSlashCommands(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("/help")
	m := model{
		input: input,
		startupGuide: StartupGuide{
			Active: true,
			Status: "Startup check failed: API key unauthorized",
		},
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if !strings.Contains(updated.statusNote, "Help opened") {
		t.Fatalf("expected /help to execute under startup guide, got status %q", updated.statusNote)
	}
}

func TestRenderStartupGuidePanelInFooter(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	m := model{
		input: input,
		width: 100,
		startupGuide: StartupGuide{
			Active: true,
			Title:  "AI provider not ready",
			Status: "Startup check failed: missing API key",
			Lines:  []string{"1) Add API key"},
		},
	}

	footer := m.renderFooter()
	for _, want := range []string{"AI provider not ready", "missing API key", "Add API key"} {
		if !strings.Contains(footer, want) {
			t.Fatalf("expected footer to contain %q", want)
		}
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
	for _, input := range []string{"continue plan", "\u7ee7\u7eed"} {
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
		"Ctrl+C copy/quit",
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

func TestShiftEnterInsertsNewlineWithoutSubmitting(t *testing.T) {
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

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("shift+enter")})
	updated := got.(model)

	if len(updated.chatItems) != 0 {
		t.Fatalf("expected shift+enter not to submit")
	}
	if updated.input.Value() != "first line\n" {
		t.Fatalf("expected shift+enter to insert newline, got %q", updated.input.Value())
	}
}
func TestCtrlJInsertsNewlineWithoutSubmitting(t *testing.T) {
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

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlJ})
	updated := got.(model)

	if len(updated.chatItems) != 0 {
		t.Fatalf("expected ctrl+j not to submit prompt")
	}
	if updated.input.Value() != "first line\n" {
		t.Fatalf("expected ctrl+j to insert newline, got %q", updated.input.Value())
	}
}

func TestAltVPastesClipboardImage(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	m.clipboard = fakeClipboardImageReader{
		mediaType: "image/png",
		data:      []byte("clipboard"),
		fileName:  "clipboard.png",
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}, Alt: true})
	updated := got.(model)
	if updated.input.Value() != "[Image #1]" {
		t.Fatalf("expected alt+v to paste clipboard image placeholder, got %q", updated.input.Value())
	}
	if !strings.Contains(updated.statusNote, "Attached image from clipboard") {
		t.Fatalf("expected clipboard status note, got %q", updated.statusNote)
	}
}

func TestCtrlVPastesClipboardImage(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	m.clipboard = fakeClipboardImageReader{
		mediaType: "image/png",
		data:      []byte("clipboard"),
		fileName:  "clipboard.png",
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlV})
	updated := got.(model)
	if updated.input.Value() != "[Image #1]" {
		t.Fatalf("expected ctrl+v to paste clipboard image placeholder, got %q", updated.input.Value())
	}
}

func TestCtrlVControlMarkerRunePastesClipboardImage(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	m.clipboard = fakeClipboardImageReader{
		mediaType: "image/png",
		data:      []byte("clipboard"),
		fileName:  "clipboard.png",
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\x16'}})
	updated := got.(model)
	if updated.input.Value() != "[Image #1]" {
		t.Fatalf("expected ctrl+v control marker to paste clipboard image placeholder, got %q", updated.input.Value())
	}
}

func TestCtrlVWithoutImageShowsStatusNote(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	m.clipboard = fakeClipboardImageReader{
		err: errors.New("clipboard has no image"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlV})
	updated := got.(model)
	if updated.input.Value() != "" {
		t.Fatalf("expected input to stay empty, got %q", updated.input.Value())
	}
	if !strings.Contains(strings.ToLower(updated.statusNote), "clipboard has no image") {
		t.Fatalf("expected no-image status note, got %q", updated.statusNote)
	}
}

func TestTerminalPasteEventWithEmptyPayloadPastesClipboardImage(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	m.clipboard = fakeClipboardImageReader{
		mediaType: "image/png",
		data:      []byte("clipboard"),
		fileName:  "clipboard.png",
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Paste: true})
	updated := got.(model)
	if updated.input.Value() != "[Image #1]" {
		t.Fatalf("expected empty paste event to attach clipboard image, got %q", updated.input.Value())
	}
}

func TestTerminalPasteEventWithTextDoesNotForceClipboardImage(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	m.clipboard = fakeClipboardImageReader{
		err: errors.New("clipboard image unavailable"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("hello"), Paste: true})
	updated := got.(model)
	if updated.input.Value() != "hello" {
		t.Fatalf("expected text paste to remain text, got %q", updated.input.Value())
	}
	if strings.Contains(updated.input.Value(), "[Image #") {
		t.Fatalf("expected no image placeholder for text paste")
	}
}

func TestRapidRuneInputForImagePathTriggersFallbackPlaceholder(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat

	imagePath := filepath.Join(m.workspace, "drag.jpg")
	if err := os.WriteFile(imagePath, []byte("jpg-bytes"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	for _, r := range imagePath {
		got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		next := got.(model)
		m = &next
	}
	if m.input.Value() != "[Image #1]" {
		t.Fatalf("expected rapid path input to convert to placeholder, got %q", m.input.Value())
	}
}

func TestImmediateEnterAfterPasteStillSubmits(t *testing.T) {
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

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.chatItems) < 1 {
		t.Fatalf("expected enter to submit immediately, got %d chat items", len(updated.chatItems))
	}
	if updated.chatItems[0].Body != "first line" {
		t.Fatalf("expected submitted body to match input text, got %q", updated.chatItems[0].Body)
	}
}

func TestPasteEnterDoesNotSubmitAndKeepsNewline(t *testing.T) {
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

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter, Paste: true})
	updated := got.(model)

	if len(updated.chatItems) != 0 {
		t.Fatalf("expected paste enter not to submit, got %d chat items", len(updated.chatItems))
	}
	if !strings.Contains(updated.input.Value(), "\n") {
		t.Fatalf("expected pasted enter to be inserted as newline, got %q", updated.input.Value())
	}
}

func TestSuppressedEnterAfterPasteIsInsertedAsNewline(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("line1")
	input.CursorEnd()

	m := model{
		screen:         screenChat,
		input:          input,
		workspace:      "E:\\bytemind",
		sess:           session.New("E:\\bytemind"),
		lastPasteAt:    time.Now(),
		lastInputAt:    time.Now(),
		inputBurstSize: 12,
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.chatItems) != 0 {
		t.Fatalf("expected suppressed enter not to submit, got %d chat items", len(updated.chatItems))
	}
	if updated.input.Value() != "line1\n" {
		t.Fatalf("expected suppressed enter to become newline, got %q", updated.input.Value())
	}
}

func TestEnterSubmitsMultilinePrompt(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetWidth(40)
	input.SetHeight(3)
	input.SetValue("first line\nsecond line")
	input.CursorEnd()

	m := model{
		screen:    screenChat,
		input:     input,
		workspace: "E:\\bytemind",
		sess:      session.New("E:\\bytemind"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.chatItems) < 1 {
		t.Fatalf("expected enter to submit multiline prompt, got %d chat items", len(updated.chatItems))
	}
	if updated.chatItems[0].Body != "first line\nsecond line" {
		t.Fatalf("expected multiline body to be preserved, got %q", updated.chatItems[0].Body)
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
		"/skill clear",
		"/skill delete <name>",
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
		"Ctrl+C copy/quit",
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

func TestSyncInputStyleShowsStartupStepPlaceholder(t *testing.T) {
	input := textarea.New()
	m := model{
		input: input,
		startupGuide: StartupGuide{
			Active:       true,
			CurrentField: startupFieldModel,
		},
	}

	m.syncInputStyle()

	if !strings.Contains(m.input.Placeholder, "Step 3/4") {
		t.Fatalf("expected startup step placeholder, got %q", m.input.Placeholder)
	}
	if !strings.Contains(m.input.Placeholder, "model") {
		t.Fatalf("expected startup model placeholder, got %q", m.input.Placeholder)
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

	for _, want := range []string{"/help", "/session", "/skills-select", "/new", "/compact", "/quit"} {
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

func TestHandleSlashCompactCompactsSession(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.Messages = append(sess.Messages,
		llm.NewUserTextMessage("first ask"),
		llm.NewAssistantTextMessage(strings.Repeat("history details ", 30)),
		llm.NewUserTextMessage("second ask"),
		llm.NewAssistantTextMessage(strings.Repeat("more details ", 30)),
	)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &compactCommandTestClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "Goal: keep building\nDone: reviewed history\nPending: continue coding"},
		},
	}
	runner := agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
			Stream:   false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	m := model{
		runner:    runner,
		store:     store,
		sess:      sess,
		workspace: workspace,
		screen:    screenChat,
	}
	if err := m.handleSlashCommand("/compact"); err != nil {
		t.Fatalf("expected /compact to succeed, got %v", err)
	}
	if m.statusNote != "Conversation compacted." {
		t.Fatalf("expected compacted status note, got %q", m.statusNote)
	}
	if len(sess.Messages) != 1 || sess.Messages[0].Role != llm.RoleAssistant {
		t.Fatalf("expected compacted session summary message, got %#v", sess.Messages)
	}
	if !strings.Contains(sess.Messages[0].Text(), "Goal: keep building") {
		t.Fatalf("expected persisted summary content, got %q", sess.Messages[0].Text())
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one compaction LLM request, got %d", len(client.requests))
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
	if _, err := runner.ActivateSkill(sess, "/bug-investigation", nil); err != nil {
		t.Fatalf("expected /bug-investigation activation to succeed, got %v", err)
	}
	if m.sess.ActiveSkill == nil || m.sess.ActiveSkill.Name != "bug-investigation" {
		t.Fatalf("expected bug-investigation active before switch, got %#v", m.sess.ActiveSkill)
	}
	if _, err := runner.ActivateSkill(sess, "/review", map[string]string{"severity": "high"}); err != nil {
		t.Fatalf("expected /review activation to succeed, got %v", err)
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

func TestHandleSlashSkillAuthorIsUnsupported(t *testing.T) {
	workspace := t.TempDir()

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
		input:     textarea.New(),
	}

	if err := m.handleSlashCommand("/skill author"); err == nil {
		t.Fatalf("expected /skill author to fail")
	} else if !strings.Contains(err.Error(), "usage: /skill <clear|delete> ...") {
		t.Fatalf("unexpected error for /skill author: %v", err)
	}
	if err := m.handleSlashCommand("/skill author review-plus review backend changes and report risks"); err == nil {
		t.Fatalf("expected /skill author <name> to fail")
	} else if !strings.Contains(err.Error(), "usage: /skill <clear|delete> ...") {
		t.Fatalf("unexpected error for /skill author <name>: %v", err)
	}
}

func TestHandleSlashSkillDeleteDeletesProjectSkill(t *testing.T) {
	workspace := t.TempDir()
	skillDir := filepath.Join(workspace, ".bytemind", "skills", "review-plus")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{"name":"review-plus","description":"review"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# review-plus"), 0o644); err != nil {
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
		input:     textarea.New(),
	}

	if _, err := runner.ActivateSkill(sess, "/review-plus", nil); err != nil {
		t.Fatalf("expected activate before clear, got %v", err)
	}
	if err := m.handleSlashCommand("/skill delete review-plus"); err != nil {
		t.Fatalf("expected /skill delete to succeed, got %v", err)
	}
	if _, err := os.Stat(skillDir); !os.IsNotExist(err) {
		t.Fatalf("expected skill directory removed, stat err=%v", err)
	}
	if m.sess.ActiveSkill != nil {
		t.Fatalf("expected active skill cleared, got %#v", m.sess.ActiveSkill)
	}
	if len(m.chatItems) < 2 || !strings.Contains(m.chatItems[len(m.chatItems)-1].Body, "Deleted project skill") {
		t.Fatalf("expected clear command response, got %#v", m.chatItems)
	}
}

func TestHandleSlashSkillClearOnlyClearsActiveSkill(t *testing.T) {
	workspace := t.TempDir()
	skillDir := filepath.Join(workspace, ".bytemind", "skills", "review-plus")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{"name":"review-plus","description":"review"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# review-plus"), 0o644); err != nil {
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
		input:     textarea.New(),
	}

	if _, err := runner.ActivateSkill(sess, "/review-plus", nil); err != nil {
		t.Fatalf("expected activate before clear, got %v", err)
	}
	if err := m.handleSlashCommand("/skill clear"); err != nil {
		t.Fatalf("expected /skill clear to succeed, got %v", err)
	}
	if m.sess.ActiveSkill != nil {
		t.Fatalf("expected active skill cleared, got %#v", m.sess.ActiveSkill)
	}
	if len(m.chatItems) < 2 || !strings.Contains(m.chatItems[len(m.chatItems)-1].Body, "Cleared active skill") {
		t.Fatalf("expected clear status response, got %#v", m.chatItems)
	}
}

func TestCommandPaletteDoesNotExposeSkillAuthor(t *testing.T) {
	input := textarea.New()
	input.SetValue("/skill")
	m := model{input: input}
	items := m.filteredCommands()
	for _, item := range items {
		if strings.EqualFold(strings.TrimSpace(item.Name), "/skill author") {
			t.Fatalf("command palette should not expose /skill author, got %+v", item)
		}
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
		if item.Name == "review" && item.Usage == "/review" && item.Kind == "skill" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /review skill command in filtered commands, got %+v", items)
	}
}

func TestFilteredCommandsIncludeProjectSkillSlashCommands(t *testing.T) {
	workspace := t.TempDir()
	skillDir := filepath.Join(workspace, ".bytemind", "skills", "review-plus")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "skill.json"), []byte(`{
  "name":"review-plus",
  "description":"review project changes",
  "entry":{"slash":"/review-plus"}
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# review-plus"), 0o644); err != nil {
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
	input.SetValue("/review")
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
		if item.Name == "review-plus" && item.Usage == "/review-plus" && item.Kind == "skill" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected /review-plus project skill command in filtered commands, got %+v", items)
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

func TestCommandPaletteBusyPlainTextQueuesBTW(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("focus only on unit tests")
	input.CursorEnd()

	canceled := false
	m := model{
		screen:      screenChat,
		commandOpen: true,
		busy:        true,
		input:       input,
		runCancel:   func() { canceled = true },
		sess:        session.New("E:\\bytemind"),
		workspace:   "E:\\bytemind",
	}

	got, _ := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if !canceled {
		t.Fatalf("expected command palette busy submit to cancel active run")
	}
	if updated.commandOpen {
		t.Fatalf("expected command palette to close after busy plain-text submit")
	}
	if len(updated.pendingBTW) != 1 || updated.pendingBTW[0] != "focus only on unit tests" {
		t.Fatalf("expected plain text to queue as btw, got %#v", updated.pendingBTW)
	}
	if !updated.interrupting {
		t.Fatalf("expected busy plain-text submit to enter interrupting state")
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

func TestMentionPaletteEnterImageCandidateKeepsMentionTextAndBindsAsset(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	if err := os.WriteFile(filepath.Join(m.workspace, "2.1.jpg"), []byte("jpg"), 0o644); err != nil {
		t.Fatalf("write image fixture: %v", err)
	}

	m.input.SetValue("@2.1")
	m.mentionIndex = mention.NewStaticWorkspaceFileIndex([]mention.Candidate{
		{Path: "2.1.jpg", BaseName: "2.1.jpg", TypeTag: "jpg"},
	}, 0, false)
	m.syncInputOverlays()
	if !m.mentionOpen {
		t.Fatalf("expected mention palette to open")
	}

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if cmd != nil {
		t.Fatalf("expected image mention selection not to submit")
	}
	if updated.input.Value() != "@2.1.jpg " {
		t.Fatalf("expected image mention to keep @path text, got %q", updated.input.Value())
	}
	if updated.mentionOpen {
		t.Fatalf("expected mention palette to close after image selection")
	}
	if !strings.Contains(updated.statusNote, "Attached image") {
		t.Fatalf("expected attached image note, got %q", updated.statusNote)
	}
	if len(updated.sess.Conversation.Assets.Images) != 1 {
		t.Fatalf("expected image metadata to be stored, got %d", len(updated.sess.Conversation.Assets.Images))
	}
	key := normalizeImageMentionPath("2.1.jpg")
	if strings.TrimSpace(string(updated.inputImageMentions[key])) == "" {
		t.Fatalf("expected mention image binding for key %q", key)
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
	for _, want := range []string{"/help", "/session", "/skills-select"} {
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
			{Kind: "tool", Title: "Tool Call | read_file", Body: "Read internal/tui/model.go lines 1-20", Status: "done"},
		},
	}

	got := m.renderConversation()
	if !strings.Contains(got, "Tool Call | read_file") {
		t.Fatalf("expected conversation to show tool entry, got %q", got)
	}
	if !strings.Contains(got, "Read internal/tui/model.go lines 1-20") {
		t.Fatalf("expected conversation to show tool summary, got %q", got)
	}
}

func TestRebuildSessionTimelineParsesUserToolResultParts(t *testing.T) {
	sess := &session.Session{
		Messages: []llm.Message{
			llm.NewUserTextMessage("please inspect"),
			{
				Role: llm.RoleAssistant,
				Parts: []llm.Part{{
					Type: llm.PartToolUse,
					ToolUse: &llm.ToolUsePart{
						ID:        "call-1",
						Name:      "read_file",
						Arguments: `{"path":"a.txt"}`,
					},
				}},
			},
			llm.NewToolResultMessage("call-1", `{"path":"a.txt","content":"ok"}`),
		},
	}

	items, runs := rebuildSessionTimeline(sess)
	if len(items) != 2 {
		t.Fatalf("expected user + tool items, got %#v", items)
	}
	if items[1].Kind != "tool" || !strings.Contains(items[1].Title, "Tool Call | read_file") {
		t.Fatalf("expected tool item from tool_result part, got %#v", items[1])
	}
	if len(runs) != 1 || runs[0].Name != "read_file" {
		t.Fatalf("expected tool run reconstructed, got %#v", runs)
	}
}

func TestRebuildSessionTimelineFallsBackToGenericToolNameForUnknownToolUseID(t *testing.T) {
	sess := &session.Session{
		Messages: []llm.Message{
			llm.NewToolResultMessage("missing-call-id", `{"ok":true}`),
		},
	}

	items, runs := rebuildSessionTimeline(sess)
	if len(items) != 1 {
		t.Fatalf("expected only one tool item, got %#v", items)
	}
	if items[0].Kind != "tool" || items[0].Title != "Tool Call | tool" {
		t.Fatalf("expected fallback tool title for unknown tool use id, got %#v", items[0])
	}
	if len(runs) != 1 || runs[0].Name != "tool" {
		t.Fatalf("expected fallback tool run name, got %#v", runs)
	}
}

func TestRebuildSessionTimelineParsesLegacyToolRoleMessage(t *testing.T) {
	sess := &session.Session{
		Messages: []llm.Message{
			llm.NewAssistantTextMessage("analysis complete"),
			{
				Role:       llm.Role("tool"),
				ToolCallID: "missing-call-id",
				Content:    `{"path":"a.txt","content":"ok"}`,
			},
		},
	}

	items, runs := rebuildSessionTimeline(sess)
	if len(items) != 2 {
		t.Fatalf("expected assistant + tool items, got %#v", items)
	}
	if items[0].Kind != "assistant" || !strings.Contains(items[0].Body, "analysis complete") {
		t.Fatalf("expected assistant text item from legacy message, got %#v", items[0])
	}
	if items[1].Kind != "tool" || items[1].Title != "Tool Call | tool" {
		t.Fatalf("expected fallback tool title for legacy tool message, got %#v", items[1])
	}
	if len(runs) != 1 || runs[0].Name != "tool" {
		t.Fatalf("expected tool run reconstructed from legacy tool message, got %#v", runs)
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
	if strings.TrimSpace(m.chatItems[2].Body) != "" {
		t.Fatalf("expected tool call body to hide params, got %q", m.chatItems[2].Body)
	}

	m.handleAgentEvent(agent.Event{
		Type:       agent.EventToolCallCompleted,
		ToolName:   "read_file",
		ToolResult: `{"path":"internal/tui/model.go","start_line":1,"end_line":20}`,
	})
	if len(m.chatItems) != 3 {
		t.Fatalf("expected completed tool to update existing tool call, got %d", len(m.chatItems))
	}
	if m.chatItems[2].Kind != "tool" || !strings.Contains(m.chatItems[2].Title, "Tool Call | read_file") {
		t.Fatalf("expected tool call entry after completion, got %+v", m.chatItems[2])
	}
	if m.chatItems[2].Status != "done" {
		t.Fatalf("expected completed tool call status to be done, got %q", m.chatItems[2].Status)
	}
	if !strings.Contains(m.chatItems[2].Body, "Read internal/tui/model.go lines 1-20") {
		t.Fatalf("expected completed tool summary in tool call item, got %q", m.chatItems[2].Body)
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
	if m.chatItems[1].Status != "thinking" || !strings.Contains(m.chatItems[1].Body, "Thinking") {
		t.Fatalf("expected thinking assistant card after delta, got %+v", m.chatItems[1])
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
	if strings.TrimSpace(m.chatItems[1].Body) != "" {
		t.Fatalf("expected tool call entry to omit params body, got %q", m.chatItems[1].Body)
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
	if strings.TrimSpace(m.chatItems[1].Body) != "" {
		t.Fatalf("expected tool call entry to omit params body, got %q", m.chatItems[1].Body)
	}
}

func TestRenderChatSectionToolHeaderOmitsStatusWords(t *testing.T) {
	got := renderChatSection(chatEntry{
		Kind:   "tool",
		Title:  "Tool Call | list_files",
		Body:   "",
		Status: "running",
	}, 64)

	if strings.Contains(got, "running") || strings.Contains(got, "done") || strings.Contains(got, "pending") {
		t.Fatalf("expected tool header to omit status words, got %q", got)
	}
	if strings.Contains(got, "params:") || strings.Contains(got, "{\"") {
		t.Fatalf("expected tool section to hide params content, got %q", got)
	}
}

func TestAssistantDeltaPlanningTextRendersAsThinking(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "please inspect this project", Status: "final"},
		},
		streamingIndex: -1,
	}

	m.handleAgentEvent(agent.Event{
		Type:    agent.EventAssistantDelta,
		Content: "I will first inspect structure and config, then code organization and dependencies, and finally verify with build and tests.",
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
	if m.chatItems[1].Title != thinkingLabel || m.chatItems[1].Status != "thinking_done" {
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

func TestRunFinishedKeepsStreamingSlotForLateAssistantMessage(t *testing.T) {
	m := model{
		async: make(chan tea.Msg, 1),
		busy:  true,
		chatItems: []chatEntry{
			{Kind: "user", Title: "You", Body: "test", Status: "final"},
			{Kind: "assistant", Title: assistantLabel, Body: "received,", Status: "streaming"},
		},
		streamingIndex: 1,
	}

	got, _ := m.Update(runFinishedMsg{})
	updated := got.(model)
	if updated.streamingIndex != 1 {
		t.Fatalf("expected run finished to keep streaming index for late final message, got %d", updated.streamingIndex)
	}

	updated.handleAgentEvent(agent.Event{
		Type:    agent.EventAssistantMessage,
		Content: "received, response looks good.",
	})

	if len(updated.chatItems) != 2 {
		t.Fatalf("expected late final message to update existing assistant card, got %d items", len(updated.chatItems))
	}
	last := updated.chatItems[1]
	if last.Status != "final" || strings.TrimSpace(last.Body) != "received, response looks good." {
		t.Fatalf("expected assistant card to be finalized in place, got %+v", last)
	}
}
func TestBusyInputStillEditable(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		screen:    screenChat,
		busy:      true,
		input:     input,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	updated := got.(model)
	if updated.input.Value() != "a" {
		t.Fatalf("expected busy input to stay editable, got %q", updated.input.Value())
	}
}

func TestBusyEnterQueuesBTWAndCancelsRun(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("focus only on unit tests")
	input.CursorEnd()

	canceled := false
	m := model{
		screen:    screenChat,
		busy:      true,
		input:     input,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		runCancel: func() { canceled = true },
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if !canceled {
		t.Fatalf("expected busy enter to cancel the active run")
	}
	if !updated.interrupting {
		t.Fatalf("expected model to enter interrupting state")
	}
	if len(updated.pendingBTW) != 1 || updated.pendingBTW[0] != "focus only on unit tests" {
		t.Fatalf("expected pending btw queue to capture input, got %#v", updated.pendingBTW)
	}
	if updated.input.Value() != "" {
		t.Fatalf("expected btw submit to reset input, got %q", updated.input.Value())
	}
	if len(updated.chatItems) != 1 || updated.chatItems[0].Body != "focus only on unit tests" {
		t.Fatalf("expected btw submit to append a user chat entry, got %#v", updated.chatItems)
	}
	if !strings.Contains(updated.chatItems[0].Meta, "btw") {
		t.Fatalf("expected btw marker in chat meta, got %q", updated.chatItems[0].Meta)
	}
}

func TestBusyEnterSuppressedAfterRecentMultilinePaste(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("Design a plugin platform\n- dynamic plugin loading\n- permission isolation")
	input.CursorEnd()

	canceled := false
	m := model{
		screen:      screenChat,
		busy:        true,
		input:       input,
		lastPasteAt: time.Now(),
		sess:        session.New("E:\\bytemind"),
		workspace:   "E:\\bytemind",
		runCancel:   func() { canceled = true },
	}

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if cmd != nil {
		t.Fatalf("expected suppressed enter not to schedule a command")
	}
	if canceled {
		t.Fatalf("expected suppressed enter not to cancel current run")
	}
	if updated.interrupting || len(updated.pendingBTW) != 0 || len(updated.chatItems) != 0 {
		t.Fatalf("expected no BTW side effects, got interrupting=%v pending=%#v chat=%#v", updated.interrupting, updated.pendingBTW, updated.chatItems)
	}
}

func TestBusyEnterSuppressedForRecentPasteBurstSingleLine(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("dynamic plugin loading")
	input.CursorEnd()

	canceled := false
	now := time.Now()
	m := model{
		screen:      screenChat,
		busy:        true,
		input:       input,
		lastPasteAt: now,
		lastInputAt: now,
		sess:        session.New("E:\\bytemind"),
		workspace:   "E:\\bytemind",
		runCancel:   func() { canceled = true },
	}

	got, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if cmd != nil {
		t.Fatalf("expected suppressed burst enter not to schedule a command")
	}
	if canceled {
		t.Fatalf("expected suppressed burst enter not to cancel current run")
	}
	if updated.interrupting || len(updated.pendingBTW) != 0 || len(updated.chatItems) != 0 {
		t.Fatalf("expected no BTW side effects, got interrupting=%v pending=%#v chat=%#v", updated.interrupting, updated.pendingBTW, updated.chatItems)
	}
}

func TestBusyEnterInToolPhaseDefersBTWCancel(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("change plan after this step")
	input.CursorEnd()

	canceled := false
	m := model{
		screen:    screenChat,
		busy:      true,
		phase:     "tool",
		input:     input,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
		runCancel: func() { canceled = true },
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if canceled {
		t.Fatalf("expected tool phase btw to defer cancel until tool step completes")
	}
	if !updated.interrupting || !updated.interruptSafe {
		t.Fatalf("expected deferred interrupt flags, got interrupting=%v interruptSafe=%v", updated.interrupting, updated.interruptSafe)
	}
	if updated.statusNote != "BTW queued. Waiting for current tool step to finish..." {
		t.Fatalf("expected deferred tool note, got %q", updated.statusNote)
	}
}

func TestRenderChatCardToolUsesVisualSeparator(t *testing.T) {
	got := renderChatCard(chatEntry{
		Kind:   "tool",
		Title:  "Tool Call | read_file",
		Body:   "Read internal/tui/model.go lines 1-20",
		Status: "done",
	}, 64)

	if !strings.Contains(got, "\u2502") && !strings.Contains(got, "|") {
		t.Fatalf("expected tool card to include a left border separator, got %q", got)
	}
	if !strings.Contains(got, "Tool Call | read_file") {
		t.Fatalf("expected tool card title to render, got %q", got)
	}
}

func TestSubmitBTWWithoutRunCancelRestartsImmediately(t *testing.T) {
	input := textarea.New()
	input.Focus()
	m := model{
		async:     make(chan tea.Msg, 1),
		busy:      true,
		mode:      modeBuild,
		input:     input,
		sess:      session.New("E:\\bytemind"),
		workspace: "E:\\bytemind",
	}

	got, cmd := m.submitBTW("continue with deletion")
	updated := got.(model)

	if cmd == nil {
		t.Fatalf("expected fallback btw path to start a new run command")
	}
	if !updated.busy {
		t.Fatalf("expected model to become busy after immediate btw restart")
	}
	if updated.interrupting {
		t.Fatalf("expected interrupting state to clear after immediate restart")
	}
	if updated.interruptSafe {
		t.Fatalf("expected interruptSafe to be false after immediate restart")
	}
	if len(updated.pendingBTW) != 0 {
		t.Fatalf("expected pending btw queue to be consumed, got %#v", updated.pendingBTW)
	}
	if updated.runCancel == nil {
		t.Fatalf("expected immediate restart to set runCancel")
	}
	if updated.statusNote != "BTW accepted. Restarting with your update..." {
		t.Fatalf("expected immediate restart status note, got %q", updated.statusNote)
	}
}

func TestToolCallCompletedTriggersDeferredBTWCancel(t *testing.T) {
	canceled := false
	m := model{
		interrupting:  true,
		interruptSafe: true,
		pendingBTW:    []string{"change plan"},
		runCancel:     func() { canceled = true },
	}

	m.handleAgentEvent(agent.Event{
		Type:       agent.EventToolCallCompleted,
		ToolName:   "read_file",
		ToolResult: `{"path":"internal/tui/model.go","start_line":1,"end_line":3}`,
	})

	if !canceled {
		t.Fatalf("expected deferred btw cancel to trigger after tool completion")
	}
	if m.interruptSafe {
		t.Fatalf("expected deferred interrupt flag to clear after cancel")
	}
	if m.phase != "interrupting" {
		t.Fatalf("expected phase to switch to interrupting, got %q", m.phase)
	}
}

func TestRunFinishedWithPendingBTWRestartsRun(t *testing.T) {
	m := model{
		async:        make(chan tea.Msg, 1),
		busy:         true,
		activeRunID:  2,
		runSeq:       2,
		interrupting: true,
		pendingBTW:   []string{"first update", "second update"},
		mode:         modeBuild,
		sess:         session.New("E:\\bytemind"),
		workspace:    "E:\\bytemind",
	}

	got, cmd := m.Update(runFinishedMsg{RunID: 2, Err: context.Canceled})
	updated := got.(model)

	if cmd == nil {
		t.Fatalf("expected interrupted run to schedule a follow-up run")
	}
	if !updated.busy {
		t.Fatalf("expected model to restart immediately with pending btw prompt")
	}
	if len(updated.chatItems) != 1 || updated.chatItems[0].Kind != "system" {
		t.Fatalf("expected system restart notice before resumed run, got %#v", updated.chatItems)
	}
	if !strings.Contains(updated.chatItems[0].Body, "BTW interrupt accepted") {
		t.Fatalf("expected btw restart notice, got %#v", updated.chatItems[0])
	}
	if !strings.Contains(updated.chatItems[0].Body, "2 updates") {
		t.Fatalf("expected restart notice to include update count, got %#v", updated.chatItems[0])
	}
	if updated.interrupting {
		t.Fatalf("expected interrupting state to clear after restart")
	}
	if len(updated.pendingBTW) != 0 {
		t.Fatalf("expected pending btw queue to be consumed, got %#v", updated.pendingBTW)
	}
	if updated.runCancel == nil {
		t.Fatalf("expected restart to register a new cancel function")
	}
	if updated.phase != "thinking" {
		t.Fatalf("expected restart phase to return to thinking, got %q", updated.phase)
	}
	if !strings.Contains(updated.statusNote, "Restarting with 2 updates") {
		t.Fatalf("expected restart status note, got %q", updated.statusNote)
	}
	if updated.activeRunID == 0 {
		t.Fatalf("expected resumed run to have a new active run id")
	}
}

func TestClassifyRunFinish(t *testing.T) {
	if got := classifyRunFinish(nil, false); got != runFinishReasonCompleted {
		t.Fatalf("expected completed, got %q", got)
	}
	if got := classifyRunFinish(context.Canceled, false); got != runFinishReasonCanceled {
		t.Fatalf("expected canceled, got %q", got)
	}
	if got := classifyRunFinish(errors.New("boom"), false); got != runFinishReasonFailed {
		t.Fatalf("expected failed, got %q", got)
	}
	if got := classifyRunFinish(nil, true); got != runFinishReasonBTWRestart {
		t.Fatalf("expected btw restart, got %q", got)
	}
}

func TestQueueBTWUpdateKeepsMostRecentEntries(t *testing.T) {
	queue, dropped := queueBTWUpdate([]string{"1", "2", "3", "4", "5"}, "6")
	if dropped != 1 {
		t.Fatalf("expected one dropped entry, got %d", dropped)
	}
	if len(queue) != maxPendingBTW {
		t.Fatalf("expected capped queue length %d, got %d", maxPendingBTW, len(queue))
	}
	want := []string{"2", "3", "4", "5", "6"}
	for i := range want {
		if queue[i] != want[i] {
			t.Fatalf("expected queue[%d]=%q, got %q", i, want[i], queue[i])
		}
	}
}

func TestFormatBTWUpdateScope(t *testing.T) {
	if got := formatBTWUpdateScope(0); got != "your latest update" {
		t.Fatalf("expected default scope text, got %q", got)
	}
	if got := formatBTWUpdateScope(1); got != "your latest update" {
		t.Fatalf("expected single-entry scope text, got %q", got)
	}
	if got := formatBTWUpdateScope(3); got != "3 updates" {
		t.Fatalf("expected multi-entry scope text, got %q", got)
	}
}

func TestComposeBTWPromptSingleEntryKeepsContinuationContext(t *testing.T) {
	got := composeBTWPrompt([]string{"delete calculator.py"})
	if !strings.Contains(got, "Continue the same task") {
		t.Fatalf("expected single btw prompt to preserve continuation context, got %q", got)
	}
	if !strings.Contains(got, "delete calculator.py") {
		t.Fatalf("expected single btw prompt to include update content, got %q", got)
	}
}

func TestComposeBTWPromptIgnoresEmptyEntries(t *testing.T) {
	got := composeBTWPrompt([]string{"", "   ", "\n\t"})
	if got != "" {
		t.Fatalf("expected empty btw prompt when all entries are blank, got %q", got)
	}
}

func TestSubmitBTWShowsDropHintWhenQueueCapped(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("new update")
	input.CursorEnd()

	m := model{
		screen:       screenChat,
		busy:         true,
		interrupting: true,
		input:        input,
		pendingBTW:   []string{"1", "2", "3", "4", "5"},
		sess:         session.New("E:\\bytemind"),
		workspace:    "E:\\bytemind",
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.pendingBTW) != maxPendingBTW {
		t.Fatalf("expected capped pending queue length %d, got %d", maxPendingBTW, len(updated.pendingBTW))
	}
	if updated.pendingBTW[0] != "2" || updated.pendingBTW[len(updated.pendingBTW)-1] != "new update" {
		t.Fatalf("expected oldest entry to be dropped, got %#v", updated.pendingBTW)
	}
	if !strings.Contains(updated.statusNote, "dropped 1 older") {
		t.Fatalf("expected drop hint in status note, got %q", updated.statusNote)
	}
}

func TestNewSessionClearsInterruptState(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := session.New(workspace)
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}

	input := textarea.New()
	input.SetValue("pending input")
	m := model{
		store:         store,
		sess:          current,
		workspace:     workspace,
		input:         input,
		pendingBTW:    []string{"keep this"},
		interrupting:  true,
		interruptSafe: true,
		runCancel:     func() {},
		activeRunID:   9,
	}

	if err := m.newSession(); err != nil {
		t.Fatalf("expected newSession to succeed, got %v", err)
	}
	if m.interrupting || m.interruptSafe {
		t.Fatalf("expected interrupt flags to clear, got interrupting=%v interruptSafe=%v", m.interrupting, m.interruptSafe)
	}
	if len(m.pendingBTW) != 0 {
		t.Fatalf("expected pending btw queue cleared, got %#v", m.pendingBTW)
	}
	if m.runCancel != nil {
		t.Fatalf("expected runCancel cleared on new session")
	}
	if m.activeRunID != 0 {
		t.Fatalf("expected activeRunID reset, got %d", m.activeRunID)
	}
	if m.screen != screenLanding {
		t.Fatalf("expected new session to switch to landing screen, got %q", m.screen)
	}
}

func TestResumeSessionClearsInterruptState(t *testing.T) {
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	workspace := t.TempDir()
	current := session.New(workspace)
	target := session.New(workspace)
	if err := store.Save(current); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(target); err != nil {
		t.Fatal(err)
	}

	m := model{
		store:         store,
		sess:          current,
		workspace:     workspace,
		sessions:      []session.Summary{{ID: target.ID}},
		pendingBTW:    []string{"queued"},
		interrupting:  true,
		interruptSafe: true,
		runCancel:     func() {},
		activeRunID:   7,
	}

	if err := m.resumeSession(target.ID); err != nil {
		t.Fatalf("expected resumeSession to succeed, got %v", err)
	}
	if m.sess == nil || m.sess.ID != target.ID {
		t.Fatalf("expected target session to become active, got %#v", m.sess)
	}
	if m.interrupting || m.interruptSafe {
		t.Fatalf("expected interrupt flags to clear, got interrupting=%v interruptSafe=%v", m.interrupting, m.interruptSafe)
	}
	if len(m.pendingBTW) != 0 {
		t.Fatalf("expected pending btw queue cleared, got %#v", m.pendingBTW)
	}
	if m.runCancel != nil {
		t.Fatalf("expected runCancel cleared on resume")
	}
	if m.activeRunID != 0 {
		t.Fatalf("expected activeRunID reset, got %d", m.activeRunID)
	}
	if m.screen != screenChat {
		t.Fatalf("expected resume to switch to chat screen, got %q", m.screen)
	}
}

func TestUpdateIgnoresStaleRunFinishedMsg(t *testing.T) {
	m := model{
		async:       make(chan tea.Msg, 1),
		busy:        true,
		activeRunID: 5,
		statusNote:  "Running...",
		phase:       "responding",
	}

	got, cmd := m.Update(runFinishedMsg{RunID: 4})
	updated := got.(model)

	if cmd == nil {
		t.Fatalf("expected stale run message handling to keep waiting for async events")
	}
	if !updated.busy {
		t.Fatalf("expected stale run message not to stop the active run")
	}
	if updated.activeRunID != 5 {
		t.Fatalf("expected active run id to remain unchanged, got %d", updated.activeRunID)
	}
	if updated.statusNote != "Running..." {
		t.Fatalf("expected stale run message not to rewrite status, got %q", updated.statusNote)
	}
}

func TestBTWCommandInIdleSubmitsPrompt(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("/btw tighten the test scope")
	input.CursorEnd()
	m := model{
		screen:    screenChat,
		input:     input,
		workspace: "E:\\bytemind",
		sess:      session.New("E:\\bytemind"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if len(updated.chatItems) == 0 || updated.chatItems[0].Body != "tighten the test scope" {
		t.Fatalf("expected /btw in idle mode to submit its message, got %#v", updated.chatItems)
	}
	if !updated.busy {
		t.Fatalf("expected /btw in idle mode to start a run")
	}
}

func TestBTWCommandRequiresMessage(t *testing.T) {
	input := textarea.New()
	input.Focus()
	input.SetValue("/btw")
	input.CursorEnd()
	m := model{
		screen:    screenChat,
		input:     input,
		workspace: "E:\\bytemind",
		sess:      session.New("E:\\bytemind"),
	}

	got, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)
	if updated.statusNote != "usage: /btw <message>" {
		t.Fatalf("expected usage hint for empty /btw, got %q", updated.statusNote)
	}
	if updated.busy {
		t.Fatalf("expected empty /btw not to start a run")

	}
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
	if !strings.Contains(got, "Explanation") {
		t.Fatalf("expected explanation text to remain, got %q", got)
	}
	if !strings.Contains(got, "- first") {
		t.Fatalf("expected markdown list marker to be normalized, got %q", got)
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

func TestFormatChatBodyHelpMarkdownAppliesVisualStyles(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "# Bytemind Help\n## Entry Points\n- `/help`: show help",
	}

	got := formatChatBody(item, 80)
	if !strings.Contains(got, "Bytemind Help") {
		t.Fatalf("expected help title text to remain, got %q", got)
	}
	if !strings.Contains(got, "`/help`") {
		t.Fatalf("expected help markdown list to keep inline command formatting, got %q", got)
	}
}

func TestFormatChatBodyHighlightsSemanticChineseLines(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "\u7b2c\u4e00\u9636\u6bb5\uff1a\u57fa\u7840\u51c6\u5907\uff081-2\u4e2a\u6708\uff09\n\u5b66\u4e60\u5185\u5bb9\uff1a\n\\u76ee\\u6807\\uff1a \\u5efa\\u7acb\\u57fa\\u7840\\u80fd\\u529b",
	}
	got := formatChatBody(item, 80)
	for _, want := range []string{
		"\u7b2c\u4e00\u9636\u6bb5\uff1a\u57fa\u7840\u51c6\u5907\uff081-2\u4e2a\u6708\uff09",
		"\u5b66\u4e60\u5185\u5bb9\uff1a",
		"\\u76ee\\u6807\\uff1a \\u5efa\\u7acb\\u57fa\\u7840\\u80fd\\u529b",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected semantic lines to remain (%q), got %q", want, got)
		}
	}
}

func TestFormatChatBodyNonAssistantUsesSemanticHighlightPipeline(t *testing.T) {
	item := chatEntry{
		Kind: "system",
		Body: "\\u6ce8\\u610f\\uff1a \\u8be5\\u64cd\\u4f5c\\u4e0d\\u53ef\\u64a4\\u9500\n\\u76ee\\u6807\\uff1a \\u5148\\u5907\\u4efd\\u6570\\u636e",
	}
	got := formatChatBody(item, 80)
	for _, want := range []string{
		"\\u6ce8\\u610f\\uff1a \\u8be5\\u64cd\\u4f5c\\u4e0d\\u53ef\\u64a4\\u9500",
		"\\u76ee\\u6807\\uff1a \\u5148\\u5907\\u4efd\\u6570\\u636e",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected semantic plain body rendering (%q), got %q", want, got)
		}
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

func TestFormatChatBodyStripsInlineMarkdownTokens(t *testing.T) {
	item := chatEntry{
		Kind: "assistant",
		Body: "We are **ByteMind** project, support go test ./... and [docs](https://example.com/docs).",
	}

	got := formatChatBody(item, 120)
	for _, unwanted := range []string{"**", "`", "[", "]("} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("expected inline markdown token %q to be removed, got %q", unwanted, got)
		}
	}
	if !strings.Contains(got, "ByteMind") {
		t.Fatalf("expected bold content to remain after normalization, got %q", got)
	}
	if !strings.Contains(got, "go test ./...") {
		t.Fatalf("expected inline code content to remain after normalization, got %q", got)
	}
	if !strings.Contains(got, "docs (https://example.com/docs)") {
		t.Fatalf("expected markdown link to be normalized to plain text, got %q", got)
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

func TestShouldKeepStreamingIndexOnRunFinishedBranches(t *testing.T) {
	m := model{
		chatItems: []chatEntry{
			{Kind: "assistant", Status: "streaming"},
			{Kind: "assistant", Status: "thinking"},
			{Kind: "assistant", Status: "pending"},
			{Kind: "assistant", Status: "final"},
			{Kind: "tool", Status: "streaming"},
		},
	}

	for i, want := range []bool{true, true, true, false, false} {
		m.streamingIndex = i
		if got := m.shouldKeepStreamingIndexOnRunFinished(); got != want {
			t.Fatalf("unexpected keep-streaming result at index %d: got %v want %v", i, got, want)
		}
	}

	m.streamingIndex = -1
	if m.shouldKeepStreamingIndexOnRunFinished() {
		t.Fatalf("expected negative streaming index to return false")
	}
	m.streamingIndex = len(m.chatItems)
	if m.shouldKeepStreamingIndexOnRunFinished() {
		t.Fatalf("expected out-of-range streaming index to return false")
	}
}

func TestScrollbarTrackBoundsAndDragScrollbarTo(t *testing.T) {
	input := textarea.New()
	input.SetWidth(80)
	input.SetHeight(3)

	m := model{
		screen:     screenChat,
		width:      120,
		height:     32,
		input:      input,
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
		chatItems: []chatEntry{
			{Kind: "assistant", Body: strings.Repeat("line\n", 260), Status: "final"},
		},
	}
	m.refreshViewport()

	x, top, bottom, ok := m.scrollbarTrackBounds()
	if !ok {
		t.Fatalf("expected scrollbar track bounds to be available")
	}
	if x < 0 || bottom < top {
		t.Fatalf("unexpected scrollbar bounds: x=%d top=%d bottom=%d", x, top, bottom)
	}

	m.scrollbarDragOffset = 0
	m.dragScrollbarTo(bottom)
	if m.viewport.YOffset == 0 {
		t.Fatalf("expected dragging to scrollbar bottom to increase viewport offset")
	}
	afterBottom := m.viewport.YOffset
	m.dragScrollbarTo(top)
	if m.viewport.YOffset >= afterBottom {
		t.Fatalf("expected dragging to top to reduce offset, got %d -> %d", afterBottom, m.viewport.YOffset)
	}

	// Guard branch: track bounds unavailable.
	before := m.viewport.YOffset
	m.screen = screenLanding
	m.dragScrollbarTo(bottom)
	if m.viewport.YOffset != before {
		t.Fatalf("expected drag to no-op when track bounds are unavailable")
	}

	// Guard branch: no scrollable range (maxOffset == 0).
	m.screen = screenChat
	m.chatItems = []chatEntry{{Kind: "assistant", Body: "single line", Status: "final"}}
	m.refreshViewport()
	before = m.viewport.YOffset
	m.dragScrollbarTo(top)
	if m.viewport.YOffset != before {
		t.Fatalf("expected drag to no-op when content has no scrollable range")
	}
}

func TestHandleMouseScrollbarDragLifecycle(t *testing.T) {
	input := textarea.New()
	input.SetWidth(80)
	input.SetHeight(3)

	m := model{
		screen:         screenChat,
		width:          120,
		height:         32,
		input:          input,
		viewport:       viewport.New(60, 10),
		tokenUsage:     newTokenUsageComponent(),
		chatAutoFollow: true,
		chatItems: []chatEntry{
			{Kind: "assistant", Body: strings.Repeat("row\n", 280), Status: "final"},
		},
	}
	m.refreshViewport()

	x, top, bottom, ok := m.scrollbarTrackBounds()
	if !ok {
		t.Fatalf("expected scrollbar bounds for drag test")
	}

	// Click near track bottom so we exercise "track click jump + start drag" branch.
	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      bottom,
	})
	pressed := got.(model)
	if !pressed.draggingScrollbar {
		t.Fatalf("expected dragging mode after pressing scrollbar track")
	}
	if pressed.chatAutoFollow {
		t.Fatalf("expected auto-follow to be disabled once dragging starts")
	}

	beforeOffset := pressed.viewport.YOffset
	got, _ = pressed.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionMotion,
		X:      x,
		Y:      top,
	})
	moved := got.(model)
	if moved.viewport.YOffset == beforeOffset {
		t.Fatalf("expected motion while dragging to update viewport offset")
	}

	got, _ = moved.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      top,
	})
	released := got.(model)
	if released.draggingScrollbar {
		t.Fatalf("expected release to end scrollbar dragging")
	}
}

func TestHandleMouseGuardBranchesAndThumbPress(t *testing.T) {
	input := textarea.New()
	input.SetWidth(80)
	input.SetHeight(3)

	// Release should clear dragging even when another overlay short-circuits later logic.
	m := model{
		screen:            screenChat,
		width:             120,
		height:            28,
		input:             input,
		viewport:          viewport.New(60, 10),
		tokenUsage:        newTokenUsageComponent(),
		draggingScrollbar: true,
		helpOpen:          true,
	}
	got, _ := m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionRelease,
		Button: tea.MouseButtonLeft,
	})
	updated := got.(model)
	if updated.draggingScrollbar {
		t.Fatalf("expected release to clear dragging even when help modal is open")
	}

	// Unsupported screen should return without changes.
	m = model{screen: screenKind("other"), viewport: viewport.New(20, 4)}
	before := m.viewport.YOffset
	got, _ = m.handleMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	updated = got.(model)
	if updated.viewport.YOffset != before {
		t.Fatalf("expected unsupported screen to ignore mouse event")
	}

	// Sessions modal open on chat screen should block viewport scrolling.
	m = model{
		screen:       screenChat,
		sessionsOpen: true,
		viewport: func() (vp viewport.Model) {
			vp = viewport.New(40, 5)
			vp.SetContent(strings.Repeat("line\n", 60))
			return vp
		}(),
	}
	before = m.viewport.YOffset
	got, _ = m.handleMouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	updated = got.(model)
	if updated.viewport.YOffset != before {
		t.Fatalf("expected sessions-open state to ignore mouse wheel scrolling")
	}

	// Clicking directly on thumb should use the direct-offset branch.
	m = model{
		screen:     screenChat,
		width:      120,
		height:     32,
		input:      input,
		viewport:   viewport.New(60, 10),
		tokenUsage: newTokenUsageComponent(),
		chatItems: []chatEntry{
			{Kind: "assistant", Body: strings.Repeat("thumb\n", 220), Status: "final"},
		},
	}
	m.refreshViewport()
	x, trackTop, _, ok := m.scrollbarTrackBounds()
	if !ok {
		t.Fatalf("expected scrollbar bounds for thumb click")
	}
	thumbTop, thumbHeight, _, visible := m.scrollbarLayout(m.viewport.Height, m.viewport.TotalLineCount(), m.viewport.YOffset)
	if !visible || thumbHeight <= 0 {
		t.Fatalf("expected visible thumb for thumb-click branch")
	}
	insideThumbY := trackTop + thumbTop
	got, _ = m.handleMouse(tea.MouseMsg{
		Action: tea.MouseActionPress,
		Button: tea.MouseButtonLeft,
		X:      x,
		Y:      insideThumbY,
	})
	updated = got.(model)
	if !updated.draggingScrollbar {
		t.Fatalf("expected thumb press to start dragging")
	}
	if updated.scrollbarDragOffset != 0 {
		t.Fatalf("expected thumb-top press to use zero drag offset, got %d", updated.scrollbarDragOffset)
	}
}

func TestRenderTokenBadgeAndScrollbarHelpers(t *testing.T) {
	m := model{
		screen:     screenChat,
		width:      110,
		height:     30,
		input:      textarea.New(),
		viewport:   viewport.New(50, 8),
		tokenUsage: newTokenUsageComponent(),
	}
	m.viewport.SetContent(strings.Repeat("line\n", 120))
	m.tokenUsage.displayUsed = 2345
	_ = m.tokenUsage.SetUsage(2345, 5000)
	m.refreshViewport()

	compact := m.renderTokenBadge(79)
	if strings.Contains(compact, "/") {
		t.Fatalf("expected compact badge under width threshold, got %q", compact)
	}
	full := m.renderTokenBadge(80)
	if !strings.Contains(full, "token: 2,345") {
		t.Fatalf("expected full badge to show token count, got %q", full)
	}

	if got := m.renderScrollbar(0, 10, 0); got != "" {
		t.Fatalf("expected empty scrollbar when view height is zero, got %q", got)
	}
	bar := m.renderScrollbar(8, 120, 5)
	if lines := strings.Count(bar, "\n") + 1; lines != 8 {
		t.Fatalf("expected scrollbar to have 8 visual rows, got %d", lines)
	}

	thumbTop, thumbHeight, maxOffset, visible := m.scrollbarLayout(8, 200, 9999)
	if !visible || thumbHeight <= 0 || maxOffset <= 0 {
		t.Fatalf("expected visible scrollbar layout with valid dimensions")
	}
	if thumbTop < 0 || thumbTop > 8-thumbHeight {
		t.Fatalf("expected clamped thumb top, got top=%d height=%d", thumbTop, thumbHeight)
	}

	thumbTop, thumbHeight, maxOffset, visible = m.scrollbarLayout(8, 0, 0)
	if !visible || thumbTop != 0 || thumbHeight != 8 || maxOffset != 0 {
		t.Fatalf("expected zero-content layout fallback, got top=%d height=%d max=%d visible=%v", thumbTop, thumbHeight, maxOffset, visible)
	}
}

func TestWrapLineSmartBranchCoverage(t *testing.T) {
	if got := wrapLineSmart("abc", 0); len(got) != 1 || got[0] != "abc" {
		t.Fatalf("expected width<=0 to return original line, got %#v", got)
	}
	if got := wrapLineSmart("", 10); len(got) != 1 || got[0] != "" {
		t.Fatalf("expected empty line to remain empty, got %#v", got)
	}

	wideRune := wrapLineSmart("\u4f60\u597d", 1)
	if len(wideRune) < 2 || wideRune[0] != "\u4f60" {
		t.Fatalf("expected wide-rune fallback split, got %#v", wideRune)
	}

	words := wrapLineSmart("hello world", 6)
	if len(words) < 2 || words[0] != "hello" {
		t.Fatalf("expected split at word boundary, got %#v", words)
	}
}

func TestMarkdownNormalizationHelpers(t *testing.T) {
	if got := normalizeAssistantMarkdownLine(""); got != "" {
		t.Fatalf("expected empty line to normalize to empty, got %q", got)
	}
	if got := normalizeAssistantMarkdownLine("> ## Heading"); got != "Heading" {
		t.Fatalf("expected quote heading to normalize, got %q", got)
	}
	if got := normalizeAssistantMarkdownLine(" - [x] done **item** "); got != " - [x] done item" {
		t.Fatalf("expected checkbox normalization, got %q", got)
	}
	if got := normalizeAssistantMarkdownLine("1. [Doc](https://example.com)"); got != "1. Doc (https://example.com)" {
		t.Fatalf("expected ordered list with markdown link normalization, got %q", got)
	}
	if got := normalizeAssistantMarkdownLine("| --- | :---: |"); got != "" {
		t.Fatalf("expected table divider to be stripped, got %q", got)
	}
	if got := normalizeAssistantMarkdownLine("| a | b |"); got != "a | b" {
		t.Fatalf("expected table row to normalize, got %q", got)
	}

	if marker, rest, ok := splitOrderedListItem("12. step one"); !ok || marker != "12." || rest != "step one" {
		t.Fatalf("expected ordered list split, got marker=%q rest=%q ok=%v", marker, rest, ok)
	}
	if _, _, ok := splitOrderedListItem("a. not-ordered"); ok {
		t.Fatalf("expected invalid ordered-list marker to fail split")
	}

	if !isMarkdownTableDivider("| --- | :---: |") {
		t.Fatalf("expected markdown table divider to be detected")
	}
	if isMarkdownTableDivider("| a | b |") {
		t.Fatalf("expected non-divider row not to be treated as divider")
	}

	normalizedLinks := stripMarkdownLinks("see [Doc](https://x.test) and ![img](https://img.test)")
	if !strings.Contains(normalizedLinks, "Doc (https://x.test)") {
		t.Fatalf("expected standard link to preserve URL in text, got %q", normalizedLinks)
	}
	if !strings.Contains(normalizedLinks, "img") || strings.Contains(normalizedLinks, "img.test") {
		t.Fatalf("expected image link to keep label only, got %q", normalizedLinks)
	}

	broken := "[Doc](https://example.com"
	if got := stripMarkdownLinks(broken); got != broken {
		t.Fatalf("expected malformed markdown link to remain unchanged, got %q", got)
	}
}

func TestThinkingFilters(t *testing.T) {
	if isMeaningfulThinking("I will call read_file first.", "read_file") {
		t.Fatalf("expected generic tool-intent phrase not to be treated as meaningful thinking")
	}
	if !isMeaningfulThinking("I will first inspect the code and then patch tests.", "") {
		t.Fatalf("expected concrete planning thought to be meaningful")
	}
	if shouldRenderThinkingFromDelta("I will call read_file now.") {
		t.Fatalf("expected generic call text not to render as thinking delta")
	}
	if !shouldRenderThinkingFromDelta("First, I will inspect the failing branch and then patch tests.") {
		t.Fatalf("expected structured reasoning marker to trigger thinking rendering")
	}
}

func TestHandleKeyPasteCompressesLongTextImmediately(t *testing.T) {
	m := newImagePipelineModel(t)
	m.screen = screenChat
	longPaste := strings.Join([]string{
		"func normalize(items []string) []string {",
		"    out := make([]string, 0, len(items))",
		"    for _, item := range items {",
		"        v := strings.TrimSpace(item)",
		"        if v == \"\" {",
		"            continue",
		"        }",
		"        out = append(out, strings.ToLower(v))",
		"    }",
		"    sort.Strings(out)",
		"    return out",
		"}",
	}, "\n")

	msg := tea.KeyMsg{
		Type:  tea.KeyRunes,
		Runes: []rune(longPaste),
		Paste: true,
	}
	got, _ := m.handleKey(msg)
	updated := got.(model)
	if !regexp.MustCompile(`^\[Paste #\d+ ~\d+ lines\]$`).MatchString(updated.input.Value()) {
		t.Fatalf("expected immediate marker replacement for pasted long text, got %q", updated.input.Value())
	}
	if strings.Contains(updated.input.Value(), "normalize(items") {
		t.Fatalf("expected no raw pasted code to remain in input, got %q", updated.input.Value())
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
