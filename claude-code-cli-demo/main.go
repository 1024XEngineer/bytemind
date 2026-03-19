package main

import (
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// 定义颜色常量
const (
	PrimaryColor   = "#7D56F4" // Claude Code 主色调紫色
	TextColor      = "#FFFFFF"
	BackgroundColor = "#0D1117" // 深色背景，类似 Claude Code
)

// 样式定义
var (
	// 主样式
	primaryStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(PrimaryColor))

	// 加载动画样式
	spinnerStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(PrimaryColor)).
			MarginRight(2)

	// 加载文本样式
	loadingTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(TextColor)).
				Bold(true)

	// 结果框样式
	resultBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(PrimaryColor)).
			Padding(1, 2).
			Margin(1, 0).
			Width(60).
			Background(lipgloss.Color(BackgroundColor)).
			Foreground(lipgloss.Color(TextColor))

	// 标题样式
	titleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color(PrimaryColor)).
			Bold(true).
			MarginBottom(1)

	// 内容样式
	contentStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C9D1D9")). // 浅灰色文本
			MarginBottom(1)
)

// LoadingDoneMsg 表示加载完成的消息
type LoadingDoneMsg struct{}

// Model 是应用程序的状态
type Model struct {
	spinner    spinner.Model
	loading    bool
	loadingMsg string
	result     string
	width      int
	height     int
	startTime  time.Time
}

// NewModel 初始化模型
func NewModel() Model {
	// 创建 spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = spinnerStyle

	return Model{
		spinner:    s,
		loading:    true,
		loadingMsg: "正在分析项目结构...",
		result: `项目分析完成！

• 语言分布: Go (85%), Markdown (10%), Shell (5%)
• 代码复杂度: 中等
• 测试覆盖率: 78%
• 依赖项: 12个直接依赖
• 建议: 添加更多单元测试

分析耗时: 2.8秒`,
		startTime: time.Now(),
	}
}

// Init 初始化命令
func (m Model) Init() tea.Cmd {
	// 同时启动 spinner 和 3秒定时器
	return tea.Batch(
		m.spinner.Tick,
		m.startTimer(),
	)
}

// startTimer 启动一个3秒的定时器
func (m Model) startTimer() tea.Cmd {
	return tea.Tick(3*time.Second, func(t time.Time) tea.Msg {
		return LoadingDoneMsg{}
	})
}

// Update 处理消息并更新模型
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		// 更新窗口尺寸
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		// 按 q 或 ctrl+c 退出
		if msg.Type == tea.KeyCtrlC || msg.String() == "q" {
			return m, tea.Quit
		}

	case LoadingDoneMsg:
		// 加载完成，切换到结果界面
		m.loading = false
		return m, nil
	}

	// 如果还在加载中，更新 spinner
	if m.loading {
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	return m, nil
}

// View 渲染界面
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "初始化中..."
	}

	// 居中容器
	container := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center).
		Render

	if m.loading {
		// 加载界面
		loadingContent := lipgloss.JoinVertical(
			lipgloss.Center,
			spinnerStyle.Render(m.spinner.View()),
			loadingTextStyle.Render(m.loadingMsg),
			lipgloss.NewStyle().Foreground(lipgloss.Color("#8B949E")).Render(fmt.Sprintf("已耗时: %.1f秒", time.Since(m.startTime).Seconds())),
		)
		return container(loadingContent)
	}

	// 结果界面
	resultContent := lipgloss.JoinVertical(
		lipgloss.Left,
		titleStyle.Render("📊 项目分析报告"),
		contentStyle.Render(m.result),
		lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8B949E")).
			Italic(true).
			Render("按 q 或 Ctrl+C 退出"),
	)

	// 将结果框居中显示
	resultBox := resultBoxStyle.Render(resultContent)
	return container(resultBox)
}

func main() {
	// 创建 Bubble Tea 程序
	p := tea.NewProgram(
		NewModel(),
		tea.WithAltScreen(), // 使用全屏模式
	)

	// 启动程序
	if _, err := p.Run(); err != nil {
		fmt.Printf("程序出错: %v\n", err)
	}
}