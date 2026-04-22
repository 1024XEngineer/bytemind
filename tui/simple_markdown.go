package tui

import (
	"fmt"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// SimpleMarkdownRenderer - 简约的Markdown渲染器
type SimpleMarkdownRenderer struct {
	style *MarkdownStyleConfig
	width int
	cache *SimpleCache
}

// MarkdownStyleConfig - 简约的样式配置
type MarkdownStyleConfig struct {
	// 基础颜色
	TextColor    lipgloss.Color
	AccentColor  lipgloss.Color
	MutedColor   lipgloss.Color
	SuccessColor lipgloss.Color
	WarningColor lipgloss.Color
	ErrorColor   lipgloss.Color

	// 代码块颜色
	CodeBgColor     lipgloss.Color
	CodeBorderColor lipgloss.Color

	// 高亮颜色
	HighlightColors map[string]lipgloss.Color
}

// SimpleCache - 简约的缓存机制
type SimpleCache struct {
	cache map[string]string
	mutex sync.RWMutex
}

// NewSimpleCache 创建新的简单缓存
func NewSimpleCache() *SimpleCache {
	return &SimpleCache{
		cache: make(map[string]string),
	}
}

// Get 从缓存获取值
func (c *SimpleCache) Get(key string) (string, bool) {
	c.mutex.RLock()
	defer c.mutex.RUnlock()
	value, ok := c.cache[key]
	return value, ok
}

// Set 设置缓存值
func (c *SimpleCache) Set(key, value string) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.cache[key] = value
}

// NewSimpleMarkdownRenderer 创建新的简约Markdown渲染器
func NewSimpleMarkdownRenderer(width int) *SimpleMarkdownRenderer {
	// 默认样式配置
	config := &MarkdownStyleConfig{
		TextColor:       semanticColors.TextBase,
		AccentColor:     semanticColors.Accent,
		MutedColor:      semanticColors.TextMuted,
		SuccessColor:    semanticColors.Success,
		WarningColor:    semanticColors.Warning,
		ErrorColor:      semanticColors.Danger,
		CodeBgColor:     semanticColors.CodeBg,
		CodeBorderColor: semanticColors.CodeBorder,
		HighlightColors: map[string]lipgloss.Color{
			"yellow": semanticColors.HighlightYellow,
			"blue":   semanticColors.HighlightBlue,
			"green":  semanticColors.HighlightGreen,
			"red":    semanticColors.HighlightRed,
			"purple": semanticColors.HighlightPurple,
			"orange": semanticColors.HighlightOrange,
		},
	}

	return &SimpleMarkdownRenderer{
		style: config,
		width: width,
		cache: NewSimpleCache(),
	}
}

// Render 渲染Markdown文本
func (r *SimpleMarkdownRenderer) Render(markdown string) string {
	// 检查缓存
	if cached, ok := r.cache.Get(markdown); ok {
		return cached
	}

	// 解析Markdown
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	reader := text.NewReader([]byte(markdown))
	document := md.Parser().Parse(reader)

	// 渲染
	result := r.renderNode(document, reader.Source())

	// 缓存结果
	r.cache.Set(markdown, result)

	return result
}

// renderNode 渲染单个节点
func (r *SimpleMarkdownRenderer) renderNode(node gast.Node, source []byte) string {
	var result strings.Builder

	// 遍历子节点
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		result.WriteString(r.renderNodeInternal(child, source))
	}

	return result.String()
}

// renderNodeInternal 渲染节点内部实现
func (r *SimpleMarkdownRenderer) renderNodeInternal(node gast.Node, source []byte) string {
	switch n := node.(type) {
	case *gast.Heading:
		return r.renderHeading(n, source)
	case *gast.Paragraph:
		return r.renderParagraph(n, source)
	case *gast.FencedCodeBlock:
		return r.renderCodeBlock(n, source)
	case *gast.CodeBlock:
		return r.renderCodeBlock(n, source)
	case *gast.List:
		return r.renderList(n, source)
	case *gast.Blockquote:
		return r.renderBlockquote(n, source)
	// Skip table rendering for simplicity
	case *gast.ThematicBreak:
		return r.renderThematicBreak()
	case *gast.Text:
		return r.processText(string(n.Text(source)))
	case *gast.Emphasis:
		return r.renderEmphasis(n, source)
	case *gast.String:
		return r.processText(string(n.Value))
	case *gast.Link:
		return r.renderLink(n, source)
	case *gast.CodeSpan:
		return r.renderCodeSpan(n, source)
	case *gast.HTMLBlock:
		return "" // 跳过HTML块
	default:
		return r.renderNode(node, source)
	}
}

// renderHeading 渲染标题
func (r *SimpleMarkdownRenderer) renderHeading(node *gast.Heading, source []byte) string {
	text := r.collectText(node, source)

	// 根据级别添加不同的符号
	var prefix string
	switch node.Level {
	case 1:
		prefix = "◆ " // 使用Unicode符号
	case 2:
		prefix = "◇ "
	case 3:
		prefix = "• "
	case 4:
		prefix = "· "
	default:
		prefix = "• "
	}

	style := r.style.Heading(node.Level)
	return fmt.Sprintf("%s%s\n", style.Render(prefix+text), prefix)
}

// renderParagraph 渲染段落
func (r *SimpleMarkdownRenderer) renderParagraph(node *gast.Paragraph, source []byte) string {
	text := r.collectText(node, source)
	processedText := r.processHighlights(text)
	return fmt.Sprintf("%s\n\n", r.style.Text().Render(processedText))
}

// renderCodeBlock 渲染代码块
func (r *SimpleMarkdownRenderer) renderCodeBlock(node gast.Node, source []byte) string {
	var language string
	var code string

	switch n := node.(type) {
	case *gast.FencedCodeBlock:
		language = string(n.Language(source))
		code = string(n.Lines().Value(source))
	default:
		code = string(node.Lines().Value(source))
	}

	// 添加语法高亮
	highlightedCode := r.highlightCode(code, language)

	// 构建更美观的代码块
	// 使用更丰富的边框样式
	borderWidth := r.width - 4
	if borderWidth < 20 {
		borderWidth = 20
	}

	// 顶部边框，包含语言信息
	if language == "" {
		language = "code"
	}

	result := fmt.Sprintf("┌─ %s ", language)
	result += strings.Repeat("─", max(0, borderWidth-len(language)-8))
	result += "┐\n"

	// 代码内容，每行添加行号和边框
	lines := strings.Split(highlightedCode, "\n")
	lineNumWidth := len(fmt.Sprintf("%d", len(lines)))

	for i, line := range lines {
		lineNum := i + 1
		lineNumStr := fmt.Sprintf("%*d", lineNumWidth, lineNum)
		result += fmt.Sprintf("│ %s │ %-*s │\n", lineNumStr, borderWidth-lineNumWidth-6, line)
	}

	// 底部边框
	result += "└"
	result += strings.Repeat("─", max(0, borderWidth-2))
	result += "┘\n"

	return r.style.Code().Render(result) + "\n"
}

// highlightCode 代码语法高亮
func (r *SimpleMarkdownRenderer) highlightCode(code, language string) string {
	// 简化实现，只添加语言标识，不进行复杂的语法高亮
	// 这样保持了简洁性，同时提供了基本的代码标识
	return code
}

// renderList 渲染列表
func (r *SimpleMarkdownRenderer) renderList(node *gast.List, source []byte) string {
	var result strings.Builder

	isOrdered := node.IsOrdered()
	start := node.Start
	index := start

	// 根据列表层级选择不同的符号
	level := 0 // 可以根据节点层级来设置，这里简化处理
	bulletGlyph := "• "
	if level > 0 {
		bulletGlyph = "◦ "
	}

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if item, ok := child.(*gast.ListItem); ok {
			text := r.collectText(item, source)

			// 检查是否是任务列表项
			if firstChild := item.FirstChild(); firstChild != nil {
				if textNode, ok := firstChild.(*gast.Text); ok {
					textStr := string(textNode.Text(source))
					if strings.HasPrefix(textStr, "[ ] ") {
						text = "☐ " + textStr[3:] + "\n"
					} else if strings.HasPrefix(textStr, "[x] ") {
						text = "✓ " + textStr[3:] + "\n"
					}
				}
			}

			if isOrdered {
				result.WriteString(fmt.Sprintf("%d. %s\n", index, text))
				index++
			} else {
				result.WriteString(fmt.Sprintf("%s%s\n", bulletGlyph, text))
			}
		}
	}

	return result.String() + "\n"
}

// renderBlockquote 渲染引用块
func (r *SimpleMarkdownRenderer) renderBlockquote(node *gast.Blockquote, source []byte) string {
	text := r.collectText(node, source)
	lines := strings.Split(text, "\n")

	var result strings.Builder
	for _, line := range lines {
		if strings.TrimSpace(line) != "" {
			// 使用更美观的引用符号
			result.WriteString(fmt.Sprintf("▍ %s\n", line))
		} else {
			result.WriteString("▍\n")
		}
	}

	return result.String() + "\n"
}

// renderTable 渲染表格
func (r *SimpleMarkdownRenderer) renderTable(node gast.Node, source []byte) string {
	// 简单的表格渲染实现
	var result strings.Builder

	// 这里需要解析表格结构，但由于goldmark的表格在extension中
	// 我们使用一个简化的实现，只显示表格内容
	text := r.collectText(node, source)
	lines := strings.Split(text, "\n")

	if len(lines) == 0 {
		return ""
	}

	// 尝试解析表格行和列
	var tableRows [][]string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// 简单的分割，假设使用|作为分隔符
		if strings.Contains(line, "|") {
			parts := strings.Split(line, "|")
			// 移除首尾的空元素
			if len(parts) > 2 {
				parts = parts[1 : len(parts)-1]
				// 清理每个单元格的内容
				for i, part := range parts {
					parts[i] = strings.TrimSpace(part)
				}
				tableRows = append(tableRows, parts)
			}
		}
	}

	if len(tableRows) == 0 {
		// 如果解析失败，返回原始文本
		return text + "\n"
	}

	// 计算每列的最大宽度
	maxCols := 0
	for _, row := range tableRows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	colWidths := make([]int, maxCols)
	for i := 0; i < maxCols; i++ {
		maxWidth := 10 // 最小宽度
		for _, row := range tableRows {
			if i < len(row) {
				width := len(row[i])
				if width > maxWidth {
					maxWidth = width
				}
			}
		}
		colWidths[i] = maxWidth
	}

	// 渲染表格
	// 顶部边框
	result.WriteString("┌")
	for i, width := range colWidths {
		result.WriteString(strings.Repeat("─", width+2)) // +2 for padding
		if i < len(colWidths)-1 {
			result.WriteString("┬")
		}
	}
	result.WriteString("┐\n")

	// 渲染表格内容
	for rowIndex, row := range tableRows {
		result.WriteString("│")
		for colIndex, width := range colWidths {
			cellContent := ""
			if colIndex < len(row) {
				cellContent = row[colIndex]
			}
			// 添加空格填充
			padding := width - len(cellContent) + 1
			result.WriteString(" " + cellContent + strings.Repeat(" ", padding))
			result.WriteString("│")
		}
		result.WriteString("\n")

		// 在表头后添加分隔线
		if rowIndex == 0 {
			result.WriteString("├")
			for i, width := range colWidths {
				result.WriteString(strings.Repeat("─", width+2))
				if i < len(colWidths)-1 {
					result.WriteString("┼")
				}
			}
			result.WriteString("┤\n")
		}
	}

	// 底部边框
	result.WriteString("└")
	for i, width := range colWidths {
		result.WriteString(strings.Repeat("─", width+2))
		if i < len(colWidths)-1 {
			result.WriteString("┴")
		}
	}
	result.WriteString("┘\n")

	return result.String()
}

// renderThematicBreak 渲染分隔线
func (r *SimpleMarkdownRenderer) renderThematicBreak() string {
	// 使用更美观的分隔线样式
	rule := strings.Repeat("─", max(0, r.width))
	return fmt.Sprintf("\n%s\n\n", rule)
}

// renderEmphasis 渲染强调
func (r *SimpleMarkdownRenderer) renderEmphasis(node *gast.Emphasis, source []byte) string {
	text := r.collectText(node, source)
	return r.style.Emphasis().Render(text)
}

// renderStrong 渲染粗体
func (r *SimpleMarkdownRenderer) renderStrong(node *gast.Emphasis, source []byte) string {
	text := r.collectText(node, source)
	return r.style.Strong().Render(text)
}

// renderDel 渲染删除线
func (r *SimpleMarkdownRenderer) renderDel(node *gast.Text, source []byte) string {
	text := string(node.Text(source))
	return r.style.Strikethrough().Render(text)
}

// renderLink 渲染链接
func (r *SimpleMarkdownRenderer) renderLink(node *gast.Link, source []byte) string {
	text := r.collectText(node, source)
	url := string(node.Destination)
	return r.style.Link().Render(fmt.Sprintf("%s (%s)", text, url))
}

// renderCodeSpan 渲染内联代码
func (r *SimpleMarkdownRenderer) renderCodeSpan(node *gast.CodeSpan, source []byte) string {
	text := string(node.Text(source))
	return r.style.InlineCode().Render(text)
}

// processText 处理文本
func (r *SimpleMarkdownRenderer) processText(text string) string {
	return text // 简化实现
}

// processHighlights 处理文本高亮
func (r *SimpleMarkdownRenderer) processHighlights(text string) string {
	// 处理 ==text== 高亮
	text = strings.ReplaceAll(text, "==", "")

	// 简化实现，只处理基本的高亮
	return text
}

// collectText 收集节点中的所有文本
func (r *SimpleMarkdownRenderer) collectText(node gast.Node, source []byte) string {
	var result strings.Builder

	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if textNode, ok := child.(*gast.Text); ok {
			result.WriteString(string(textNode.Text(source)))
		} else {
			result.WriteString(r.renderNodeInternal(child, source))
		}
	}

	return result.String()
}

// 样式工厂方法

// Heading 返回标题样式
func (s *MarkdownStyleConfig) Heading(level int) lipgloss.Style {
	style := lipgloss.NewStyle().Foreground(s.TextColor).Bold(true)

	switch level {
	case 1:
		return style.Foreground(s.AccentColor).Underline(true)
	case 2:
		return style.Foreground(s.AccentColor)
	case 3:
		return style.Foreground(s.MutedColor)
	default:
		return style.Foreground(s.MutedColor)
	}
}

// Text 返回普通文本样式
func (s *MarkdownStyleConfig) Text() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.TextColor)
}

// Code 返回代码块样式
func (s *MarkdownStyleConfig) Code() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(s.TextColor).
		Background(s.CodeBgColor).
		Padding(0, 1)
}

// Emphasis 返回强调样式
func (s *MarkdownStyleConfig) Emphasis() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.AccentColor).Italic(true)
}

// Strong 返回粗体样式
func (s *MarkdownStyleConfig) Strong() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.TextColor).Bold(true)
}

// Strikethrough 返回删除线样式
func (s *MarkdownStyleConfig) Strikethrough() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.MutedColor).Strikethrough(true)
}

// Link 返回链接样式
func (s *MarkdownStyleConfig) Link() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(s.AccentColor).Underline(true)
}

// InlineCode 返回内联代码样式
func (s *MarkdownStyleConfig) InlineCode() lipgloss.Style {
	return lipgloss.NewStyle().
		Foreground(s.TextColor).
		Background(s.CodeBgColor).
		Bold(true)
}

// Highlight 返回高亮样式
func (s *MarkdownStyleConfig) Highlight(color string) lipgloss.Style {
	if c, ok := s.HighlightColors[color]; ok {
		return lipgloss.NewStyle().Background(c)
	}
	return lipgloss.NewStyle().Background(s.HighlightColors["yellow"])
}
