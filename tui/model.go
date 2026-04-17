package tui

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"

	"bytemind/internal/assets"
	"bytemind/internal/config"
	"bytemind/internal/history"
	"bytemind/internal/llm"
	"bytemind/internal/mention"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"

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
	thinkingLabel              = "thinking"
	chatTitleLabel             = "Bytemind Chat"
	tuiTitleLabel              = "Bytemind TUI"
	footerHintText             = "tab agents | / commands | drag select | Ctrl+C copy/quit | Ctrl+F history | Ctrl+L sessions"
	conversationViewportZoneID = "bytemind:conversation:viewport"
	inputEditorZoneID          = "bytemind:input:editor"
	thinkingSpinnerFPS         = 80 * time.Millisecond
	pasteAggregateDebounce     = 120 * time.Millisecond
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
	Event Event
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
	Request ApprovalRequest
	Reply   chan approvalDecision
}

type promptHistoryLoadedMsg struct {
	Entries []history.PromptEntry
	Err     error
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

type pasteFinalizeMsg struct {
	ID int
}

type pasteSessionState struct {
	active       bool
	startedAt    time.Time
	lastEventAt  time.Time
	sourceKind   string
	baseInput    string
	bufferedText string
	sawMultiline bool
	finalizeID   int
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
	runner     Runner
	store      SessionStore
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
	runStartedAt          time.Time
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
	pasteSubmitGuardUntil time.Time
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
	promptHistoryLoading  bool
	promptHistoryLoadErr  string
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
	pasteSession          pasteSessionState
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

	spin := newThinkingSpinner()

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

	if opts.Runner != nil {
		opts.Runner.SetObserver(func(event Event) {
			async <- agentEventMsg{Event: event}
		})
		opts.Runner.SetApprovalHandler(func(req ApprovalRequest) (bool, error) {
			reply := make(chan approvalDecision, 1)
			async <- approvalRequestMsg{Request: req, Reply: reply}
			decision := <-reply
			return decision.Approved, decision.Err
		})
	}

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
		tea.EnableBracketedPaste,
		textarea.Blink,
		waitForAsync(m.async),
		m.tokenUsage.tickCmd(),
		m.loadSessionsCmd(),
	)
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if payload, ok := extractBracketedPastePayload(msg); ok {
		return m.handlePastePayload(payload)
	}

	switch msg := msg.(type) {
	case pasteFinalizeMsg:
		if !m.hasActivePasteSession() {
			return m, nil
		}
		m.finalizePasteSession(msg.ID)
		return m, nil
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
		if strings.EqualFold(strings.TrimSpace(m.phase), "thinking") || m.streamingIndex >= 0 {
			m.updateThinkingCard()
			m.refreshViewport()
		}
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
			cmd := m.beginRun(prompt, string(m.mode), note)
			if len(m.chatItems) >= 2 {
				last := m.chatItems[len(m.chatItems)-1]
				prev := m.chatItems[len(m.chatItems)-2]
				if last.Kind == "assistant" && last.Status == "pending" && prev.Kind == "system" {
					m.chatItems = m.chatItems[:len(m.chatItems)-1]
					m.streamingIndex = -1
				}
			}
			return m, cmd
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
			m.clampSessionCursor()
		}
		return m, nil
	case promptHistoryLoadedMsg:
		m.promptHistoryLoading = false
		m.promptHistoryLoaded = true
		if msg.Err != nil {
			m.promptHistoryEntries = nil
			m.promptHistoryLoadErr = msg.Err.Error()
			m.refreshPromptSearchMatches()
			if m.promptSearchOpen {
				m.statusNote = "Prompt history unavailable: " + compact(msg.Err.Error(), 72)
			}
			return m, nil
		}
		m.promptHistoryEntries = msg.Entries
		m.promptHistoryLoadErr = ""
		m.refreshPromptSearchMatches()
		if m.promptSearchOpen {
			if len(m.promptSearchMatches) == 0 {
				if m.promptSearchMode == promptSearchModePanel {
					m.statusNote = "History panel opened. No matching prompts."
				} else {
					m.statusNote = "No matching prompts."
				}
			} else {
				if m.promptSearchMode == promptSearchModePanel {
					m.statusNote = fmt.Sprintf("History panel ready (%d matches).", len(m.promptSearchMatches))
				} else {
					m.statusNote = fmt.Sprintf("Prompt history ready (%d matches).", len(m.promptSearchMatches))
				}
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

func (m model) pasteFragmentFromKey(msg tea.KeyMsg) (string, string, bool) {
	if msg.Paste {
		switch {
		case msg.Type == tea.KeyEnter:
			return "\n", "paste-key", true
		case len(msg.Runes) > 0:
			return string(msg.Runes), "paste-key", true
		default:
			return "", "", false
		}
	}
	if m.hasActivePasteSession() {
		switch {
		case msg.Type == tea.KeyEnter && m.shouldAppendEnterToActivePasteSession():
			return "\n", "rapid-enter", true
		case msg.Type == tea.KeyRunes && len(msg.Runes) > 0:
			return string(msg.Runes), "rune", true
		default:
			return "", "", false
		}
	}
	if msg.Type != tea.KeyRunes || len(msg.Runes) == 0 {
		return "", "", false
	}
	fragment := string(msg.Runes)
	if shouldStartPasteAggregationFromRunes(fragment) {
		return fragment, "rune", true
	}
	return "", "", false
}

func shouldStartPasteAggregationFromRunes(fragment string) bool {
	normalized := normalizeNewlines(fragment)
	trimmed := strings.TrimSpace(normalized)
	if trimmed == "" {
		return false
	}
	if strings.Contains(normalized, "\n") {
		return true
	}
	if len([]rune(fragment)) <= 1 {
		return false
	}
	if looksLikeMarkdownPasteFragment(trimmed) {
		return true
	}
	return len([]rune(trimmed)) >= pasteBurstImmediateMinChars
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
		return m, m.openPromptSearch(promptSearchModeQuick)
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
		return m.handleSessionsModalKey(msg)
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

	if msg.Type == tea.KeyEnter && !msg.Paste && !m.shouldSuppressEnterAfterPaste() && m.shouldTreatRapidEnterAsPasteContinuation() {
		before := m.input.Value()
		after := before + "\n"
		m.setInputValue(after)
		_ = m.handleInputMutation(before, after, "rapid-enter")
		m.syncInputOverlays()
		return m, nil
	}

	if fragment, source, ok := m.pasteFragmentFromKey(msg); ok {
		return m, m.ingestPasteFragment(fragment, source)
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
			if err := m.openSessionsModal(); err != nil {
				m.statusNote = err.Error()
				return m, nil
			}
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
		if m.hasActivePasteSession() {
			m.finalizePasteSession(m.pasteSession.finalizeID)
			return m, nil
		}
		rawValue := m.input.Value()
		if next, handled := m.handleSuppressedPasteEnter(rawValue); handled {
			return next, nil
		}
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
		updated := m.handleInputMutation(before, after, mutationSource)
		if updated == after && m.input.Value() == before && after != before {
			m.input = preview
		}
		after = m.input.Value()
	} else {
		m.input, cmd = m.input.Update(msg)
		after = m.input.Value()
		if after != before {
			_ = m.handleInputMutation(before, after, mutationSource)
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
	if !m.pasteSubmitGuardUntil.IsZero() && time.Now().Before(m.pasteSubmitGuardUntil) {
		return true
	}
	if m.hasImplicitPasteBurst() {
		return true
	}
	if m.lastPasteAt.IsZero() {
		return false
	}
	if time.Since(m.lastPasteAt) > pasteSubmitGuard {
		return false
	}
	if strings.Contains(m.input.Value(), "\n") {
		return true
	}
	if strings.Contains(m.input.Value(), "[Paste #") || strings.Contains(m.input.Value(), "[Pasted #") {
		return true
	}
	return time.Since(m.lastInputAt) <= 120*time.Millisecond
}

func (m model) hasImplicitPasteBurst() bool {
	if m.lastInputAt.IsZero() {
		return false
	}
	if time.Since(m.lastInputAt) > 250*time.Millisecond {
		return false
	}
	if m.inputBurstSize < pasteBurstImmediateMinChars {
		return false
	}
	trimmed := strings.TrimSpace(m.input.Value())
	if trimmed == "" {
		return false
	}
	if strings.Contains(trimmed, "[Paste #") || strings.Contains(trimmed, "[Pasted #") {
		return true
	}
	if strings.Contains(trimmed, "\n") {
		return true
	}
	if m.busy && m.inputBurstSize >= pasteBurstImmediateMinChars {
		return looksLikePastedFragment(trimmed)
	}
	if m.inputBurstSize < pasteBurstCharThreshold {
		return false
	}
	return looksLikePastedFragment(trimmed)
}

func (m model) shouldTreatRapidEnterAsPasteContinuation() bool {
	if m.lastInputAt.IsZero() {
		return false
	}
	if time.Since(m.lastInputAt) > 400*time.Millisecond {
		return false
	}
	trimmed := strings.TrimSpace(m.input.Value())
	if trimmed == "" {
		return false
	}
	if m.inputBurstSize >= 2 && looksLikeMarkdownPasteFragment(trimmed) {
		return true
	}
	return m.inputBurstSize >= pasteBurstImmediateMinChars
}

func (m model) shouldAppendEnterToActivePasteSession() bool {
	if !m.hasActivePasteSession() {
		return false
	}
	if m.pasteSession.lastEventAt.IsZero() {
		return false
	}
	return time.Since(m.pasteSession.lastEventAt) <= 400*time.Millisecond
}

func looksLikeMarkdownPasteFragment(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	switch {
	case strings.HasPrefix(value, "#"),
		strings.HasPrefix(value, ">"),
		strings.HasPrefix(value, "- "),
		strings.HasPrefix(value, "* "),
		strings.HasPrefix(value, "```"):
		return true
	}
	if len(value) >= 3 && value[0] >= '0' && value[0] <= '9' {
		if dot := strings.Index(value, ". "); dot > 0 && dot <= 2 {
			return true
		}
	}
	return false
}

func (m model) handleSuppressedPasteEnter(rawValue string) (tea.Model, bool) {
	if !m.shouldSuppressEnterAfterPaste() {
		return m, false
	}
	rawValue = strings.TrimSpace(rawValue)
	if rawValue == "" {
		return m, true
	}
	if markerChain, ok := extractLeadingCompressedMarker(rawValue); ok {
		tail := strings.TrimSpace(strings.TrimPrefix(rawValue, markerChain))
		if tail != "" {
			if m.shouldCompressPastedText(tail, "paste-enter") || m.isLongPastedText(tail) {
				marker, content, err := m.compressPastedText(tail)
				if err != nil {
					m.statusNote = err.Error()
					return m, true
				}
				combined := strings.TrimSpace(markerChain) + marker
				m.setInputValue(combined)
				m.syncInputOverlays()
				m.statusNote = fmt.Sprintf("Detected another pasted block and compressed it as %s (%d lines). Press Enter again to send.", marker, content.Lines)
				return m, true
			}
			if len(tail) >= 24 || strings.Contains(tail, "\n") {
				m.setInputValue(strings.TrimSpace(markerChain))
				m.syncInputOverlays()
				m.statusNote = "Detected continued paste chunk after compressed marker. Kept compressed markers only; press Enter again to send."
				return m, true
			}
		}
		return m, true
	}
	if m.shouldCompressPastedText(rawValue, "paste-enter") || (m.hasImplicitPasteBurst() && m.isLongPastedText(rawValue)) {
		marker, content, err := m.compressPastedText(rawValue)
		if err != nil {
			m.statusNote = err.Error()
			return m, true
		}
		m.setInputValue(marker)
		m.syncInputOverlays()
		m.statusNote = fmt.Sprintf("Long pasted text (%d lines) compressed as %s. Press Enter again to send. Use [Paste #%s] or [Paste #%s line10~line20] later.", content.Lines, marker, content.ID, content.ID)
		return m, true
	}
	m.pasteSubmitGuardUntil = time.Time{}
	return m, true
}

func (m *model) armPasteSubmitGuard(now time.Time) {
	if m == nil {
		return
	}
	if now.IsZero() {
		now = time.Now()
	}
	m.pasteSubmitGuardUntil = now.Add(pasteSubmitGuard)
}

func extractBracketedPastePayload(msg tea.Msg) (string, bool) {
	if msg == nil {
		return "", false
	}
	t := reflect.TypeOf(msg)
	if t == nil || t.PkgPath() != "github.com/charmbracelet/bubbletea" || t.Name() != "PasteMsg" {
		return "", false
	}
	v := reflect.ValueOf(msg)
	if !v.IsValid() || v.Kind() != reflect.String {
		return "", false
	}
	return v.String(), true
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
	return fmt.Sprintf("%s thinking...", m.spinner.View())
}

func (m model) thinkingDoneText() string {
	if m.runStartedAt.IsZero() {
		return "Synthesis complete"
	}
	seconds := int(time.Since(m.runStartedAt).Round(time.Second).Seconds())
	if seconds < 0 {
		seconds = 0
	}
	return fmt.Sprintf("Synthesis complete %ds", seconds)
}

func newThinkingSpinner() spinner.Model {
	spin := spinner.New()
	spin.Spinner = spinner.Spinner{
		Frames: []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"},
		FPS:    thinkingSpinnerFPS,
	}
	spin.Style = lipgloss.NewStyle().Foreground(colorThinkingBlue)
	return spin
}

func (m *model) resetThinkingSpinner() tea.Cmd {
	m.spinner = newThinkingSpinner()
	return m.spinner.Tick
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
