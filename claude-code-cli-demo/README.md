# Claude Code CLI 演示程序 - 运行指南

## 程序概述

这是一个使用 Go 语言和 Bubble Tea 框架构建的高性能 CLI 交互演示程序。程序模拟了 AI 分析项目结构的过程，具有优雅的动画过渡和 Claude Code 风格的界面设计。

## 运行要求

- Go 1.24+ 环境
- 支持 ANSI 转义序列的终端（Windows Terminal、iTerm2、GNOME Terminal 等）

## 运行方式

### 1. 打开终端
请打开您的本地终端应用程序（如 Windows Terminal、PowerShell、CMD 等）。

### 2. 进入项目目录
```bash
cd /c/Users/ASUS/claude-code-cli-demo
```

### 3. 运行程序
```bash
go run main.go
```

### 4. 退出程序
- 按 `q` 键
- 或按 `Ctrl+C`

## 预期效果

1. **启动界面**（持续约3秒）：
   - 紫色动态 Spinner 动画
   - "正在分析项目结构..." 文本
   - 已耗时显示

2. **结果界面**：
   - 紫色圆角边框的结果框
   - 项目分析报告
   - Claude Code 风格的设计

## 程序特性

✅ **MVU架构** - 声明式状态管理
✅ **Claude Code 审美** - 紫色(#7D56F4)主色调
✅ **平滑动画** - Spinner 加载 + 自动过渡
✅ **响应式设计** - 适应不同终端尺寸
✅ **优雅退出** - 支持键盘快捷键

## 常见问题

### Q: 程序卡在"初始化中..."怎么办？
A: 请确保在**本地终端**中运行，而不是通过 IDE 的内置终端或后台进程。

### Q: 看不到颜色或样式？
A: 请使用支持 ANSI 颜色的终端，如 Windows Terminal。

### Q: 如何修改加载时间？
A: 编辑 `main.go` 第120行，将 `3*time.Second` 改为您想要的时间。

## 代码结构

```
main.go
├── 样式定义 (lipgloss)
├── 消息类型定义
├── Model 结构体 (状态管理)
├── Init() 方法 (初始化)
├── Update() 方法 (消息处理)
├── View() 方法 (界面渲染)
└── main() 函数 (程序入口)
```

## 技术栈

- **bubbletea** - TUI 框架 (MVU 架构)
- **lipgloss** - 样式处理 (CSS-in-TUI)
- **bubbles** - 组件库 (Spinner)

## 扩展建议

1. 添加更多交互组件（输入框、列表等）
2. 实现真实的项目分析逻辑
3. 添加配置文件支持
4. 支持更多键盘快捷键
5. 添加主题切换功能

---

**注意**：这是一个演示程序，展示了高性能 CLI 交互设计的最佳实践。代码注释详尽，便于学习和扩展。