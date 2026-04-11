package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"bytemind/internal/agent"
	"bytemind/internal/assets"
	"bytemind/internal/config"
	"bytemind/internal/history"
	"bytemind/internal/llm"
	"bytemind/internal/mention"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/provider"
	"bytemind/internal/session"
	"bytemind/internal/tools"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
	"github.com/mattn/go-runewidth"
)

const (
	defaultSessionLimit        = 8
	scrollStep                 = 3
	scrollbarWidth             = 1
	mouseZoneAutoProbeMaxDelta = 4
	commandPageSize            = 3
	mentionPageSize            = 5
	maxPendingBTW              = 5
	promptSearchPageSize       = 5
	promptSearchLoadLimit      = 50000
	promptSearchResultCap      = 200
	pasteSubmitGuard           = 400 * time.Millisecond
	mouseSelectionScrollTick   = 60 * time.Millisecond
	assistantLabel             = "Bytemind"
	thinkingLabel              = "Bytemind"
	chatTitleLabel             = "Bytemind Chat"
	tuiTitleLabel              = "Bytemind TUI"
	footerHintText             = "tab agents | / commands | drag select | Ctrl+C copy/quit | Ctrl+F history | Ctrl+L sessions"
	conversationViewportZoneID = "bytemind:conversation:viewport"
	inputEditorZoneID          = "bytemind:input:editor"
)

type footerShortcutHint struct {
	Key   string
	Label string
}

var footerShortcutHints = []footerShortcutHint{
	{Key: "tab", Label: "agents"},
	{Key: "/", Label: "commands"},
	{Key: "Ctrl+F", Label: "history"},
	{Key: "Ctrl+L", Label: "sessions"},
	{Key: "Ctrl+C", Label: "copy/quit"},
}

var promptSearchFilterHints = []footerShortcutHint{
	{Key: "ws:<kw>", Label: "workspace"},
	{Key: "sid:<kw>", Label: "session"},
}

var promptSearchActionHints = []footerShortcutHint{
	{Key: "PgUp/PgDn", Label: "page"},
	{Key: "Ctrl+F", Label: "next"},
	{Key: "Ctrl+S", Label: "prev"},
	{Key: "Enter", Label: "apply"},
	{Key: "Esc", Label: "close"},
}

var zoneInitOnce sync.Once

type screenKind string

const (
	screenLanding screenKind = "landing"
	screenChat    screenKind = "chat"
)

type agentMode string

const (
	modeBuild agentMode = "build"
	modePlan  agentMode = "plan"
)

type promptSearchMode string

const (
	promptSearchModeQuick promptSearchMode = "quick"
	promptSearchModePanel promptSearchMode = "panel"
)

const (
	startupFieldType    = "type"
	startupFieldBaseURL = "base_url"
	startupFieldModel   = "model"
	startupFieldAPIKey  = "api_key"
)

var startupFieldOrder = []string{
	startupFieldType,
	startupFieldBaseURL,
	startupFieldModel,
	startupFieldAPIKey,
}

type chatEntry struct {
	Kind   string
	Title  string
	Meta   string
	Body   string
	Status string
}

type viewportSelectionPoint struct {
	Col int
	Row int
}

type viewportTopLookupCache struct {
	left           int
	expectedTop    int
	viewportWidth  int
	viewportHeight int
	viewportOffset int
	top            int
	found          bool
	valid          bool
}

type commandItem struct {
	Name        string
	Usage       string
	Description string
	Group       string
	Kind        string
}

func (c commandItem) FilterValue() string {
	return strings.ToLower(strings.TrimPrefix(c.Usage, "/") + " " + c.Description)
}

type toolRun struct {
	Name    string
	Summary string
	Lines   []string
	Status  string
}

type approvalPrompt struct {
	Command string
	Reason  string
	Reply   chan approvalDecision
}

type approvalDecision struct {
	Approved bool
	Err      error
}

type agentEventMsg struct {
	Event agent.Event
}

type runFinishedMsg struct {
	RunID int
	Err   error
}

type runFinishReason string

const (
	runFinishReasonCompleted  runFinishReason = "completed"
	runFinishReasonFailed     runFinishReason = "failed"
	runFinishReasonCanceled   runFinishReason = "canceled"
	runFinishReasonBTWRestart runFinishReason = "btw_restart"
)

type approvalRequestMsg struct {
	Request tools.ApprovalRequest
	Reply   chan approvalDecision
}

type sessionsLoadedMsg struct {
	Summaries []session.Summary
	Err       error
}

type tokenUsagePulledMsg struct {
	Used    int
	Input   int
	Output  int
	Context int
	Err     error
}

type selectionToastExpiredMsg struct {
	ID int
}

type mouseSelectionScrollTickMsg struct {
	ID int
}

var commandItems = []commandItem{
	{Name: "/help", Usage: "/help", Description: "Show usage and supported commands.", Kind: "command"},
	{Name: "/session", Usage: "/session", Description: "Open the recent session list.", Kind: "command"},
	{Name: "/new", Usage: "/new", Description: "Start a fresh session in this workspace.", Kind: "command"},
	{Name: "/compact", Usage: "/compact", Description: "Compress long session history into a continuation summary.", Kind: "command"},
	{Name: "/btw", Usage: "/btw <message>", Description: "Interject while a run is in progress.", Kind: "command"},
	{Name: "/quit", Usage: "/quit", Description: "Exit the current TUI window.", Kind: "command"},
	{Name: "/skills", Usage: "/skills", Description: "List available skills and current active skill.", Kind: "command"},
	{Name: "/skill clear", Usage: "/skill clear", Description: "Clear active skill for this session.", Kind: "command"},
}

type model struct {
	runner     *agent.Runner
	store      *session.Store
	sess       *session.Session
	imageStore assets.ImageStore
	cfg        config.Config
	workspace  string

	width  int
	height int

	async    chan tea.Msg
	viewport viewport.Model
	copyView viewport.Model
	planView viewport.Model
	input    textarea.Model
	spinner  spinner.Model

	viewportContentCache  string
	viewportTopCache      viewportTopLookupCache
	chatItems             []chatEntry
	toolRuns              []toolRun
	plan                  planpkg.State
	sessions              []session.Summary
	sessionLimit          int
	sessionCursor         int
	commandCursor         int
	mentionCursor         int
	screen                screenKind
	mode                  agentMode
	sessionsOpen          bool
	helpOpen              bool
	commandOpen           bool
	mentionOpen           bool
	promptSearchOpen      bool
	busy                  bool
	streamingIndex        int
	statusNote            string
	phase                 string
	llmConnected          bool
	approval              *approvalPrompt
	mentionQuery          string
	mentionToken          mention.Token
	mentionResults        []mention.Candidate
	mentionIndex          *mention.WorkspaceFileIndex
	mentionRecent         map[string]int
	mentionSeq            int
	lastPasteAt           time.Time
	lastInputAt           time.Time
	inputBurstSize        int
	chatAutoFollow        bool
	draggingScrollbar     bool
	scrollbarDragOffset   int
	mouseSelecting        bool
	mouseSelectionMouseX  int
	mouseSelectionMouseY  int
	mouseSelectionTickID  int
	mouseSelectionActive  bool
	mouseSelectionStart   viewportSelectionPoint
	mouseSelectionEnd     viewportSelectionPoint
	inputMouseSelecting   bool
	inputSelectionActive  bool
	inputSelectionStart   viewportSelectionPoint
	inputSelectionEnd     viewportSelectionPoint
	selectionToast        string
	selectionToastID      int
	tokenUsage            tokenUsageComponent
	tokenUsedTotal        int
	tokenBudget           int
	tokenInput            int
	tokenOutput           int
	tokenContext          int
	tokenHasOfficialUsage bool
	tempEstimatedOutput   int
	tokenEstimator        *realtimeTokenEstimator
	promptHistoryLoaded   bool
	promptHistoryEntries  []history.PromptEntry
	promptSearchMode      promptSearchMode
	promptSearchQuery     string
	promptSearchMatches   []history.PromptEntry
	promptSearchCursor    int
	promptSearchBaseInput string
	inputImageRefs        map[int]llm.AssetID
	inputImageMentions    map[string]llm.AssetID
	orphanedImages        map[llm.AssetID]time.Time
	nextImageID           int
	pastedContents        map[string]pastedContent
	pastedOrder           []string
	nextPasteID           int
	pastedStateLoaded     bool
	lastCompressedPasteAt time.Time
	clipboard             clipboardImageReader
	clipboardText         clipboardTextWriter
	runCancel             context.CancelFunc
	pendingBTW            []string
	interrupting          bool
	interruptSafe         bool
	runSeq                int
	activeRunID           int
	startupGuide          StartupGuide
	mouseYOffset          int
}

func newModel(opts Options) model {
	ensureZoneManager()
	async := make(chan tea.Msg, 128)

	input := textarea.New()
	input.Placeholder = "Ask Bytemind to inspect, change, or verify this workspace..."
	input.Focus()
	input.CharLimit = 0
	input.SetWidth(72)
	input.SetHeight(2)
	input.ShowLineNumbers = false
	input.Prompt = ""

	spin := spinner.New()
	spin.Spinner = spinner.MiniDot
	spin.Style = lipgloss.NewStyle().Foreground(colorAccent)

	vp := viewport.New(0, 0)
	vp.YPosition = 0
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = scrollStep

	copyVP := viewport.New(0, 0)
	copyVP.YPosition = 0

	planVP := viewport.New(0, 0)
	planVP.YPosition = 0
	planVP.MouseWheelEnabled = true
	planVP.MouseWheelDelta = scrollStep

	chatItems, toolRuns := rebuildSessionTimeline(opts.Session)

	opts.Runner.SetObserver(agent.ObserverFunc(func(event agent.Event) {
		async <- agentEventMsg{Event: event}
	}))
	opts.Runner.SetApprovalHandler(func(req tools.ApprovalRequest) (bool, error) {
		reply := make(chan approvalDecision, 1)
		async <- approvalRequestMsg{Request: req, Reply: reply}
		decision := <-reply
		return decision.Approved, decision.Err
	})

	m := model{
		runner:             opts.Runner,
		store:              opts.Store,
		sess:               opts.Session,
		imageStore:         opts.ImageStore,
		cfg:                opts.Config,
		workspace:          opts.Workspace,
		async:              async,
		viewport:           vp,
		copyView:           copyVP,
		planView:           planVP,
		input:              input,
		spinner:            spin,
		chatItems:          chatItems,
		toolRuns:           toolRuns,
		plan:               copyPlanState(opts.Session.Plan),
		sessions:           nil,
		sessionLimit:       defaultSessionLimit,
		screen:             initialScreen(opts.Session),
		mode:               toAgentMode(opts.Session.Mode),
		streamingIndex:     -1,
		statusNote:         "Ready.",
		phase:              "idle",
		llmConnected:       true,
		chatAutoFollow:     true,
		mentionIndex:       mention.NewWorkspaceFileIndex(opts.Workspace),
		tokenUsage:         newTokenUsageComponent(),
		tokenBudget:        max(1, opts.Config.TokenQuota),
		tokenEstimator:     newRealtimeTokenEstimator(opts.Config.Provider.Model),
		inputImageRefs:     make(map[int]llm.AssetID, 8),
		inputImageMentions: make(map[string]llm.AssetID, 8),
		orphanedImages:     make(map[llm.AssetID]time.Time, 8),
		nextImageID:        nextSessionImageID(opts.Session),
		pastedContents:     make(map[string]pastedContent, maxStoredPastedContents),
		pastedOrder:        make([]string, 0, maxStoredPastedContents),
		nextPasteID:        1,
		clipboard:          defaultClipboardImageReader{},
		clipboardText:      defaultClipboardTextWriter{},
		startupGuide:       opts.StartupGuide,
		mouseYOffset:       resolveMouseYOffset(),
	}
	if opts.StartupGuide.Active {
		m.statusNote = opts.StartupGuide.Status
		m.llmConnected = false
		m.phase = "error"
		m.initializeStartupGuide()
	}
	m.restoreTokenUsageFromSession(opts.Session)
	_ = m.tokenUsage.SetUsage(m.tokenUsedTotal, 0)
	m.tokenUsage.SetUnavailable(!m.tokenHasOfficialUsage)
	m.tokenUsage.SetBreakdown(m.tokenInput, m.tokenOutput, m.tokenContext)
	m.ensureSessionImageAssets()
	m.ensurePastedContentState()
	m.syncInputStyle()
	m.syncInputOverlays()
	if m.mentionIndex != nil {
		go m.mentionIndex.Prewarm()
	}
	return m
}

func ensureZoneManager() {
	zoneInitOnce.Do(func() {
		zone.NewGlobal()
	})
}

func resolveMouseYOffset() int {
	raw := strings.TrimSpace(os.Getenv("BYTEMIND_MOUSE_Y_OFFSET"))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return clamp(value, -10, 10)
}

func (m model) Init() tea.Cmd {
	return tea.Batch(
		textarea.Blink,
		waitForAsync(m.async),
		m.tokenUsage.tickCmd(),
		m.loadSessionsCmd(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.resize()
		return m, nil
	case spinner.TickMsg:
		if !m.busy {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		m.updateThinkingCard()
		m.refreshViewport()
		return m, cmd
	case agentEventMsg:
		m.handleAgentEvent(msg.Event)
		m.refreshViewport()
		return m, waitForAsync(m.async)
	case runFinishedMsg:
		if msg.RunID > 0 && msg.RunID != m.activeRunID {
			return m, waitForAsync(m.async)
		}
		m.busy = false
		m.runCancel = nil
		m.activeRunID = 0
		m.interruptSafe = false
		shouldResumeBTW := m.interrupting && len(m.pendingBTW) > 0
		m.interrupting = false
		finishReason := classifyRunFinish(msg.Err, shouldResumeBTW)
		if shouldResumeBTW {
			updateScope := formatBTWUpdateScope(len(m.pendingBTW))
			prompt := composeBTWPrompt(m.pendingBTW)
			m.pendingBTW = nil
			note := fmt.Sprintf("BTW accepted. Restarting with %s...", updateScope)
			if msg.Err != nil && !errors.Is(msg.Err, context.Canceled) {
				note = fmt.Sprintf("Previous run ended early. Restarting with %s from BTW...", updateScope)
			}
			m.appendChat(chatEntry{
				Kind:   "system",
				Title:  "System",
				Body:   fmt.Sprintf("BTW interrupt accepted. Restarting with %s.", updateScope),
				Status: "final",
			})
			return m, m.beginRun(prompt, string(m.mode), note)
		}
		m.pendingBTW = nil
		switch finishReason {
		case runFinishReasonCompleted:
			if !m.shouldKeepStreamingIndexOnRunFinished() {
				m.streamingIndex = -1
			}
			m.statusNote = "Ready."
			m.phase = "idle"
		case runFinishReasonCanceled:
			m.streamingIndex = -1
			m.statusNote = "Run canceled."
			m.phase = "idle"
			m.llmConnected = true
		case runFinishReasonFailed:
			m.streamingIndex = -1
			m.statusNote = "Run failed: " + msg.Err.Error()
			m.phase = "error"
			m.llmConnected = false
			m.failLatestAssistant(msg.Err.Error())
		default:
			m.streamingIndex = -1
			m.statusNote = "Ready."
			m.phase = "idle"
		}
		m.refreshViewport()
		return m, tea.Batch(waitForAsync(m.async), m.loadSessionsCmd())
	case approvalRequestMsg:
		m.approval = &approvalPrompt{
			Command: msg.Request.Command,
			Reason:  msg.Request.Reason,
			Reply:   msg.Reply,
		}
		m.statusNote = "Approval required."
		m.phase = "approval"
		return m, waitForAsync(m.async)
	case sessionsLoadedMsg:
		if msg.Err == nil {
			m.sessions = msg.Summaries
			if m.sessionCursor >= len(m.sessions) && len(m.sessions) > 0 {
				m.sessionCursor = len(m.sessions) - 1
			}
		}
		return m, nil
	case tokenUsagePulledMsg:
		// Account-level usage is not session-accurate; ignore in session-only mode.
		return m, nil
	case selectionToastExpiredMsg:
		if msg.ID == m.selectionToastID {
			m.selectionToast = ""
		}
		return m, nil
	case mouseSelectionScrollTickMsg:
		return m.handleMouseSelectionScrollTick(msg)
	case tokenMonitorTickMsg:
		cmd, _ := m.tokenUsage.Update(msg)
		return m, cmd
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if !m.sessionsOpen && !m.helpOpen && !m.commandOpen && m.approval == nil {
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.handleInputMutation(before, m.input.Value(), "")
			m.syncInputOverlays()
		}
		return m, cmd
	}

	return m, nil
}

func (m model) shouldKeepStreamingIndexOnRunFinished() bool {
	if m.streamingIndex < 0 || m.streamingIndex >= len(m.chatItems) {
		return false
	}
	item := m.chatItems[m.streamingIndex]
	if item.Kind != "assistant" {
		return false
	}
	status := strings.TrimSpace(strings.ToLower(item.Status))
	return status == "streaming" || status == "thinking" || status == "pending"
}

func (m *model) ensureViewportMouse() {
	m.viewport.MouseWheelEnabled = true
	if m.viewport.MouseWheelDelta <= 0 {
		m.viewport.MouseWheelDelta = scrollStep
	}
}

func (m *model) ensurePlanMouse() {
	m.planView.MouseWheelEnabled = true
	if m.planView.MouseWheelDelta <= 0 {
		m.planView.MouseWheelDelta = scrollStep
	}
}

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

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.hasCopyableSelection() {
			return m, m.copyCurrentSelection()
		}
		if m.approval != nil {
			m.approval.Reply <- approvalDecision{Approved: false}
		}
		if m.runCancel != nil {
			m.runCancel()
		}
		return m, tea.Quit
	}

	if m.promptSearchOpen {
		return m.handlePromptSearchKey(msg)
	}

	switch msg.String() {
	case "esc":
		if m.hasCopyableSelection() {
			m.clearMouseSelection()
			m.clearInputSelection()
			m.statusNote = "Selection cleared."
			return m, nil
		}
	case "tab":
		if m.commandOpen || m.mentionOpen || m.sessionsOpen || m.helpOpen || m.approval != nil {
			break
		}
		m.toggleMode()
		return m, nil
	case "ctrl+g":
		if m.approval == nil {
			m.helpOpen = !m.helpOpen
		}
		return m, nil
	case "ctrl+f":
		if m.approval != nil || m.helpOpen || m.sessionsOpen || m.commandOpen || m.mentionOpen {
			return m, nil
		}
		m.openPromptSearch(promptSearchModeQuick)
		return m, nil
	}

	if m.approval != nil {
		switch msg.String() {
		case "y", "Y", "enter":
			m.approval.Reply <- approvalDecision{Approved: true}
			m.statusNote = "Shell command approved."
			m.phase = "tool"
			m.approval = nil
		case "n", "N", "esc":
			m.approval.Reply <- approvalDecision{Approved: false}
			m.statusNote = "Shell command rejected."
			m.phase = "thinking"
			m.approval = nil
		}
		return m, nil
	}

	if m.helpOpen {
		if msg.String() == "esc" || msg.String() == "ctrl+g" {
			m.helpOpen = false
		}
		return m, nil
	}

	if m.commandOpen {
		return m.handleCommandPaletteKey(msg)
	}

	if m.mentionOpen {
		return m.handleMentionPaletteKey(msg)
	}

	if m.sessionsOpen {
		switch msg.String() {
		case "esc":
			m.sessionsOpen = false
		case "up", "k":
			if m.sessionCursor > 0 {
				m.sessionCursor--
			}
		case "down", "j":
			if m.sessionCursor < len(m.sessions)-1 {
				m.sessionCursor++
			}
		case "enter":
			if m.busy || len(m.sessions) == 0 {
				return m, nil
			}
			if err := m.resumeSession(m.sessions[m.sessionCursor].ID); err != nil {
				m.statusNote = err.Error()
			} else {
				m.sessionsOpen = false
			}
		}
		return m, nil
	}

	if isInputNewlineKey(msg) {
		before := m.input.Value()
		var cmd tea.Cmd
		// Preserve multiline input shortcuts without triggering submit.
		m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.input.Value() != before {
			m.handleInputMutation(before, m.input.Value(), inputMutationSource(msg))
			m.syncInputOverlays()
		}
		return m, cmd
	}

	ctrlVPasteDetected := isCtrlVPasteKey(msg)
	// Prefer Ctrl+V image paste first. If clipboard has no image, fall through
	// so regular terminal paste behavior can continue.
	if ctrlVPasteDetected {
		if note := m.handleEmptyClipboardPaste(); strings.TrimSpace(note) != "" {
			m.statusNote = note
			if strings.Contains(note, "Attached image from clipboard") {
				m.syncInputOverlays()
				return m, nil
			}
			if !isClipboardNoImageNote(note) {
				m.syncInputOverlays()
				return m, nil
			}
		}
	}

	switch msg.String() {
	case "ctrl+l":
		if !m.busy {
			m.sessionsOpen = true
		}
		return m, m.loadSessionsCmd()
	case "alt+v":
		if note := m.handleEmptyClipboardPaste(); strings.TrimSpace(note) != "" {
			m.statusNote = note
		}
		m.syncInputOverlays()
		return m, nil
	case "ctrl+n":
		if !m.busy && m.screen == screenChat {
			if err := m.newSession(); err != nil {
				m.statusNote = err.Error()
			}
		}
		return m, m.loadSessionsCmd()
	case "home":
		m.viewport.GotoTop()
		m.syncCopyViewOffset()
		m.chatAutoFollow = false
		return m, nil
	case "end":
		m.viewport.GotoBottom()
		m.syncCopyViewOffset()
		m.chatAutoFollow = true
		return m, nil
	}

	if msg.String() == "enter" && !msg.Paste {
		if m.shouldSuppressEnterAfterPaste() {
			if m.busy {
				return m, nil
			}
			before := m.input.Value()
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if m.input.Value() != before {
				m.handleInputMutation(before, m.input.Value(), "paste-enter")
				m.syncInputOverlays()
			}
			return m, cmd
		}
		rawValue := m.input.Value()
		if markerChain, ok := extractLeadingCompressedMarker(rawValue); ok {
			tail := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rawValue), markerChain))
			if tail != "" {
				if m.shouldCompressPastedText(tail, "paste-enter") {
					marker, content, err := m.compressPastedText(tail)
					if err != nil {
						m.statusNote = err.Error()
						return m, nil
					}
					combined := strings.TrimSpace(markerChain) + marker
					m.setInputValue(combined)
					m.syncInputOverlays()
					m.statusNote = fmt.Sprintf("Detected another pasted block and compressed it as %s (%d lines). Press Enter again to send.", marker, content.Lines)
					return m, nil
				}
				if len(tail) >= 24 || strings.Contains(tail, "\n") {
					m.setInputValue(strings.TrimSpace(markerChain))
					m.syncInputOverlays()
					m.statusNote = "Detected continued paste chunk after compressed marker. Kept compressed markers only; press Enter again to send."
					return m, nil
				}
			}
		}
		// Check whether the input has already been compressed.
		isAlreadyCompressed := strings.Contains(rawValue, "[Paste #") || strings.Contains(rawValue, "[Pasted #")

		// Compress long pasted content before sending.
		if !isAlreadyCompressed && m.shouldCompressPastedText(rawValue, inputMutationSource(msg)) {
			marker, content, err := m.compressPastedText(rawValue)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			m.setInputValue(marker)
			m.syncInputOverlays()
			m.statusNote = fmt.Sprintf("Long pasted text (%d lines) compressed as %s. Press Enter again to send. Use [Paste #%s] or [Paste #%s line10~line20] later.", content.Lines, marker, content.ID, content.ID)
			return m, nil
		}
		value := strings.TrimSpace(rawValue)
		if m.startupGuide.Active && !strings.HasPrefix(value, "/") {
			if err := m.handleStartupGuideSubmission(rawValue); err != nil {
				m.statusNote = err.Error()
			}
			m.screen = screenLanding
			return m, nil
		}
		if value == "" {
			return m, nil
		}
		if isBTWCommand(value) {
			btw, err := extractBTWText(value)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			if m.busy {
				return m.submitBTW(btw)
			}
			return m.submitPrompt(btw)
		}
		if value == "/quit" {
			return m, tea.Quit
		}
		if m.busy {
			if strings.HasPrefix(value, "/") {
				m.statusNote = "This command is unavailable while a run is in progress. Use /btw <message> or plain text."
				return m, nil
			}
			return m.submitBTW(value)
		}
		if isContinueExecutionInput(value) && planpkg.HasStructuredPlan(m.plan) {
			state, err := preparePlanForContinuation(m.plan)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			m.plan = state
			m.mode = modeBuild
			if m.sess != nil {
				m.sess.Mode = planpkg.ModeBuild
				m.sess.Plan = copyPlanState(state)
				if m.store != nil {
					if err := m.store.Save(m.sess); err != nil {
						m.statusNote = err.Error()
						return m, nil
					}
				}
			}
		}
		if strings.HasPrefix(value, "/") {
			m.input.Reset()
			next, cmd, err := m.executeCommand(value)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			return next, cmd
		}
		return m.submitPrompt(value)
	}

	mutationSource := inputMutationSource(msg)
	before := m.input.Value()
	var cmd tea.Cmd
	var after string
	if msg.Paste {
		preview := m.input
		preview, cmd = preview.Update(msg)
		after = preview.Value()
		// Apply paste pipeline before committing raw pasted text to input,
		// so long-paste markers appear immediately without visible flicker.
		m.handleInputMutation(before, after, mutationSource)
		if m.input.Value() == before && after != before {
			m.input = preview
		}
		after = m.input.Value()
	} else {
		m.input, cmd = m.input.Update(msg)
		after = m.input.Value()
		if after != before {
			m.handleInputMutation(before, after, mutationSource)
			after = m.input.Value()
		}
	}
	triggerClipboardImagePaste := shouldTriggerClipboardImagePaste(before, after, mutationSource)
	if ctrlVPasteDetected {
		triggerClipboardImagePaste = false
	}
	if !triggerClipboardImagePaste && msg.Paste {
		_, inserted, _ := insertionDiff(before, after)
		cleanInserted := strings.TrimSpace(strings.ReplaceAll(inserted, ctrlVMarkerRune, ""))
		if cleanInserted == "" {
			triggerClipboardImagePaste = true
		}
	}
	if triggerClipboardImagePaste {
		if cleaned, changed := stripCtrlVMarker(m.input.Value()); changed {
			m.setInputValue(cleaned)
		}
		if note := m.handleEmptyClipboardPaste(); strings.TrimSpace(note) != "" {
			m.statusNote = note
		}
	}
	m.syncInputOverlays()
	return m, cmd
}

func (m model) shouldSuppressEnterAfterPaste() bool {
	if m.lastPasteAt.IsZero() {
		return false
	}
	if time.Since(m.lastPasteAt) > pasteSubmitGuard {
		return false
	}
	if strings.Contains(m.input.Value(), "\n") {
		return true
	}
	return time.Since(m.lastInputAt) <= 120*time.Millisecond
}

func (m model) handleCommandPaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.filteredCommands()
	switch {
	case isPageUpKey(msg):
		if len(items) > 0 {
			m.commandCursor = max(0, m.commandCursor-commandPageSize)
		}
		return m, nil
	case isPageDownKey(msg):
		if len(items) > 0 {
			m.commandCursor = min(len(items)-1, m.commandCursor+commandPageSize)
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.closeCommandPalette()
		return m, nil
	case "up":
		if len(items) > 0 {
			m.commandCursor = max(0, m.commandCursor-1)
		}
		return m, nil
	case "down":
		if len(items) > 0 {
			m.commandCursor = min(len(items)-1, m.commandCursor+1)
		}
		return m, nil
	case "enter":
		selected, ok := m.selectedCommandItem()
		if !ok {
			value := strings.TrimSpace(m.input.Value())
			if value == "" {
				return m, nil
			}
			if value == "/quit" {
				m.closeCommandPalette()
				return m, tea.Quit
			}
			if m.busy {
				if isBTWCommand(value) {
					btw, err := extractBTWText(value)
					if err != nil {
						m.statusNote = err.Error()
						return m, nil
					}
					m.closeCommandPalette()
					return m.submitBTW(btw)
				}
				if strings.HasPrefix(value, "/") {
					m.statusNote = "This command is unavailable while a run is in progress. Use /btw <message>."
					return m, nil
				}
				m.closeCommandPalette()
				return m.submitBTW(value)
			}
			m.closeCommandPalette()
			m.input.Reset()
			next, cmd, err := m.executeCommand(value)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			return next, cmd
		}
		m.closeCommandPalette()
		if shouldExecuteFromPalette(selected) || selected.Name == "/continue" {
			if selected.Name == "/quit" {
				return m, tea.Quit
			}
			if m.busy {
				m.statusNote = "This command is unavailable while a run is in progress. Use /btw <message>."
				return m, nil
			}
			m.input.Reset()
			next, cmd, err := m.executeCommand(selected.Name)
			if err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			return next, cmd
		}
		m.setInputValue(selected.Usage)
		m.statusNote = selected.Description
		return m, nil
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.handleInputMutation(before, m.input.Value(), msg.String())
		m.syncInputOverlays()
	}
	return m, cmd
}

func (m model) handleMentionPaletteKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	items := m.mentionResults
	switch {
	case isPageUpKey(msg):
		if len(items) > 0 {
			m.mentionCursor = max(0, m.mentionCursor-mentionPageSize)
		}
		return m, nil
	case isPageDownKey(msg):
		if len(items) > 0 {
			m.mentionCursor = min(len(items)-1, m.mentionCursor+mentionPageSize)
		}
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.closeMentionPalette()
		return m, nil
	case "up", "k":
		if len(items) > 0 {
			m.mentionCursor = max(0, m.mentionCursor-1)
		}
		return m, nil
	case "down", "j":
		if len(items) > 0 {
			m.mentionCursor = min(len(items)-1, m.mentionCursor+1)
		}
		return m, nil
	case "tab":
		selected, ok := m.selectedMentionCandidate()
		if !ok {
			return m, nil
		}
		m.applyMentionSelection(selected)
		return m, nil
	case "enter":
		selected, ok := m.selectedMentionCandidate()
		if !ok {
			m.closeMentionPalette()
			return m.handleKey(msg)
		}
		m.applyMentionSelection(selected)
		return m, nil
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.handleInputMutation(before, m.input.Value(), msg.String())
		m.syncInputOverlays()
	}
	return m, cmd
}

func (m *model) applyMentionSelection(selected mention.Candidate) {
	m.recordRecentMention(selected.Path)

	if assetID, note, isImage := m.ingestMentionImageCandidate(selected.Path); isImage {
		if strings.TrimSpace(string(assetID)) != "" {
			m.bindMentionImageAsset(selected.Path, assetID)
			nextValue := mention.InsertIntoInput(m.input.Value(), m.mentionToken, selected.Path)
			m.setInputValue(nextValue)
			if strings.TrimSpace(note) != "" {
				m.statusNote = note
			} else {
				m.statusNote = "Attached image: @" + filepath.ToSlash(strings.TrimSpace(selected.Path))
			}
			m.closeMentionPalette()
			m.syncInputOverlays()
			return
		}
		if strings.TrimSpace(note) != "" {
			m.statusNote = note
		}
	}

	nextValue := mention.InsertIntoInput(m.input.Value(), m.mentionToken, selected.Path)
	m.setInputValue(nextValue)
	m.statusNote = "Inserted mention: " + selected.Path
	m.closeMentionPalette()
	m.syncInputOverlays()
}

func (m *model) ingestMentionImageCandidate(path string) (assetID llm.AssetID, note string, isImage bool) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", "", false
	}

	resolved := path
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(m.workspace, resolved)
	}
	resolved = filepath.Clean(resolved)

	info, err := os.Stat(resolved)
	if err != nil || info.IsDir() {
		return "", "", false
	}
	if _, ok := mediaTypeFromPath(resolved); !ok {
		return "", "", false
	}

	placeholder, note, ok := m.ingestImageFromPath(resolved)
	if !ok {
		return "", note, true
	}
	imageID, ok := imageIDFromPlaceholder(placeholder)
	if !ok {
		return "", "image ingest failed: invalid placeholder id", true
	}
	assetID, _, ok = m.findAssetByImageID(imageID)
	if !ok {
		return "", "image ingest failed: asset metadata missing", true
	}
	return assetID, note, true
}

func (m *model) bindMentionImageAsset(path string, assetID llm.AssetID) {
	if m == nil {
		return
	}
	key := normalizeImageMentionPath(path)
	if key == "" || strings.TrimSpace(string(assetID)) == "" {
		return
	}
	if m.inputImageMentions == nil {
		m.inputImageMentions = make(map[string]llm.AssetID, 8)
	}
	if m.orphanedImages == nil {
		m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	}
	if prev, ok := m.inputImageMentions[key]; ok && prev != assetID {
		m.orphanedImages[prev] = time.Now().UTC()
	}
	m.inputImageMentions[key] = assetID
	delete(m.orphanedImages, assetID)
}

func (m *model) openCommandPalette() {
	m.commandOpen = true
	m.commandCursor = 0
	m.setInputValue("/")
	m.closeMentionPalette()
	m.syncInputOverlays()
}

func (m *model) openPromptSearch(mode promptSearchMode) {
	m.ensurePromptHistoryLoaded()
	m.promptSearchMode = mode
	m.promptSearchBaseInput = m.input.Value()
	m.promptSearchQuery = ""
	m.promptSearchCursor = 0
	m.promptSearchOpen = true
	m.commandOpen = false
	m.closeMentionPalette()
	m.refreshPromptSearchMatches()
	if len(m.promptSearchMatches) == 0 {
		if mode == promptSearchModePanel {
			m.statusNote = "History panel opened. No matching prompts."
		} else {
			m.statusNote = "No matching prompts."
		}
	} else {
		if mode == promptSearchModePanel {
			m.statusNote = fmt.Sprintf("History panel ready (%d matches).", len(m.promptSearchMatches))
		} else {
			m.statusNote = fmt.Sprintf("Prompt history ready (%d matches).", len(m.promptSearchMatches))
		}
	}
}

func (m *model) closePromptSearch(restoreInput bool) {
	if restoreInput {
		m.setInputValue(m.promptSearchBaseInput)
	}
	m.promptSearchOpen = false
	m.promptSearchMode = ""
	m.promptSearchQuery = ""
	m.promptSearchMatches = nil
	m.promptSearchCursor = 0
	m.promptSearchBaseInput = ""
	m.syncInputOverlays()
}

func (m *model) ensurePromptHistoryLoaded() {
	if m.promptHistoryLoaded {
		return
	}
	entries, err := history.LoadRecentPrompts(promptSearchLoadLimit)
	if err != nil {
		m.promptHistoryEntries = nil
		m.promptHistoryLoaded = true
		m.statusNote = "Prompt history unavailable: " + compact(err.Error(), 72)
		return
	}
	m.promptHistoryEntries = entries
	m.promptHistoryLoaded = true
}

func (m *model) refreshPromptSearchMatches() {
	tokens, workspaceFilter, sessionFilter := parsePromptSearchQuery(m.promptSearchQuery)
	limit := promptSearchResultCap
	if m.promptSearchMode == promptSearchModePanel {
		limit = promptSearchLoadLimit
	}
	matches := make([]history.PromptEntry, 0, min(len(m.promptHistoryEntries), limit))
	for i := len(m.promptHistoryEntries) - 1; i >= 0; i-- {
		entry := m.promptHistoryEntries[i]
		prompt := strings.TrimSpace(entry.Prompt)
		if prompt == "" {
			continue
		}
		workspaceValue := strings.ToLower(strings.TrimSpace(entry.Workspace))
		if workspaceFilter != "" && !strings.Contains(workspaceValue, workspaceFilter) {
			continue
		}
		sessionValue := strings.ToLower(strings.TrimSpace(entry.SessionID))
		if sessionFilter != "" && !strings.Contains(sessionValue, sessionFilter) {
			continue
		}
		promptLower := strings.ToLower(prompt)
		if !matchAllTokens(promptLower, tokens) {
			continue
		}
		matches = append(matches, entry)
		if len(matches) >= limit {
			break
		}
	}

	m.promptSearchMatches = matches
	if len(matches) == 0 {
		m.promptSearchCursor = 0
		return
	}
	m.promptSearchCursor = clamp(m.promptSearchCursor, 0, len(matches)-1)
}

func (m *model) stepPromptSearch(delta int) {
	if len(m.promptSearchMatches) == 0 {
		return
	}
	next := m.promptSearchCursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.promptSearchMatches) {
		next = len(m.promptSearchMatches) - 1
	}
	m.promptSearchCursor = next
}

func (m *model) trimPromptSearchQuery() {
	if m.promptSearchQuery == "" {
		return
	}
	runes := []rune(m.promptSearchQuery)
	m.promptSearchQuery = string(runes[:len(runes)-1])
	m.refreshPromptSearchMatches()
}

func (m *model) toggleMode() {
	if m.mode == modeBuild {
		m.mode = modePlan
		if m.plan.Phase == planpkg.PhaseNone {
			m.plan.Phase = planpkg.PhaseDrafting
		}
		m.statusNote = "Switched to Plan mode. Draft the plan before executing."
	} else {
		m.mode = modeBuild
		m.statusNote = "Switched to Build mode. Execution still requires confirmation."
	}
	if m.sess != nil {
		m.sess.Mode = planpkg.NormalizeMode(string(m.mode))
		m.sess.Plan = copyPlanState(m.plan)
		if m.store != nil {
			_ = m.store.Save(m.sess)
		}
	}
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
}

func (m *model) closeCommandPalette() {
	m.commandOpen = false
	m.commandCursor = 0
	m.closeMentionPalette()
	m.input.Reset()
}

func (m model) selectedCommandItem() (commandItem, bool) {
	items := m.filteredCommands()
	if len(items) == 0 {
		return commandItem{}, false
	}
	index := clamp(m.commandCursor, 0, len(items)-1)
	return items[index], true
}

func (m *model) noteInputMutation(before, after, source string) {
	now := time.Now()
	delta := len(after) - len(before)
	if delta < 0 {
		delta = 0
	}

	if now.Sub(m.lastInputAt) <= 80*time.Millisecond {
		m.inputBurstSize += max(1, delta)
	} else {
		m.inputBurstSize = max(1, delta)
	}
	m.lastInputAt = now

	if source == "paste-enter" ||
		source == "ctrl+v" ||
		delta > 1 ||
		strings.Contains(after[lenCommonPrefix(before, after):], "\n") ||
		m.inputBurstSize >= 4 {
		m.lastPasteAt = now
	}
}

func (m *model) handleInputMutation(before, after, source string) {
	m.noteInputMutation(before, after, source)

	updated, note := m.applyInputImagePipeline(before, after, source)
	if updated == after {
		fallbackUpdated, fallbackNote := m.applyWholeInputImagePathFallback(after, source)
		if fallbackUpdated != after {
			updated = fallbackUpdated
		}
		if strings.TrimSpace(note) == "" {
			note = fallbackNote
		}
	}

	pasteUpdated, pasteNote := m.applyLongPastedTextPipeline(before, updated, source)
	if pasteUpdated != updated {
		updated = pasteUpdated
	}
	if strings.TrimSpace(note) == "" {
		note = pasteNote
	}
	if locked, changed := m.protectCompressedMarkerChain(before, updated, source); changed {
		updated = locked
		if strings.TrimSpace(note) == "" {
			note = "Paste marker is locked to prevent accidental edits."
		}
	}

	if updated != after {
		m.setInputValue(updated)
	}
	if strings.TrimSpace(note) != "" {
		m.statusNote = note
	}
}

func lenCommonPrefix(a, b string) int {
	limit := min(len(a), len(b))
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return limit
}

func (m *model) beginRun(prompt, mode, note string) tea.Cmd {
	return m.beginRunWithInput(agent.RunPromptInput{
		UserMessage: llm.NewUserTextMessage(prompt),
		DisplayText: prompt,
	}, mode, note)
}

func (m *model) beginRunWithInput(promptInput agent.RunPromptInput, mode, note string) tea.Cmd {
	runCtx, cancel := context.WithCancel(context.Background())
	m.runSeq++
	runID := m.runSeq
	m.activeRunID = runID
	m.runCancel = cancel
	m.streamingIndex = -1
	if strings.TrimSpace(note) == "" {
		note = "Request sent to LLM. Waiting for response..."
	}
	m.statusNote = note
	m.phase = "thinking"
	m.llmConnected = true
	m.busy = true
	m.chatAutoFollow = true
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return tea.Batch(m.startRunCmd(runCtx, runID, promptInput, mode), m.spinner.Tick, waitForAsync(m.async))
}

func (m model) submitPrompt(value string) (tea.Model, tea.Cmd) {
	promptInput, displayText, err := m.buildPromptInput(value)
	if err != nil {
		m.statusNote = err.Error()
		return m, nil
	}

	m.input.Reset()
	m.screen = screenChat
	if m.promptHistoryLoaded {
		entry := history.PromptEntry{
			Timestamp: time.Now().UTC(),
			Workspace: strings.TrimSpace(m.workspace),
			Prompt:    strings.TrimSpace(displayText),
		}
		if m.sess != nil {
			entry.SessionID = m.sess.ID
		}
		if entry.Prompt != "" {
			m.promptHistoryEntries = append(m.promptHistoryEntries, entry)
			if len(m.promptHistoryEntries) > promptSearchLoadLimit {
				m.promptHistoryEntries = m.promptHistoryEntries[len(m.promptHistoryEntries)-promptSearchLoadLimit:]
			}
		}
	}
	m.appendChat(chatEntry{
		Kind:   "user",
		Title:  "You",
		Meta:   formatUserMeta(m.currentModelLabel(), time.Now()),
		Body:   displayText,
		Status: "final",
	})
	return m, m.beginRunWithInput(promptInput, string(m.mode), "Request sent to LLM. Waiting for response...")
}

func (m model) submitBTW(value string) (tea.Model, tea.Cmd) {
	value = strings.TrimSpace(value)
	if value == "" {
		return m, nil
	}

	m.input.Reset()
	m.screen = screenChat
	m.appendChat(chatEntry{
		Kind:   "user",
		Title:  "You",
		Meta:   formatUserMeta(m.currentModelLabel(), time.Now()) + " | btw",
		Body:   value,
		Status: "final",
	})
	var dropped int
	m.pendingBTW, dropped = queueBTWUpdate(m.pendingBTW, value)
	m.chatAutoFollow = true

	if m.interrupting {
		if dropped > 0 {
			m.statusNote = fmt.Sprintf("Queued BTW update (%d pending, dropped %d older). Waiting for current run to stop...", len(m.pendingBTW), dropped)
		} else {
			m.statusNote = fmt.Sprintf("Queued BTW update (%d pending). Waiting for current run to stop...", len(m.pendingBTW))
		}
		m.phase = "interrupting"
		if m.width > 0 && m.height > 0 {
			m.syncLayoutForCurrentScreen()
			m.refreshViewport()
		}
		return m, nil
	}

	wasToolPhase := m.phase == "tool"
	m.interrupting = true
	m.phase = "interrupting"
	if m.runCancel != nil {
		if wasToolPhase {
			m.interruptSafe = true
			m.statusNote = "BTW queued. Waiting for current tool step to finish..."
		} else {
			m.interruptSafe = false
			m.statusNote = "BTW received. Stopping current run..."
			m.runCancel()
		}
	} else {
		prompt := composeBTWPrompt(m.pendingBTW)
		m.pendingBTW = nil
		m.interrupting = false
		m.interruptSafe = false
		return m, m.beginRun(prompt, string(m.mode), "BTW accepted. Restarting with your update...")
	}
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return m, nil
}

func (m *model) handleAgentEvent(event agent.Event) {
	switch event.Type {
	case agent.EventRunStarted:
		m.tempEstimatedOutput = 0
	case agent.EventAssistantDelta:
		m.phase = "responding"
		m.statusNote = "LLM is responding..."
		m.llmConnected = true
		m.appendAssistantDelta(event.Content)
	case agent.EventAssistantMessage:
		m.llmConnected = true
		m.finishAssistantMessage(event.Content)
	case agent.EventToolCallStarted:
		m.phase = "tool"
		m.llmConnected = true
		m.finalizeAssistantTurnForTool(event.ToolName)
		m.appendChat(chatEntry{
			Kind:   "tool",
			Title:  "Tool Call | " + event.ToolName,
			Body:   "",
			Status: "running",
		})
		m.toolRuns = append(m.toolRuns, toolRun{
			Name:    event.ToolName,
			Summary: "Tool call started.",
			Status:  "running",
		})
		m.statusNote = "Running tool: " + event.ToolName
	case agent.EventToolCallCompleted:
		summary, lines, status := summarizeTool(event.ToolName, event.ToolResult)
		m.finishLatestToolCall(event.ToolName, joinSummary(summary, lines), status)
		if len(m.toolRuns) > 0 {
			index := len(m.toolRuns) - 1
			m.toolRuns[index].Summary = summary
			m.toolRuns[index].Lines = lines
			m.toolRuns[index].Status = status
		}
		m.statusNote = summary
		m.phase = "thinking"
		if m.interruptSafe && m.interrupting && len(m.pendingBTW) > 0 && m.runCancel != nil {
			m.interruptSafe = false
			m.phase = "interrupting"
			m.statusNote = "BTW received. Stopping current run..."
			m.runCancel()
		}
	case agent.EventPlanUpdated:
		m.plan = copyPlanState(event.Plan)
		m.phase = string(planpkg.NormalizePhase(string(m.plan.Phase)))
		if m.phase == "none" {
			m.phase = "plan"
		}
		m.statusNote = fmt.Sprintf("Plan updated with %d step(s).", len(m.plan.Steps))
	case agent.EventUsageUpdated:
		m.applyUsage(event.Usage)
	case agent.EventRunFinished:
		if strings.TrimSpace(event.Content) != "" {
			m.statusNote = "Run finished."
		}
		m.phase = "idle"
	}
}

func (m *model) applyUsage(usage llm.Usage) {
	m.tokenHasOfficialUsage = true
	input := max(0, usage.InputTokens)
	output := max(0, usage.OutputTokens)
	context := max(0, usage.ContextTokens)
	used := usage.TotalTokens
	if used == 0 {
		used = input + output + context
	}
	used = max(0, used)
	if used == 0 && input == 0 && output == 0 && context == 0 {
		return
	}

	// Replace provisional stream estimate with provider-confirmed usage.
	if m.tempEstimatedOutput > 0 {
		m.tokenUsedTotal = max(0, m.tokenUsedTotal-m.tempEstimatedOutput)
		m.tokenOutput = max(0, m.tokenOutput-m.tempEstimatedOutput)
	}
	m.tempEstimatedOutput = 0

	m.tokenUsedTotal += used
	m.tokenInput += input
	m.tokenOutput += output
	m.tokenContext += context
	_ = m.tokenUsage.SetUsage(m.tokenUsedTotal, 0)
	m.tokenUsage.SetUnavailable(false)
	m.tokenUsage.SetBreakdown(m.tokenInput, m.tokenOutput, m.tokenContext)
}

func (m *model) appendAssistantDelta(delta string) {
	if delta == "" {
		return
	}
	if m.streamingIndex >= 0 && m.streamingIndex < len(m.chatItems) {
		current := m.chatItems[m.streamingIndex].Body
		if m.chatItems[m.streamingIndex].Status == "pending" ||
			m.chatItems[m.streamingIndex].Status == "thinking" ||
			current == m.thinkingText() {
			m.chatItems[m.streamingIndex].Body = delta
		} else if strings.HasPrefix(delta, current) {
			m.chatItems[m.streamingIndex].Body = delta
		} else if strings.HasSuffix(current, delta) {
			// Some providers may repeat the latest chunk; ignore it.
		} else {
			m.chatItems[m.streamingIndex].Body += delta
		}
		m.applyAssistantDeltaPresentation(&m.chatItems[m.streamingIndex])
		return
	}
	m.chatItems = append(m.chatItems, chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   delta,
		Status: "streaming",
	})
	m.streamingIndex = len(m.chatItems) - 1
	m.applyAssistantDeltaPresentation(&m.chatItems[m.streamingIndex])
}

func (m *model) applyAssistantDeltaPresentation(item *chatEntry) {
	if item == nil || item.Kind != "assistant" {
		return
	}
	if shouldRenderThinkingFromDelta(item.Body) {
		item.Title = thinkingLabel
		item.Status = "thinking"
		return
	}
	item.Title = assistantLabel
	item.Status = "streaming"
}

func (m *model) finishAssistantMessage(content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	if m.streamingIndex >= 0 && m.streamingIndex < len(m.chatItems) {
		current := &m.chatItems[m.streamingIndex]
		if current.Status == "thinking" &&
			strings.TrimSpace(current.Body) != "" &&
			current.Body != m.thinkingText() {
			current.Title = thinkingLabel
			current.Status = "thinking"
			m.streamingIndex = -1
		} else {
			current.Title = assistantLabel
			current.Body = content
			current.Status = "final"
			m.streamingIndex = -1
			return
		}
	}
	if len(m.chatItems) > 0 {
		last := &m.chatItems[len(m.chatItems)-1]
		if last.Kind == "assistant" && last.Title == assistantLabel && strings.TrimSpace(last.Body) == content {
			last.Status = "final"
			return
		}
	}
	m.chatItems = append(m.chatItems, chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   content,
		Status: "final",
	})
}

func (m *model) appendChat(item chatEntry) {
	m.chatItems = append(m.chatItems, item)
}

func (m *model) finalizeAssistantTurnForTool(toolName string) {
	if m.streamingIndex >= 0 && m.streamingIndex < len(m.chatItems) {
		item := &m.chatItems[m.streamingIndex]
		if item.Kind == "assistant" {
			if !isMeaningfulThinking(item.Body, toolName) {
				m.removeStreamingAssistantPlaceholder()
				return
			}
			item.Title = thinkingLabel
			item.Status = "thinking"
			m.streamingIndex = -1
			return
		}
	}
}

func (m *model) removeStreamingAssistantPlaceholder() {
	if m.streamingIndex < 0 || m.streamingIndex >= len(m.chatItems) {
		m.streamingIndex = -1
		return
	}
	if m.chatItems[m.streamingIndex].Kind == "assistant" {
		m.chatItems = append(m.chatItems[:m.streamingIndex], m.chatItems[m.streamingIndex+1:]...)
	}
	m.streamingIndex = -1
}

func (m *model) appendAssistantToolFollowUp(toolName, summary, status string) {
	step := assistantToolFollowUp(toolName, summary, status)
	if step == "" {
		return
	}
	if len(m.chatItems) > 0 {
		last := &m.chatItems[len(m.chatItems)-1]
		if last.Kind == "assistant" && strings.TrimSpace(last.Body) == step {
			last.Title = thinkingLabel
			last.Status = "thinking"
			return
		}
	}
	m.appendChat(chatEntry{
		Kind:   "assistant",
		Title:  thinkingLabel,
		Body:   step,
		Status: "thinking",
	})
}

func (m *model) finishLatestToolCall(name, body, status string) {
	title := "Tool Call | " + name
	for i := len(m.chatItems) - 1; i >= 0; i-- {
		if m.chatItems[i].Kind != "tool" {
			continue
		}
		if m.chatItems[i].Title != title && strings.TrimSpace(name) != "" {
			continue
		}
		m.chatItems[i].Title = title
		m.chatItems[i].Body = body
		m.chatItems[i].Status = status
		return
	}
	m.appendChat(chatEntry{
		Kind:   "tool",
		Title:  title,
		Body:   body,
		Status: status,
	})
}

func (m *model) updateThinkingCard() {
	if !m.busy || m.streamingIndex < 0 || m.streamingIndex >= len(m.chatItems) {
		return
	}
	item := &m.chatItems[m.streamingIndex]
	if item.Kind != "assistant" || (item.Status != "pending" && item.Status != "thinking") {
		return
	}
	item.Title = thinkingLabel
	item.Status = "thinking"
	item.Body = m.thinkingText()
}

func (m *model) failLatestAssistant(errText string) {
	errText = strings.TrimSpace(errText)
	if errText == "" {
		errText = "Unknown provider error"
	}
	if len(m.chatItems) == 0 {
		m.appendChat(chatEntry{
			Kind:   "assistant",
			Title:  assistantLabel,
			Body:   "Request failed: " + errText,
			Status: "error",
		})
		return
	}
	for i := len(m.chatItems) - 1; i >= 0; i-- {
		if m.chatItems[i].Kind == "assistant" {
			m.chatItems[i].Body = "Request failed: " + errText
			m.chatItems[i].Status = "error"
			return
		}
	}
	m.appendChat(chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   "Request failed: " + errText,
		Status: "error",
	})
}

func (m *model) refreshViewport() {
	m.syncViewportSize()
	m.syncTokenUsageBounds()
	chatOffset := m.viewport.YOffset
	keepChatBottom := m.chatAutoFollow || m.viewport.AtBottom()
	conversationContent := m.renderConversation()
	m.viewportContentCache = conversationContent
	m.viewport.SetContent(conversationContent)
	m.copyView.SetContent(m.renderConversationCopy())
	if keepChatBottom {
		m.viewport.GotoBottom()
		m.copyView.GotoBottom()
		m.chatAutoFollow = true
	} else {
		m.viewport.SetYOffset(chatOffset)
		m.copyView.SetYOffset(chatOffset)
	}
	m.syncCopyViewOffset()

	planOffset := m.planView.YOffset
	m.planView.SetContent(m.planPanelContent(max(16, m.planView.Width)))
	m.planView.SetYOffset(planOffset)
}

func (m *model) syncTokenUsageBounds() {
	if m.screen != screenChat || m.width <= 0 || m.height <= 0 {
		m.tokenUsage.SetBounds(0, 0, 0, 0)
		return
	}
	width := max(24, m.chatPanelInnerWidth())
	badge := strings.TrimSpace(m.renderTokenBadge(width))
	if badge == "" {
		m.tokenUsage.SetBounds(0, 0, 0, 0)
		return
	}
	badgeW := lipgloss.Width(badge)
	badgeH := lipgloss.Height(badge)
	x := panelStyle.GetHorizontalFrameSize()/2 + max(0, width-badgeW-1)
	y := panelStyle.GetVerticalFrameSize() / 2
	m.tokenUsage.SetBounds(x, y, badgeW, badgeH)
}

func (m *model) syncLayoutForCurrentScreen() {
	if m.width > 0 {
		if m.screen == screenLanding {
			m.input.SetWidth(m.landingInputContentWidth())
		} else {
			m.input.SetWidth(m.chatInputContentWidth())
		}
	}
	m.syncInputStyle()
	m.syncViewportSize()
}

func (m *model) resize() {
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
}

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
	base := m.renderLanding()
	if m.screen == screenChat {
		chatContent := lipgloss.JoinVertical(lipgloss.Left, m.renderMainPanel(), m.renderFooter())
		base = panelStyle.Width(m.chatPanelWidth()).Render(chatContent)
	}

	rendered := base
	switch {
	case m.helpOpen:
		rendered = renderModal(m.width, m.height, m.renderHelpModal())
	case m.sessionsOpen:
		rendered = renderModal(m.width, m.height, m.renderSessionsModal())
	}
	return zone.Scan(rendered)
}

func (m *model) SetUsage(used, total int) tea.Cmd {
	m.tokenHasOfficialUsage = true
	m.tokenUsage.SetUnavailable(false)
	return m.tokenUsage.SetUsage(used, 0)
}

func (m model) renderConversation() string {
	if len(m.chatItems) == 0 {
		return mutedStyle.Render("No messages yet. Start with an instruction like \"analyze this repo\" or \"implement a TUI shell\".")
	}
	width := m.viewport.Width
	if width <= 0 {
		width = m.conversationPanelWidth()
	}
	width = max(24, width)
	blocks := make([]string, 0, len(m.chatItems))
	for i := 0; i < len(m.chatItems); {
		item := m.chatItems[i]
		if item.Kind == "user" {
			blocks = append(blocks, renderChatRow(item, width))
			i++
			continue
		}

		j := i
		for j < len(m.chatItems) && m.chatItems[j].Kind != "user" {
			j++
		}
		blocks = append(blocks, renderBytemindRunRow(m.chatItems[i:j], width))
		i = j
	}
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
}

func (m model) renderConversationCopy() string {
	if len(m.chatItems) == 0 {
		return "No messages yet. Start with an instruction like \"analyze this repo\" or \"implement a TUI shell\"."
	}
	width := m.viewport.Width
	if width <= 0 {
		width = m.conversationPanelWidth()
	}
	width = max(24, width)
	blocks := make([]string, 0, len(m.chatItems))
	for i := 0; i < len(m.chatItems); {
		item := m.chatItems[i]
		if item.Kind == "user" {
			blocks = append(blocks, renderChatCopySection(item, width))
			i++
			continue
		}

		j := i
		for j < len(m.chatItems) && m.chatItems[j].Kind != "user" {
			j++
		}

		runParts := make([]string, 0, j-i)
		for _, runItem := range m.chatItems[i:j] {
			runParts = append(runParts, renderChatCopySection(runItem, width))
		}
		blocks = append(blocks, strings.Join(runParts, "\n\n"))
		i = j
	}
	return strings.Join(blocks, "\n\n")
}

func renderChatCopySection(item chatEntry, width int) string {
	title := strings.TrimSpace(item.Title)
	status := strings.TrimSpace(item.Status)
	if status == "final" {
		status = ""
	}
	switch item.Kind {
	case "assistant":
		if strings.EqualFold(item.Status, "thinking") {
			title = "thinking"
			status = ""
		}
	case "user":
		if strings.TrimSpace(item.Meta) != "" {
			title = strings.TrimSpace(item.Meta)
		}
	}

	if title == "" {
		switch item.Kind {
		case "assistant":
			title = assistantLabel
		case "user":
			title = "You"
		case "tool":
			title = "Tool"
		default:
			title = "Message"
		}
	}
	if status != "" {
		title += "  " + status
	}

	body := strings.TrimRight(formatChatBody(item, width), "\n")
	if item.Kind == "tool" && strings.TrimSpace(body) == "" {
		return title
	}
	if strings.TrimSpace(body) == "" {
		return title
	}
	return title + "\n" + body
}

func (m *model) syncViewportSize() {
	if m.width == 0 || m.height == 0 {
		return
	}
	footerHeight := lipgloss.Height(m.renderFooter())
	bodyHeight := m.height - footerHeight
	if bodyHeight < 6 {
		bodyHeight = 6
	}
	statusHeight := lipgloss.Height(m.renderStatusBar())
	panelInnerHeight := max(4, bodyHeight-panelStyle.GetVerticalFrameSize()-statusHeight-1)
	m.planView.Width = 0
	m.planView.Height = 0
	contentHeight := max(3, panelInnerHeight)
	m.viewport.Width = max(8, m.conversationPanelWidth()-scrollbarWidth)
	m.viewport.Height = contentHeight
	m.copyView.Width = m.viewport.Width
	m.copyView.Height = m.viewport.Height
	m.syncCopyViewOffset()
}

func (m *model) syncCopyViewOffset() {
	if m == nil {
		return
	}
	m.copyView.SetYOffset(m.viewport.YOffset)
}

func (m model) renderMainPanel() string {
	width := max(24, m.chatPanelInnerWidth())
	badge := strings.TrimSpace(m.renderTopRightCluster(width))
	conversation := lipgloss.JoinHorizontal(
		lipgloss.Top,
		m.renderConversationViewport(),
		m.renderScrollbar(m.viewport.Height, m.viewport.TotalLineCount(), m.viewport.YOffset),
	)
	if badge == "" {
		return lipgloss.JoinVertical(lipgloss.Left, m.renderStatusBar(), "", conversation)
	}

	badgeW := lipgloss.Width(badge)
	statusW := max(12, width-badgeW-2)
	status := m.renderStatusBarWithWidth(statusW)
	header := lipgloss.JoinHorizontal(lipgloss.Top, status, "  ", badge)

	parts := []string{header}
	if popup := strings.TrimSpace(m.tokenUsage.PopupView()); popup != "" {
		parts = append(parts, lipgloss.PlaceHorizontal(width, lipgloss.Right, popup))
	}
	parts = append(parts, "", conversation)
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) renderTopRightCluster(width int) string {
	parts := make([]string, 0, 2)
	if toast := strings.TrimSpace(m.selectionToast); toast != "" {
		parts = append(parts, selectionToastStyle.Render(toast))
	}
	if badge := strings.TrimSpace(m.renderTokenBadge(width)); badge != "" {
		parts = append(parts, badge)
	}
	return strings.Join(parts, "  ")
}

func (m model) renderTokenBadge(width int) string {
	return m.tokenUsage.View()
}

func (m model) renderScrollbar(viewHeight, contentHeight, currentOffset int) string {
	thumbTop, thumbHeight, _, visible := m.scrollbarLayout(viewHeight, contentHeight, currentOffset)
	if !visible {
		return ""
	}
	trackStyle := scrollbarTrackStyle.Copy().Background(lipgloss.Color("#1B1D22"))
	thumbStyle := scrollbarThumbIdleStyle.Copy().Background(lipgloss.Color("#C2C7CF"))
	if m.draggingScrollbar {
		thumbStyle = scrollbarThumbActiveStyle.Copy().Background(lipgloss.Color("#E5E7EB"))
	}
	lines := make([]string, 0, viewHeight)
	for row := 0; row < viewHeight; row++ {
		if row >= thumbTop && row < thumbTop+thumbHeight {
			lines = append(lines, thumbStyle.Render(" "))
			continue
		}
		lines = append(lines, trackStyle.Render(" "))
	}
	return strings.Join(lines, "\n")
}

func (m model) scrollbarLayout(viewHeight, contentHeight, currentOffset int) (thumbTop, thumbHeight, maxOffset int, visible bool) {
	if viewHeight <= 0 {
		return 0, 0, 0, false
	}
	if contentHeight <= 0 {
		contentHeight = viewHeight
	}
	maxOffset = max(0, contentHeight-viewHeight)
	if maxOffset == 0 {
		return 0, viewHeight, 0, true
	}

	thumbHeight = roundedScaledDivision(viewHeight, viewHeight, contentHeight)
	thumbHeight = clamp(thumbHeight, 1, viewHeight)

	trackRange := max(0, viewHeight-thumbHeight)
	if trackRange == 0 {
		return 0, thumbHeight, maxOffset, true
	}
	offset := clamp(currentOffset, 0, maxOffset)
	thumbTop = roundedScaledDivision(offset, trackRange, maxOffset)
	thumbTop = clamp(thumbTop, 0, trackRange)
	return thumbTop, thumbHeight, maxOffset, true
}

func (m model) scrollbarTrackBounds() (x, top, bottom int, ok bool) {
	if m.screen != screenChat || m.viewport.Width <= 0 || m.viewport.Height <= 0 {
		return 0, 0, 0, false
	}
	left, right, top, bottom, ok := m.conversationViewportBoundsByLayout()
	if !ok {
		return 0, 0, 0, false
	}
	return right + 1, top, bottom, left >= 0
}

func (m *model) dragScrollbarTo(mouseY int) {
	_, trackTop, _, ok := m.scrollbarTrackBounds()
	if !ok {
		return
	}
	_, thumbHeight, maxOffset, visible := m.scrollbarLayout(m.viewport.Height, m.viewport.TotalLineCount(), m.viewport.YOffset)
	if !visible || maxOffset == 0 {
		return
	}
	trackRange := max(0, m.viewport.Height-thumbHeight)
	if trackRange == 0 {
		m.viewport.SetYOffset(0)
		m.syncCopyViewOffset()
		return
	}
	desiredTop := mouseY - trackTop - m.scrollbarDragOffset
	desiredTop = clamp(desiredTop, 0, trackRange)
	offset := roundedScaledDivision(desiredTop, maxOffset, trackRange)
	m.viewport.SetYOffset(clamp(offset, 0, maxOffset))
	m.syncCopyViewOffset()
}

func roundedScaledDivision(value, scale, denominator int) int {
	if denominator <= 0 || value <= 0 || scale <= 0 {
		return 0
	}
	// Use int64 math to avoid overflow when terminal dimensions are large.
	numerator := int64(value)*int64(scale) + int64(denominator)/2
	result := numerator / int64(denominator)
	maxInt := int64(^uint(0) >> 1)
	if result > maxInt {
		return int(maxInt)
	}
	return int(result)
}

func stripANSIText(value string) string {
	return xansi.Strip(value)
}

func (m model) renderLanding() string {
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
		Render(zone.Mark(inputEditorZoneID, m.renderInputEditorView()))
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

func (m model) renderFooter() string {
	ensureZoneManager()
	inputBorder := m.inputBorderStyle().
		Width(m.chatPanelInnerWidth()).
		Render(zone.Mark(inputEditorZoneID, m.renderInputEditorView()))
	parts := make([]string, 0, 4)
	if m.approval != nil {
		parts = append(parts, m.renderApprovalBanner())
	}
	if m.startupGuide.Active {
		parts = append(parts, m.renderStartupGuidePanel())
	} else if m.promptSearchOpen {
		parts = append(parts, m.renderPromptSearchPalette())
	} else if m.mentionOpen {
		parts = append(parts, m.renderMentionPalette())
	} else if m.commandOpen {
		parts = append(parts, m.renderCommandPalette())
	}
	if banner := m.renderActiveSkillBanner(); banner != "" {
		parts = append(parts, banner)
	}
	parts = append(parts, inputBorder, m.renderFooterInfoLine())
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

func (m model) renderModeTabs() string {
	buildStyle := modeTabStyle.Copy().Foreground(colorMuted)
	planStyle := modeTabStyle.Copy().Foreground(colorMuted)
	if m.mode == modeBuild {
		buildStyle = buildStyle.Copy().Foreground(colorAccent).Bold(true)
	} else {
		planStyle = planStyle.Copy().Foreground(colorThinking).Bold(true)
	}
	parts := []string{
		buildStyle.Render("Build"),
		planStyle.Render("Plan"),
	}
	return lipgloss.JoinHorizontal(lipgloss.Left, parts...)
}

func (m model) renderFooterInfoLine() string {
	width := max(24, m.chatPanelInnerWidth())
	left := m.renderModeTabs()
	modelName := strings.TrimSpace(m.currentModelLabel())
	if modelName == "-" {
		modelName = ""
	}
	right := renderFooterInfoRight(modelName, 1<<30)

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := width - leftW - rightW
	if gap < 2 {
		available := max(10, width-leftW-2)
		if available <= 10 {
			return lipgloss.NewStyle().Width(width).Render(renderFooterInfoRight(modelName, width))
		}
		compacted := renderFooterInfoRight(modelName, available)
		gap = width - leftW - lipgloss.Width(compacted)
		return lipgloss.NewStyle().Width(width).Render(left + strings.Repeat(" ", max(2, gap)) + compacted)
	}

	return lipgloss.NewStyle().Width(width).Render(left + strings.Repeat(" ", gap) + right)
}

func renderFooterInfoRight(modelName string, maxWidth int) string {
	maxWidth = max(1, maxWidth)
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return renderInlineShortcutHintsCompacted(footerShortcutHints, maxWidth)
	}
	modelText := compact(modelName, maxWidth)
	modelWidth := runewidth.StringWidth(modelText)
	if modelWidth >= maxWidth {
		return mutedStyle.Render(modelText)
	}
	dividerPlain := "  |  "
	dividerWidth := runewidth.StringWidth(dividerPlain)
	remaining := maxWidth - modelWidth - dividerWidth
	if remaining <= 0 {
		return mutedStyle.Render(modelText)
	}
	hints := renderInlineShortcutHintsCompacted(footerShortcutHints, remaining)
	if strings.TrimSpace(hints) == "" {
		return mutedStyle.Render(modelText)
	}
	return mutedStyle.Render(modelText) + footerHintDividerStyle.Render(dividerPlain) + hints
}

func renderFooterShortcutHints() string {
	return renderInlineShortcutHints(footerShortcutHints)
}

func renderInlineShortcutHints(hints []footerShortcutHint) string {
	parts := make([]string, 0, len(hints))
	for _, hint := range hints {
		parts = append(parts, footerHintKeyStyle.Render(hint.Key)+" "+footerHintLabelStyle.Render(hint.Label))
	}
	return strings.Join(parts, footerHintDividerStyle.Render("  |  "))
}

func renderInlineShortcutHintsCompacted(hints []footerShortcutHint, maxWidth int) string {
	maxWidth = max(1, maxWidth)
	dividerPlain := "  |  "
	dividerWidth := runewidth.StringWidth(dividerPlain)

	used := 0
	parts := make([]string, 0, len(hints)*2)
	for _, hint := range hints {
		key := strings.TrimSpace(hint.Key)
		label := strings.TrimSpace(hint.Label)
		if key == "" {
			continue
		}
		segmentPlain := key
		segmentStyled := footerHintKeyStyle.Render(key)
		if label != "" {
			segmentPlain += " " + label
			segmentStyled += " " + footerHintLabelStyle.Render(label)
		}
		needDivider := len(parts) > 0
		prefixWidth := 0
		if needDivider {
			prefixWidth = dividerWidth
		}
		segmentWidth := runewidth.StringWidth(segmentPlain)
		if used+prefixWidth+segmentWidth <= maxWidth {
			if needDivider {
				parts = append(parts, footerHintDividerStyle.Render(dividerPlain))
				used += dividerWidth
			}
			parts = append(parts, segmentStyled)
			used += segmentWidth
			continue
		}

		remaining := maxWidth - used - prefixWidth
		if remaining <= 0 {
			break
		}
		if needDivider {
			parts = append(parts, footerHintDividerStyle.Render(dividerPlain))
			used += dividerWidth
		}

		keyWidth := runewidth.StringWidth(key)
		if keyWidth >= remaining {
			parts = append(parts, footerHintKeyStyle.Render(compact(key, remaining)))
			break
		}
		if label == "" {
			parts = append(parts, footerHintKeyStyle.Render(key))
			break
		}
		labelSpace := remaining - keyWidth - 1
		if labelSpace <= 0 {
			parts = append(parts, footerHintKeyStyle.Render(key))
			break
		}
		parts = append(parts, footerHintKeyStyle.Render(key)+" "+footerHintLabelStyle.Render(compact(label, labelSpace)))
		break
	}
	return strings.Join(parts, "")
}

func (m model) renderSessionsModal() string {
	lines := []string{modalTitleStyle.Render("Recent Sessions"), mutedStyle.Render("Up/Down to select, Enter to resume, Esc to close"), ""}
	if len(m.sessions) == 0 {
		lines = append(lines, "No sessions available.")
	} else {
		for i, summary := range m.sessions {
			prefix := "  "
			style := lipgloss.NewStyle()
			if i == m.sessionCursor {
				prefix = "> "
				style = style.Foreground(colorAccent).Bold(true)
			}
			line := fmt.Sprintf("%s%s  %s  %d msgs", prefix, shortID(summary.ID), summary.UpdatedAt.Local().Format("2006-01-02 15:04"), summary.MessageCount)
			lines = append(lines, style.Render(line))
			lines = append(lines, mutedStyle.Render("   "+summary.Workspace))
			if summary.LastUserMessage != "" {
				lines = append(lines, mutedStyle.Render("   "+summary.LastUserMessage))
			}
			lines = append(lines, "")
		}
	}
	return modalBoxStyle.Width(min(96, max(56, m.width-12))).Render(strings.Join(lines, "\n"))
}

func (m model) renderHelpModal() string {
	return modalBoxStyle.Width(min(88, max(54, m.width-16))).Render(
		lipgloss.JoinVertical(lipgloss.Left, modalTitleStyle.Render("Help"), m.helpText()),
	)
}

func (m model) renderApprovalBanner() string {
	width := max(24, m.chatPanelInnerWidth())
	commandWidth := max(20, width-4)
	lines := []string{
		accentStyle.Render("Approval required"),
		mutedStyle.Render("Reason: " + trimPreview(m.approval.Reason, commandWidth)),
		codeStyle.Width(commandWidth).Render(m.approval.Command),
		mutedStyle.Render("Y / Enter approve    N / Esc reject"),
	}
	return approvalBannerStyle.Width(width).Render(strings.Join(lines, "\n"))
}

func (m model) renderActiveSkillBanner() string {
	if m.sess == nil || m.sess.ActiveSkill == nil {
		return ""
	}
	name := strings.TrimSpace(m.sess.ActiveSkill.Name)
	if name == "" {
		return ""
	}

	line := "Active skill: " + name
	if len(m.sess.ActiveSkill.Args) > 0 {
		keys := make([]string, 0, len(m.sess.ActiveSkill.Args))
		for key := range m.sess.ActiveSkill.Args {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		pairs := make([]string, 0, len(keys))
		for _, key := range keys {
			pairs = append(pairs, fmt.Sprintf("%s=%s", key, m.sess.ActiveSkill.Args[key]))
		}
		line += " | args: " + strings.Join(pairs, ", ")
	}

	width := max(24, m.chatPanelInnerWidth())
	return activeSkillBannerStyle.Width(width).Render(accentStyle.Render(line))
}

func (m model) renderStatusBar() string {
	return m.renderStatusBarWithWidth(max(24, m.chatPanelInnerWidth()))
}

func (m model) renderStatusBarWithWidth(width int) string {
	stepTitle := currentOrNextStepTitle(m.plan)
	if stepTitle == "" {
		stepTitle = "-"
	}
	left := strings.Join([]string{
		"Mode: " + strings.ToUpper(string(m.mode)),
		"Phase: " + m.currentPhaseLabel(),
		"Step: " + stepTitle,
		"Skill: " + m.currentSkillLabel(),
	}, "  |  ")
	right := strings.Join([]string{
		fmt.Sprintf("%d msgs", len(m.chatItems)),
		"Session: " + m.currentSessionLabel(),
		"Follow: " + m.autoFollowLabel(),
		"Model: " + m.currentModelLabel(),
	}, "  |  ")

	line := m.renderTopInfoLine(left, right, width)
	return statusBarStyle.Width(width).Render(line)
}

func (m model) renderTopInfoLine(left, right string, width int) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if width <= 0 {
		return strings.TrimSpace(left + " | " + right)
	}

	leftW := runewidth.StringWidth(left)
	rightW := runewidth.StringWidth(right)
	if leftW+rightW+2 > width {
		return compact(left+"  |  "+right, width)
	}
	gap := width - leftW - rightW
	return left + strings.Repeat(" ", max(2, gap)) + right
}

func (m model) renderPromptSearchPalette() string {
	width := m.commandPaletteWidth()
	items := m.promptSearchMatches
	modeLabel := "search"
	if m.promptSearchMode == promptSearchModePanel {
		modeLabel = "panel"
	}
	if len(items) == 0 {
		query := strings.TrimSpace(m.promptSearchQuery)
		if query == "" {
			query = "(all)"
		}
		content := []string{
			commandPaletteMetaStyle.Render("Prompt history " + modeLabel),
			commandPaletteMetaStyle.Render("query: "+query+"  (filters: ") + renderInlineShortcutHints(promptSearchFilterHints) + commandPaletteMetaStyle.Render(")"),
			commandPaletteMetaStyle.Render("No matching prompts."),
			commandPaletteMetaStyle.Render("Type to filter  ") +
				renderInlineShortcutHints([]footerShortcutHint{
					{Key: "PgUp/PgDn", Label: "page"},
					{Key: "Enter", Label: "apply"},
					{Key: "Esc", Label: "close"},
				}),
		}
		return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, content...))
	}

	selected, _ := m.selectedPromptSearchEntry()
	rowWidth := max(1, width-commandPaletteStyle.GetHorizontalFrameSize())
	rows := make([]string, 0, promptSearchPageSize+3)
	for _, item := range m.visiblePromptSearchEntriesPage() {
		rowStyle := commandPaletteRowStyle
		textStyle := commandPaletteDescStyle
		if item.Timestamp.Equal(selected.Timestamp) && item.SessionID == selected.SessionID && item.Prompt == selected.Prompt {
			rowStyle = commandPaletteSelectedRowStyle
			textStyle = commandPaletteSelectedDescStyle
		}
		workspaceName := filepath.Base(strings.TrimSpace(item.Workspace))
		if workspaceName == "" || workspaceName == "." {
			workspaceName = strings.TrimSpace(item.Workspace)
		}
		if workspaceName == "" {
			workspaceName = "-"
		}
		meta := fmt.Sprintf("%s  ws:%s  sid:%s", item.Timestamp.Local().Format("01-02 15:04"), compact(workspaceName, 16), compact(item.SessionID, 12))
		rowText := compact(strings.TrimSpace(item.Prompt), max(12, rowWidth-2))
		rows = append(rows, rowStyle.Width(rowWidth).Render(textStyle.Render(rowText)))
		rows = append(rows, rowStyle.Width(rowWidth).Render(commandPaletteMetaStyle.Render(compact(meta, max(12, rowWidth-2)))))
	}
	for len(rows) < promptSearchPageSize*2 {
		rows = append(rows, commandPaletteRowStyle.Width(rowWidth).Render(""))
	}

	query := strings.TrimSpace(m.promptSearchQuery)
	if query == "" {
		query = "(all)"
	}
	meta := commandPaletteMetaStyle.Render(fmt.Sprintf("%s  query:%s", modeLabel, compact(query, 24))) +
		footerHintDividerStyle.Render("  |  ") +
		renderInlineShortcutHints(promptSearchFilterHints) +
		footerHintDividerStyle.Render("  |  ") +
		renderInlineShortcutHints(promptSearchActionHints)
	rows = append(rows, meta)
	return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m model) renderStartupGuidePanel() string {
	width := max(24, m.commandPaletteWidth())
	title := strings.TrimSpace(m.startupGuide.Title)
	if title == "" {
		title = "Provider setup required"
	}
	status := strings.TrimSpace(m.startupGuide.Status)
	if status == "" {
		status = "AI provider is not available."
	}

	innerWidth := max(20, width-commandPaletteStyle.GetHorizontalFrameSize())
	content := make([]string, 0, 2+len(m.startupGuide.Lines))
	content = append(content, accentStyle.Render(title))
	content = append(content, commandPaletteMetaStyle.Width(innerWidth).Render(status))
	for _, line := range m.startupGuide.Lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		content = append(content, commandPaletteMetaStyle.Width(innerWidth).Render(line))
	}
	content = append(content, commandPaletteMetaStyle.Width(innerWidth).Render(startupGuideInputHint(m.startupGuide.CurrentField)))

	return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, content...))
}

func (m *model) handleStartupGuideSubmission(rawInput string) error {
	rawInput = strings.TrimSpace(rawInput)

	field := strings.TrimSpace(m.startupGuide.CurrentField)
	if !isStartupGuideField(field) {
		field = startupFieldType
	}
	if explicitField, explicitValue, ok := parseStartupConfigInput(rawInput); ok {
		field = explicitField
		rawInput = explicitValue
	}

	switch field {
	case startupFieldType, startupFieldBaseURL, startupFieldModel:
		value, err := m.resolveStartupFieldValue(field, rawInput)
		if err != nil {
			return err
		}
		if err := m.applyStartupConfigField(field, value); err != nil {
			return err
		}
		next := startupNextField(field)
		if next == "" {
			next = startupFieldAPIKey
		}
		m.setStartupGuideStep(next, "")
		m.input.Reset()
		return nil
	case startupFieldAPIKey:
		return m.verifyAndFinalizeStartupAPIKey(rawInput)
	default:
		return fmt.Errorf("unsupported setup field: %s", field)
	}
}

func (m *model) verifyAndFinalizeStartupAPIKey(rawInput string) error {
	apiKey := sanitizeAPIKeyInput(rawInput)
	if apiKey == "" {
		return fmt.Errorf("please paste a non-empty API key")
	}

	checkCfg := m.cfg.Provider
	checkCfg.APIKey = apiKey
	check := provider.CheckAvailability(context.Background(), checkCfg)
	if !check.Ready {
		m.llmConnected = false
		m.phase = "error"
		m.setStartupGuideStep(startupFieldAPIKey, startupGuideIssueHint(check))
		return nil
	}

	writtenPath, saveErr := config.UpsertProviderAPIKey(m.startupGuide.ConfigPath, apiKey)

	if envName := strings.TrimSpace(checkCfg.APIKeyEnv); envName != "" {
		if err := os.Setenv(envName, apiKey); err != nil {
			warnSetenv(envName, err)
		}
	} else {
		if err := os.Setenv("BYTEMIND_API_KEY", apiKey); err != nil {
			warnSetenv("BYTEMIND_API_KEY", err)
		}
	}

	client, err := provider.NewClient(checkCfg)
	if err != nil {
		return err
	}
	if m.runner != nil {
		m.runner.UpdateProvider(checkCfg, client)
	}
	m.cfg.Provider = checkCfg
	m.startupGuide.Active = false
	m.statusNote = "Provider configured and verified. You can start chatting."
	m.llmConnected = true
	m.phase = "idle"
	if saveErr != nil {
		m.statusNote = "Provider verified, but config save failed: " + compact(saveErr.Error(), 80)
	} else if strings.TrimSpace(writtenPath) != "" {
		m.statusNote = "Provider configured and verified. Saved to " + compact(writtenPath, 48)
	}
	m.syncInputStyle()
	m.input.Reset()
	return nil
}

func (m *model) applyStartupConfigField(field, value string) error {
	field = strings.TrimSpace(field)
	value = strings.TrimSpace(value)
	if value == "" {
		return fmt.Errorf("%s cannot be empty", field)
	}
	persistValue := value

	switch field {
	case "model":
		m.cfg.Provider.Model = value
	case "base_url":
		m.cfg.Provider.BaseURL = value
	case "type":
		normalized, ok := normalizeStartupProviderType(value)
		if !ok {
			return fmt.Errorf("provider must be openai-compatible or anthropic")
		}
		m.cfg.Provider.Type = normalized
		persistValue = normalized
	default:
		return fmt.Errorf("unsupported setup field: %s", field)
	}

	writtenPath, err := config.UpsertProviderField(m.startupGuide.ConfigPath, field, persistValue)
	if err != nil {
		return err
	}
	if strings.TrimSpace(writtenPath) != "" {
		m.startupGuide.ConfigPath = writtenPath
	}
	return nil
}

func parseStartupConfigInput(raw string) (field, value string, ok bool) {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if lower == "" {
		return "", "", false
	}

	parse := func(alias, normalized string) (string, string, bool) {
		for _, sep := range []string{"=", ":"} {
			prefix := alias + sep
			if strings.HasPrefix(lower, prefix) {
				val := strings.TrimSpace(trimmed[len(prefix):])
				return normalized, val, true
			}
		}
		return "", "", false
	}

	for _, candidate := range []struct {
		alias      string
		normalized string
	}{
		{alias: "model", normalized: "model"},
		{alias: "base_url", normalized: "base_url"},
		{alias: "baseurl", normalized: "base_url"},
		{alias: "base-url", normalized: "base_url"},
		{alias: "provider", normalized: "type"},
		{alias: "type", normalized: "type"},
		{alias: "provider_type", normalized: "type"},
		{alias: "api_key", normalized: "api_key"},
		{alias: "apikey", normalized: "api_key"},
		{alias: "key", normalized: "api_key"},
	} {
		if field, value, ok := parse(candidate.alias, candidate.normalized); ok {
			return field, value, true
		}
	}

	return "", "", false
}

func sanitizeAPIKeyInput(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.Trim(value, "\"'")
	lower := strings.ToLower(value)
	if strings.HasPrefix(lower, "authorization: bearer ") {
		value = strings.TrimSpace(value[len("authorization: bearer "):])
	}
	if strings.HasPrefix(lower, "bearer ") {
		value = strings.TrimSpace(value[len("bearer "):])
	}
	return strings.TrimSpace(value)
}

func normalizeStartupProviderType(value string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "openai-compatible", "openai_compatible", "openai":
		return "openai-compatible", true
	case "anthropic":
		return "anthropic", true
	default:
		return "", false
	}
}

func isStartupGuideField(field string) bool {
	switch field {
	case startupFieldType, startupFieldBaseURL, startupFieldModel, startupFieldAPIKey:
		return true
	default:
		return false
	}
}

func startupNextField(current string) string {
	for i, field := range startupFieldOrder {
		if field == current {
			if i+1 >= len(startupFieldOrder) {
				return ""
			}
			return startupFieldOrder[i+1]
		}
	}
	return startupFieldType
}

func startupFieldStep(field string) (int, int) {
	for i, item := range startupFieldOrder {
		if item == field {
			return i + 1, len(startupFieldOrder)
		}
	}
	return 1, len(startupFieldOrder)
}

func startupFieldName(field string) string {
	switch field {
	case startupFieldType:
		return "provider"
	case startupFieldBaseURL:
		return "base_url"
	case startupFieldModel:
		return "model"
	case startupFieldAPIKey:
		return "api_key"
	default:
		return field
	}
}

func startupProviderDefaultBaseURL(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "anthropic":
		return "https://api.anthropic.com"
	default:
		return "https://api.openai.com/v1"
	}
}

func startupProviderDefaultModel(providerType string) string {
	switch strings.ToLower(strings.TrimSpace(providerType)) {
	case "anthropic":
		return ""
	default:
		return "GPT-5.4"
	}
}

func (m model) startupCurrentValue(field string) string {
	switch field {
	case startupFieldType:
		return strings.TrimSpace(m.cfg.Provider.Type)
	case startupFieldBaseURL:
		return strings.TrimSpace(m.cfg.Provider.BaseURL)
	case startupFieldModel:
		return strings.TrimSpace(m.cfg.Provider.Model)
	default:
		return ""
	}
}

func (m *model) resolveStartupFieldValue(field, rawInput string) (string, error) {
	value := strings.TrimSpace(rawInput)
	if value != "" {
		return value, nil
	}

	current := m.startupCurrentValue(field)
	if current != "" {
		return current, nil
	}

	switch field {
	case startupFieldType:
		return "openai-compatible", nil
	case startupFieldBaseURL:
		return startupProviderDefaultBaseURL(m.cfg.Provider.Type), nil
	case startupFieldModel:
		if fallback := startupProviderDefaultModel(m.cfg.Provider.Type); fallback != "" {
			return fallback, nil
		}
		return "", fmt.Errorf("please enter model name for provider %s", strings.TrimSpace(m.cfg.Provider.Type))
	default:
		return "", fmt.Errorf("%s cannot be empty", startupFieldName(field))
	}
}

func (m *model) initializeStartupGuide() {
	field := strings.TrimSpace(m.startupGuide.CurrentField)
	if !isStartupGuideField(field) {
		field = startupFieldType
	}
	m.setStartupGuideStep(field, "")
}

func (m *model) setStartupGuideStep(field, issue string) {
	if !isStartupGuideField(field) {
		field = startupFieldType
	}
	step, total := startupFieldStep(field)
	fieldName := startupFieldName(field)
	if strings.TrimSpace(issue) == "" {
		m.startupGuide.Status = fmt.Sprintf("Step %d/%d: set %s.", step, total, fieldName)
	} else {
		m.startupGuide.Status = fmt.Sprintf("Step %d/%d: set %s. %s", step, total, fieldName, issue)
	}
	m.statusNote = m.startupGuide.Status
	m.startupGuide.CurrentField = field
	m.startupGuide.Lines = startupGuideStepLines(field, m.cfg, m.startupGuide.ConfigPath, issue)
	m.syncInputStyle()
}

func startupGuideStepLines(field string, cfg config.Config, configPath, issue string) []string {
	lines := make([]string, 0, 8)
	switch field {
	case startupFieldType:
		lines = append(lines, "Enter provider: openai-compatible or anthropic.")
	case startupFieldBaseURL:
		lines = append(lines, "Enter provider base_url.")
		lines = append(lines, "Example: https://api.deepseek.com")
	case startupFieldModel:
		lines = append(lines, "Enter model name.")
		lines = append(lines, "Example: deepseek-chat or GPT-5.4")
	case startupFieldAPIKey:
		lines = append(lines, "Paste API key and press Enter.")
		lines = append(lines, "Bytemind will verify it automatically.")
	}

	switch field {
	case startupFieldType, startupFieldBaseURL, startupFieldModel:
		current := ""
		switch field {
		case startupFieldType:
			current = strings.TrimSpace(cfg.Provider.Type)
		case startupFieldBaseURL:
			current = strings.TrimSpace(cfg.Provider.BaseURL)
		case startupFieldModel:
			current = strings.TrimSpace(cfg.Provider.Model)
		}
		if current == "" {
			lines = append(lines, "Press Enter to use default.")
		} else {
			lines = append(lines, "Press Enter to keep current: "+current)
		}
	}
	if strings.TrimSpace(issue) != "" {
		lines = append(lines, "Issue: "+issue)
	}
	if strings.TrimSpace(configPath) != "" {
		lines = append(lines, "Config file: "+configPath)
	}
	return lines
}

func startupGuideIssueHint(check provider.Availability) string {
	reason := strings.ToLower(strings.TrimSpace(check.Reason))
	switch {
	case strings.Contains(reason, "missing api key"):
		return "No API key is configured yet."
	case strings.Contains(reason, "unauthorized"):
		return "The API key was rejected by the provider."
	case strings.Contains(reason, "failed to reach"):
		return "Cannot reach provider endpoint. Check proxy or network."
	case strings.Contains(reason, "not found"):
		return "Provider endpoint path looks incorrect."
	default:
		if strings.TrimSpace(check.Reason) == "" {
			return "Provider check failed."
		}
		return compact(strings.TrimSpace(check.Reason), 90)
	}
}

func (m model) renderCommandPalette() string {
	width := m.commandPaletteWidth()
	items := m.filteredCommands()
	if len(items) == 0 {
		return commandPaletteStyle.Width(width).Render(
			commandPaletteMetaStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render("No matching commands."),
		)
	}

	selected, _ := m.selectedCommandItem()
	nameWidth := min(26, max(14, width/4))
	descWidth := max(12, width-commandPaletteStyle.GetHorizontalFrameSize()-nameWidth-4)
	rows := make([]string, 0, commandPageSize+1)
	for _, item := range m.visibleCommandItemsPage() {
		rowStyle := commandPaletteRowStyle
		nameStyle := commandPaletteNameStyle
		descStyle := commandPaletteDescStyle
		if item.Name == selected.Name {
			rowStyle = commandPaletteSelectedRowStyle
			nameStyle = commandPaletteSelectedNameStyle
			descStyle = commandPaletteSelectedDescStyle
		}

		name := nameStyle.Width(nameWidth).Render(item.Usage)
		desc := descStyle.Width(descWidth).Render(compact(item.Description, max(12, descWidth)))
		rows = append(rows, rowStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render(
			lipgloss.JoinHorizontal(lipgloss.Top, name, "  ", desc),
		))
	}
	for len(rows) < commandPageSize {
		rows = append(rows, commandPaletteRowStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render(""))
	}
	rows = append(rows, commandPaletteMetaStyle.Render("Up/Down move  PgUp/PgDn page  Enter run  Esc close"))
	return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m model) renderMentionPalette() string {
	width := m.commandPaletteWidth()
	items := m.mentionResults
	if len(items) == 0 {
		return commandPaletteStyle.Width(width).Render(
			commandPaletteMetaStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render("No matching files in workspace."),
		)
	}

	selected, _ := m.selectedMentionCandidate()
	nameWidth := min(26, max(12, width/4))
	descWidth := max(12, width-commandPaletteStyle.GetHorizontalFrameSize()-nameWidth-4)
	rows := make([]string, 0, mentionPageSize+1)
	for _, item := range m.visibleMentionItemsPage() {
		rowStyle := commandPaletteRowStyle
		nameStyle := commandPaletteNameStyle
		descStyle := commandPaletteDescStyle
		if item.Path == selected.Path {
			rowStyle = commandPaletteSelectedRowStyle
			nameStyle = commandPaletteSelectedNameStyle
			descStyle = commandPaletteSelectedDescStyle
		}

		nameText := item.BaseName
		if tag := strings.TrimSpace(item.TypeTag); tag != "" {
			nameText = "[" + tag + "] " + nameText
		}
		if m.hasRecentMention(item.Path) {
			nameText = "* " + nameText
		} else {
			nameText = "  " + nameText
		}

		name := nameStyle.Width(nameWidth).Render(compact(nameText, max(12, nameWidth)))
		desc := descStyle.Width(descWidth).Render(compact(item.Path, max(12, descWidth)))
		rows = append(rows, rowStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render(
			lipgloss.JoinHorizontal(lipgloss.Top, name, "  ", desc),
		))
	}
	for len(rows) < mentionPageSize {
		rows = append(rows, commandPaletteRowStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render(""))
	}
	metaText := "* recent  Type @query to search  Up/Down move  Enter/Tab insert  Esc close"
	if m.mentionIndex != nil {
		stats := m.mentionIndex.Stats()
		if stats.Truncated && stats.MaxFiles > 0 {
			metaText = fmt.Sprintf("* recent  indexed first %d files  Enter/Tab insert  Esc close", stats.MaxFiles)
		}
	}
	rows = append(rows, commandPaletteMetaStyle.Render(metaText))
	return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m *model) handleSlashCommand(input string) error {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return nil
	}

	switch fields[0] {
	case "/help":
		m.screen = screenChat
		m.appendChat(chatEntry{
			Kind:   "user",
			Title:  "You",
			Meta:   formatUserMeta(m.currentModelLabel(), time.Now()),
			Body:   input,
			Status: "final",
		})
		m.appendChat(chatEntry{Kind: "assistant", Title: assistantLabel, Body: m.helpText(), Status: "final"})
		m.statusNote = "Help opened in the conversation view."
		return nil
	case "/session":
		m.sessionsOpen = true
		m.statusNote = "Opened recent sessions."
		return nil
	case "/skills":
		return m.runSkillsListCommand(input)
	case "/skill":
		return m.runSkillCommand(input, fields)
	case "/new":
		return m.newSession()
	case "/compact":
		return m.runCompactCommand(input)
	default:
		return m.runDirectSkillCommand(input, fields)
	}
}

func (m model) executeCommand(input string) (tea.Model, tea.Cmd, error) {
	if err := m.handleSlashCommand(input); err != nil {
		return m, nil, err
	}
	m.refreshViewport()
	return m, m.loadSessionsCmd(), nil
}

func (m *model) runSkillsListCommand(input string) error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	skillsList, diagnostics := m.runner.ListSkills()
	active, hasActive := m.runner.GetActiveSkill(m.sess)

	lines := make([]string, 0, len(skillsList)+8)
	if hasActive {
		lines = append(lines, fmt.Sprintf("Active skill: %s (%s)", active.Name, active.Scope))
	} else {
		lines = append(lines, "Active skill: none")
	}
	lines = append(lines, "")
	if len(skillsList) == 0 {
		lines = append(lines, "No skills discovered.")
	} else {
		lines = append(lines, "Available skills:")
		for _, skill := range skillsList {
			lines = append(lines, fmt.Sprintf("- %s (%s): %s", skill.Name, skill.Scope, skill.Description))
		}
	}
	if len(diagnostics) > 0 {
		lines = append(lines, "", "Diagnostics:")
		for _, diag := range diagnostics {
			lines = append(lines, fmt.Sprintf("- [%s] %s (%s): %s", diag.Level, diag.Skill, diag.Path, diag.Message))
		}
	}

	m.appendCommandExchange(input, strings.Join(lines, "\n"))
	m.statusNote = fmt.Sprintf("Discovered %d skill(s).", len(skillsList))
	return nil
}

func (m *model) runSkillCommand(input string, fields []string) error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	if len(fields) < 2 {
		return fmt.Errorf("usage: /skill <clear|delete> ...")
	}
	switch strings.ToLower(strings.TrimSpace(fields[1])) {
	case "clear":
		return m.runSkillStateClearCommand(input, fields)
	case "delete":
		return m.runSkillDeleteCommand(input, fields)
	default:
		return fmt.Errorf("usage: /skill <clear|delete> ...")
	}
}

func (m *model) runSkillStateClearCommand(input string, fields []string) error {
	if len(fields) != 2 {
		return fmt.Errorf("usage: /skill clear")
	}

	activeName := ""
	if m.sess != nil && m.sess.ActiveSkill != nil {
		activeName = strings.TrimSpace(m.sess.ActiveSkill.Name)
	}
	if err := m.runner.ClearActiveSkill(m.sess); err != nil {
		return err
	}

	message := "No active skill in this session; state remains empty."
	if activeName != "" {
		message = fmt.Sprintf("Cleared active skill `%s` from this session.", activeName)
	}
	m.appendCommandExchange(input, message)
	m.statusNote = "Skill state cleared"
	return nil
}

func (m *model) runSkillDeleteCommand(input string, fields []string) error {
	if len(fields) < 3 {
		return fmt.Errorf("usage: /skill delete <name>")
	}
	name := strings.TrimSpace(strings.TrimPrefix(fields[2], "/"))
	if name == "" {
		return fmt.Errorf("usage: /skill delete <name>")
	}

	result, err := m.runner.ClearSkill(name)
	if err != nil {
		return err
	}

	lines := []string{
		fmt.Sprintf("Deleted project skill `%s`.", result.Name),
		fmt.Sprintf("Dir: %s", result.Dir),
	}

	if m.sess != nil && m.sess.ActiveSkill != nil && strings.EqualFold(strings.TrimSpace(m.sess.ActiveSkill.Name), strings.TrimSpace(result.Name)) {
		if clearErr := m.runner.ClearActiveSkill(m.sess); clearErr == nil {
			lines = append(lines, "Cleared active skill in this session as well.")
		}
	}
	m.appendCommandExchange(input, strings.Join(lines, "\n"))
	m.statusNote = "Skill deleted"
	return nil
}

func (m *model) runCompactCommand(input string) error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	if m.sess == nil {
		return fmt.Errorf("session is unavailable")
	}
	type sessionCompactor interface {
		CompactSession(ctx context.Context, sess *session.Session) (string, bool, error)
	}
	compactor, ok := any(m.runner).(sessionCompactor)
	if !ok {
		return fmt.Errorf("compact is unavailable in this build")
	}
	summary, changed, err := compactor.CompactSession(context.Background(), m.sess)
	if err != nil {
		return err
	}
	if !changed {
		m.appendCommandExchange(input, "No compaction needed yet. Start a longer conversation first.")
		m.statusNote = "No compaction needed."
		return nil
	}
	preview := compact(summary, 360)
	response := "Conversation compacted for long-context continuation."
	if strings.TrimSpace(preview) != "" {
		response += "\nSummary preview: " + preview
	}
	m.chatItems, m.toolRuns = rebuildSessionTimeline(m.sess)
	m.appendCommandExchange(input, response)
	m.statusNote = "Conversation compacted."
	return nil
}

func (m *model) runDirectSkillCommand(input string, fields []string) error {
	if len(fields) == 0 {
		return nil
	}
	name := strings.TrimSpace(fields[0])
	if !strings.HasPrefix(name, "/") || !m.isKnownSkillCommand(name) {
		return fmt.Errorf("unknown command: %s", fields[0])
	}
	args, err := parseSkillArgs(fields[1:])
	if err != nil {
		return err
	}
	return m.activateSkillCommand(input, name, args)
}

func (m *model) activateSkillCommand(input, name string, args map[string]string) error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	skill, err := m.runner.ActivateSkill(m.sess, name, args)
	if err != nil {
		return err
	}
	response := fmt.Sprintf("Activated skill `%s` (%s).\nTool policy: %s\nEntry: %s", skill.Name, skill.Scope, skill.ToolPolicy.Policy, skill.Entry.Slash)
	if len(args) > 0 {
		argParts := make([]string, 0, len(args))
		keys := make([]string, 0, len(args))
		for key := range args {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			argParts = append(argParts, fmt.Sprintf("%s=%s", key, args[key]))
		}
		response += "\nArgs: " + strings.Join(argParts, ", ")
	}
	m.appendCommandExchange(input, response)
	m.statusNote = "Skill activated."
	return nil
}

func parseSkillArgs(parts []string) (map[string]string, error) {
	if len(parts) == 0 {
		return nil, nil
	}
	args := make(map[string]string, len(parts))
	for _, part := range parts {
		pieces := strings.SplitN(part, "=", 2)
		if len(pieces) != 2 {
			return nil, fmt.Errorf("invalid skill arg %q, expected k=v", part)
		}
		key := strings.TrimSpace(pieces[0])
		value := strings.TrimSpace(pieces[1])
		if key == "" || value == "" {
			return nil, fmt.Errorf("invalid skill arg %q, expected k=v", part)
		}
		args[key] = value
	}
	if len(args) == 0 {
		return nil, nil
	}
	return args, nil
}

func (m *model) appendCommandExchange(command, response string) {
	m.screen = screenChat
	m.appendChat(chatEntry{
		Kind:   "user",
		Title:  "You",
		Meta:   formatUserMeta(m.currentModelLabel(), time.Now()),
		Body:   command,
		Status: "final",
	})
	m.appendChat(chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   response,
		Status: "final",
	})
}

func (m *model) newSession() error {
	next := session.New(m.workspace)
	if err := m.store.Save(next); err != nil {
		return err
	}
	m.sess = next
	m.screen = screenLanding
	m.plan = planpkg.State{}
	m.mode = modeBuild
	m.chatItems = nil
	m.toolRuns = nil
	m.streamingIndex = -1
	m.statusNote = "Started a new session."
	m.chatAutoFollow = true
	m.restoreTokenUsageFromSession(next)
	_ = m.tokenUsage.SetUsage(m.tokenUsedTotal, 0)
	m.tokenUsage.SetUnavailable(!m.tokenHasOfficialUsage)
	m.tokenUsage.SetBreakdown(m.tokenInput, m.tokenOutput, m.tokenContext)
	m.inputImageRefs = make(map[int]llm.AssetID, 8)
	m.inputImageMentions = make(map[string]llm.AssetID, 8)
	m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	m.nextImageID = nextSessionImageID(next)
	m.ensureSessionImageAssets()
	m.pastedContents = make(map[string]pastedContent, maxStoredPastedContents)
	m.pastedOrder = make([]string, 0, maxStoredPastedContents)
	m.nextPasteID = 1
	m.pastedStateLoaded = false
	m.lastCompressedPasteAt = time.Time{}
	m.ensurePastedContentState()
	m.pendingBTW = nil
	m.interrupting = false
	m.interruptSafe = false
	m.runCancel = nil
	m.activeRunID = 0
	m.input.Reset()
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return nil
}

func (m *model) resumeSession(prefix string) error {
	id, err := resolveSessionID(m.sessions, prefix)
	if err != nil {
		return err
	}
	next, err := m.store.Load(id)
	if err != nil {
		return err
	}
	if !sameWorkspace(m.workspace, next.Workspace) {
		return fmt.Errorf("session %s belongs to workspace %s", next.ID, next.Workspace)
	}
	m.sess = next
	m.screen = screenChat
	m.plan = copyPlanState(next.Plan)
	m.mode = toAgentMode(next.Mode)
	m.chatItems, m.toolRuns = rebuildSessionTimeline(next)
	m.streamingIndex = -1
	m.statusNote = "Resumed session " + shortID(next.ID)
	m.chatAutoFollow = true
	m.restoreTokenUsageFromSession(next)
	_ = m.tokenUsage.SetUsage(m.tokenUsedTotal, 0)
	m.tokenUsage.SetUnavailable(!m.tokenHasOfficialUsage)
	m.tokenUsage.SetBreakdown(m.tokenInput, m.tokenOutput, m.tokenContext)
	m.inputImageRefs = make(map[int]llm.AssetID, 8)
	m.inputImageMentions = make(map[string]llm.AssetID, 8)
	m.orphanedImages = make(map[llm.AssetID]time.Time, 8)
	m.nextImageID = nextSessionImageID(next)
	m.ensureSessionImageAssets()
	m.pastedContents = make(map[string]pastedContent, maxStoredPastedContents)
	m.pastedOrder = make([]string, 0, maxStoredPastedContents)
	m.nextPasteID = 1
	m.pastedStateLoaded = false
	m.lastCompressedPasteAt = time.Time{}
	m.ensurePastedContentState()
	m.syncInputImageRefs("")
	m.pendingBTW = nil
	m.interrupting = false
	m.interruptSafe = false
	m.runCancel = nil
	m.activeRunID = 0
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return nil
}

func (m model) startRunCmd(runCtx context.Context, runID int, prompt agent.RunPromptInput, mode string) tea.Cmd {
	return func() tea.Msg {
		go func() {
			_, err := m.runner.RunPromptWithInput(runCtx, m.sess, prompt, mode, io.Discard)
			m.async <- runFinishedMsg{RunID: runID, Err: err}
		}()
		return nil
	}
}

func (m model) loadSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		if m.store == nil {
			return sessionsLoadedMsg{}
		}
		summaries, _, err := m.store.List(m.sessionLimit)
		return sessionsLoadedMsg{Summaries: summaries, Err: err}
	}
}

func (m model) fetchRemoteTokenUsageCmd() tea.Cmd {
	return func() tea.Msg {
		usage, err := fetchCurrentMonthUsage(m.cfg)
		if err != nil {
			return tokenUsagePulledMsg{Err: err}
		}
		return tokenUsagePulledMsg{
			Used:    usage.Used,
			Input:   usage.Input,
			Output:  usage.Output,
			Context: usage.Context,
		}
	}
}

func (m *model) restoreTokenUsageFromSession(sess *session.Session) {
	m.tempEstimatedOutput = 0
	m.tokenHasOfficialUsage = false
	m.tokenUsedTotal = 0
	m.tokenInput = 0
	m.tokenOutput = 0
	m.tokenContext = 0

	if sess != nil {
		m.accumulateTokenUsage(sess.Messages)
	}
}

func (m *model) accumulateTokenUsage(messages []llm.Message) {
	for _, msg := range messages {
		if msg.Usage == nil {
			continue
		}
		m.tokenHasOfficialUsage = true
		used := msg.Usage.TotalTokens
		if used <= 0 {
			used = msg.Usage.InputTokens + msg.Usage.OutputTokens + msg.Usage.ContextTokens
		}
		m.tokenUsedTotal += max(0, used)
		m.tokenInput += max(0, msg.Usage.InputTokens)
		m.tokenOutput += max(0, msg.Usage.OutputTokens)
		m.tokenContext += max(0, msg.Usage.ContextTokens)
	}
}

func waitForAsync(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func rebuildSessionTimeline(sess *session.Session) ([]chatEntry, []toolRun) {
	items := make([]chatEntry, 0, len(sess.Messages))
	runs := make([]toolRun, 0, 8)
	callNames := map[string]string{}

	for _, message := range sess.Messages {
		message.Normalize()
		switch message.Role {
		case "user":
			userTextParts := make([]string, 0, len(message.Parts))
			for _, part := range message.Parts {
				if part.Text != nil {
					userTextParts = append(userTextParts, part.Text.Value)
				}
				if part.ToolResult == nil {
					continue
				}
				name := callNames[part.ToolResult.ToolUseID]
				if name == "" {
					name = "tool"
				}
				summary, lines, status := summarizeTool(name, part.ToolResult.Content)
				items = append(items, chatEntry{
					Kind:   "tool",
					Title:  "Tool Call | " + name,
					Body:   joinSummary(summary, lines),
					Status: status,
				})
				runs = append(runs, toolRun{Name: name, Summary: summary, Lines: lines, Status: status})
			}
			userText := strings.Join(userTextParts, "")
			if strings.TrimSpace(userText) != "" {
				items = append(items, chatEntry{Kind: "user", Title: "You", Body: userText, Status: "final"})
			}
		case "assistant":
			for _, call := range message.ToolCalls {
				callNames[call.ID] = call.Function.Name
			}
			if strings.TrimSpace(message.Text()) != "" {
				items = append(items, chatEntry{Kind: "assistant", Title: assistantLabel, Body: message.Text(), Status: "final"})
			}
		case "tool":
			name := callNames[message.ToolCallID]
			if name == "" {
				name = "tool"
			}
			summary, lines, status := summarizeTool(name, message.Content)
			items = append(items, chatEntry{
				Kind:   "tool",
				Title:  "Tool Call | " + name,
				Body:   joinSummary(summary, lines),
				Status: status,
			})
			runs = append(runs, toolRun{Name: name, Summary: summary, Lines: lines, Status: status})
		}
	}
	return items, runs
}

func renderChatCard(item chatEntry, width int) string {
	border := chatAssistantStyle
	switch item.Kind {
	case "user":
		border = chatUserStyle
	case "tool":
		border = chatAssistantStyle
	case "system":
		border = chatSystemStyle
	default:
		if item.Status == "thinking" {
			border = chatThinkingStyle
		}
	}
	contentWidth := max(8, width-border.GetHorizontalFrameSize())
	rendered := border.Width(contentWidth).Render(renderChatSection(item, contentWidth))
	if item.Kind != "tool" {
		return rendered
	}

	sep := lipgloss.NewStyle().Foreground(colorTool).Render("|")
	lines := strings.Split(rendered, "\n")
	for i := range lines {
		if strings.TrimSpace(lines[i]) == "" {
			lines[i] = "  " + lines[i]
			continue
		}
		lines[i] = sep + " " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func renderChatSection(item chatEntry, width int) string {
	title := cardTitleStyle.Foreground(colorAccent)
	bodyStyle := chatBodyStyle
	toolCallTitle := cardTitleStyle.Foreground(lipgloss.Color("#E5B567")).Bold(true)
	toolResultTitle := cardTitleStyle.Foreground(lipgloss.Color("#7AC7FF")).Bold(true)
	status := item.Status
	displayTitle := item.Title
	if status == "final" {
		status = ""
	}
	switch item.Kind {
	case "user":
		title = cardTitleStyle.Foreground(colorUser)
	case "tool":
		if strings.HasPrefix(displayTitle, "Tool Result | ") {
			title = toolResultTitle
		} else {
			title = toolCallTitle
		}
		bodyStyle = toolBodyStyle
		status = ""
	case "system":
		title = cardTitleStyle.Foreground(colorMuted)
	default:
		if item.Status == "thinking" {
			title = cardTitleStyle.Foreground(colorMuted).Faint(true)
			bodyStyle = thinkingBodyStyle
			displayTitle = "thinking"
			status = ""
		}
	}
	headContent := title.Render(displayTitle)
	if item.Kind == "user" && strings.TrimSpace(item.Meta) != "" {
		headContent = mutedStyle.Copy().Faint(true).Render(item.Meta)
	}
	if status != "" {
		headContent = lipgloss.JoinHorizontal(lipgloss.Left, headContent, mutedStyle.Render("  "+status))
	}
	head := lipgloss.NewStyle().
		Width(width).
		Render(headContent)
	if item.Kind == "tool" && strings.TrimSpace(item.Body) == "" {
		return head
	}
	body := bodyStyle.Width(width).Render(formatChatBody(item, width))
	return lipgloss.JoinVertical(lipgloss.Left, head, body)
}

func renderChatRow(item chatEntry, width int) string {
	bubbleWidth := chatBubbleWidth(item, width)
	card := renderChatCard(item, bubbleWidth)
	return lipgloss.NewStyle().
		MarginBottom(1).
		Render(lipgloss.PlaceHorizontal(width, lipgloss.Left, card))
}

func renderBytemindRunRow(items []chatEntry, width int) string {
	if len(items) == 0 {
		return ""
	}
	card := renderBytemindRunCard(items, width)
	return lipgloss.NewStyle().
		MarginBottom(1).
		Render(lipgloss.PlaceHorizontal(width, lipgloss.Left, card))
}

func renderBytemindRunCard(items []chatEntry, width int) string {
	outer := chatAssistantStyle
	contentWidth := max(8, width-outer.GetHorizontalFrameSize())
	sections := make([]string, 0, len(items))
	for _, item := range items {
		sections = append(sections, renderChatSection(item, contentWidth))
	}
	return outer.Width(contentWidth).Render(strings.Join(sections, "\n"))
}

func renderModal(width, height int, modal string) string {
	if width == 0 || height == 0 {
		return modal
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
}

func formatChatBody(item chatEntry, width int) string {
	text := strings.ReplaceAll(item.Body, "\r\n", "\n")
	if item.Kind != "assistant" {
		return strings.TrimRight(wrapPlainText(text, width), "\n")
	}
	return strings.TrimRight(renderAssistantBody(text, width), "\n")
}

func wrapPlainText(text string, width int) string {
	if width <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	wrapped := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		for _, part := range wrapLineSmart(line, width) {
			wrapped = append(wrapped, strings.TrimRight(part, " "))
		}
	}
	return strings.Join(wrapped, "\n")
}

func wrapLineSmart(line string, width int) []string {
	if width <= 0 {
		return []string{line}
	}
	runes := []rune(line)
	if len(runes) == 0 {
		return []string{""}
	}

	out := make([]string, 0, 4)
	start := 0
	for start < len(runes) {
		curWidth := 0
		end := start
		lastSpaceEnd := -1

		for i := start; i < len(runes); i++ {
			rw := runewidth.RuneWidth(runes[i])
			if rw < 0 {
				rw = 0
			}
			if curWidth+rw > width {
				break
			}
			curWidth += rw
			end = i + 1
			if unicode.IsSpace(runes[i]) {
				lastSpaceEnd = i + 1
			}
		}

		if end == start {
			// Fallback for extra-wide single rune.
			end = start + 1
		} else if lastSpaceEnd > start && end < len(runes) {
			end = lastSpaceEnd
		}

		segment := strings.TrimRightFunc(string(runes[start:end]), unicode.IsSpace)
		if segment == "" {
			segment = string(runes[start:end])
		}
		out = append(out, segment)
		start = end
		for start < len(runes) && unicode.IsSpace(runes[start]) {
			start++
		}
	}

	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func tidyAssistantSpacing(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines)+4)
	inCodeBlock := false
	prevBlank := true

	for _, raw := range lines {
		line := strings.TrimRight(raw, " \t")
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if !prevBlank && len(out) > 0 {
				out = append(out, "")
			}
			out = append(out, line)
			inCodeBlock = !inCodeBlock
			prevBlank = false
			continue
		}

		if inCodeBlock {
			out = append(out, line)
			prevBlank = trimmed == ""
			continue
		}

		if trimmed == "" {
			if !prevBlank && len(out) > 0 {
				out = append(out, "")
			}
			prevBlank = true
			continue
		}

		if needsLeadingBlankLine(trimmed) && !prevBlank && len(out) > 0 {
			out = append(out, "")
		}

		out = append(out, line)
		prevBlank = false
	}

	return strings.Join(out, "\n")
}

func needsLeadingBlankLine(line string) bool {
	if strings.HasPrefix(line, "#") {
		return true
	}
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || strings.HasPrefix(line, "> ") {
		return true
	}
	if len(line) >= 3 && line[1] == '.' && line[2] == ' ' && line[0] >= '0' && line[0] <= '9' {
		return true
	}
	return false
}

func renderAssistantBody(text string, width int) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inCodeBlock := false
	prevBlank := true

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}

		plainLine := line
		if !inCodeBlock {
			plainLine = normalizeAssistantMarkdownLine(line)
		}
		if strings.TrimSpace(plainLine) == "" {
			if !prevBlank {
				out = append(out, "")
			}
			prevBlank = true
			continue
		}
		out = append(out, wrapPlainText(plainLine, width))
		prevBlank = false
	}

	return strings.Join(out, "\n")
}

var assistantInlineTokenReplacer = strings.NewReplacer(
	"**", "",
	"__", "",
	"~~", "",
	"`", "",
)

func normalizeAssistantMarkdownLine(line string) string {
	indentWidth := len(line) - len(strings.TrimLeft(line, " \t"))
	indent := line[:indentWidth]
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return ""
	}

	for strings.HasPrefix(trimmed, ">") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, ">"))
	}
	if trimmed == "" {
		return ""
	}

	if strings.HasPrefix(trimmed, "#") {
		level := 0
		for level < len(trimmed) && trimmed[level] == '#' {
			level++
		}
		if level > 0 && (level == len(trimmed) || trimmed[level] == ' ') {
			trimmed = strings.TrimSpace(trimmed[level:])
		}
	}

	if isMarkdownTableDivider(trimmed) {
		return ""
	}

	prefix := ""
	switch {
	case strings.HasPrefix(trimmed, "- [ ] "):
		prefix = "- [ ] "
		trimmed = strings.TrimSpace(trimmed[len("- [ ] "):])
	case strings.HasPrefix(strings.ToLower(trimmed), "- [x] "):
		prefix = "- [x] "
		trimmed = strings.TrimSpace(trimmed[len("- [x] "):])
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "), strings.HasPrefix(trimmed, "+ "):
		prefix = "- "
		trimmed = strings.TrimSpace(trimmed[2:])
	default:
		if marker, rest, ok := splitOrderedListItem(trimmed); ok {
			prefix = marker + " "
			trimmed = rest
		}
	}

	if looksLikeMarkdownTable(trimmed) {
		parts := make([]string, 0, 8)
		for _, cell := range strings.Split(trimmed, "|") {
			cell = strings.TrimSpace(cell)
			if cell == "" {
				continue
			}
			parts = append(parts, cell)
		}
		trimmed = strings.Join(parts, " | ")
	}

	trimmed = stripMarkdownLinks(trimmed)
	trimmed = assistantInlineTokenReplacer.Replace(trimmed)
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return ""
	}
	return indent + prefix + trimmed
}

func splitOrderedListItem(line string) (marker string, rest string, ok bool) {
	if len(line) < 3 {
		return "", "", false
	}
	index := 0
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	if index == 0 || len(line) <= index+1 || line[index] != '.' || line[index+1] != ' ' {
		return "", "", false
	}
	return line[:index+1], strings.TrimSpace(line[index+2:]), true
}

func isMarkdownTableDivider(line string) bool {
	compact := strings.ReplaceAll(strings.TrimSpace(line), " ", "")
	if compact == "" || strings.Count(compact, "|") < 1 {
		return false
	}
	for _, ch := range compact {
		switch ch {
		case '|', '-', ':':
		default:
			return false
		}
	}
	return true
}

func stripMarkdownLinks(line string) string {
	if line == "" {
		return line
	}

	var b strings.Builder
	b.Grow(len(line))
	for i := 0; i < len(line); {
		start := -1
		isImage := false
		switch {
		case i+1 < len(line) && line[i] == '!' && line[i+1] == '[':
			start = i + 2
			isImage = true
		case line[i] == '[':
			start = i + 1
		}

		if start < 0 {
			b.WriteByte(line[i])
			i++
			continue
		}

		mid := strings.Index(line[start:], "](")
		if mid < 0 {
			b.WriteByte(line[i])
			i++
			continue
		}
		textEnd := start + mid
		urlStart := textEnd + 2
		urlEndRel := strings.IndexByte(line[urlStart:], ')')
		if urlEndRel < 0 {
			b.WriteByte(line[i])
			i++
			continue
		}
		urlEnd := urlStart + urlEndRel
		label := strings.TrimSpace(line[start:textEnd])
		url := strings.TrimSpace(line[urlStart:urlEnd])
		if label != "" {
			b.WriteString(label)
		}
		if url != "" {
			if !isImage {
				b.WriteString(" (")
				b.WriteString(url)
				b.WriteString(")")
			}
		}
		i = urlEnd + 1
	}
	return b.String()
}

func isMarkdownHeading(line string) bool {
	return strings.HasPrefix(line, "# ") ||
		strings.HasPrefix(line, "## ") ||
		strings.HasPrefix(line, "### ")
}

func renderMarkdownHeading(line string, width int) string {
	level := 0
	for level < len(line) && line[level] == '#' {
		level++
	}
	text := strings.TrimSpace(line[level:])
	style := assistantHeading3Style
	switch level {
	case 1:
		style = assistantHeading1Style
	case 2:
		style = assistantHeading2Style
	}
	wrapped := strings.Split(wrapPlainText(text, width), "\n")
	rendered := make([]string, 0, len(wrapped))
	for _, part := range wrapped {
		rendered = append(rendered, style.Render(part))
	}
	return strings.Join(rendered, "\n")
}

func isMarkdownListItem(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return true
	}
	return isOrderedListItem(line)
}

func isOrderedListItem(line string) bool {
	if len(line) < 3 {
		return false
	}
	index := 0
	for index < len(line) && line[index] >= '0' && line[index] <= '9' {
		index++
	}
	return index > 0 && len(line) > index+1 && line[index] == '.' && line[index+1] == ' '
}

func renderMarkdownListItem(line string, width int) string {
	indentWidth := len(line) - len(strings.TrimLeft(line, " "))
	indent := strings.Repeat(" ", indentWidth)
	trimmed := strings.TrimSpace(line)
	marker := ""
	content := ""

	switch {
	case strings.HasPrefix(trimmed, "- "), strings.HasPrefix(trimmed, "* "):
		marker = trimmed[:1]
		content = strings.TrimSpace(trimmed[2:])
	default:
		for i := 0; i < len(trimmed); i++ {
			if trimmed[i] == '.' && i+1 < len(trimmed) && trimmed[i+1] == ' ' {
				marker = trimmed[:i+1]
				content = strings.TrimSpace(trimmed[i+2:])
				break
			}
		}
	}

	if content == "" {
		content = trimmed
	}

	prefix := indent + marker + " "
	contentWidth := max(8, width-runewidth.StringWidth(prefix))
	wrapped := strings.Split(wrapPlainText(content, contentWidth), "\n")
	lines := make([]string, 0, len(wrapped))
	for i, part := range wrapped {
		if i == 0 {
			lines = append(lines, indent+listMarkerStyle.Render(marker)+" "+part)
			continue
		}
		lines = append(lines, indent+strings.Repeat(" ", runewidth.StringWidth(marker))+" "+part)
	}
	return strings.Join(lines, "\n")
}

func renderMarkdownQuote(line string, width int) string {
	content := strings.TrimSpace(strings.TrimPrefix(line, ">"))
	wrapped := strings.Split(wrapPlainText(content, max(8, width-2)), "\n")
	rendered := make([]string, 0, len(wrapped))
	for _, part := range wrapped {
		rendered = append(rendered, quoteLineStyle.Render(part))
	}
	return strings.Join(rendered, "\n")
}

func looksLikeMarkdownTable(line string) bool {
	return strings.Count(line, "|") >= 2
}

func chatBubbleWidth(item chatEntry, width int) int {
	if width <= 28 {
		return width
	}
	return width
}

func summarizeTool(name, payload string) (string, []string, string) {
	var envelope struct {
		OK    *bool  `json:"ok"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(payload), &envelope); err == nil && envelope.Error != "" {
		return envelope.Error, nil, "error"
	}

	switch name {
	case "list_files":
		var result struct {
			Root  string `json:"root"`
			Items []struct {
				Path string `json:"path"`
				Type string `json:"type"`
			} `json:"items"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, min(3, len(result.Items)))
			for i := 0; i < min(3, len(result.Items)); i++ {
				lines = append(lines, result.Items[i].Type+" "+result.Items[i].Path)
			}
			return fmt.Sprintf("Listed %d items under %s", len(result.Items), emptyDot(result.Root)), lines, "done"
		}
	case "read_file":
		var result struct {
			Path      string `json:"path"`
			StartLine int    `json:"start_line"`
			EndLine   int    `json:"end_line"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			return fmt.Sprintf("Read %s lines %d-%d", result.Path, result.StartLine, result.EndLine), nil, "done"
		}
	case "search_text":
		var result struct {
			Query   string `json:"query"`
			Matches []struct {
				Path string `json:"path"`
				Line int    `json:"line"`
				Text string `json:"text"`
			} `json:"matches"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, min(3, len(result.Matches)))
			for i := 0; i < min(3, len(result.Matches)); i++ {
				match := result.Matches[i]
				lines = append(lines, fmt.Sprintf("%s:%d %s", match.Path, match.Line, compact(match.Text, 72)))
			}
			return fmt.Sprintf("Found %d match(es) for %q", len(result.Matches), result.Query), lines, "done"
		}
	case "web_search":
		var result struct {
			Query   string `json:"query"`
			Results []struct {
				Title string `json:"title"`
				URL   string `json:"url"`
			} `json:"results"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, min(3, len(result.Results)))
			for i := 0; i < min(3, len(result.Results)); i++ {
				item := result.Results[i]
				title := compact(item.Title, 56)
				if strings.TrimSpace(title) == "" {
					title = item.URL
				}
				lines = append(lines, title+" - "+item.URL)
			}
			return fmt.Sprintf("Searched web for %q (%d result(s))", result.Query, len(result.Results)), lines, "done"
		}
	case "web_fetch":
		var result struct {
			URL        string `json:"url"`
			StatusCode int    `json:"status_code"`
			Title      string `json:"title"`
			Content    string `json:"content"`
			Truncated  bool   `json:"truncated"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, 2)
			if strings.TrimSpace(result.Title) != "" {
				lines = append(lines, "title: "+compact(result.Title, 72))
			}
			if strings.TrimSpace(result.Content) != "" {
				lines = append(lines, "preview: "+compact(result.Content, 72))
			}
			if result.Truncated {
				lines = append(lines, "content truncated")
			}
			return fmt.Sprintf("Fetched %s (HTTP %d)", result.URL, result.StatusCode), lines, "done"
		}
	case "write_file":
		var result struct {
			Path         string `json:"path"`
			BytesWritten int    `json:"bytes_written"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			return fmt.Sprintf("Wrote %s (%d bytes)", result.Path, result.BytesWritten), nil, "done"
		}
	case "replace_in_file":
		var result struct {
			Path     string `json:"path"`
			Replaced int    `json:"replaced"`
			OldCount int    `json:"old_count"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			return fmt.Sprintf("Updated %s (%d/%d)", result.Path, result.Replaced, result.OldCount), nil, "done"
		}
	case "apply_patch":
		var result struct {
			Operations []struct {
				Type string `json:"type"`
				Path string `json:"path"`
			} `json:"operations"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, min(4, len(result.Operations)))
			for i := 0; i < min(4, len(result.Operations)); i++ {
				lines = append(lines, result.Operations[i].Type+" "+result.Operations[i].Path)
			}
			return fmt.Sprintf("Patched %d file operation(s)", len(result.Operations)), lines, "done"
		}
	case "update_plan":
		var result struct {
			Plan planpkg.State `json:"plan"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, min(4, len(result.Plan.Steps)))
			for i := 0; i < min(4, len(result.Plan.Steps)); i++ {
				step := result.Plan.Steps[i]
				lines = append(lines, fmt.Sprintf("[%s] %s", step.Status, step.Title))
			}
			return fmt.Sprintf("Updated plan with %d step(s)", len(result.Plan.Steps)), lines, "done"
		}
	case "run_shell":
		var result struct {
			OK       bool   `json:"ok"`
			ExitCode int    `json:"exit_code"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, 2)
			if text := strings.TrimSpace(result.Stdout); text != "" {
				lines = append(lines, "stdout: "+compact(strings.Split(text, "\n")[0], 72))
			}
			if text := strings.TrimSpace(result.Stderr); text != "" {
				lines = append(lines, "stderr: "+compact(strings.Split(text, "\n")[0], 72))
			}
			status := "done"
			if !result.OK {
				status = "warn"
			}
			return fmt.Sprintf("Shell exited with code %d", result.ExitCode), lines, status
		}
	}

	return compact(payload, 96), nil, "done"
}

func joinSummary(summary string, lines []string) string {
	if len(lines) == 0 {
		return summary
	}
	return summary + "\n" + strings.Join(lines, "\n")
}

func summarizeArgs(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || raw == "{}" {
		return "default arguments"
	}
	return compact(raw, 88)
}

func legacyAssistantToolIntro(toolName string) string {
	if strings.TrimSpace(toolName) == "" {
		return "I will check the relevant context first."
	}
	return fmt.Sprintf("I will call `%s` to inspect the relevant context first.", toolName)
}

func legacyAssistantToolFollowUp(toolName, summary, status string) string {
	if strings.TrimSpace(summary) == "" {
		return "I have the tool result. Let me organize the next step."
	}
	switch status {
	case "error", "warn":
		return fmt.Sprintf("`%s` returned a result. I will continue from that signal.", toolName)
	default:
		return fmt.Sprintf("`%s` finished successfully. I will keep using the result.", toolName)
	}
}

func assistantToolIntro(toolName string) string {
	if strings.TrimSpace(toolName) == "" {
		return "I will check the relevant context first."
	}
	return fmt.Sprintf("I will call `%s` to inspect the relevant context first.", toolName)
}

func assistantToolFollowUp(toolName, summary, status string) string {
	if strings.TrimSpace(summary) == "" {
		return "I have the tool result. Let me organize the next step."
	}
	switch status {
	case "error", "warn":
		return fmt.Sprintf("`%s` returned a result. I will continue from that signal.", toolName)
	default:
		return fmt.Sprintf("`%s` finished successfully. I will keep using the result.", toolName)
	}
}

func isMeaningfulThinking(body, toolName string) bool {
	raw := strings.TrimSpace(body)
	if raw == "" {
		return false
	}
	normalized := strings.ToLower(strings.ReplaceAll(raw, "`", ""))
	toolName = strings.ToLower(strings.TrimSpace(toolName))

	genericPrefixes := []string{
		"i will call ",
		"i'll call ",
		"let me call ",
		"i am going to call ",
		"i'm going to call ",
		"i will use ",
		"i'll use ",
		"let me use ",
		"i will run ",
		"let me run ",
		"i will check the relevant context first",
		"i have the tool result. let me organize the next step.",
	}
	for _, prefix := range genericPrefixes {
		if strings.HasPrefix(normalized, prefix) {
			return false
		}
	}

	if toolName != "" {
		toolIntentPhrases := []string{
			fmt.Sprintf("call %s", toolName),
			fmt.Sprintf("use %s", toolName),
			fmt.Sprintf("run %s", toolName),
		}
		for _, phrase := range toolIntentPhrases {
			if strings.Contains(normalized, phrase) && strings.Contains(normalized, "inspect") {
				return false
			}
		}
	}

	cnPrefixes := []string{
		"i will call",
		"i will use",
		"let me call",
		"let me use",
		"let me run",
		"tool result",
	}
	for _, prefix := range cnPrefixes {
		if strings.HasPrefix(raw, prefix) {
			return false
		}
	}

	return true
}

func shouldRenderThinkingFromDelta(body string) bool {
	text := strings.TrimSpace(body)
	if text == "" {
		return false
	}
	if !isMeaningfulThinking(text, "") {
		return false
	}
	lower := strings.ToLower(text)
	reasoningMarkers := []string{
		"i will first",
		"first,",
		"then",
		"finally",
		"approach",
		"systematically",
		"through build and test",
		"\u6211\u4f1a\u5148",
		"\u5148\u4e86\u89e3",
		"\u7136\u540e",
		"\u6700\u540e",
		"\u901a\u8fc7\u6784\u5efa\u548c\u6d4b\u8bd5",
		"\u7cfb\u7edf\u6027",
	}
	for _, marker := range reasoningMarkers {
		if strings.Contains(lower, marker) || strings.Contains(text, marker) {
			return true
		}
	}
	return false
}

func (m model) thinkingText() string {
	return fmt.Sprintf("%s Thinking... request already sent to the LLM, waiting for response.", m.spinner.View())
}

func (m *model) syncCommandPalette() {
	value := strings.TrimSpace(m.input.Value())
	if !strings.HasPrefix(value, "/") {
		m.commandOpen = false
		m.commandCursor = 0
		return
	}
	m.commandOpen = true
	m.closeMentionPalette()
	items := m.filteredCommands()
	if len(items) == 0 {
		m.commandCursor = 0
		return
	}
	if m.commandCursor < 0 || m.commandCursor >= len(items) {
		m.commandCursor = 0
	}
}

func (m *model) syncInputOverlays() {
	if m.startupGuide.Active || m.promptSearchOpen {
		return
	}
	m.syncCommandPalette()
	m.syncMentionPalette()
	m.syncInputImageRefs(m.input.Value())
}

func (m model) handlePromptSearchKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch {
	case isPageUpKey(msg):
		m.stepPromptSearch(-promptSearchPageSize)
		return m, nil
	case isPageDownKey(msg):
		m.stepPromptSearch(promptSearchPageSize)
		return m, nil
	}

	switch msg.String() {
	case "esc":
		m.closePromptSearch(true)
		m.statusNote = "Prompt search canceled."
		return m, nil
	case "enter":
		selected, ok := m.selectedPromptSearchEntry()
		if ok {
			m.setInputValue(selected.Prompt)
			m.closePromptSearch(false)
			m.statusNote = "Prompt restored from history."
			return m, nil
		}
		m.closePromptSearch(true)
		m.statusNote = "No prompt selected."
		return m, nil
	case "ctrl+f", "down", "j":
		m.stepPromptSearch(1)
		return m, nil
	case "ctrl+s", "up", "k":
		m.stepPromptSearch(-1)
		return m, nil
	case "home":
		m.stepPromptSearch(-len(m.promptSearchMatches))
		return m, nil
	case "end":
		m.stepPromptSearch(len(m.promptSearchMatches))
		return m, nil
	case "backspace", "ctrl+h":
		m.trimPromptSearchQuery()
		return m, nil
	}

	switch msg.Type {
	case tea.KeyBackspace:
		m.trimPromptSearchQuery()
		return m, nil
	case tea.KeySpace:
		m.promptSearchQuery += " "
		m.refreshPromptSearchMatches()
		return m, nil
	case tea.KeyRunes:
		m.promptSearchQuery += string(msg.Runes)
		m.refreshPromptSearchMatches()
		return m, nil
	default:
		return m, nil
	}
}

func (m *model) syncMentionPalette() {
	if m.commandOpen {
		m.closeMentionPalette()
		return
	}
	token, ok := mention.FindActiveToken(m.input.Value())
	if !ok {
		m.closeMentionPalette()
		return
	}

	if m.mentionIndex == nil {
		m.mentionIndex = mention.NewWorkspaceFileIndex(m.workspace)
	}
	results := m.mentionIndex.SearchWithRecency(token.Query, mentionPageSize*3, m.mentionRecent)
	m.mentionOpen = true
	m.mentionQuery = token.Query
	m.mentionToken = token
	m.mentionResults = results

	if len(results) == 0 {
		m.mentionCursor = 0
		return
	}
	if m.mentionCursor < 0 || m.mentionCursor >= len(results) {
		m.mentionCursor = 0
	}
}

func (m *model) closeMentionPalette() {
	m.mentionOpen = false
	m.mentionCursor = 0
	m.mentionQuery = ""
	m.mentionToken = mention.Token{}
	m.mentionResults = nil
}

func (m *model) recordRecentMention(path string) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	if m.mentionRecent == nil {
		m.mentionRecent = make(map[string]int, 16)
	}
	m.mentionSeq++
	m.mentionRecent[path] = m.mentionSeq
}

func (m model) hasRecentMention(path string) bool {
	if m.mentionRecent == nil {
		return false
	}
	return m.mentionRecent[path] > 0
}

func (m model) filteredCommands() []commandItem {
	value := strings.TrimSpace(m.input.Value())
	query := commandFilterQuery(value, "")
	items := m.commandPaletteItems()
	if query == "" {
		return items
	}

	result := make([]commandItem, 0, len(items))
	for _, item := range items {
		if matchesCommandItem(item, query) {
			result = append(result, item)
		}
	}
	return result
}

func (m model) commandPaletteItems() []commandItem {
	base := visibleCommandItems("")
	skillItems := m.skillCommandItems()
	if len(skillItems) == 0 {
		return base
	}

	items := make([]commandItem, 0, len(base)+len(skillItems))
	seen := make(map[string]struct{}, len(base)+len(skillItems))
	items = append(items, base...)
	for _, item := range base {
		seen[strings.ToLower(strings.TrimSpace(item.Usage))] = struct{}{}
	}
	for _, item := range skillItems {
		key := strings.ToLower(strings.TrimSpace(item.Usage))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		items = append(items, item)
	}
	return items
}

func (m model) skillCommandItems() []commandItem {
	if m.runner == nil {
		return nil
	}
	skillsList, _ := m.runner.ListSkills()
	if len(skillsList) == 0 {
		return nil
	}

	items := make([]commandItem, 0, len(skillsList))
	seen := make(map[string]struct{}, len(skillsList))
	for _, skill := range skillsList {
		name := strings.TrimSpace(skill.Entry.Slash)
		if name == "" {
			name = "/" + strings.TrimSpace(skill.Name)
		}
		if name == "" {
			continue
		}
		if !strings.HasPrefix(name, "/") {
			name = "/" + name
		}
		name = "/" + strings.TrimLeft(strings.TrimSpace(name), "/")
		key := strings.ToLower(name)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}

		description := strings.TrimSpace(skill.Description)
		if description == "" {
			description = fmt.Sprintf("Activate %s for this session.", skill.Name)
		}
		items = append(items, commandItem{
			Name:        name,
			Usage:       name,
			Description: description,
			Kind:        "skill",
		})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Usage < items[j].Usage
	})
	return items
}

func (m model) commandPaletteWidth() int {
	switch m.screen {
	case screenLanding:
		return max(28, m.landingInputShellWidth())
	default:
		return max(32, m.chatPanelInnerWidth())
	}
}

func (m model) visibleCommandItemsPage() []commandItem {
	items := m.filteredCommands()
	if len(items) == 0 {
		return nil
	}
	cursor := clamp(m.commandCursor, 0, len(items)-1)
	start := (cursor / commandPageSize) * commandPageSize
	end := min(len(items), start+commandPageSize)
	return items[start:end]
}

func (m model) selectedMentionCandidate() (mention.Candidate, bool) {
	if len(m.mentionResults) == 0 {
		return mention.Candidate{}, false
	}
	index := clamp(m.mentionCursor, 0, len(m.mentionResults)-1)
	return m.mentionResults[index], true
}

func (m model) visibleMentionItemsPage() []mention.Candidate {
	if len(m.mentionResults) == 0 {
		return nil
	}
	cursor := clamp(m.mentionCursor, 0, len(m.mentionResults)-1)
	start := (cursor / mentionPageSize) * mentionPageSize
	end := min(len(m.mentionResults), start+mentionPageSize)
	return m.mentionResults[start:end]
}

func (m model) selectedPromptSearchEntry() (history.PromptEntry, bool) {
	if len(m.promptSearchMatches) == 0 {
		return history.PromptEntry{}, false
	}
	index := clamp(m.promptSearchCursor, 0, len(m.promptSearchMatches)-1)
	return m.promptSearchMatches[index], true
}

func (m model) visiblePromptSearchEntriesPage() []history.PromptEntry {
	if len(m.promptSearchMatches) == 0 {
		return nil
	}
	cursor := clamp(m.promptSearchCursor, 0, len(m.promptSearchMatches)-1)
	start := (cursor / promptSearchPageSize) * promptSearchPageSize
	end := min(len(m.promptSearchMatches), start+promptSearchPageSize)
	return m.promptSearchMatches[start:end]
}

func (m *model) setInputValue(value string) {
	m.input.SetValue(value)
	m.input.CursorEnd()
}

func shouldExecuteFromPalette(item commandItem) bool {
	if item.Kind == "skill" {
		return true
	}
	switch item.Name {
	case "/help", "/session", "/skills", "/skill clear", "/new", "/compact", "/quit":
		return true
	default:
		return false
	}
}

func (m model) helpText() string {
	return strings.Join([]string{
		"Entry points",
		"Run `go run ./cmd/bytemind chat` from the repository root to open the TUI.",
		"The chat command opens the landing screen first, then enters the conversation view after you submit a prompt.",
		"Run `go run ./cmd/bytemind run -prompt \"...\"` for one-shot execution.",
		"",
		"Slash commands",
		"/help: show this help inside the conversation.",
		"/session: open recent sessions.",
		"/skills: list discovered skills and diagnostics.",
		"/<skill-name> [k=v...]: activate a skill for this session.",
		"/skill clear: clear the active skill in this session.",
		"/skill delete <name>: delete the specified project skill.",
		"/new: start a fresh session.",
		"/compact: summarize long history into a compact continuation context.",
		"/btw <message>: interject while a run is in progress.",
		"/quit: exit the TUI.",
		"",
		"UI notes",
		"Tab toggles between Build and Plan modes.",
		"Plan mode keeps the plan panel visible and focused on structured steps.",
		"Use Ctrl+G to open or close the help panel.",
		"Use Ctrl+F to search prompt history and restore previous input.",
		"Drag across the conversation with the left mouse button to select text, then press Ctrl+C to copy it.",
		"If provider setup is required, paste an API key in the input and press Enter.",
		"Long pasted code/text is compressed to [Paste #N ~X lines].",
		"Use [Paste], [Paste #N], [Paste line3], or [Paste #N line3~line7] to expand references.",
		"After restoring a session with a saved plan, type 'continue execution' to resume it.",
		"Approval requests appear above the input area when a shell command needs confirmation.",
		"The footer keeps only the essential shortcuts: tab agents, / commands, drag select, Ctrl+C copy/quit, Ctrl+F history, Ctrl+L sessions.",
	}, "\n")
}
func visibleCommandItems(group string) []commandItem {
	items := make([]commandItem, 0, len(commandItems))
	for _, item := range commandItems {
		if group == "" {
			if item.Kind == "group" || item.Group == "" {
				items = append(items, item)
			}
			continue
		}
		if item.Kind == "command" && item.Group == group {
			items = append(items, item)
		}
	}
	return items
}

func (m model) isKnownSkillCommand(command string) bool {
	if m.runner == nil {
		return false
	}
	normalized := normalizeSkillCommand(command)
	if normalized == "" {
		return false
	}
	skillsList, _ := m.runner.ListSkills()
	for _, skill := range skillsList {
		if normalizeSkillCommand(skill.Name) == normalized {
			return true
		}
		if normalizeSkillCommand(skill.Entry.Slash) == normalized {
			return true
		}
		for _, alias := range skill.Aliases {
			if normalizeSkillCommand(alias) == normalized {
				return true
			}
		}
	}
	return false
}

func normalizeSkillCommand(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.TrimLeft(name, "/")
	return strings.TrimSpace(name)
}

func commandFilterQuery(value, group string) string {
	value = strings.TrimSpace(value)
	if value == "" || value == "/" {
		return ""
	}
	value = strings.TrimPrefix(value, "/")
	if group != "" {
		if strings.HasPrefix(value, group) {
			value = strings.TrimSpace(strings.TrimPrefix(value, group))
		}
	}
	return strings.ToLower(strings.TrimSpace(strings.TrimPrefix(value, "/")))
}

func matchesCommandItem(item commandItem, query string) bool {
	if query == "" {
		return true
	}
	query = strings.ToLower(query)
	name := strings.ToLower(strings.TrimPrefix(item.Name, "/"))
	usage := strings.ToLower(strings.TrimPrefix(item.Usage, "/"))
	return strings.HasPrefix(name, query) ||
		strings.HasPrefix(usage, query)
}

func matchAllTokens(text string, tokens []string) bool {
	if len(tokens) == 0 {
		return true
	}
	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		if !strings.Contains(text, token) {
			return false
		}
	}
	return true
}

func parsePromptSearchQuery(raw string) (tokens []string, workspaceFilter, sessionFilter string) {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(raw)))
	tokens = make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		switch {
		case strings.HasPrefix(field, "ws:"):
			workspaceFilter = strings.TrimSpace(strings.TrimPrefix(field, "ws:"))
		case strings.HasPrefix(field, "workspace:"):
			workspaceFilter = strings.TrimSpace(strings.TrimPrefix(field, "workspace:"))
		case strings.HasPrefix(field, "sid:"):
			sessionFilter = strings.TrimSpace(strings.TrimPrefix(field, "sid:"))
		case strings.HasPrefix(field, "session:"):
			sessionFilter = strings.TrimSpace(strings.TrimPrefix(field, "session:"))
		default:
			tokens = append(tokens, field)
		}
	}
	return tokens, workspaceFilter, sessionFilter
}

func (m model) chatPanelWidth() int {
	return max(20, m.width)
}

func (m model) chatPanelInnerWidth() int {
	width := m.chatPanelWidth() - panelStyle.GetHorizontalFrameSize()
	return max(12, width)
}

func (m model) chatInputContentWidth() int {
	width := m.chatPanelInnerWidth() - m.inputBorderStyle().GetHorizontalFrameSize()
	return max(18, width)
}

func (m model) landingInputShellWidth() int {
	return min(72, max(36, m.width/2))
}

func (m model) landingInputContentWidth() int {
	width := m.landingInputShellWidth() - landingInputStyle.GetHorizontalFrameSize()
	return max(24, width)
}

func (m model) inputBorderStyle() lipgloss.Style {
	return inputStyle.BorderForeground(m.modeAccentColor())
}

func (m model) modeAccentColor() lipgloss.Color {
	if m.mode == modePlan {
		return colorThinking
	}
	return colorAccent
}

func (m *model) syncInputStyle() {
	if m.startupGuide.Active {
		m.input.Placeholder = startupGuideInputPlaceholder(m.startupGuide.CurrentField)
	} else {
		m.input.Placeholder = "Ask Bytemind to inspect, change, or verify this workspace..."
	}
	m.input.Prompt = ""
	setInputHeightSafe(&m.input, 2)
}

func setInputHeightSafe(input *textarea.Model, height int) {
	if input == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	input.SetHeight(height)
}

func startupGuideInputHint(field string) string {
	switch strings.TrimSpace(field) {
	case startupFieldType:
		return "Enter provider and press Enter."
	case startupFieldBaseURL:
		return "Enter base_url and press Enter."
	case startupFieldModel:
		return "Enter model and press Enter."
	case startupFieldAPIKey:
		return "Paste API key and press Enter to verify."
	default:
		return "Input value then press Enter."
	}
}

func startupGuideInputPlaceholder(field string) string {
	switch strings.TrimSpace(field) {
	case startupFieldType:
		return "Step 1/4: provider (openai-compatible or anthropic)"
	case startupFieldBaseURL:
		return "Step 2/4: base_url (example: https://api.deepseek.com)"
	case startupFieldModel:
		return "Step 3/4: model (example: deepseek-chat)"
	case startupFieldAPIKey:
		return "Step 4/4: paste API key and press Enter..."
	default:
		return "Complete setup and press Enter..."
	}
}

func resolveSessionID(summaries []session.Summary, prefix string) (string, error) {
	matches := make([]string, 0, 4)
	for _, summary := range summaries {
		if summary.ID == prefix {
			return summary.ID, nil
		}
		if strings.HasPrefix(summary.ID, prefix) {
			matches = append(matches, summary.ID)
		}
	}
	switch len(matches) {
	case 0:
		return "", fmt.Errorf("session not found: %s", prefix)
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("session prefix %q matched multiple sessions", prefix)
	}
}

func sameWorkspace(a, b string) bool {
	left, err := filepath.Abs(a)
	if err != nil {
		left = a
	}
	right, err := filepath.Abs(b)
	if err != nil {
		right = b
	}
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

func parsePlanSteps(raw string) []string {
	parts := strings.Split(raw, "|")
	steps := make([]string, 0, len(parts))
	for _, part := range parts {
		step := strings.TrimSpace(part)
		if step != "" {
			steps = append(steps, step)
		}
	}
	return steps
}

func canContinuePlan(state planpkg.State) bool {
	state = planpkg.NormalizeState(state)
	if !planpkg.HasStructuredPlan(state) {
		return false
	}
	switch planpkg.NormalizePhase(string(state.Phase)) {
	case planpkg.PhaseBlocked, planpkg.PhaseCompleted:
		return false
	default:
		return true
	}
}

func currentOrNextStepTitle(state planpkg.State) string {
	state = planpkg.NormalizeState(state)
	if step, ok := planpkg.CurrentStep(state); ok && strings.TrimSpace(step.Title) != "" {
		return strings.TrimSpace(step.Title)
	}
	for _, step := range state.Steps {
		if planpkg.NormalizeStepStatus(string(step.Status)) == planpkg.StepPending && strings.TrimSpace(step.Title) != "" {
			return strings.TrimSpace(step.Title)
		}
	}
	return ""
}

func isBTWCommand(input string) bool {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 {
		return false
	}
	return fields[0] == "/btw"
}

func extractBTWText(input string) (string, error) {
	fields := strings.Fields(strings.TrimSpace(input))
	if len(fields) == 0 || fields[0] != "/btw" {
		return "", errors.New("usage: /btw <message>")
	}
	text := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), fields[0]))
	if text == "" {
		return "", errors.New("usage: /btw <message>")
	}
	return text, nil
}

func composeBTWPrompt(entries []string) string {
	cleaned := make([]string, 0, len(entries))
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed != "" {
			cleaned = append(cleaned, trimmed)
		}
	}
	if len(cleaned) == 0 {
		return ""
	}
	if len(cleaned) == 1 {
		return strings.Join([]string{
			"User sent a BTW update while you were executing an existing task.",
			"Continue the same task from the latest progress, and apply this update with high priority unless it explicitly changes the goal:",
			cleaned[0],
		}, "\n")
	}
	lines := make([]string, 0, len(cleaned)+2)
	lines = append(lines, "User sent multiple BTW updates during execution. Later items have higher priority:")
	for i, entry := range cleaned {
		lines = append(lines, fmt.Sprintf("%d. %s", i+1, entry))
	}
	lines = append(lines, "Please continue the same task with these updates and keep unfinished steps unless explicitly changed.")
	return strings.Join(lines, "\n")
}

func formatBTWUpdateScope(count int) string {
	if count <= 1 {
		return "your latest update"
	}
	return fmt.Sprintf("%d updates", count)
}

func queueBTWUpdate(queue []string, value string) ([]string, int) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return queue, 0
	}
	queue = append(queue, trimmed)
	if len(queue) <= maxPendingBTW {
		return queue, 0
	}
	dropped := len(queue) - maxPendingBTW
	return append([]string(nil), queue[dropped:]...), dropped
}

func classifyRunFinish(err error, restartedByBTW bool) runFinishReason {
	if restartedByBTW {
		return runFinishReasonBTWRestart
	}
	if err == nil {
		return runFinishReasonCompleted
	}
	if errors.Is(err, context.Canceled) {
		return runFinishReasonCanceled
	}
	return runFinishReasonFailed
}

func isContinueExecutionInput(input string) bool {
	normalized := strings.ToLower(strings.TrimSpace(input))
	switch normalized {
	case "continue",
		"continue execution",
		"continue plan",
		"resume",
		"resume execution",
		"\u7ee7\u7eed",
		"\u7ee7\u7eed\u6267\u884c",
		"\u7ee7\u7eed\u505a",
		"\u7ee7\u7eed\u4efb\u52a1":
		return true
	default:
		return false
	}
}

func (m model) currentPhaseLabel() string {
	if phase := m.planPhaseLabel(); phase != "none" {
		return phase
	}
	if strings.TrimSpace(m.phase) != "" {
		return strings.TrimSpace(m.phase)
	}
	return "idle"
}

func (m model) currentSessionLabel() string {
	if m.sess == nil {
		return "none"
	}
	return shortID(m.sess.ID)
}

func (m model) autoFollowLabel() string {
	if m.chatAutoFollow {
		return "auto"
	}
	return "manual"
}

func (m model) currentModelLabel() string {
	if model := strings.TrimSpace(m.cfg.Provider.Model); model != "" {
		return model
	}
	return "-"
}

func (m model) currentSkillLabel() string {
	if m.sess == nil || m.sess.ActiveSkill == nil {
		return "none"
	}
	name := strings.TrimSpace(m.sess.ActiveSkill.Name)
	if name == "" {
		return "none"
	}
	return name
}

func preparePlanForContinuation(state planpkg.State) (planpkg.State, error) {
	state = planpkg.NormalizeState(state)
	if !planpkg.HasStructuredPlan(state) {
		return state, fmt.Errorf("no structured plan is available to continue")
	}
	switch planpkg.NormalizePhase(string(state.Phase)) {
	case planpkg.PhaseBlocked:
		if state.BlockReason != "" {
			return state, fmt.Errorf("plan is blocked: %s", state.BlockReason)
		}
		return state, fmt.Errorf("plan is blocked and cannot continue yet")
	case planpkg.PhaseCompleted:
		return state, fmt.Errorf("plan is already completed")
	}

	if _, ok := planpkg.CurrentStep(state); !ok {
		for i := range state.Steps {
			if planpkg.NormalizeStepStatus(string(state.Steps[i].Status)) == planpkg.StepPending {
				state.Steps[i].Status = planpkg.StepInProgress
				break
			}
		}
	}

	state.Phase = planpkg.PhaseExecuting
	if strings.TrimSpace(state.NextAction) == "" {
		state.NextAction = planpkg.DefaultNextAction(state)
	}
	return planpkg.NormalizeState(state), nil
}

func copyPlanState(state planpkg.State) planpkg.State {
	return planpkg.CloneState(state)
}

func toAgentMode(mode planpkg.AgentMode) agentMode {
	if planpkg.NormalizeMode(string(mode)) == planpkg.ModePlan {
		return modePlan
	}
	return modeBuild
}

func (m model) hasPlanPanel() bool {
	return false
}

func (m model) showPlanSidebar() bool {
	return m.hasPlanPanel() && m.chatPanelInnerWidth() >= 104
}

func (m model) planPanelWidth() int {
	if !m.showPlanSidebar() {
		return m.chatPanelInnerWidth()
	}
	return clamp(m.chatPanelInnerWidth()/3, 30, 42)
}

func (m model) conversationPanelWidth() int {
	width := m.chatPanelInnerWidth()
	if m.showPlanSidebar() {
		width -= m.planPanelWidth() + 1
	}
	return max(24, width)
}

func (m model) planModeLabel() string {
	if m.mode == modePlan {
		return "PLAN"
	}
	return "BUILD"
}

func (m model) planPhaseLabel() string {
	phase := planpkg.NormalizePhase(string(m.plan.Phase))
	if phase == planpkg.PhaseNone && m.mode == modePlan {
		phase = planpkg.PhaseDrafting
	}
	if phase == planpkg.PhaseNone {
		return "none"
	}
	return string(phase)
}

func (m model) renderPlanPanel(width int) string {
	width = max(24, width)
	return modalBoxStyle.Width(width).Render(m.planView.View())
}

func (m model) planPanelContent(width int) string {
	width = max(16, width)
	lines := []string{
		accentStyle.Render(m.planModeLabel()),
		mutedStyle.Render("Phase: " + m.planPhaseLabel()),
	}

	if goal := strings.TrimSpace(m.plan.Goal); goal != "" {
		lines = append(lines, "", cardTitleStyle.Render("Goal"), wrapPlainText(goal, width))
	}
	if summary := strings.TrimSpace(m.plan.Summary); summary != "" {
		lines = append(lines, "", cardTitleStyle.Render("Summary"), wrapPlainText(summary, width))
	}

	lines = append(lines, "", cardTitleStyle.Render("Steps"))
	if len(m.plan.Steps) == 0 {
		lines = append(lines, mutedStyle.Render("No structured plan yet. Use update_plan to create one."))
	} else {
		for _, step := range m.plan.Steps {
			lines = append(lines, m.renderPlanStep(step, width), "")
		}
		if len(lines) > 0 && lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}

	if nextAction := strings.TrimSpace(m.plan.NextAction); nextAction != "" {
		lines = append(lines, "", cardTitleStyle.Render("Next Action"), wrapPlainText(nextAction, width))
	}
	if reason := strings.TrimSpace(m.plan.BlockReason); reason != "" {
		lines = append(lines, "", cardTitleStyle.Render("Blocked Reason"), errorStyle.Render(wrapPlainText(reason, width)))
	}

	return strings.Join(lines, "\n")
}

func (m model) planPanelRenderHeight() int {
	if !m.hasPlanPanel() {
		return 0
	}
	return m.planView.Height + modalBoxStyle.GetVerticalFrameSize()
}

func (m model) renderPlanStep(step planpkg.Step, width int) string {
	header := fmt.Sprintf("%s %s", statusGlyph(string(step.Status)), step.Title)
	parts := []string{wrapPlainText(header, width)}
	if desc := strings.TrimSpace(step.Description); desc != "" {
		parts = append(parts, mutedStyle.Render(wrapPlainText(desc, width)))
	}
	if len(step.Files) > 0 {
		parts = append(parts, mutedStyle.Render("Files: "+compact(strings.Join(step.Files, ", "), width)))
	}
	if len(step.Verify) > 0 {
		parts = append(parts, mutedStyle.Render("Verify: "+compact(strings.Join(step.Verify, " | "), width)))
	}
	if risk := strings.TrimSpace(string(step.Risk)); risk != "" {
		parts = append(parts, mutedStyle.Render("Risk: "+risk))
	}
	return strings.Join(parts, "\n")
}
func (m model) sessionText() string {
	if m.sess == nil {
		return "No active session."
	}
	return strings.Join([]string{
		fmt.Sprintf("Session ID: %s", m.sess.ID),
		fmt.Sprintf("Workspace: %s", m.sess.Workspace),
		fmt.Sprintf("Updated: %s", m.sess.UpdatedAt.Local().Format("2006-01-02 15:04:05")),
		fmt.Sprintf("Messages: %d", len(m.sess.Messages)),
	}, "\n")
}

func statusGlyph(status string) string {
	switch planpkg.NormalizeStepStatus(status) {
	case planpkg.StepCompleted:
		return doneStyle.Render("v")
	case planpkg.StepInProgress:
		return accentStyle.Render(">")
	case planpkg.StepBlocked:
		return errorStyle.Render("!")
	default:
		switch status {
		case "warn":
			return warnStyle.Render("!")
		case "error":
			return errorStyle.Render("x")
		default:
			return mutedStyle.Render("-")
		}
	}
}

func formatUserMeta(model string, at time.Time) string {
	model = strings.TrimSpace(model)
	if model == "" {
		model = "-"
	}
	return fmt.Sprintf("> you @ %s [%s]", model, at.Format("15:04:05"))
}

func shortID(id string) string {
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func trimPreview(text string, limit int) string {
	return compact(text, limit)
}

func compact(text string, limit int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" {
		return ""
	}
	if limit <= 0 || runewidth.StringWidth(text) <= limit {
		return text
	}
	if limit <= runewidth.StringWidth("...") {
		return runewidth.Truncate(text, limit, "")
	}
	return runewidth.Truncate(text, limit, "...")
}

func emptyDot(path string) string {
	if strings.TrimSpace(path) == "" {
		return "."
	}
	return path
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func initialScreen(sess *session.Session) screenKind {
	if sess == nil || len(sess.Messages) == 0 {
		return screenLanding
	}
	return screenChat
}
