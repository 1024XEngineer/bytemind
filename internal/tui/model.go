package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"bytemind/internal/agent"
	"bytemind/internal/config"
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
	pasteSubmitGuard    = 400 * time.Millisecond
	assistantLabel      = "Bytemind"
	thinkingLabel       = "Bytemind"
	chatTitleLabel      = "Bytemind Chat"
	tuiTitleLabel       = "Bytemind TUI"
)

type screenKind string

const (
	screenLanding screenKind = "landing"
	screenChat    screenKind = "chat"
)

type chatEntry struct {
	Kind   string
	Title  string
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
	{Name: "/help", Usage: "/help", Description: "打开帮助面板，查看当前可用命令和基础用法。", Kind: "command"},
	{Name: "/new", Usage: "/new", Description: "在当前工作区创建一个全新的持久会话。", Kind: "command"},
	{Name: "/quit", Usage: "/quit", Description: "退出当前 TUI 界面。", Kind: "command"},
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
	input    textarea.Model
	spinner  spinner.Model

	chatItems      []chatEntry
	toolRuns       []toolRun
	plan           []session.PlanItem
	sessions       []session.Summary
	sessionLimit   int
	sessionCursor  int
	commandCursor  int
	screen         screenKind
	sessionsOpen   bool
	helpOpen       bool
	commandOpen    bool
	busy           bool
	streamingIndex int
	statusNote     string
	phase          string
	llmConnected   bool
	approval       *approvalPrompt
	lastPasteAt    time.Time
	lastInputAt    time.Time
	inputBurstSize int
}

func newModel(opts Options) model {
	async := make(chan tea.Msg, 128)

	input := textarea.New()
	input.Placeholder = "Ask Bytemind to inspect, change, or verify this workspace..."
	input.Focus()
	input.CharLimit = 0
	input.SetWidth(72)
	input.SetHeight(3)
	input.ShowLineNumbers = false
	input.Prompt = "> "

	spin := spinner.New()
	spin.Spinner = spinner.MiniDot
	spin.Style = lipgloss.NewStyle().Foreground(colorAccent)

	vp := viewport.New(0, 0)
	vp.YPosition = 0
	vp.MouseWheelEnabled = true
	vp.MouseWheelDelta = scrollStep

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
		input:          input,
		spinner:        spin,
		chatItems:      chatItems,
		toolRuns:       toolRuns,
		plan:           copyPlan(opts.Session.Plan),
		sessions:       summaries,
		sessionLimit:   defaultSessionLimit,
		screen:         initialScreen(opts.Session),
		streamingIndex: -1,
		statusNote:     "Ready.",
		phase:          "idle",
		llmConnected:   true,
	}
	m.syncInputStyle()
	m.syncCommandPalette()
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
			m.syncCommandPalette()
		}
		return m, cmd
	}

	return m, nil
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.helpOpen || m.commandOpen || m.approval != nil {
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
		m.ensureViewportMouse()
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.viewport.LineUp(scrollStep)
			return m, nil
		case tea.MouseButtonWheelDown:
			m.viewport.LineDown(scrollStep)
			return m, nil
		default:
			m.ensureViewportMouse()
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
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

func (m model) mouseOverChatInput(y int) bool {
	if m.height <= 0 {
		return false
	}
	footerHeight := lipgloss.Height(m.renderFooter())
	footerTop := max(0, m.height-footerHeight)
	inputHeight := lipgloss.Height(
		m.inputBorderStyle().
			Width(m.chatPanelInnerWidth()).
			Render(m.input.View()),
	)
	inputTop := footerTop + panelStyle.GetVerticalFrameSize()/2
	if m.approval != nil {
		inputTop += lipgloss.Height(m.renderApprovalBanner())
	}
	inputBottom := inputTop + max(1, inputHeight) - 1
	return y >= inputTop && y <= inputBottom
}

func (m model) mouseOverLandingInput(y int) bool {
	if m.height <= 0 {
		return false
	}
	logoHeight := lipgloss.Height(landingLogoStyle.Render(strings.Join([]string{
		"   ___       __        __  ___ _           __",
		"  / _ )__ __/ /____   /  |/  (_)__  ___ _/ /",
		" / _  / // / __/ -_) / /|_/ / / _ \\/ _ `/ _ \\",
		"/____/\\_, /\\__/\\__/ /_/  /_/_/_//_/\\_,_/_.__/",
		"     /___/",
	}, "\n")))
	titleHeight := lipgloss.Height(landingTitleStyle.Render(chatTitleLabel))
	subtitleHeight := lipgloss.Height(mutedStyle.Render("Start with a prompt below. Press Enter to open the full conversation workspace."))
	inputHeight := lipgloss.Height(
		landingInputStyle.Copy().
			BorderForeground(colorAccent).
			Width(m.landingInputShellWidth()).
			Render(m.input.View()),
	)
	hintHeight := lipgloss.Height(mutedStyle.Render("Type / for supported commands, Ctrl+C to quit."))
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
	case "?":
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
		if msg.String() == "esc" || msg.String() == "?" {
			m.helpOpen = false
		}
		return m, nil
	}

	if m.commandOpen {
		return m.handleCommandPaletteKey(msg)
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
		return m, nil
	case "end":
		m.viewport.GotoBottom()
		return m, nil
	}

	if m.busy {
		return m, nil
	}

	if msg.String() == "enter" {
		if !m.lastPasteAt.IsZero() && time.Since(m.lastPasteAt) < pasteSubmitGuard {
			before := m.input.Value()
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(tea.KeyMsg{Type: tea.KeyEnter})
			if m.input.Value() != before {
				m.noteInputMutation(before, m.input.Value(), "paste-enter")
				m.syncCommandPalette()
			}
			return m, cmd
		}
		value := strings.TrimSpace(m.input.Value())
		if value == "" {
			return m, nil
		}
		if value == "/quit" {
			return m, tea.Quit
		}
		if strings.HasPrefix(value, "/") {
			m.input.Reset()
			if err := m.handleSlashCommand(value); err != nil {
				m.statusNote = err.Error()
			}
			m.refreshViewport()
			return m, m.loadSessionsCmd()
		}
		return m.submitPrompt(value)
	}

	before := m.input.Value()
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	if m.input.Value() != before {
		m.noteInputMutation(before, m.input.Value(), msg.String())
	}
	m.syncCommandPalette()
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
	case "up", "k":
		if len(items) > 0 {
			m.commandCursor = max(0, m.commandCursor-1)
		}
		return m, nil
	case "down", "j":
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
			if err := m.handleSlashCommand(value); err != nil {
				m.statusNote = err.Error()
			}
			m.refreshViewport()
			return m, m.loadSessionsCmd()
		}
		m.closeCommandPalette()
		if shouldExecuteFromPalette(selected) {
			if selected.Name == "/quit" {
				return m, tea.Quit
			}
			m.input.Reset()
			if err := m.handleSlashCommand(selected.Name); err != nil {
				m.statusNote = err.Error()
			}
			m.refreshViewport()
			return m, m.loadSessionsCmd()
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
		m.syncCommandPalette()
	}
	return m, cmd
}

func (m *model) openCommandPalette() {
	m.commandOpen = true
	m.commandCursor = 0
	m.setInputValue("/")
	m.syncCommandPalette()
}

func (m *model) closeCommandPalette() {
	m.commandOpen = false
	m.commandCursor = 0
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
		Body:   value,
		Status: "final",
	})
	m.chatItems = append(m.chatItems, chatEntry{
		Kind:   "assistant",
		Title:  thinkingLabel,
		Body:   m.thinkingText(),
		Status: "thinking",
	})
	m.streamingIndex = len(m.chatItems) - 1
	m.statusNote = "Request sent to LLM. Waiting for response..."
	m.phase = "thinking"
	m.llmConnected = true
	m.busy = true
	m.syncLayoutForCurrentScreen()
	m.refreshViewport()
	return m, tea.Batch(m.startRunCmd(value), m.spinner.Tick, waitForAsync(m.async))
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
		m.plan = copyPlan(event.Plan)
		m.statusNote = fmt.Sprintf("Plan updated with %d step(s).", len(m.plan))
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
			m.chatItems[m.streamingIndex].Title = assistantLabel
			m.chatItems[m.streamingIndex].Body = delta
		} else if strings.HasPrefix(delta, current) {
			m.chatItems[m.streamingIndex].Body = delta
		} else if strings.HasSuffix(current, delta) {
			// Some providers may repeat the latest chunk; ignore it.
		} else {
			m.chatItems[m.streamingIndex].Body += delta
		}
		m.chatItems[m.streamingIndex].Status = "streaming"
		return
	}
	m.chatItems = append(m.chatItems, chatEntry{
		Kind:   "assistant",
		Title:  assistantLabel,
		Body:   delta,
		Status: "streaming",
	})
	m.streamingIndex = len(m.chatItems) - 1
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
			item.Body = assistantToolIntro(toolName)
			item.Title = thinkingLabel
			item.Status = "thinking"
			m.streamingIndex = -1
			return
		}
	}
	if len(m.chatItems) > 0 {
		last := &m.chatItems[len(m.chatItems)-1]
		if last.Kind == "assistant" {
			return
		}
	}
	m.appendChat(chatEntry{
		Kind:   "assistant",
		Title:  thinkingLabel,
		Body:   assistantToolIntro(toolName),
		Status: "thinking",
	})
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
	m.viewport.SetContent(m.renderConversation())
	m.viewport.GotoBottom()
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
	m.syncLayoutForCurrentScreen()
	m.refreshViewport()
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
		mainPanel := panelStyle.Width(m.chatPanelWidth()).Render(m.renderMainPanel())
		base = lipgloss.JoinVertical(
			lipgloss.Left,
			mainPanel,
			m.renderFooter(),
		)
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
	width := max(24, m.viewport.Width)
	blocks := make([]string, 0, len(m.chatItems))
	for _, item := range m.chatItems {
		blocks = append(blocks, renderChatRow(item, width))
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
	panelInnerWidth := m.chatPanelInnerWidth()
	panelInnerHeight := max(4, bodyHeight-panelStyle.GetVerticalFrameSize())
	contentHeight := max(3, panelInnerHeight)
	m.viewport.Width = panelInnerWidth
	m.viewport.Height = contentHeight
}

func (m model) renderMainPanel() string {
	return m.viewport.View()
}

func (m model) renderLanding() string {
	logo := landingLogoStyle.Render(strings.Join([]string{
		"   ___       __        __  ___ _           __",
		"  / _ )__ __/ /____   /  |/  (_)__  ___ _/ /",
		" / _  / // / __/ -_) / /|_/ / / _ \\/ _ `/ _ \\",
		"/____/\\_, /\\__/\\__/ /_/  /_/_/_//_/\\_,_/_.__/",
		"     /___/",
	}, "\n"))
	title := landingTitleStyle.Render(chatTitleLabel)
	subtitle := mutedStyle.Render("Start with a prompt below. Press Enter to open the full conversation workspace.")
	inputBox := landingInputStyle.Copy().
		BorderForeground(colorAccent).
		Width(m.landingInputShellWidth()).
		Render(m.input.View())
	parts := []string{logo, "", title, subtitle, ""}
	if m.commandOpen {
		parts = append(parts, m.renderCommandPalette(), "")
	}
	parts = append(parts, inputBox, "", mutedStyle.Render("Type / for supported commands, Ctrl+L for sessions, Ctrl+C to quit."))
	content := lipgloss.JoinVertical(lipgloss.Center, parts...)
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) renderFooter() string {
	hint := mutedStyle.Width(m.chatPanelInnerWidth()).Render("/ commands  -  Ctrl+L sessions  -  Ctrl+C quit")
	inputBorder := m.inputBorderStyle().
		Width(m.chatPanelInnerWidth()).
		Render(m.input.View())
	parts := make([]string, 0, 4)
	if m.approval != nil {
		parts = append(parts, m.renderApprovalBanner())
	}
	if m.commandOpen {
		parts = append(parts, m.renderCommandPalette())
	}
	parts = append(parts, inputBorder, hint)
	content := lipgloss.JoinVertical(lipgloss.Left, parts...)
	return panelStyle.Width(m.chatPanelWidth()).Render(content)
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
		lipgloss.JoinVertical(lipgloss.Left, modalTitleStyle.Render("帮助"), m.helpText()),
	)
}

func (m model) renderApprovalBanner() string {
	width := max(24, m.chatPanelInnerWidth())
	commandWidth := max(20, width-4)
	lines := []string{
		accentStyle.Render("需要你的确认"),
		mutedStyle.Render("原因: " + trimPreview(m.approval.Reason, commandWidth)),
		codeStyle.Width(commandWidth).Render(m.approval.Command),
		mutedStyle.Render("Y / Enter 同意    N / Esc 拒绝"),
	}
	return approvalBannerStyle.Width(width).Render(strings.Join(lines, "\n"))
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

func (m *model) handleSlashCommand(input string) error {
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return nil
	}

	switch fields[0] {
	case "/help":
		m.screen = screenChat
		m.appendChat(chatEntry{Kind: "user", Title: "You", Body: input, Status: "final"})
		m.appendChat(chatEntry{Kind: "assistant", Title: assistantLabel, Body: m.helpText(), Status: "final"})
		m.statusNote = "已在聊天区显示帮助说明。"
		return nil
	case "/new":
		return m.newSession()
	default:
		return fmt.Errorf("unknown command: %s", fields[0])
	}
}

func (m *model) newSession() error {
	next := session.New(m.workspace)
	if err := m.store.Save(next); err != nil {
		return err
	}
	m.sess = next
	m.screen = screenLanding
	m.plan = nil
	m.chatItems = nil
	m.toolRuns = nil
	m.streamingIndex = -1
	m.statusNote = "Started a new session."
	m.input.Reset()
	m.syncLayoutForCurrentScreen()
	m.refreshViewport()
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
	m.plan = copyPlan(next.Plan)
	m.chatItems, m.toolRuns = rebuildSessionTimeline(next)
	m.streamingIndex = -1
	m.statusNote = "Resumed session " + shortID(next.ID)
	m.syncLayoutForCurrentScreen()
	m.refreshViewport()
	return nil
}

func (m model) startRunCmd(prompt string) tea.Cmd {
	return func() tea.Msg {
		go func() {
			_, err := m.runner.RunPrompt(context.Background(), m.sess, prompt, io.Discard)
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
		switch message.Role {
		case "user":
			items = append(items, chatEntry{Kind: "user", Title: "You", Body: message.Content, Status: "final"})
		case "assistant":
			for _, call := range message.ToolCalls {
				callNames[call.ID] = call.Function.Name
			}
			if strings.TrimSpace(message.Content) != "" {
				items = append(items, chatEntry{Kind: "assistant", Title: assistantLabel, Body: message.Content, Status: "final"})
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
	title := cardTitleStyle.Foreground(colorAccent)
	border := chatAssistantStyle
	bodyStyle := lipgloss.NewStyle()
	status := item.Status
	switch item.Kind {
	case "user":
		title = cardTitleStyle.Foreground(colorUser)
		border = chatUserStyle
	case "tool":
		title = cardTitleStyle.Foreground(colorTool)
		border = chatToolStyle
	case "system":
		title = cardTitleStyle.Foreground(colorMuted)
		border = chatSystemStyle
	default:
		if item.Status == "thinking" {
			title = cardTitleStyle.Foreground(colorThinking)
			border = chatThinkingStyle
			bodyStyle = thinkingBodyStyle
			status = ""
		}
	}
	outerWidth := max(12, width)
	innerWidth := max(8, outerWidth-border.GetHorizontalFrameSize())
	headContent := title.Render(item.Title)
	if status != "" {
		headContent = lipgloss.JoinHorizontal(lipgloss.Left, headContent, mutedStyle.Render("  "+status))
	}
	head := lipgloss.NewStyle().
		Width(innerWidth).
		Render(headContent)
	body := bodyStyle.Width(innerWidth).Render(formatChatBody(item, innerWidth))
	return border.Width(outerWidth).Render(lipgloss.JoinVertical(lipgloss.Left, head, body))
}

func renderChatRow(item chatEntry, width int) string {
	bubbleWidth := chatBubbleWidth(item, width)
	card := renderChatCard(item, bubbleWidth)
	return lipgloss.PlaceHorizontal(width, lipgloss.Left, card)
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
			Plan []session.PlanItem `json:"plan"`
		}
		if json.Unmarshal([]byte(payload), &result) == nil {
			lines := make([]string, 0, min(4, len(result.Plan)))
			for i := 0; i < min(4, len(result.Plan)); i++ {
				lines = append(lines, fmt.Sprintf("[%s] %s", result.Plan[i].Status, result.Plan[i].Step))
			}
			return fmt.Sprintf("Updated plan with %d step(s)", len(result.Plan)), lines, "done"
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
		return "我先查看一下相关内容。"
	}
	return fmt.Sprintf("我先调用 `%s` 看一下相关内容。", toolName)
}

func legacyAssistantToolFollowUp(toolName, summary, status string) string {
	if strings.TrimSpace(summary) == "" {
		return "我拿到工具结果了，继续整理一下。"
	}
	switch status {
	case "error", "warn":
		return fmt.Sprintf("`%s` 已返回结果，我先根据这个情况继续判断。", toolName)
	default:
		return fmt.Sprintf("`%s` 已经执行完了，我继续往下整理。", toolName)
	}
}

func assistantToolIntro(toolName string) string {
	if strings.TrimSpace(toolName) == "" {
		return "我先查看一下相关内容。"
	}
	return fmt.Sprintf("我先调用 `%s` 看一下相关内容。", toolName)
}

func assistantToolFollowUp(toolName, summary, status string) string {
	if strings.TrimSpace(summary) == "" {
		return "我拿到工具结果了，继续整理一下。"
	}
	switch status {
	case "error", "warn":
		return fmt.Sprintf("`%s` 已返回结果，我先根据这个情况继续判断。", toolName)
	default:
		return fmt.Sprintf("`%s` 已经执行完了，我继续往下整理。", toolName)
	}
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
	items := m.filteredCommands()
	if len(items) == 0 {
		m.commandCursor = 0
		return
	}
	if m.commandCursor < 0 || m.commandCursor >= len(items) {
		m.commandCursor = 0
	}
}

func (m model) filteredCommands() []commandItem {
	value := strings.TrimSpace(m.input.Value())
	query := commandFilterQuery(value, "")
	items := visibleCommandItems("")
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

func (m *model) setInputValue(value string) {
	m.input.SetValue(value)
	m.input.CursorEnd()
}

func shouldExecuteFromPalette(item commandItem) bool {
	switch item.Name {
	case "/help", "/new", "/quit":
		return true
	default:
		return false
	}
}

func (m model) helpText() string {
	return strings.Join([]string{
		"进入方式",
		"在仓库根目录运行 `go run ./cmd/bytemind chat` 即可启动。",
		"`go run ./cmd/bytemind chat` 会先进入带大 Logo 的启动页。",
		"在启动页输入内容并按 Enter 后，会进入正式聊天界面。",
		"`go run ./cmd/bytemind run -prompt \"...\"` 可用于一次性执行模式。",
		"",
		"斜杠命令",
		"/help: 查看帮助说明。",
		"/new: 新建一个会话。",
		"/quit: 退出 TUI。",
		"",
		"当前界面",
		"启动页是一个居中的 Logo 加输入框。",
		"主界面只显示用户消息和助手回复，不把聊天内容塞到旁边区域。",
		"顶部状态栏会显示工作区、provider、model、审批策略和当前状态。",
		"如果需要 shell 审批，会在聊天输入区上方显示确认条等待确认。",
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
	desc := strings.ToLower(item.Description)
	return strings.HasPrefix(name, query) ||
		strings.HasPrefix(usage, query) ||
		strings.Contains(desc, query)
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
	return inputStyle.BorderForeground(colorAccent)
}

func (m *model) syncInputStyle() {
	if m.screen == screenLanding {
		m.input.Placeholder = "Ask Bytemind to inspect, change, or verify this workspace..."
	} else {
		m.input.Placeholder = "Continue the conversation..."
	}
	m.input.Prompt = "> "
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

func copyPlan(plan []session.PlanItem) []session.PlanItem {
	if len(plan) == 0 {
		return nil
	}
	cloned := make([]session.PlanItem, len(plan))
	copy(cloned, plan)
	return cloned
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
	switch status {
	case "completed", "done":
		return doneStyle.Render("x")
	case "in_progress", "running":
		return accentStyle.Render(">")
	case "warn":
		return warnStyle.Render("!")
	case "error":
		return errorStyle.Render("x")
	default:
		return mutedStyle.Render("-")
	}
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
