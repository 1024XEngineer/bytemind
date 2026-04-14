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
	{Name: "/skills-select", Usage: "/skills-select", Description: "Open the loaded skills picker.", Kind: "command"},
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
	skillsOpen            bool
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
	runStartedAt          time.Time
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
		m.runStartedAt = time.Time{}
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
		if m.approval != nil || m.helpOpen || m.sessionsOpen || m.skillsOpen || m.commandOpen || m.mentionOpen {
			return m, nil
		}
		m.openPromptSearch(promptSearchModeQuick)
		return m, nil
	case "ctrl+k":
		if m.approval != nil || m.helpOpen || m.sessionsOpen || m.commandOpen || m.mentionOpen || m.busy {
			return m, nil
		}
		if err := m.openSkillsPicker(); err != nil {
			m.statusNote = err.Error()
		}
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

	if m.skillsOpen {
		items := m.skillPickerItems()
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
			m.skillsOpen = false
			m.commandCursor = 0
		case "up", "k":
			if len(items) > 0 {
				m.commandCursor = max(0, m.commandCursor-1)
			}
		case "down", "j":
			if len(items) > 0 {
				m.commandCursor = min(len(items)-1, m.commandCursor+1)
			}
		case "enter":
			if err := m.activateSelectedSkill(); err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
			m.skillsOpen = false
			m.commandCursor = 0
		}
		return m, nil
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
	m.skillsOpen = false
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
	m.runStartedAt = time.Now()
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
	case m.skillsOpen:
		rendered = renderModal(m.width, m.height, m.renderSkillsModal())
	}
	return zone.Scan(rendered)
}

func (m *model) SetUsage(used, total int) tea.Cmd {
	m.tokenHasOfficialUsage = true
	m.tokenUsage.SetUnavailable(false)
	return m.tokenUsage.SetUsage(used, 0)
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

func (m model) renderSkillsModal() string {
	lines := []string{modalTitleStyle.Render("Loaded Skills"), mutedStyle.Render("Up/Down to select, Enter to activate, Esc to close"), ""}
	items := m.skillPickerItems()
	if len(items) == 0 {
		lines = append(lines, "No loaded skills available.")
	} else {
		activeName := ""
		if m.sess != nil && m.sess.ActiveSkill != nil {
			activeName = strings.TrimSpace(m.sess.ActiveSkill.Name)
		}
		for i, item := range items {
			prefix := "  "
			style := lipgloss.NewStyle()
			if i == clamp(m.commandCursor, 0, len(items)-1) {
				prefix = "> "
				style = style.Foreground(colorAccent).Bold(true)
			}
			label := fmt.Sprintf("%s%s", prefix, item.Name)
			if strings.EqualFold(activeName, item.Name) {
				label += "  (active)"
			}
			lines = append(lines, style.Render(label))
			if strings.TrimSpace(item.Description) != "" {
				lines = append(lines, mutedStyle.Render("   "+item.Description))
			}
			lines = append(lines, "")
		}
	}
	return modalBoxStyle.Width(min(96, max(56, m.width-12))).Render(strings.Join(lines, "\n"))
}

func (m model) renderHelpModal() string {

	modalWidth := min(88, max(54, m.width-16))
	innerWidth := max(20, modalWidth-modalBoxStyle.GetHorizontalFrameSize())
	body := renderHelpMarkdown(m.helpText(), innerWidth)
	return modalBoxStyle.Width(modalWidth).Render(
		lipgloss.JoinVertical(lipgloss.Left, modalTitleStyle.Render("Help"), body),
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
	case "/skills-select":
		return m.openSkillsPicker()
	case "/skills":
		return m.runSkillsListCommand(input)
	case "/skill":
		return m.runSkillCommand(input, fields)
	case "/new":
		return m.newSession()
	case "/compact":
		return m.runCompactCommand(input)
	default:
		return fmt.Errorf("unknown command: %s", fields[0])
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

func (m *model) openSkillsPicker() error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	skillsList, _ := m.runner.ListSkills()
	if len(skillsList) == 0 {
		m.statusNote = "No loaded skills available."
		return nil
	}
	m.skillsOpen = true
	m.commandOpen = false
	m.sessionsOpen = false
	m.commandCursor = 0
	m.statusNote = "Opened loaded skills picker."
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

func (m *model) activateSelectedSkill() error {
	if m.runner == nil {
		return fmt.Errorf("runner is unavailable")
	}
	items := m.skillPickerItems()
	if len(items) == 0 {
		return nil
	}
	index := clamp(m.commandCursor, 0, len(items)-1)
	selected := items[index]
	return m.activateSkillCommand(selected.Usage, selected.Usage, nil)
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

func runIndicatorPhaseText(phase string) string {
	switch strings.ToLower(strings.TrimSpace(phase)) {
	case "thinking":
		return "Thinking..."
	case "responding":
		return "Responding..."
	case "tool":
		return "Running tool..."
	case "interrupting":
		return "Interrupting..."
	case "approval":
		return "Waiting for approval..."
	default:
		return "Working..."
	}
}

func formatElapsedClock(startedAt, now time.Time) string {
	if startedAt.IsZero() || now.Before(startedAt) {
		return "00:00"
	}
	seconds := int(now.Sub(startedAt).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	minutes := seconds / 60
	secs := seconds % 60
	return fmt.Sprintf("%02d:%02d", minutes, secs)
}

func (m model) runIndicatorText() string {
	spin := strings.TrimSpace(m.spinner.View())
	if spin == "" {
		spin = "•"
	}
	return fmt.Sprintf("%s %s (%s)", spin, runIndicatorPhaseText(m.phase), formatElapsedClock(m.runStartedAt, time.Now()))
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
	items := visibleCommandItems("")
	skills := m.skillPickerItems()
	if len(skills) == 0 {
		return items
	}
	merged := make([]commandItem, 0, len(items)+len(skills))
	merged = append(merged, items...)
	merged = append(merged, skills...)
	return merged
}

func (m model) skillPickerItems() []commandItem {
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
		name := strings.TrimSpace(skill.Name)
		if name == "" {
			continue
		}
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
			Usage:       "/" + name,
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

func (m model) chatPanelWidth() int {
	return max(20, m.width)
}

func (m model) chatPanelInnerWidth() int {
	width := m.chatPanelWidth() - panelStyle.GetHorizontalFrameSize()
	return max(12, width)
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
