package ui

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type ChatMessage struct {
	Role    string
	Content string
}

type BubbleChatUI struct {
	program *tea.Program
}

type bubbleModel struct {
	width      int
	height     int
	workspace  string
	modelName  string
	mode       string
	tools      []string
	status     string
	messages   []ChatMessage
	input      textinput.Model
	viewport   viewport.Model
	promptKind string
	promptText string
	responseCh chan string
}

type appendMessageMsg struct {
	role    string
	content string
}

type setMetaMsg struct {
	workspace string
	model     string
	mode      string
	tools     []string
}

type setStatusMsg struct{ status string }

type promptRequestMsg struct {
	prompt string
	kind   string
	resp   chan string
}

type quitMsg struct{}

func NewBubbleChatUI(_ io.Reader, out io.Writer) *BubbleChatUI {
	input := textinput.New()
	input.Placeholder = "Type a request, for example: create todo.html"
	input.Focus()
	input.CharLimit = 4000
	input.Prompt = "> "
	input.Width = 80

	vp := viewport.New(80, 20)
	vp.SetContent("")

	model := bubbleModel{
		status:   "Ready",
		mode:     "analyze",
		input:    input,
		viewport: vp,
		messages: nil,
	}

	program := tea.NewProgram(model, tea.WithAltScreen(), tea.WithOutput(out))
	return &BubbleChatUI{program: program}
}

func (u *BubbleChatUI) Run() error {
	_, err := u.program.Run()
	return err
}

func (u *BubbleChatUI) Stop() {
	u.program.Send(quitMsg{})
}

func (u *BubbleChatUI) Configure(workspace, model, mode string, tools []string) {
	u.program.Send(setMetaMsg{workspace: workspace, model: model, mode: mode, tools: append([]string(nil), tools...)})
}

func (u *BubbleChatUI) SetModeInfo(mode string, tools []string) {
	u.program.Send(setMetaMsg{mode: mode, tools: append([]string(nil), tools...)})
}

func (u *BubbleChatUI) SetStatus(status string) {
	u.program.Send(setStatusMsg{status: status})
}

func (u *BubbleChatUI) AddMessage(role, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	u.program.Send(appendMessageMsg{role: role, content: content})
}

func (u *BubbleChatUI) Printf(format string, args ...any) {
	u.AddMessage("system", fmt.Sprintf(format, args...))
}

func (u *BubbleChatUI) Println(args ...any) {
	u.AddMessage("system", fmt.Sprint(args...))
}

func (u *BubbleChatUI) PromptYesNo(question string) (bool, error) {
	resp := make(chan string, 1)
	u.program.Send(promptRequestMsg{prompt: question + " [y/N]", kind: "yesno", resp: resp})
	answer := strings.ToLower(strings.TrimSpace(<-resp))
	return answer == "y" || answer == "yes", nil
}

func (u *BubbleChatUI) PromptLine(prompt string) (string, error) {
	resp := make(chan string, 1)
	u.program.Send(promptRequestMsg{prompt: prompt, kind: "line", resp: resp})
	return <-resp, nil
}

func (u *BubbleChatUI) PrintBox(title string, lines ...string) {
	u.AddMessage(strings.ToLower(title), strings.Join(lines, "\n"))
}

func (m bubbleModel) Init() tea.Cmd { return nil }

func (m bubbleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.input.Width = max(20, msg.Width-6)
		m.viewport.Width = max(20, msg.Width-6)
		m.viewport.Height = max(8, msg.Height-14)
		m.syncViewport()
	case appendMessageMsg:
		m.messages = append(m.messages, ChatMessage{Role: msg.role, Content: msg.content})
		if len(m.messages) > 80 {
			m.messages = m.messages[len(m.messages)-80:]
		}
		m.syncViewport()
	case setMetaMsg:
		if msg.workspace != "" {
			m.workspace = msg.workspace
		}
		if msg.model != "" {
			m.modelName = msg.model
		}
		if msg.mode != "" {
			m.mode = msg.mode
		}
		if msg.tools != nil {
			m.tools = append([]string(nil), msg.tools...)
		}
	case setStatusMsg:
		if strings.TrimSpace(msg.status) == "" {
			m.status = "Ready"
		} else {
			m.status = msg.status
		}
	case promptRequestMsg:
		m.promptKind = msg.kind
		m.promptText = msg.prompt
		m.responseCh = msg.resp
		m.input.SetValue("")
		m.input.Focus()
		if msg.kind == "yesno" {
			m.status = "Approval required"
			m.messages = append(m.messages, ChatMessage{Role: "approval", Content: msg.prompt})
		} else {
			m.status = "Input required"
		}
		m.syncViewport()
	case quitMsg:
		return m, tea.Quit
	case tea.KeyMsg:
		if m.responseCh != nil && m.promptKind == "line" {
			switch msg.String() {
			case "f2", "ctrl+a":
				return m.submitPromptValue("/mode analyze")
			case "f3", "ctrl+f":
				return m.submitPromptValue("/mode full")
			}
		}
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "enter":
			if m.responseCh != nil {
				return m.submitPromptValue(strings.TrimSpace(m.input.Value()))
			}
		}
	}
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m bubbleModel) View() string {
	if m.width == 0 {
		return "Loading ForgeCLI Bubble TUI..."
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	panelStyle := lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(0, 1)
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	activeTab := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("35")).Padding(0, 1)
	inactiveTab := lipgloss.NewStyle().Foreground(lipgloss.Color("252")).Background(lipgloss.Color("238")).Padding(0, 1)

	analyzeTab := inactiveTab.Render("Analyze")
	fullTab := inactiveTab.Render("Full")
	if strings.EqualFold(m.mode, "analyze") {
		analyzeTab = activeTab.Render("Analyze")
	} else if strings.EqualFold(m.mode, "full") {
		fullTab = activeTab.Render("Full")
	}
	modeTabs := lipgloss.JoinHorizontal(lipgloss.Left, analyzeTab, " ", fullTab)

	top := lipgloss.JoinHorizontal(lipgloss.Top,
		panelStyle.Width(max(24, m.width/3)).Render(strings.Join([]string{
			headerStyle.Render("Workspace"),
			fallbackText(m.workspace, "-"),
			"",
			headerStyle.Render("Model"),
			fallbackText(m.modelName, "-"),
		}, "\n")),
		panelStyle.Width(max(24, m.width/4)).Render(strings.Join([]string{
			headerStyle.Render("Mode"),
			modeTabs,
			mutedStyle.Render("F2 / Ctrl+A = Analyze"),
			mutedStyle.Render("F3 / Ctrl+F = Full"),
			"",
			headerStyle.Render("Status"),
			fallbackText(m.status, "Ready"),
		}, "\n")),
		panelStyle.Width(max(28, m.width-(max(24, m.width/3)+max(24, m.width/4)+6))).Render(strings.Join([]string{
			headerStyle.Render("Tools"),
			fallbackText(strings.Join(m.tools, ", "), "-"),
			"",
			mutedStyle.Render("Commands: /help  /tools  /mode analyze  /mode full  /reset  /exit"),
		}, "\n")),
	)

	conversation := panelStyle.Width(m.width - 2).Height(max(10, m.height-10)).Render(m.viewport.View())
	prompt := fallbackText(m.promptText, "> Type a message")
	inputBox := panelStyle.Width(m.width - 2).Render(headerStyle.Render("Input") + "\n" + mutedStyle.Render(prompt) + "\n" + m.input.View())
	return lipgloss.JoinVertical(lipgloss.Left, top, conversation, inputBox)
}

func (m *bubbleModel) syncViewport() {
	lines := make([]string, 0, len(m.messages)*2)
	for _, message := range m.messages {
		lines = append(lines, formatMessage(message.Role, message.Content), "")
	}
	m.viewport.SetContent(strings.TrimSpace(strings.Join(lines, "\n")))
	m.viewport.GotoBottom()
}

func (m bubbleModel) submitPromptValue(value string) (tea.Model, tea.Cmd) {
	if value == "" && m.promptKind == "yesno" {
		value = "n"
	}
	if value != "" {
		m.messages = append(m.messages, ChatMessage{Role: "user", Content: value})
	}
	resp := m.responseCh
	m.responseCh = nil
	m.promptKind = ""
	m.promptText = ""
	m.status = "Ready"
	m.input.SetValue("")
	m.syncViewport()
	return m, func() tea.Msg {
		resp <- value
		return nil
	}
}

func formatMessage(role, content string) string {
	label := strings.ToUpper(strings.TrimSpace(role))
	if label == "" {
		label = "SYSTEM"
	}
	return label + ":\n" + content
}

func fallbackText(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
