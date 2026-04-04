package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/mention"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/tools"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

const (
	defaultSessionLimit = 8
	scrollStep          = 3
	commandPageSize     = 3
	mentionPageSize     = 5
	pasteSubmitGuard    = 400 * time.Millisecond
	assistantLabel      = "Bytemind"
	thinkingLabel       = "Bytemind"
	chatTitleLabel      = "Bytemind Chat"
	tuiTitleLabel       = "Bytemind TUI"
	footerHintText      = "tab agents | / commands | Ctrl+L sessions | Ctrl+C quit"
)

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

type chatEntry struct {
	Kind   string
	Title  string
	Meta   string
	Body   string
	Status string
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
	Err error
}

type approvalRequestMsg struct {
	Request tools.ApprovalRequest
	Reply   chan approvalDecision
}

type sessionsLoadedMsg struct {
	Summaries []session.Summary
	Err       error
}

var commandItems = []commandItem{
	{Name: "/help", Usage: "/help", Description: "Show usage and supported commands.", Kind: "command"},
	{Name: "/session", Usage: "/session", Description: "Open the recent session list.", Kind: "command"},
	{Name: "/new", Usage: "/new", Description: "Start a fresh session in this workspace.", Kind: "command"},
	{Name: "/quit", Usage: "/quit", Description: "Exit the current TUI window.", Kind: "command"},
	{Name: "/skills", Usage: "/skills", Description: "List available skills and current active skill.", Kind: "command"},
	{Name: "/skill clear", Usage: "/skill clear", Description: "Clear active skill for this session.", Kind: "command"},
}

type model struct {
	runner    *agent.Runner
	store     *session.Store
	sess      *session.Session
	cfg       config.Config
	workspace string

	width  int
	height int

	async    chan tea.Msg
	viewport viewport.Model
	planView viewport.Model
	input    textarea.Model
	spinner  spinner.Model

	chatItems      []chatEntry
	toolRuns       []toolRun
	plan           planpkg.State
	sessions       []session.Summary
	sessionLimit   int
	sessionCursor  int
	commandCursor  int
	mentionCursor  int
	screen         screenKind
	mode           agentMode
	sessionsOpen   bool
	helpOpen       bool
	commandOpen    bool
	mentionOpen    bool
	busy           bool
	streamingIndex int
	statusNote     string
	phase          string
	llmConnected   bool
	approval       *approvalPrompt
	mentionQuery   string
	mentionToken   mention.Token
	mentionResults []mention.Candidate
	mentionIndex   *mention.WorkspaceFileIndex
	mentionRecent  map[string]int
	mentionSeq     int
	lastPasteAt    time.Time
	lastInputAt    time.Time
	inputBurstSize int
	chatAutoFollow bool
}

func newModel(opts Options) model {
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

	planVP := viewport.New(0, 0)
	planVP.YPosition = 0
	planVP.MouseWheelEnabled = true
	planVP.MouseWheelDelta = scrollStep

	chatItems, toolRuns := rebuildSessionTimeline(opts.Session)
	summaries, _, _ := opts.Store.List(defaultSessionLimit)

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
		runner:         opts.Runner,
		store:          opts.Store,
		sess:           opts.Session,
		cfg:            opts.Config,
		workspace:      opts.Workspace,
		async:          async,
		viewport:       vp,
		planView:       planVP,
		input:          input,
		spinner:        spin,
		chatItems:      chatItems,
		toolRuns:       toolRuns,
		plan:           copyPlanState(opts.Session.Plan),
		sessions:       summaries,
		sessionLimit:   defaultSessionLimit,
		screen:         initialScreen(opts.Session),
		mode:           toAgentMode(opts.Session.Mode),
		streamingIndex: -1,
		statusNote:     "Ready.",
		phase:          "idle",
		llmConnected:   true,
		chatAutoFollow: true,
		mentionIndex:   mention.NewWorkspaceFileIndex(opts.Workspace),
	}
	m.syncInputStyle()
	m.syncInputOverlays()
	if m.mentionIndex != nil {
		go m.mentionIndex.Prewarm()
	}
	return m
}

func (m model) Init() tea.Cmd {
	return tea.Batch(textarea.Blink, waitForAsync(m.async))
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
		m.busy = false
		m.streamingIndex = -1
		if msg.Err != nil {
			m.statusNote = "Run failed: " + msg.Err.Error()
			m.phase = "error"
			m.llmConnected = false
			m.failLatestAssistant(msg.Err.Error())
		} else {
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
	case tea.MouseMsg:
		return m.handleMouse(msg)
	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	if !m.busy && !m.sessionsOpen && !m.helpOpen && !m.commandOpen && m.approval == nil {
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.noteInputMutation(before, m.input.Value(), "")
			m.syncInputOverlays()
		}
		return m, cmd
	}

	return m, nil
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.helpOpen || m.commandOpen || m.mentionOpen || m.approval != nil {
		return m, nil
	}
	if m.screen != screenChat && m.screen != screenLanding {
		return m, nil
	}
	if m.screen == screenChat && m.sessionsOpen {
		return m, nil
	}
	if m.mouseOverInput(msg.Y) {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.scrollInput(-scrollStep)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.scrollInput(scrollStep)
			return m, nil
		default:
			return m, nil
		}
	}
	if m.screen == screenChat {
		if m.mouseOverPlan(msg.X, msg.Y) {
			m.ensurePlanMouse()
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.planView.LineUp(scrollStep)
				return m, nil
			case tea.MouseButtonWheelDown:
				m.planView.LineDown(scrollStep)
				return m, nil
			default:
				var cmd tea.Cmd
				m.planView, cmd = m.planView.Update(msg)
				return m, cmd
			}
		}
		m.ensureViewportMouse()
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.viewport.LineUp(scrollStep)
			m.chatAutoFollow = false
			return m, nil
		case tea.MouseButtonWheelDown:
			m.viewport.LineDown(scrollStep)
			m.chatAutoFollow = m.viewport.AtBottom()
			return m, nil
		default:
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			m.chatAutoFollow = m.viewport.AtBottom()
			return m, cmd
		}
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
	if m.commandOpen {
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
	titleHeight := 0
	subtitleHeight := 0
	inputHeight := lipgloss.Height(
		landingInputStyle.Copy().
			BorderForeground(m.modeAccentColor()).
			Width(m.landingInputShellWidth()).
			Render(m.input.View()),
	)
	hintHeight := lipgloss.Height(mutedStyle.Render(footerHintText))
	contentHeight := logoHeight + 1 + titleHeight + subtitleHeight + 1 + inputHeight + 1 + hintHeight
	contentTop := max(0, (m.height-contentHeight)/2)
	inputTop := contentTop + logoHeight + 1 + titleHeight + subtitleHeight + 1
	inputBottom := inputTop + max(1, inputHeight) - 1
	return y >= inputTop && y <= inputBottom
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c":
		if m.approval != nil {
			m.approval.Reply <- approvalDecision{Approved: false}
		}
		return m, tea.Quit
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

	switch msg.String() {
	case "ctrl+l":
		if !m.busy {
			m.sessionsOpen = true
		}
		return m, m.loadSessionsCmd()
	case "ctrl+n":
		if !m.busy && m.screen == screenChat {
			if err := m.newSession(); err != nil {
				m.statusNote = err.Error()
			}
		}
		return m, m.loadSessionsCmd()
	case "home":
		m.viewport.GotoTop()
		m.chatAutoFollow = false
		return m, nil
	case "end":
		m.viewport.GotoBottom()
		m.chatAutoFollow = true
		return m, nil
	}

	if m.busy {
		return m, nil
	}

	if msg.Type == tea.KeyEnter && msg.Alt {
		before := m.input.Value()
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if m.input.Value() != before {
			m.noteInputMutation(before, m.input.Value(), "alt+enter")
			m.syncInputOverlays()
		}
		return m, cmd
	}

	if msg.String() == "enter" {
		if !m.lastPasteAt.IsZero() && time.Since(m.lastPasteAt) < pasteSubmitGuard {
			before := m.input.Value()
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if m.input.Value() != before {
				m.noteInputMutation(before, m.input.Value(), "paste-enter")
				m.syncInputOverlays()
			}
			return m, cmd
		}
		value := strings.TrimSpace(m.input.Value())
		if value == "" {
			return m, nil
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
		if value == "/quit" {
			return m, tea.Quit
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

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.noteInputMutation(before, m.input.Value(), msg.String())
	}
	m.syncInputOverlays()
	return m, cmd
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
		m.noteInputMutation(before, m.input.Value(), msg.String())
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
		m.recordRecentMention(selected.Path)
		nextValue := mention.InsertIntoInput(m.input.Value(), m.mentionToken, selected.Path)
		m.setInputValue(nextValue)
		m.statusNote = "Inserted mention: " + selected.Path
		m.closeMentionPalette()
		m.syncInputOverlays()
		return m, nil
	case "enter":
		selected, ok := m.selectedMentionCandidate()
		if !ok {
			m.closeMentionPalette()
			return m.handleKey(msg)
		}
		m.recordRecentMention(selected.Path)
		nextValue := mention.InsertIntoInput(m.input.Value(), m.mentionToken, selected.Path)
		m.setInputValue(nextValue)
		m.statusNote = "Inserted mention: " + selected.Path
		m.closeMentionPalette()
		m.syncInputOverlays()
		return m, nil
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.noteInputMutation(before, m.input.Value(), msg.String())
		m.syncInputOverlays()
	}
	return m, cmd
}

func (m *model) openCommandPalette() {
	m.commandOpen = true
	m.commandCursor = 0
	m.setInputValue("/")
	m.closeMentionPalette()
	m.syncInputOverlays()
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

func lenCommonPrefix(a, b string) int {
	limit := min(len(a), len(b))
	for i := 0; i < limit; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return limit
}

func (m model) submitPrompt(value string) (tea.Model, tea.Cmd) {
	m.input.Reset()
	m.screen = screenChat
	m.appendChat(chatEntry{
		Kind:   "user",
		Title:  "You",
		Meta:   formatUserMeta(m.currentModelLabel(), time.Now()),
		Body:   value,
		Status: "final",
	})
	m.streamingIndex = -1
	m.statusNote = "Request sent to LLM. Waiting for response..."
	m.phase = "thinking"
	m.llmConnected = true
	m.busy = true
	m.chatAutoFollow = true
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return m, tea.Batch(m.startRunCmd(value, string(m.mode)), m.spinner.Tick, waitForAsync(m.async))
}

func (m *model) handleAgentEvent(event agent.Event) {
	switch event.Type {
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
			Body:   "params: " + summarizeArgs(event.ToolArguments),
			Status: "running",
		})
		m.toolRuns = append(m.toolRuns, toolRun{
			Name:    event.ToolName,
			Summary: "params: " + summarizeArgs(event.ToolArguments),
			Status:  "running",
		})
		m.statusNote = "Running tool: " + event.ToolName
	case agent.EventToolCallCompleted:
		summary, lines, status := summarizeTool(event.ToolName, event.ToolResult)
		m.appendChat(chatEntry{
			Kind:   "tool",
			Title:  "Tool Result | " + event.ToolName,
			Body:   joinSummary(summary, lines),
			Status: status,
		})
		if len(m.toolRuns) > 0 {
			index := len(m.toolRuns) - 1
			m.toolRuns[index].Summary = summary
			m.toolRuns[index].Lines = lines
			m.toolRuns[index].Status = status
		}
		m.statusNote = summary
		m.phase = "thinking"
	case agent.EventPlanUpdated:
		m.plan = copyPlanState(event.Plan)
		m.phase = string(planpkg.NormalizePhase(string(m.plan.Phase)))
		if m.phase == "none" {
			m.phase = "plan"
		}
		m.statusNote = fmt.Sprintf("Plan updated with %d step(s).", len(m.plan.Steps))
	case agent.EventRunFinished:
		if strings.TrimSpace(event.Content) != "" {
			m.statusNote = "Run finished."
		}
		m.phase = "idle"
	}
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
	chatOffset := m.viewport.YOffset
	keepChatBottom := m.chatAutoFollow || m.viewport.AtBottom()
	m.viewport.SetContent(m.renderConversation())
	if keepChatBottom {
		m.viewport.GotoBottom()
		m.chatAutoFollow = true
	} else {
		m.viewport.SetYOffset(chatOffset)
	}

	planOffset := m.planView.YOffset
	m.planView.SetContent(m.planPanelContent(max(16, m.planView.Width)))
	m.planView.SetYOffset(planOffset)
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

	switch {
	case m.helpOpen:
		return renderModal(m.width, m.height, m.renderHelpModal())
	case m.sessionsOpen:
		return renderModal(m.width, m.height, m.renderSessionsModal())
	default:
		return base
	}
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
	m.viewport.Width = m.conversationPanelWidth()
	m.viewport.Height = contentHeight
}

func (m model) renderMainPanel() string {
	statusBar := m.renderStatusBar()
	return lipgloss.JoinVertical(lipgloss.Left, statusBar, "", m.viewport.View())
}

func (m model) renderLanding() string {
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
		Render(m.input.View())
	parts := []string{logo, "", m.renderModeTabs(), ""}
	if m.mentionOpen {
		parts = append(parts, m.renderMentionPalette(), "")
	} else if m.commandOpen {
		parts = append(parts, m.renderCommandPalette(), "")
	}
	parts = append(parts, inputBox, "", mutedStyle.Render(footerHintText))
	content := lipgloss.JoinVertical(lipgloss.Center, parts...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) renderFooter() string {
	inputBorder := m.inputBorderStyle().
		Width(m.chatPanelInnerWidth()).
		Render(m.input.View())
	parts := make([]string, 0, 4)
	if m.approval != nil {
		parts = append(parts, m.renderApprovalBanner())
	}
	if m.mentionOpen {
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
	rightParts := []string{footerHintText}
	if modelName := strings.TrimSpace(m.currentModelLabel()); modelName != "" && modelName != "-" {
		rightParts = append([]string{modelName}, rightParts...)
	}
	rightRaw := strings.Join(rightParts, "  |  ")
	right := mutedStyle.Render(rightRaw)

	leftW := lipgloss.Width(left)
	rightW := lipgloss.Width(right)
	gap := width - leftW - rightW
	if gap < 2 {
		available := max(10, width-leftW-2)
		if available <= 10 {
			return lipgloss.NewStyle().Width(width).Render(mutedStyle.Render(compact(rightRaw, width)))
		}
		compacted := mutedStyle.Render(compact(rightRaw, available))
		gap = width - leftW - lipgloss.Width(compacted)
		return lipgloss.NewStyle().Width(width).Render(left + strings.Repeat(" ", max(2, gap)) + compacted)
	}

	return lipgloss.NewStyle().Width(width).Render(left + strings.Repeat(" ", gap) + right)
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
	width := max(24, m.chatPanelInnerWidth())
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
	if len(fields) != 2 || fields[1] != "clear" {
		return fmt.Errorf("usage: /skill clear")
	}

	if err := m.runner.ClearActiveSkill(m.sess); err != nil {
		return err
	}
	m.appendCommandExchange(input, "Active skill cleared.")
	m.statusNote = "Skill cleared."
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
	if m.width > 0 && m.height > 0 {
		m.syncLayoutForCurrentScreen()
		m.refreshViewport()
	}
	return nil
}

func (m model) startRunCmd(prompt, mode string) tea.Cmd {
	return func() tea.Msg {
		go func() {
			_, err := m.runner.RunPrompt(context.Background(), m.sess, prompt, mode, io.Discard)
			m.async <- runFinishedMsg{Err: err}
		}()
		return nil
	}
}

func (m model) loadSessionsCmd() tea.Cmd {
	return func() tea.Msg {
		summaries, _, err := m.store.List(m.sessionLimit)
		return sessionsLoadedMsg{Summaries: summaries, Err: err}
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
					Title:  "Tool | " + name,
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
				Title:  "Tool | " + name,
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
		border = chatToolStyle
	case "system":
		border = chatSystemStyle
	default:
		if item.Status == "thinking" {
			border = chatThinkingStyle
		}
	}
	contentWidth := max(8, width-border.GetHorizontalFrameSize())
	return border.Width(contentWidth).Render(renderChatSection(item, contentWidth))
}

func renderChatSection(item chatEntry, width int) string {
	title := cardTitleStyle.Foreground(colorAccent)
	bodyStyle := chatBodyStyle
	status := item.Status
	displayTitle := item.Title
	if status == "final" {
		status = ""
	}
	switch item.Kind {
	case "user":
		title = cardTitleStyle.Foreground(colorUser)
	case "tool":
		title = cardTitleStyle.Foreground(colorMuted).Faint(true)
		bodyStyle = toolBodyStyle
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
	return strings.TrimRight(renderAssistantBody(tidyAssistantSpacing(text), width), "\n")
}

func wrapPlainText(text string, width int) string {
	if width <= 0 {
		return text
	}

	lines := strings.Split(text, "\n")
	wrapped := make([]string, 0, len(lines))
	style := lipgloss.NewStyle().MaxWidth(width)
	for _, line := range lines {
		if line == "" {
			wrapped = append(wrapped, "")
			continue
		}
		// Keep plain text within the viewport width before rendering the card.
		for _, part := range strings.Split(style.Render(runewidth.Wrap(line, width)), "\n") {
			wrapped = append(wrapped, strings.TrimRight(part, " "))
		}
	}
	return strings.Join(wrapped, "\n")
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
	codeLines := make([]string, 0, 8)

	flushCodeBlock := func() {
		if len(codeLines) == 0 {
			out = append(out, codeStyle.Width(max(12, width)).Render(""))
			return
		}
		out = append(out, codeStyle.Width(max(12, width)).Render(strings.Join(codeLines, "\n")))
		codeLines = codeLines[:0]
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			if inCodeBlock {
				flushCodeBlock()
				inCodeBlock = false
			} else {
				inCodeBlock = true
			}
			continue
		}

		if inCodeBlock {
			codeLines = append(codeLines, line)
			continue
		}

		switch {
		case trimmed == "":
			out = append(out, "")
		case isMarkdownHeading(trimmed):
			out = append(out, renderMarkdownHeading(trimmed, width))
		case isMarkdownListItem(trimmed):
			out = append(out, renderMarkdownListItem(line, width))
		case strings.HasPrefix(trimmed, ">"):
			out = append(out, renderMarkdownQuote(trimmed, width))
		case looksLikeMarkdownTable(trimmed):
			out = append(out, tableLineStyle.Render(trimmed))
		default:
			out = append(out, wrapPlainText(line, width))
		}
	}

	if inCodeBlock {
		flushCodeBlock()
	}

	return strings.Join(out, "\n")
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
		"我将调用",
		"我会调用",
		"我先调用",
		"我要调用",
		"先调用",
		"我将使用",
		"我会使用",
		"我先使用",
		"我将运行",
		"我会运行",
		"先检查相关上下文",
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
		"我会先",
		"先了解",
		"然后",
		"最后",
		"通过构建和测试",
		"系统性",
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
	m.syncCommandPalette()
	m.syncMentionPalette()
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

func (m *model) setInputValue(value string) {
	m.input.SetValue(value)
	m.input.CursorEnd()
}

func shouldExecuteFromPalette(item commandItem) bool {
	if item.Kind == "skill" {
		return true
	}
	switch item.Name {
	case "/help", "/session", "/skills", "/skill clear", "/new", "/quit":
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
		"/skill clear: clear the active skill.",
		"/new: start a fresh session.",
		"/quit: exit the TUI.",
		"",
		"UI notes",
		"Tab toggles between Build and Plan modes.",
		"Plan mode keeps the plan panel visible and focused on structured steps.",
		"Use Ctrl+G to open or close the help panel.",
		"After restoring a session with a saved plan, type 'continue execution' to resume it.",
		"Approval requests appear above the input area when a shell command needs confirmation.",
		"The footer keeps only the essential shortcuts: tab agents, / commands, Ctrl+L sessions, Ctrl+C quit.",
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
	m.input.Placeholder = "Ask Bytemind to inspect, change, or verify this workspace..."
	m.input.Prompt = ""
	m.input.SetHeight(2)
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
		return doneStyle.Render("✓")
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
