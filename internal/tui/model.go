package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/session"
	"bytemind/internal/tools"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	defaultSessionLimit = 8
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

var legacyCommandItems = []commandItem{
	{Name: "/help", Usage: "/help", Description: "打开帮助面板，查看当前可用命令和基础用法。"},
	{Name: "/session", Usage: "/session", Description: "查看当前会话 ID、工作区路径和最近更新时间。"},
	{Name: "/sessions", Usage: "/sessions [limit]", Description: "列出最近的历史会话，方便恢复之前的上下文。"},
	{Name: "/resume", Usage: "/resume <id>", Description: "按完整 ID 或前缀恢复一个已有会话。"},
	{Name: "/new", Usage: "/new", Description: "在当前工作区创建一个全新的持久会话。"},
	{Name: "/plan", Usage: "/plan", Description: "查看当前会话里保存的任务计划。"},
	{Name: "/plan create", Usage: "/plan create step one | step two | step three", Description: "按你给出的步骤创建一份新的多步骤计划。"},
	{Name: "/plan add", Usage: "/plan add <step>", Description: "给当前计划继续追加一个步骤。"},
	{Name: "/plan start", Usage: "/plan start <index>", Description: "把指定步骤标记为进行中。"},
	{Name: "/plan done", Usage: "/plan done <index>", Description: "把指定步骤标记为已完成。"},
	{Name: "/plan pending", Usage: "/plan pending <index>", Description: "把指定步骤重新标记为待处理。"},
	{Name: "/plan clear", Usage: "/plan clear", Description: "清空当前会话中的任务计划。"},
	{Name: "/quit", Usage: "/quit", Description: "退出当前 TUI 界面。"},
}

var commandItems = []commandItem{
	{Name: "/help", Usage: "/help", Description: "Open the help panel and show supported commands."},
	{Name: "/session", Usage: "/session", Description: "Show the current session id, workspace, and update time."},
	{Name: "/sessions", Usage: "/sessions [limit]", Description: "List recent sessions so you can resume prior work."},
	{Name: "/resume", Usage: "/resume <id>", Description: "Resume an existing session by full id or prefix."},
	{Name: "/new", Usage: "/new", Description: "Create a fresh session in the current workspace."},
	{Name: "/quit", Usage: "/quit", Description: "Exit the current TUI screen."},
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
	screen         screenKind
	sessionsOpen   bool
	helpOpen       bool
	commandOpen    bool
	commandCursor  int
	busy           bool
	streamingIndex int
	statusNote     string
	phase          string
	llmConnected   bool
	approval       *approvalPrompt
}

func newModel(opts Options) model {
	async := make(chan tea.Msg, 128)

	input := textarea.New()
	input.Placeholder = "Ask AICoding to inspect, change, or verify this workspace..."
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

	return m, nil
}

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.screen != screenChat || m.sessionsOpen || m.helpOpen || m.commandOpen || m.approval != nil {
		return m, nil
	}
	switch msg.Button {
	case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown, tea.MouseButtonWheelLeft, tea.MouseButtonWheelRight:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	default:
		return m, nil
	}
}

func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading TUI..."
	}

	if m.screen == screenLanding {
		base := m.renderLanding()
		switch {
		case m.helpOpen:
			return renderModal(m.width, m.height, m.renderHelpModal())
		case m.commandOpen:
			return renderModal(m.width, m.height, m.renderCommandPalette())
		default:
			return base
		}
	}

	header := m.renderHeader()
	footer := m.renderFooter()
	bodyHeight := m.height - lipgloss.Height(header) - lipgloss.Height(footer)
	if bodyHeight < 6 {
		bodyHeight = 6
	}

	mainWidth := m.chatPanelWidth()
	body := panelStyle.Width(mainWidth).Height(bodyHeight).Render(m.renderMainPanel())

	base := lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
	switch {
	case m.helpOpen:
		return renderModal(m.width, m.height, m.renderHelpModal())
	case m.sessionsOpen:
		return renderModal(m.width, m.height, m.renderSessionsModal())
	case m.approval != nil:
		return renderModal(m.width, m.height, m.renderApprovalModal())
	case m.commandOpen:
		return renderModal(m.width, m.height, m.renderCommandPalette())
	default:
		return base
	}
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
		switch msg.String() {
		case "esc":
			m.commandOpen = false
			return m, nil
		case "up", "k":
			if m.commandCursor > 0 {
				m.commandCursor--
			}
			return m, nil
		case "down", "j":
			items := m.filteredCommands()
			if m.commandCursor < len(items)-1 {
				m.commandCursor++
			}
			return m, nil
		case "enter":
			items := m.filteredCommands()
			if len(items) == 0 {
				return m, nil
			}
			selected := items[m.commandCursor]
			m.commandOpen = false
			if shouldExecuteFromPalette(selected) {
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
		if !m.busy && m.screen == screenChat {
			m.sessionsOpen = true
		}
		return m, nil
	case "ctrl+n":
		if !m.busy && m.screen == screenChat {
			if err := m.newSession(); err != nil {
				m.statusNote = err.Error()
			}
		}
		return m, m.loadSessionsCmd()
	case "pgup":
		m.viewport.PageUp()
		return m, nil
	case "pgdown":
		m.viewport.PageDown()
		return m, nil
	case "ctrl+up":
		m.viewport.LineUp(1)
		return m, nil
	case "ctrl+down":
		m.viewport.LineDown(1)
		return m, nil
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

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.syncCommandPalette()
	return m, cmd
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
		Title:  "AICoding",
		Body:   m.thinkingText(),
		Status: "pending",
	})
	m.streamingIndex = len(m.chatItems) - 1
	m.statusNote = "Request sent to LLM. Waiting for response..."
	m.phase = "thinking"
	m.llmConnected = true
	m.busy = true
	m.syncInputStyle()
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
		m.toolRuns = append(m.toolRuns, toolRun{
			Name:    event.ToolName,
			Summary: "Running with " + summarizeArgs(event.ToolArguments),
			Status:  "running",
		})
		m.statusNote = "Running tool: " + event.ToolName
	case agent.EventToolCallCompleted:
		summary, lines, status := summarizeTool(event.ToolName, event.ToolResult)
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
		if m.chatItems[m.streamingIndex].Status == "pending" {
			m.chatItems[m.streamingIndex].Body = delta
		} else {
			m.chatItems[m.streamingIndex].Body += delta
		}
		m.chatItems[m.streamingIndex].Status = "streaming"
		return
	}
	m.chatItems = append(m.chatItems, chatEntry{
		Kind:   "assistant",
		Title:  "AICoding",
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
		m.chatItems[m.streamingIndex].Body = content
		m.chatItems[m.streamingIndex].Status = "final"
		m.streamingIndex = -1
		return
	}
	m.chatItems = append(m.chatItems, chatEntry{
		Kind:   "assistant",
		Title:  "AICoding",
		Body:   content,
		Status: "final",
	})
}

func (m *model) appendChat(item chatEntry) {
	m.chatItems = append(m.chatItems, item)
}

func (m *model) updateThinkingCard() {
	if !m.busy || m.streamingIndex < 0 || m.streamingIndex >= len(m.chatItems) {
		return
	}
	item := &m.chatItems[m.streamingIndex]
	if item.Kind != "assistant" || item.Status != "pending" {
		return
	}
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
			Title:  "AICoding",
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
		Title:  "AICoding",
		Body:   "Request failed: " + errText,
		Status: "error",
	})
}

func (m *model) refreshViewport() {
	m.syncViewportSize()
	m.viewport.SetContent(m.renderConversation())
	m.viewport.GotoBottom()
}

func (m *model) resize() {
	if m.screen == screenLanding {
		m.input.SetWidth(m.landingInputContentWidth())
	} else {
		m.input.SetWidth(m.chatInputContentWidth())
	}
	m.syncInputStyle()
	m.syncViewportSize()
	m.refreshViewport()
}

func (m model) renderConversation() string {
	if len(m.chatItems) == 0 {
		return mutedStyle.Render("No messages yet. Start with an instruction like \"analyze this repo\" or \"implement a TUI shell\".")
	}
	width := max(24, m.viewport.Width)
	blocks := make([]string, 0, len(m.chatItems))
	for _, item := range m.chatItems {
		if item.Kind == "tool" {
			continue
		}
		blocks = append(blocks, renderChatRow(item, width))
	}
	return strings.Join(blocks, "\n\n")
}

func (m *model) syncViewportSize() {
	if m.width == 0 || m.height == 0 {
		return
	}
	headerHeight := lipgloss.Height(m.renderHeader())
	footerHeight := lipgloss.Height(m.renderFooter())
	bodyHeight := m.height - headerHeight - footerHeight
	if bodyHeight < 6 {
		bodyHeight = 6
	}
	panelInnerWidth := m.chatPanelInnerWidth()
	panelInnerHeight := max(4, bodyHeight-2)
	mainHeaderHeight := 3
	contentHeight := max(3, panelInnerHeight-mainHeaderHeight)
	m.viewport.Width = panelInnerWidth
	m.viewport.Height = contentHeight
}

func (m model) renderMainPanel() string {
	header := sectionTitleStyle.Render("Conversation")
	connection := "LLM request path: ready"
	if !m.llmConnected {
		connection = "LLM request path: not confirmed"
	}
	subtitle := mutedStyle.Render(connection + "  |  Chat only shows your prompt and the assistant reply.")
	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		subtitle,
		"",
		m.viewport.View(),
	)
}

func (m model) renderLanding() string {
	logo := landingLogoStyle.Render(strings.Join([]string{
		"   ___       __        __  ___ _           __",
		"  / _ )__ __/ /____   /  |/  (_)__  ___ _/ /",
		" / _  / // / __/ -_) / /|_/ / / _ \\/ _ `/ _ \\",
		"/____/\\_, /\\__/\\__/ /_/  /_/_/_//_/\\_,_/_.__/",
		"     /___/",
	}, "\n"))
	title := landingTitleStyle.Render("AICoding Chat")
	subtitle := mutedStyle.Render("Start with a prompt below. Press Enter to open the full conversation workspace.")
	inputBox := landingInputStyle.Copy().
		BorderForeground(colorAccent).
		Width(m.landingInputShellWidth()).
		Render(m.input.View())
	content := lipgloss.JoinVertical(lipgloss.Center, logo, "", title, subtitle, "", inputBox, "", mutedStyle.Render("Type / for supported commands, Ctrl+C to quit."))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m model) renderHeader() string {
	state := "idle"
	if m.approval != nil {
		state = "approval"
	} else if m.busy {
		state = m.phase
	}

	statusValue := state
	if m.busy {
		statusValue = m.spinner.View() + " " + state
	}

	left := titleStyle.Render("AICoding TUI")
	right := lipgloss.JoinHorizontal(
		lipgloss.Left,
		tagStyle.Render(filepath.Base(m.workspace)),
		tagStyle.Render(m.cfg.Provider.Type),
		tagStyle.Render(m.cfg.Provider.Model),
		tagStyle.Render("approval:"+m.cfg.ApprovalPolicy),
		tagStyle.Render("budget:"+strconv.Itoa(m.cfg.MaxIterations)),
		tagStyle.Render("phase:"+state),
		tagStyle.Render("llm:"+connectionState(m.llmConnected)),
		statusTagStyle.Render(statusValue),
	)
	line := lipgloss.JoinHorizontal(lipgloss.Center, left, spacer(max(1, m.width-lipgloss.Width(left)-lipgloss.Width(right)-4)), right)

	subtitle := subtleBorderStyle.Render(
		fmt.Sprintf("session %s | workspace %s | %s", shortID(m.sess.ID), filepath.Base(m.sess.Workspace), m.statusNote),
	)
	return lipgloss.JoinVertical(lipgloss.Left, line, subtitle)
}

func (m model) renderFooter() string {
	hint := mutedStyle.Render("Type / for commands  -  ? help  -  Ctrl+Up/Down scroll  -  Enter send  -  Ctrl+N new session  -  Ctrl+L sessions  -  Ctrl+C quit")
	inputBorder := m.inputBorderStyle().
		Width(m.chatPanelInnerWidth()).
		Render(m.input.View())
	content := lipgloss.JoinVertical(
		lipgloss.Left,
		inputBorder,
		mutedStyle.Width(m.chatPanelInnerWidth()).Render(hint),
	)
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

func (m model) renderApprovalModal() string {
	lines := []string{
		modalTitleStyle.Render("Approve Shell Command"),
		"",
		"Reason: " + m.approval.Reason,
		"",
		codeStyle.Width(min(88, max(44, m.width-20))).Render(m.approval.Command),
		"",
		mutedStyle.Render("Press Y or Enter to approve, N or Esc to reject."),
	}
	return modalBoxStyle.Width(min(96, max(56, m.width-16))).Render(strings.Join(lines, "\n"))
}

func (m model) renderCommandPalette() string {
	items := m.filteredCommands()
	lines := []string{
		modalTitleStyle.Render("可用命令"),
		mutedStyle.Render("使用上下方向键选择，按 Enter 插入，按 Esc 关闭。"),
		mutedStyle.Render("这里只显示当前版本已经真正支持的命令。"),
		"",
	}
	if len(items) == 0 {
		lines = append(lines, mutedStyle.Render("没有匹配当前输入的命令。"))
	} else {
		for i, item := range items {
			rowStyle := lipgloss.NewStyle()
			if i == m.commandCursor {
				rowStyle = rowStyle.Foreground(colorAccent).Bold(true)
			}
			lines = append(lines, rowStyle.Render(item.Usage))
			lines = append(lines, mutedStyle.Render("  "+item.Description))
			lines = append(lines, "")
		}
	}
	return modalBoxStyle.BorderForeground(colorAccent).Width(min(96, max(60, m.width-16))).Render(strings.Join(lines, "\n"))
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
		m.appendChat(chatEntry{Kind: "assistant", Title: "AICoding", Body: m.helpText(), Status: "final"})
		m.statusNote = "已在聊天区显示帮助说明。"
		return nil
	case "/session":
		m.screen = screenChat
		m.appendChat(chatEntry{Kind: "user", Title: "You", Body: input, Status: "final"})
		m.appendChat(chatEntry{Kind: "assistant", Title: "AICoding", Body: m.sessionText(), Status: "final"})
		m.statusNote = fmt.Sprintf("session %s in %s", m.sess.ID, m.sess.Workspace)
		return nil
	case "/sessions":
		if len(fields) > 1 {
			limit, err := strconv.Atoi(fields[1])
			if err != nil || limit <= 0 {
				return fmt.Errorf("/sessions limit must be a positive integer")
			}
			m.sessionLimit = limit
		}
		m.sessionsOpen = true
		return nil
	case "/resume":
		if len(fields) < 2 {
			return fmt.Errorf("usage: /resume <id>")
		}
		return m.resumeSession(fields[1])
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
	m.syncInputStyle()
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
	m.syncInputStyle()
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
				items = append(items, chatEntry{Kind: "assistant", Title: "AICoding", Body: message.Content, Status: "final"})
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
	}
	head := lipgloss.JoinHorizontal(lipgloss.Left, title.Render(item.Title), mutedStyle.Render("  "+item.Status))
	body := lipgloss.NewStyle().Width(width).Render(item.Body)
	return border.Width(width + 2).Render(lipgloss.JoinVertical(lipgloss.Left, head, body))
}

func renderChatRow(item chatEntry, width int) string {
	bubbleWidth := width * 3 / 4
	if bubbleWidth < 28 {
		bubbleWidth = width
	}
	if bubbleWidth > width {
		bubbleWidth = width
	}
	card := renderChatCard(item, bubbleWidth-2)
	align := lipgloss.Left
	if item.Kind == "user" {
		align = lipgloss.Right
	}
	return lipgloss.PlaceHorizontal(width, align, card)
}

func renderModal(width, height int, modal string) string {
	if width == 0 || height == 0 {
		return modal
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, modal)
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

func (m model) thinkingText() string {
	return fmt.Sprintf("%s Thinking... request already sent to the LLM, waiting for response.", m.spinner.View())
}

func (m *model) syncCommandPalette() {
	value := strings.TrimSpace(m.input.Value())
	if strings.HasPrefix(value, "/") {
		m.commandOpen = true
		items := m.filteredCommands()
		if len(items) == 0 {
			m.commandCursor = 0
		} else if m.commandCursor >= len(items) {
			m.commandCursor = len(items) - 1
		}
		return
	}
	m.commandOpen = false
	m.commandCursor = 0
}

func (m model) filteredCommands() []commandItem {
	value := strings.TrimSpace(m.input.Value())
	if value == "" || value == "/" {
		return visibleCommandItems()
	}
	items := visibleCommandItems()
	result := make([]commandItem, 0, len(items))
	for _, item := range items {
		if strings.HasPrefix(item.Name, value) || strings.HasPrefix(item.Usage, value) {
			result = append(result, item)
		}
	}
	return result
}

func visibleCommandItems() []commandItem {
	items := make([]commandItem, 0, len(commandItems))
	for _, item := range commandItems {
		if strings.HasPrefix(item.Name, "/plan") {
			continue
		}
		items = append(items, item)
	}
	return items
}

func (m *model) setInputValue(value string) {
	m.input.SetValue(value)
	m.input.CursorEnd()
}

func shouldExecuteFromPalette(item commandItem) bool {
	switch item.Name {
	case "/help":
		return true
	default:
		return false
	}
}

func (m model) legacyHelpText() string {
	return strings.Join([]string{
		"进入方式",
		"在仓库根目录运行 `go run ./cmd/bytemind chat` 即可启动。",
		"`go run ./cmd/bytemind chat` 会先进入带大 Logo 的启动页。",
		"在启动页输入内容并按 Enter 后，会进入正式聊天界面。",
		"`go run ./cmd/bytemind run -prompt \"...\"` 可用于一次性执行模式。",
		"",
		"斜杠命令",
		"/help: 查看帮助说明。",
		"/session: 查看当前会话 ID、工作区和更新时间。",
		"/sessions [limit]: 查看最近的历史会话列表。",
		"/resume <id>: 按完整 ID 或前缀恢复一个会话。",
		"/new: 新建一个会话。",
		"/plan: 查看当前计划。",
		"/plan create step one | step two | step three: 创建新计划。",
		"/plan add <step>: 追加一个计划步骤。",
		"/plan start <index>: 将步骤标记为进行中。",
		"/plan done <index>: 将步骤标记为已完成。",
		"/plan pending <index>: 将步骤重新标记为待处理。",
		"/plan clear: 清空当前计划。",
		"/quit: 退出 TUI。",
		"",
		"当前界面",
		"启动页是一个居中的 Logo 加输入框。",
		"主界面只显示用户消息和助手回复，不把聊天内容塞到旁边区域。",
		"顶部状态栏会显示工作区、provider、model、审批策略和当前状态。",
		"如果需要 shell 审批，会以弹窗方式暂停等待确认。",
	}, "\n")
}

func (m model) helpText() string {
	return strings.Join([]string{
		"Entry points",
		"Run `go run ./cmd/bytemind chat` to start the TUI.",
		"Run `go run ./cmd/bytemind run -prompt \"...\"` for one-shot execution.",
		"",
		"Slash commands",
		"/help: Show this help text.",
		"/session: Show the current session id, workspace, and update time.",
		"/sessions [limit]: List recent sessions.",
		"/resume <id>: Resume an existing session by id or prefix.",
		"/new: Create a new session in the current workspace.",
		"/quit: Exit the TUI.",
		"",
		"Current layout",
		"The startup screen shows a centered logo and input box.",
		"The main screen is a single chat panel for user, assistant, and tool messages.",
		"The top status bar shows workspace, provider, model, approval policy, and current state.",
		"Approval requests pause the UI and wait for confirmation in a modal.",
	}, "\n")
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
		m.input.Placeholder = "Ask AICoding to inspect, change, or verify this workspace..."
	} else {
		m.input.Placeholder = "Continue the conversation..."
	}
	m.input.Prompt = "> "
}

func connectionState(connected bool) string {
	if connected {
		return "ready"
	}
	return "unknown"
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
	if limit <= 0 || len(text) <= limit {
		return text
	}
	if limit <= 3 {
		return text[:limit]
	}
	return text[:limit-3] + "..."
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
