package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ==========================================
// 1. 全局样式系统 (Theme & Styles)
// ==========================================

type Theme struct {
	Primary lipgloss.Color
	Accent  lipgloss.Color
	Error   lipgloss.Color
	Warning lipgloss.Color
	User    lipgloss.Color
	AI      lipgloss.Color
	Tool    lipgloss.Color
	Bg      lipgloss.Color
	Muted   lipgloss.Color
}

var DefaultTheme = Theme{
	Primary: lipgloss.Color("#4C96FF"),
	Accent:  lipgloss.Color("#BD93F9"),
	Error:   lipgloss.Color("#FF5555"),
	Warning: lipgloss.Color("#FFB86C"), // 期待时的亮橙色
	User:    lipgloss.Color("#8BE9FD"),
	AI:      lipgloss.Color("#BD93F9"),
	Tool:    lipgloss.Color("#6272A4"),
	Bg:      lipgloss.Color("#282A36"),
	Muted:   lipgloss.Color("#44475A"),
}

var (
	// 输出框（聊天区）样式：使用 Primary 蓝色，并且加粗 (Bold) 显得更粗
	chatBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(DefaultTheme.Primary).
			Bold(true) // 加粗边框
	// 输入框（底部区）样式：使用 Accent 紫色，进行视觉隔离，同样加粗
	inputBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(DefaultTheme.Accent).
			Bold(true) // 加粗边框

	viewportStyle       = lipgloss.NewStyle().Padding(0, 1)
	msgUserStyle        = lipgloss.NewStyle().Foreground(DefaultTheme.User).Bold(true)
	msgAIStyle          = lipgloss.NewStyle().Foreground(DefaultTheme.AI)
	msgToolStyle        = lipgloss.NewStyle().Foreground(DefaultTheme.Tool).Italic(true)
	msgErrorBox         = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(DefaultTheme.Error).Foreground(DefaultTheme.Error).Padding(0, 1)
	footerBaseStyle     = lipgloss.NewStyle().Padding(0, 1)
	overlayBoxStyle     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(DefaultTheme.Accent).Padding(1, 3).Background(DefaultTheme.Bg)
	paletteItemSelected = lipgloss.NewStyle().Foreground(DefaultTheme.Bg).Background(DefaultTheme.Accent).Bold(true)
	paletteItemNormal   = lipgloss.NewStyle().Foreground(DefaultTheme.AI)

	// 右上角小组件的外框
	widgetBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			Padding(0, 1)
)

// ==========================================
// 2. 状态机与核心数据结构
// ==========================================

type AppState int

const (
	StateSplash AppState = iota
	StateChat
	StateCommandPalette
	StateLoading
)

type InputMode int

const (
	InputNormal InputMode = iota
	InputCommand
	InputFileRef
	InputConfirm
)

type MsgRole string

const (
	RoleUser MsgRole = "user"
	RoleAI   MsgRole = "ai"
	RoleTool MsgRole = "tool"
)

type MsgStatus int

const (
	StatusNormal MsgStatus = iota
	StatusPending
)

type Message struct {
	ID      string
	Role    MsgRole
	Content string
	Status  MsgStatus
}

var mockCommands = []string{
	"/plan     - 🧠 先分析架构，不写代码",
	"/build    - 🏗️ 开始执行构建与写代码",
	"/sessions - 📁 查看与管理历史会话",
	"/model    - 🎛️ 切换底层大语言模型",
	"/help     - 💡 查看所有可用命令",
	"/clear    - 🧹 清空当前对话",
}

type model struct {
	state     AppState
	inputMode InputMode
	width     int
	height    int

	viewport  viewport.Model
	textInput textinput.Model
	spinner   spinner.Model

	messages     []Message
	paletteIndex int
	pendingCmd   string

	avatarFrame int // 动画帧
}

// ==========================================
// 3. 动画与异步事件系统 (核心修复点)
// ==========================================

type animTickMsg time.Time
type bootFinishedMsg struct{}
type delayCompleteMsg struct {
	msgID   string
	content string
}

// 独立的动画引擎：每 250 毫秒强制刷新一次帧数，保证表情实时动态！
func animate() tea.Cmd {
	return tea.Tick(time.Millisecond*250, func(t time.Time) tea.Msg {
		return animTickMsg(t)
	})
}

// ==========================================
// 4. 核心逻辑 (Update)
// ==========================================

func (m model) updateCore(msg tea.Msg) (model, tea.Cmd) {
	var cmds []tea.Cmd
	var tiCmd, vpCmd, spCmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// 聊天区现在占据 100% 宽度！不再切分左右列
		vpWidth := m.width - 4
		// 高度减去边框、Header(占3行)、Footer(占2行)
		vpHeight := m.height - 10
		if vpHeight < 1 {
			vpHeight = 1
		}
		m.viewport = viewport.New(vpWidth, vpHeight)
		m.viewport.Style = viewportStyle
		m.refreshViewport()
		m.textInput.Width = m.width - 6

	// 接收来自动画引擎的信号，刷新表情
	case animTickMsg:
		m.avatarFrame++
		return m, animate() // 持续循环动画

	case bootFinishedMsg:
		if m.state == StateSplash {
			m.state = StateChat
			m.textInput.Focus()
		}
		return m, nil

	case spinner.TickMsg:
		m.spinner, spCmd = m.spinner.Update(msg)
		if m.hasPendingMessage() || m.state == StateSplash {
			m.refreshViewport()
		}
		return m, spCmd

	case delayCompleteMsg:
		m.state = StateChat
		for i := range m.messages {
			if m.messages[i].ID == msg.msgID {
				m.messages[i].Status = StatusNormal
				m.messages[i].Content = msg.content
			}
		}
		m.inputMode = InputNormal
		m.textInput.Placeholder = "输入消息..."
		m.textInput.Focus()
		m.refreshViewport()
		m.viewport.GotoBottom()
		return m, nil

	case tea.KeyMsg:
		if msg.Type == tea.KeyCtrlC {
			return m, tea.Quit
		}

		if m.state == StateCommandPalette {
			switch msg.Type {
			case tea.KeyUp:
				m.paletteIndex = (m.paletteIndex - 1 + len(mockCommands)) % len(mockCommands)
			case tea.KeyDown:
				m.paletteIndex = (m.paletteIndex + 1) % len(mockCommands)
			case tea.KeyEnter:
				selected := strings.Split(mockCommands[m.paletteIndex], " ")[0]
				m.textInput.SetValue(selected)
				m.textInput.CursorEnd()
				m.state = StateChat
				m.syncInputMode()
			case tea.KeyEsc:
				m.state = StateChat
			}
			return m, nil
		}

		if m.inputMode == InputConfirm {
			isY := msg.Type == tea.KeyRunes && len(msg.Runes) > 0 && (msg.Runes[0] == 'y' || msg.Runes[0] == 'Y')
			isN := msg.Type == tea.KeyRunes && len(msg.Runes) > 0 && (msg.Runes[0] == 'n' || msg.Runes[0] == 'N')

			if isY {
				m.addMessage(RoleTool, "✅ 已授权执行: "+m.pendingCmd, StatusNormal)
				m.triggerLoading("模拟执行中...")
				return m, m.simulateDelayCmd()
			} else if isN || msg.Type == tea.KeyEsc {
				m.addMessage(RoleTool, "❌ 已拒绝操作", StatusNormal)
				m.inputMode = InputNormal
				m.textInput.Focus()
			}
			return m, nil
		}

		if m.state == StateChat {
			switch msg.Type {
			case tea.KeyEnter:
				val := strings.TrimSpace(m.textInput.Value())
				if val == "" {
					return m, nil
				}
				m.textInput.SetValue("")
				m.syncInputMode()

				switch val {
				case "/clear":
					m.messages = []Message{}
					m.refreshViewport()
					return m, nil
				case "/plan", "/build", "/sessions", "/model":
					m.addMessage(RoleUser, val, StatusNormal)
					m.addMessage(RoleTool, "执行命令: "+val, StatusNormal)
					return m, nil
				case "/help":
					m.textInput.SetValue("/")
					m.state = StateCommandPalette
					m.paletteIndex = 0
					return m, nil
				}

				if strings.Contains(val, "rm -rf") {
					m.pendingCmd = val
					m.inputMode = InputConfirm
					m.textInput.Blur()
					return m, nil
				}

				m.addMessage(RoleUser, val, StatusNormal)
				m.triggerLoading("思考中...")
				return m, m.simulateDelayCmd()

			case tea.KeyPgUp, tea.KeyPgDown:
				m.viewport, vpCmd = m.viewport.Update(msg)
				return m, vpCmd
			}
		}
	}

	if m.state == StateChat && m.inputMode != InputConfirm {
		m.textInput, tiCmd = m.textInput.Update(msg)
		cmds = append(cmds, tiCmd)
		m.syncInputMode()

		if m.inputMode == InputCommand && m.textInput.Value() == "/" {
			m.state = StateCommandPalette
			m.paletteIndex = 0
		}
	}

	m.viewport, vpCmd = m.viewport.Update(msg)
	cmds = append(cmds, vpCmd)

	return m, tea.Batch(cmds...)
}

// ==========================================
// 5. 接口封装与渲染 (View)
// ==========================================

type App struct{ m model }

func (a App) Init() tea.Cmd {
	return tea.Batch(
		textinput.Blink,
		a.m.spinner.Tick,
		animate(), // 启动时注入独立动画引擎
		tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
			return bootFinishedMsg{}
		}),
	)
}

func (a App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	newModel, cmd := a.m.updateCore(msg)
	a.m = newModel
	return a, cmd
}

func (a App) View() string {
	m := a.m
	if m.width == 0 {
		return "Initializing..."
	}

	if m.state == StateSplash {
		return m.renderSplash()
	}

	// 1. 获取带有右上角小人的新版表头
	header := m.renderHeader()
	// 2. 获取占满宽度的聊天区
	chat := m.viewport.View()
	// 3. 底部输入区
	footer := m.renderFooter()

	// ⭐️ 组装上半部分：输出框（包含表头和聊天记录）
	chatAreaInner := lipgloss.JoinVertical(lipgloss.Left, header, chat)
	topBox := chatBoxStyle.
		Width(m.width - 2). // 减去左右边框的宽度
		Render(chatAreaInner)

	// ⭐️ 组装下半部分：独立的输入框
	bottomBox := inputBoxStyle.
		Width(m.width - 2). // 减去左右边框的宽度
		Render(footer)

	// ⭐️ 纵向拼接两个独立的框
	mainLayout := lipgloss.JoinVertical(lipgloss.Left, topBox, bottomBox)

	if m.state == StateCommandPalette {
		palette := m.renderCommandPalette()
		mainLayout = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, palette)
	}

	return mainLayout
}

// ==========================================
// 6. 辅助函数 (Helpers & UI 组件)
// ==========================================

// 获取表情状态引擎
func (m *model) getAvatarState() (string, string, lipgloss.Color) {
	val := m.textInput.Value()

	// 1. 危险状态：检测到 rm -rf 或确认模式
	if m.inputMode == InputConfirm || strings.Contains(val, "rm -rf") {
		frames := []string{`(°Д°)`, `(O_O;)`, `(!!˚☐˚)`, `(O_O;)`}
		return frames[m.avatarFrame%len(frames)], "DANGER", DefaultTheme.Error
	}

	// 2. 思考状态：Loading 中
	if m.state == StateLoading || m.hasPendingMessage() {
		// 动态戴墨镜的效果
		frames := []string{`( •_•)   `, `( •_•)>⌐ `, `(⌐■_■)   `, `(⌐■_■)✧  `}
		// 让思考的动画稍微慢一点
		return frames[(m.avatarFrame/2)%len(frames)], "Thinking", DefaultTheme.Primary
	}

	// 3. 期待状态：正在输入 (框内有内容)
	if len(val) > 0 {
		frames := []string{`(✧∇✧)`, `(☆ω☆)`, `(✧ω✧)`}
		return frames[m.avatarFrame%len(frames)], "Watching", DefaultTheme.Warning
	}

	// 4. 启动欢迎状态
	if m.state == StateSplash {
		frames := []string{`( ﾟ▽ﾟ)/`, `( ^▽^)/`, `( ﾟ▽ﾟ)/`, `( >▽<)/`}
		return frames[m.avatarFrame%len(frames)], "Welcome", DefaultTheme.Accent
	}

	// 5. 待机状态：偶尔眨眼
	blinkFrame := m.avatarFrame / 4
	frames := []string{`(•‿•)`, `(•‿•)`, `(•‿•)`, `(-‿-)`, `(•‿•)`}
	return frames[blinkFrame%len(frames)], "Ready", DefaultTheme.Accent
}

func (m *model) renderHeader() string {
	// 左侧信息
	logo := lipgloss.NewStyle().Foreground(DefaultTheme.Primary).Bold(true).Render(" ⚡ GOCODE ")
	status := lipgloss.NewStyle().Foreground(DefaultTheme.Muted).Render(fmt.Sprintf("Mode: %v | %s", m.inputMode, time.Now().Format("15:04")))
	leftInfo := lipgloss.JoinVertical(lipgloss.Left, logo, status)

	// 右上角动态小人 Widget
	face, text, color := m.getAvatarState()

	// 固定内部宽度避免闪烁
	widgetContent := lipgloss.NewStyle().Width(18).Align(lipgloss.Center).Render(face + " " + text)
	widget := widgetBoxStyle.BorderForeground(color).Render(widgetContent)

	// 计算左右两边中间需要填补的空格，确保 widget 牢牢贴在右上角
	spaceWidth := m.width - 4 - lipgloss.Width(leftInfo) - lipgloss.Width(widget)
	if spaceWidth < 0 {
		spaceWidth = 0
	}
	spacer := strings.Repeat(" ", spaceWidth)

	// 顶栏合并
	return lipgloss.JoinHorizontal(lipgloss.Top, leftInfo, spacer, widget) + "\n"
}

func (m *model) renderSplash() string {
	face, _, _ := m.getAvatarState()

	title := lipgloss.NewStyle().Foreground(DefaultTheme.Primary).Bold(true).Render("G O C O D E")
	sub := lipgloss.NewStyle().Foreground(DefaultTheme.Muted).Render("Booting Workspace " + m.spinner.View())
	pet := lipgloss.NewStyle().Foreground(DefaultTheme.Accent).Render(face + " WELCOME!")

	content := lipgloss.JoinVertical(lipgloss.Center, title, "\n", sub, "\n", pet)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
}

func (m *model) renderFooter() string {
	style := footerBaseStyle.Copy()
	help := "Enter: 发送 | /: 命令 | @: 文件 | PgUp/Dn: 滚动"

	switch m.inputMode {
	case InputCommand:
		help = "↑/↓: 选择 | Enter: 确认 | Esc: 取消"
	case InputConfirm:
		help = "⚠️ 确认执行危险操作？[Y]es / [N]o"
	}

	input := m.textInput.View()
	if m.state == StateLoading {
		input = lipgloss.JoinHorizontal(lipgloss.Left, m.spinner.View(), " ", msgToolStyle.Render("Agent 正在处理..."))
	} else if m.inputMode == InputConfirm {
		input = msgErrorBox.Render(" ⚡ DANGER: " + m.pendingCmd + " ")
	}

	helpLine := lipgloss.NewStyle().Foreground(DefaultTheme.Muted).Italic(true).Render("  " + help)
	return style.Width(m.width - 4).Render(lipgloss.JoinVertical(lipgloss.Left, "  "+input, helpLine))
}

func (m *model) renderCommandPalette() string {
	var sb strings.Builder
	sb.WriteString(lipgloss.NewStyle().Foreground(DefaultTheme.Accent).Bold(true).Render("✨ Quick Commands") + "\n\n")
	for i, cmd := range mockCommands {
		if i == m.paletteIndex {
			sb.WriteString(paletteItemSelected.Render(" ❯ "+cmd+" ") + "\n")
		} else {
			sb.WriteString(paletteItemNormal.Render("   "+cmd+" ") + "\n")
		}
	}
	return overlayBoxStyle.Render(sb.String())
}

func (m *model) renderMessages() string {
	var sb strings.Builder
	for _, msg := range m.messages {
		switch msg.Role {
		case RoleUser:
			sb.WriteString(msgUserStyle.Render("You ❯ ") + msg.Content + "\n\n")
		case RoleAI:
			prefix := msgAIStyle.Render("Gocode ❯ ")
			if msg.Status == StatusPending {
				sb.WriteString(prefix + m.spinner.View() + " " + lipgloss.NewStyle().Foreground(DefaultTheme.Muted).Render(msg.Content) + "\n\n")
			} else {
				sb.WriteString(prefix + msg.Content + "\n\n")
			}
		case RoleTool:
			sb.WriteString(msgToolStyle.Render("⚡ "+msg.Content) + "\n\n")
		}
	}
	return sb.String()
}

func (m *model) addMessage(role MsgRole, content string, status MsgStatus) {
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	m.messages = append(m.messages, Message{ID: id, Role: role, Content: content, Status: status})
	m.refreshViewport()
	m.viewport.GotoBottom()
}

func (m *model) triggerLoading(text string) {
	m.state = StateLoading
	m.addMessage(RoleAI, text, StatusPending)
	m.textInput.Blur()
}

func (m *model) syncInputMode() {
	val := m.textInput.Value()
	if strings.HasPrefix(val, "/") {
		m.inputMode = InputCommand
	} else if strings.Contains(val, "@") {
		m.inputMode = InputFileRef
	} else {
		m.inputMode = InputNormal
	}
}

func (m *model) refreshViewport() {
	m.viewport.SetContent(m.renderMessages())
}

func (m *model) hasPendingMessage() bool {
	for _, msg := range m.messages {
		if msg.Status == StatusPending {
			return true
		}
	}
	return false
}

func (m *model) simulateDelayCmd() tea.Cmd {
	var targetID string
	for _, msg := range m.messages {
		if msg.Status == StatusPending {
			targetID = msg.ID
			break
		}
	}
	return tea.Tick(1500*time.Millisecond, func(t time.Time) tea.Msg {
		return delayCompleteMsg{
			msgID:   targetID,
			content: "Mock 数据：已成功识别指令并处理完成。你可以继续输入或使用 / 呼出面板。",
		}
	})
}

func main() {
	m := model{
		state:     StateSplash,
		inputMode: InputNormal,
		textInput: textinput.New(),
		spinner:   spinner.New(),
	}
	m.textInput.Placeholder = "输入消息..."
	m.spinner.Spinner = spinner.Dot
	m.spinner.Style = lipgloss.NewStyle().Foreground(DefaultTheme.Accent)

	p := tea.NewProgram(App{m}, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error: %v", err)
		os.Exit(1)
	}
}
