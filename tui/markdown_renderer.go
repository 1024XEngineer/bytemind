package tui

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"unicode"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extensionast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

type markdownSurface string

const (
	markdownSurfaceAssistant markdownSurface = "assistant"
	markdownSurfaceHelp      markdownSurface = "help"
)

type MarkdownRenderResult struct {
	Display string
	Copy    string
	Lines   []markdownRenderLine
}

type markdownRenderLine struct {
	Display string
	Copy    string
}

type markdownLayoutCache struct {
	entries sync.Map // map[string]MarkdownRenderResult
}

type markdownBlockKind string

const (
	markdownBlockParagraph markdownBlockKind = "paragraph"
	markdownBlockHeading   markdownBlockKind = "heading"
	markdownBlockList      markdownBlockKind = "list"
	markdownBlockQuote     markdownBlockKind = "quote"
	markdownBlockCode      markdownBlockKind = "code"
	markdownBlockRule      markdownBlockKind = "rule"
	markdownBlockTable     markdownBlockKind = "table"
	markdownBlockBlank     markdownBlockKind = "blank"
)

type markdownBlock struct {
	Kind     markdownBlockKind
	Level    int
	Spans    []markdownSpan
	Children []markdownBlock
	List     markdownListBlock
	Code     markdownCodeBlock
	Table    markdownTableBlock
}

type markdownListBlock struct {
	Ordered bool
	Start   int
	Items   []markdownListItem
}

type markdownListItem struct {
	TaskState string
	Blocks    []markdownBlock
}

type markdownCodeBlock struct {
	Language string
	Text     string
}

type markdownTableBlock struct {
	Header []markdownTableCell
	Rows   [][]markdownTableCell
}

type markdownTableCell struct {
	Spans []markdownSpan
}

type markdownSpan struct {
	Text   string
	Style  lipgloss.Style
	Styled bool
}

type markdownLinePrefix struct {
	Display string
	Copy    string
	Width   int
}

type markdownChunk struct {
	Display string
	Copy    string
	Width   int
	IsSpace bool
}

var (
	markdownRendererCache markdownLayoutCache

	markdownParser = goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
	)

	markdownBodyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E8EDF5"))

	markdownHeading1Style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true)

	markdownHeading2Style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8FD3FF")).
				Bold(true)

	markdownHeading3Style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#B6E3FF")).
				Bold(true)

	markdownHeading4Style = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#AAB7C8")).
				Bold(true)

	markdownStrongStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true)

	markdownEmStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C7F0FF")).
			Italic(true)

	markdownStrikeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#94A3B8")).
				Faint(true)

	markdownInlineCodeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFF2A8")).
				Background(lipgloss.Color("#1D2430")).
				Bold(true)

	markdownLinkLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#74D7FF")).
				Underline(true)

	markdownLinkURLStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#9AA8BA")).
				Faint(true)

	markdownQuotePrefixStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#F1B86B")).
					Bold(true)

	markdownQuoteTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E7D5B4"))

	markdownRuleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#64748B"))

	markdownCodeBaseStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E6EDF7")).
				Background(lipgloss.Color("#101722"))

	markdownCodeLangStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#D8E4F2")).
				Background(lipgloss.Color("#21415F")).
				Padding(0, 1).
				Bold(true)

	markdownTableHeaderStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#F8FAFC")).
					Bold(true)

	markdownTableBorderStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#78B7FF")).
					Bold(true)

	markdownListBulletStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#66C2FF")).
				Bold(true)

	markdownListOrderedStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#8FD3FF")).
					Bold(true)

	markdownListDoneStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7EE7A8")).
				Bold(true)

	markdownListTodoStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFD166")).
				Bold(true)
)

func renderStructuredMarkdown(surface markdownSurface, text string, width int) MarkdownRenderResult {
	if !isTerminalOutput() {
		return markdownLegacyResult(surface, text, width)
	}

	prepared := prepareMarkdownInput(surface, text)
	prepared, records := preprocessSemanticTags(prepared)
	_, profileKey := renderColorProfile()
	cacheKey := markdownCacheKey(surface, prepared, width, profileKey)
	if cached, ok := markdownRendererCache.load(cacheKey); ok {
		return cached
	}

	result, err := renderStructuredMarkdownPrepared(surface, prepared, width)
	if err != nil {
		return markdownLegacyResult(surface, text, width)
	}

	result.Display = postprocessSemanticTags(result.Display, records)
	result.Copy = restoreSemanticTagsPlain(result.Copy, records)
	result.Display = trimRenderedTrailingSpaces(result.Display)
	result.Copy = trimRenderedTrailingSpaces(result.Copy)

	markdownRendererCache.store(cacheKey, result)
	return result
}

func renderStructuredMarkdownPrepared(surface markdownSurface, text string, width int) (MarkdownRenderResult, error) {
	wrapWidth := max(16, width)
	source := []byte(text)
	reader := textpkgNewReader(source)
	doc := markdownParser.Parser().Parse(reader)
	blocks := buildMarkdownBlocks(doc, source)
	if len(blocks) == 0 {
		return MarkdownRenderResult{
			Display: wrapPlainText(text, wrapWidth),
			Copy:    wrapPlainText(text, wrapWidth),
		}, nil
	}
	lines := renderMarkdownBlocks(blocks, wrapWidth)
	result := markdownResultFromLines(lines)
	if surface == markdownSurfaceAssistant {
		result = styleAssistantStatusLines(result)
	}
	return result, nil
}

func markdownLegacyResult(surface markdownSurface, text string, width int) MarkdownRenderResult {
	switch surface {
	case markdownSurfaceHelp:
		display := renderHelpMarkdownLegacy(text, width)
		return MarkdownRenderResult{Display: display, Copy: stripANSIForCopy(display)}
	default:
		display := renderAssistantBodyLegacy(text, width)
		return MarkdownRenderResult{Display: display, Copy: stripANSIForCopy(display)}
	}
}

func prepareMarkdownInput(surface markdownSurface, text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\t", "    ")
	if surface == markdownSurfaceAssistant {
		text = tidyAssistantSpacing(text)
	}
	if strings.Count(text, "\n```")%2 == 1 || strings.HasPrefix(strings.TrimSpace(text), "```") && strings.Count(text, "```")%2 == 1 {
		text += "\n```"
	}
	return text
}

func markdownCacheKey(surface markdownSurface, text string, width int, profile string) string {
	h := fnv.New64a()
	_, _ = h.Write([]byte(text))
	return fmt.Sprintf("%s:%d:%s:%x", surface, width, profile, h.Sum64())
}

func (c *markdownLayoutCache) load(key string) (MarkdownRenderResult, bool) {
	if c == nil {
		return MarkdownRenderResult{}, false
	}
	raw, ok := c.entries.Load(key)
	if !ok {
		return MarkdownRenderResult{}, false
	}
	result, ok := raw.(MarkdownRenderResult)
	return result, ok
}

func (c *markdownLayoutCache) store(key string, result MarkdownRenderResult) {
	if c == nil {
		return
	}
	c.entries.Store(key, result)
}

func buildMarkdownBlocks(parent gast.Node, source []byte) []markdownBlock {
	blocks := make([]markdownBlock, 0, parent.ChildCount())
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		block, ok := buildMarkdownBlock(child, source)
		if ok {
			blocks = append(blocks, block)
			continue
		}
		blocks = append(blocks, buildMarkdownBlocks(child, source)...)
	}
	return blocks
}

func buildMarkdownBlock(node gast.Node, source []byte) (markdownBlock, bool) {
	switch n := node.(type) {
	case *gast.Heading:
		return markdownBlock{
			Kind:  markdownBlockHeading,
			Level: n.Level,
			Spans: extractInlineSpans(n, source, markdownSpan{}),
		}, true
	case *gast.Paragraph:
		return markdownBlock{
			Kind:  markdownBlockParagraph,
			Spans: extractInlineSpans(n, source, markdownSpan{}),
		}, true
	case *gast.Blockquote:
		return markdownBlock{
			Kind:     markdownBlockQuote,
			Children: buildMarkdownBlocks(n, source),
		}, true
	case *gast.FencedCodeBlock:
		return markdownBlock{
			Kind: markdownBlockCode,
			Code: markdownCodeBlock{
				Language: strings.TrimSpace(string(n.Language(source))),
				Text:     collectBlockText(n, source),
			},
		}, true
	case *gast.CodeBlock:
		return markdownBlock{
			Kind: markdownBlockCode,
			Code: markdownCodeBlock{
				Text: collectBlockText(n, source),
			},
		}, true
	case *gast.List:
		items := make([]markdownListItem, 0, n.ChildCount())
		for itemNode := n.FirstChild(); itemNode != nil; itemNode = itemNode.NextSibling() {
			listItem, ok := itemNode.(*gast.ListItem)
			if !ok {
				continue
			}
			itemBlocks := buildMarkdownBlocks(listItem, source)
			taskState, updated := detectTaskState(itemBlocks)
			items = append(items, markdownListItem{
				TaskState: taskState,
				Blocks:    updated,
			})
		}
		return markdownBlock{
			Kind: markdownBlockList,
			List: markdownListBlock{
				Ordered: n.IsOrdered(),
				Start:   n.Start,
				Items:   items,
			},
		}, true
	case *gast.ThematicBreak:
		return markdownBlock{Kind: markdownBlockRule}, true
	case *extensionast.Table:
		table := markdownTableBlock{}
		for child := n.FirstChild(); child != nil; child = child.NextSibling() {
			switch row := child.(type) {
			case *extensionast.TableHeader:
				table.Header = extractTableRow(row, source)
			case *extensionast.TableRow:
				table.Rows = append(table.Rows, extractTableRow(row, source))
			}
		}
		return markdownBlock{Kind: markdownBlockTable, Table: table}, true
	default:
		return markdownBlock{}, false
	}
}

func detectTaskState(blocks []markdownBlock) (string, []markdownBlock) {
	if len(blocks) == 0 || blocks[0].Kind != markdownBlockParagraph {
		return "", blocks
	}
	text := plainTextFromSpans(blocks[0].Spans)
	lower := strings.ToLower(text)
	switch {
	case strings.HasPrefix(lower, "[x] "):
		blocks[0].Spans = trimLeadingTextFromSpans(blocks[0].Spans, len("[x] "))
		return "done", blocks
	case strings.HasPrefix(lower, "[ ] "):
		blocks[0].Spans = trimLeadingTextFromSpans(blocks[0].Spans, len("[ ] "))
		return "todo", blocks
	default:
		return "", blocks
	}
}

func extractTableRow(node gast.Node, source []byte) []markdownTableCell {
	cells := make([]markdownTableCell, 0, node.ChildCount())
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		cell, ok := child.(*extensionast.TableCell)
		if !ok {
			continue
		}
		cells = append(cells, markdownTableCell{
			Spans: extractInlineSpans(cell, source, markdownSpan{}),
		})
	}
	return cells
}

func extractInlineSpans(parent gast.Node, source []byte, inherited markdownSpan) []markdownSpan {
	spans := make([]markdownSpan, 0, parent.ChildCount())
	for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *gast.Text:
			value := strings.ReplaceAll(string(n.Segment.Value(source)), "\n", " ")
			spans = appendSpan(spans, markdownSpan{
				Text:   value,
				Style:  inherited.Style,
				Styled: inherited.Styled,
			})
			if n.SoftLineBreak() || n.HardLineBreak() {
				spans = appendSpan(spans, markdownSpan{
					Text:   " ",
					Style:  inherited.Style,
					Styled: inherited.Styled,
				})
			}
		case *gast.String:
			spans = appendSpan(spans, markdownSpan{
				Text:   string(n.Value),
				Style:  inherited.Style,
				Styled: inherited.Styled,
			})
		case *gast.CodeSpan:
			spans = appendSpan(spans, markdownSpan{
				Text:   collectInlineText(n, source),
				Style:  markdownInlineCodeStyle,
				Styled: true,
			})
		case *gast.Emphasis:
			childStyle := inherited
			if n.Level == 1 {
				childStyle.Style = inherited.Style.Inherit(markdownEmStyle)
				childStyle.Styled = true
			} else {
				childStyle.Style = inherited.Style.Inherit(markdownStrongStyle)
				childStyle.Styled = true
			}
			spans = append(spans, extractInlineSpans(n, source, childStyle)...)
		case *extensionast.Strikethrough:
			childStyle := inherited
			childStyle.Style = inherited.Style.Inherit(markdownStrikeStyle)
			childStyle.Styled = true
			spans = append(spans, extractInlineSpans(n, source, childStyle)...)
		case *gast.Link:
			label := extractInlineSpans(n, source, markdownSpan{
				Style:  inherited.Style.Inherit(markdownLinkLabelStyle),
				Styled: true,
			})
			spans = append(spans, label...)
			dest := strings.TrimSpace(string(n.Destination))
			if dest != "" {
				labelText := strings.TrimSpace(plainTextFromSpans(label))
				if labelText == "" || !strings.EqualFold(labelText, dest) {
					spans = appendSpan(spans, markdownSpan{
						Text:   " (" + dest + ")",
						Style:  markdownLinkURLStyle,
						Styled: true,
					})
				}
			}
		case *gast.AutoLink:
			url := strings.TrimSpace(string(n.URL(source)))
			if url != "" {
				spans = appendSpan(spans, markdownSpan{
					Text:   url,
					Style:  markdownLinkLabelStyle,
					Styled: true,
				})
			}
		case *extensionast.TaskCheckBox:
			state := "[ ] "
			if n.IsChecked {
				state = "[x] "
			}
			spans = appendSpan(spans, markdownSpan{Text: state})
		default:
			spans = append(spans, extractInlineSpans(n, source, inherited)...)
		}
	}
	return coalesceSpans(spans)
}

func appendSpan(spans []markdownSpan, span markdownSpan) []markdownSpan {
	if span.Text == "" {
		return spans
	}
	return append(spans, span)
}

func coalesceSpans(spans []markdownSpan) []markdownSpan {
	if len(spans) == 0 {
		return spans
	}
	out := make([]markdownSpan, 0, len(spans))
	for _, span := range spans {
		if span.Text == "" {
			continue
		}
		if len(out) == 0 {
			out = append(out, span)
			continue
		}
		prev := &out[len(out)-1]
		if prev.Styled == span.Styled && (!prev.Styled || sameRenderedStyle(prev.Style, span.Style)) {
			prev.Text += span.Text
			continue
		}
		out = append(out, span)
	}
	return out
}

func collectBlockText(node gast.Node, source []byte) string {
	lines := node.Lines()
	parts := make([]string, 0, lines.Len())
	for i := 0; i < lines.Len(); i++ {
		segment := lines.At(i)
		parts = append(parts, string(segment.Value(source)))
	}
	return strings.Join(parts, "\n")
}

func collectInlineText(node gast.Node, source []byte) string {
	var b strings.Builder
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		switch n := child.(type) {
		case *gast.Text:
			b.Write(n.Segment.Value(source))
		case *gast.String:
			b.Write(n.Value)
		default:
			b.WriteString(collectInlineText(n, source))
		}
	}
	return b.String()
}

func plainTextFromSpans(spans []markdownSpan) string {
	var b strings.Builder
	for _, span := range spans {
		b.WriteString(span.Text)
	}
	return b.String()
}

func trimLeadingTextFromSpans(spans []markdownSpan, n int) []markdownSpan {
	if n <= 0 {
		return spans
	}
	remaining := n
	out := make([]markdownSpan, 0, len(spans))
	for _, span := range spans {
		if remaining <= 0 {
			out = append(out, span)
			continue
		}
		if len(span.Text) <= remaining {
			remaining -= len(span.Text)
			continue
		}
		span.Text = span.Text[remaining:]
		remaining = 0
		out = append(out, span)
	}
	return out
}

func renderMarkdownBlocks(blocks []markdownBlock, width int) []markdownRenderLine {
	lines := make([]markdownRenderLine, 0, len(blocks)*3)
	for i, block := range blocks {
		if i > 0 && shouldSeparateMarkdownBlocks(blocks[i-1], block) {
			lines = append(lines, markdownRenderLine{})
		}
		lines = append(lines, renderMarkdownBlock(block, width)...)
	}
	return trimMarkdownBlankLines(lines)
}

func shouldSeparateMarkdownBlocks(prev, next markdownBlock) bool {
	if prev.Kind == markdownBlockHeading {
		return false
	}
	return true
}

func renderMarkdownBlock(block markdownBlock, width int) []markdownRenderLine {
	switch block.Kind {
	case markdownBlockHeading:
		style := markdownHeadingStyle(block.Level)
		first, rest := markdownHeadingPrefixes(block.Level)
		return renderWrappedSpans(block.Spans, width, first, rest, style)
	case markdownBlockParagraph:
		return renderWrappedSpans(block.Spans, width, markdownLinePrefix{}, markdownLinePrefix{}, markdownBodyStyle)
	case markdownBlockQuote:
		return renderQuotedBlocks(block.Children, width)
	case markdownBlockList:
		return renderMarkdownList(block.List, width)
	case markdownBlockCode:
		return renderMarkdownCodeBlock(block.Code, width)
	case markdownBlockRule:
		rule := strings.Repeat("─", max(12, min(width, 26)))
		return []markdownRenderLine{{
			Display: markdownRuleStyle.Render(rule),
			Copy:    rule,
		}}
	case markdownBlockTable:
		return renderMarkdownTable(block.Table, width)
	default:
		return nil
	}
}

func renderQuotedBlocks(blocks []markdownBlock, width int) []markdownRenderLine {
	prefix := markdownLinePrefix{
		Display: markdownQuotePrefixStyle.Render("▍ "),
		Copy:    "> ",
		Width:   2,
	}
	innerWidth := max(8, width-prefix.Width)
	childLines := renderMarkdownBlocks(blocks, innerWidth)
	if len(childLines) == 0 {
		return nil
	}
	out := make([]markdownRenderLine, 0, len(childLines))
	for _, line := range childLines {
		if line.Display == "" && line.Copy == "" {
			out = append(out, markdownRenderLine{
				Display: prefix.Display,
				Copy:    prefix.Copy,
			})
			continue
		}
		out = append(out, markdownRenderLine{
			Display: prefix.Display + markdownQuoteTextStyle.Render(line.Display),
			Copy:    prefix.Copy + line.Copy,
		})
	}
	return out
}

func renderMarkdownList(list markdownListBlock, width int) []markdownRenderLine {
	out := make([]markdownRenderLine, 0, len(list.Items)*2)
	index := max(1, list.Start)
	for itemIndex, item := range list.Items {
		if itemIndex > 0 {
			out = append(out, markdownRenderLine{})
		}

		markerText := "- "
		markerDisplay := markdownBodyStyle.Render(markerText)
		switch item.TaskState {
		case "done":
			markerText = "[x] "
			markerDisplay = markdownListDoneStyle.Render("[✓] ")
		case "todo":
			markerText = "[ ] "
			markerDisplay = markdownListTodoStyle.Render("[ ] ")
		case "":
			if list.Ordered {
				markerText = fmt.Sprintf("%d. ", index)
				markerDisplay = markdownListOrderedStyle.Render(markerText)
			} else {
				markerDisplay = markdownListBulletStyle.Render("• ")
			}
		}

		firstPrefix := markdownLinePrefix{
			Display: markerDisplay,
			Copy:    markerText,
			Width:   runewidth.StringWidth(markerText),
		}
		restPrefix := markdownLinePrefix{
			Display: strings.Repeat(" ", firstPrefix.Width),
			Copy:    strings.Repeat(" ", firstPrefix.Width),
			Width:   firstPrefix.Width,
		}

		for blockIndex, block := range item.Blocks {
			blockLines := renderMarkdownBlock(block, max(8, width))
			if len(blockLines) == 0 {
				continue
			}
			if blockIndex == 0 {
				blockLines = applyLinePrefixes(blockLines, firstPrefix, restPrefix)
			} else {
				blockLines = applyLinePrefixes(blockLines, restPrefix, restPrefix)
			}
			if len(out) > 0 && out[len(out)-1].Display != "" && blockIndex > 0 {
				out = append(out, markdownRenderLine{
					Display: restPrefix.Display,
					Copy:    restPrefix.Copy,
				})
			}
			out = append(out, blockLines...)
		}
		index++
	}
	return trimMarkdownBlankLines(out)
}

func applyLinePrefixes(lines []markdownRenderLine, first, rest markdownLinePrefix) []markdownRenderLine {
	if len(lines) == 0 {
		return nil
	}
	out := make([]markdownRenderLine, 0, len(lines))
	for i, line := range lines {
		prefix := rest
		if i == 0 {
			prefix = first
		}
		out = append(out, markdownRenderLine{
			Display: prefix.Display + line.Display,
			Copy:    prefix.Copy + line.Copy,
		})
	}
	return out
}

func renderMarkdownCodeBlock(block markdownCodeBlock, width int) []markdownRenderLine {
	out := make([]markdownRenderLine, 0, 8)
	lang := strings.ToLower(strings.TrimSpace(block.Language))
	borderWidth := max(12, width)
	topDisplay, topCopy := renderCodeFrameTop(lang, borderWidth)
	out = append(out, markdownRenderLine{Display: topDisplay, Copy: topCopy})

	codeLines := tokenizeCodeLines(block.Text, block.Language)
	if len(codeLines) == 0 {
		codeLines = [][]markdownSpan{{}}
	}
	linePrefix := markdownLinePrefix{
		Display: markdownTableBorderStyle.Render("│ "),
		Copy:    "  ",
		Width:   2,
	}
	for _, line := range codeLines {
		if len(line) == 0 {
			out = append(out, markdownRenderLine{
				Display: linePrefix.Display,
				Copy:    linePrefix.Copy,
			})
			continue
		}
		out = append(out, renderWrappedSpans(line, width, linePrefix, linePrefix, markdownCodeBaseStyle)...)
	}
	bottomRule := strings.Repeat("─", max(8, borderWidth))
	out = append(out, markdownRenderLine{
		Display: markdownTableBorderStyle.Render("╰" + bottomRule),
		Copy:    strings.Repeat("-", max(8, borderWidth)),
	})
	return out
}

func tokenizeCodeLines(code, language string) [][]markdownSpan {
	code = strings.ReplaceAll(code, "\r\n", "\n")
	lexer := lexers.Match(strings.TrimSpace(language))
	if lexer == nil && strings.TrimSpace(language) != "" {
		lexer = lexers.Get(strings.TrimSpace(language))
	}
	if lexer == nil {
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		return plainCodeLines(code)
	}

	iterator, err := lexer.Tokenise(nil, code)
	if err != nil {
		return plainCodeLines(code)
	}

	lines := [][]markdownSpan{{}}
	for token := iterator(); token != chroma.EOF; token = iterator() {
		style, styled := chromaTokenStyle(token.Type)
		parts := strings.Split(token.Value, "\n")
		for index, part := range parts {
			if part != "" {
				lines[len(lines)-1] = append(lines[len(lines)-1], markdownSpan{
					Text:   part,
					Style:  style,
					Styled: styled,
				})
			}
			if index < len(parts)-1 {
				lines = append(lines, []markdownSpan{})
			}
		}
	}
	return lines
}

func plainCodeLines(code string) [][]markdownSpan {
	code = strings.ReplaceAll(code, "\r\n", "\n")
	parts := strings.Split(code, "\n")
	lines := make([][]markdownSpan, 0, len(parts))
	for _, part := range parts {
		lines = append(lines, []markdownSpan{{Text: part}})
	}
	return lines
}

func chromaTokenStyle(tokenType chroma.TokenType) (lipgloss.Style, bool) {
	style := markdownCodeBaseStyle
	switch {
	case tokenType.InCategory(chroma.Comment):
		return style.Foreground(lipgloss.Color("#7C8795")), true
	case tokenType.InCategory(chroma.Keyword):
		return style.Foreground(lipgloss.Color("#9CC0FF")), true
	case tokenType.InCategory(chroma.Name):
		return style.Foreground(lipgloss.Color("#D7DEE8")), true
	case tokenType == chroma.NameFunction || tokenType.InSubCategory(chroma.NameFunction):
		return style.Foreground(lipgloss.Color("#8BD5CA")), true
	case tokenType.InCategory(chroma.LiteralString):
		return style.Foreground(lipgloss.Color("#EBCB8B")), true
	case tokenType.InCategory(chroma.LiteralNumber):
		return style.Foreground(lipgloss.Color("#A3E635")), true
	case tokenType.InCategory(chroma.Operator):
		return style.Foreground(lipgloss.Color("#F38BA8")), true
	case tokenType.InCategory(chroma.GenericDeleted):
		return style.Foreground(lipgloss.Color("#F87171")), true
	case tokenType.InCategory(chroma.GenericInserted):
		return style.Foreground(lipgloss.Color("#4ADE80")), true
	default:
		return lipgloss.Style{}, false
	}
}

func renderMarkdownTable(table markdownTableBlock, width int) []markdownRenderLine {
	if len(table.Header) == 0 && len(table.Rows) == 0 {
		return nil
	}
	colCount := len(table.Header)
	for _, row := range table.Rows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}
	if colCount == 0 {
		return nil
	}

	headerTexts := make([][]markdownSpan, colCount)
	for i := 0; i < colCount; i++ {
		headerTexts[i] = cellSpansAt(table.Header, i)
	}

	gridWidths, ok := markdownTableWidths(headerTexts, table.Rows, width)
	if !ok {
		return renderStackedMarkdownTable(headerTexts, table.Rows, width)
	}

	return renderGridMarkdownTable(headerTexts, table.Rows, gridWidths)
}

func markdownTableWidths(header []([]markdownSpan), rows [][]markdownTableCell, width int) ([]int, bool) {
	if len(header) == 0 {
		return nil, false
	}
	widths := make([]int, len(header))
	total := 0
	for col := range header {
		w := max(6, runewidth.StringWidth(plainTextFromSpans(header[col])))
		for _, row := range rows {
			cellWidth := runewidth.StringWidth(plainTextFromSpans(cellSpansAt(row, col)))
			if cellWidth > w {
				w = cellWidth
			}
		}
		w = min(w, 28)
		widths[col] = w
		total += w
	}

	total += max(0, len(widths)-1) * 3
	if total <= width {
		return widths, true
	}

	available := width - max(0, len(widths)-1)*3
	if available <= len(widths)*6 {
		return nil, false
	}
	for total > width {
		changed := false
		for i := range widths {
			if widths[i] > 6 && total > width {
				widths[i]--
				total--
				changed = true
			}
		}
		if !changed {
			return nil, false
		}
	}
	return widths, true
}

func renderGridMarkdownTable(header [][]markdownSpan, rows [][]markdownTableCell, widths []int) []markdownRenderLine {
	lines := make([]markdownRenderLine, 0, len(rows)*3+4)
	lines = append(lines, renderTableBorderLine(widths, "┌", "┬", "┐", "+", "+", "+"))
	lines = append(lines, renderTableGridRow(header, widths, markdownTableHeaderStyle)...)
	lines = append(lines, renderTableBorderLine(widths, "├", "┼", "┤", "+", "+", "+"))
	for _, row := range rows {
		cells := make([][]markdownSpan, len(widths))
		for i := range widths {
			cells[i] = cellSpansAt(row, i)
		}
		lines = append(lines, renderTableGridRow(cells, widths, markdownBodyStyle)...)
	}
	lines = append(lines, renderTableBorderLine(widths, "└", "┴", "┘", "+", "+", "+"))
	return lines
}

func renderTableGridRow(cells [][]markdownSpan, widths []int, defaultStyle lipgloss.Style) []markdownRenderLine {
	renderedCells := make([][]markdownRenderLine, len(widths))
	maxHeight := 1
	for col := range widths {
		renderedCells[col] = renderWrappedSpans(cells[col], widths[col], markdownLinePrefix{}, markdownLinePrefix{}, defaultStyle)
		if len(renderedCells[col]) == 0 {
			renderedCells[col] = []markdownRenderLine{{}}
		}
		if len(renderedCells[col]) > maxHeight {
			maxHeight = len(renderedCells[col])
		}
	}

	out := make([]markdownRenderLine, 0, maxHeight)
	leftDisplay := markdownTableBorderStyle.Render("│ ")
	separatorDisplay := markdownTableBorderStyle.Render(" │ ")
	rightDisplay := markdownTableBorderStyle.Render(" │")
	leftCopy := "| "
	separatorCopy := " | "
	rightCopy := " |"
	for row := 0; row < maxHeight; row++ {
		displayParts := make([]string, 0, len(widths))
		copyParts := make([]string, 0, len(widths))
		for col, cellWidth := range widths {
			line := markdownRenderLine{}
			if row < len(renderedCells[col]) {
				line = renderedCells[col][row]
			}
			displayParts = append(displayParts, padStyledLineRight(line.Display, line.Copy, cellWidth))
			copyParts = append(copyParts, padPlainRight(line.Copy, cellWidth))
		}
		out = append(out, markdownRenderLine{
			Display: leftDisplay + strings.Join(displayParts, separatorDisplay) + rightDisplay,
			Copy:    leftCopy + strings.Join(copyParts, separatorCopy) + rightCopy,
		})
	}
	return out
}

func renderTableBorderLine(widths []int, left, middle, right, leftCopy, middleCopy, rightCopy string) markdownRenderLine {
	displayParts := make([]string, 0, len(widths))
	copyParts := make([]string, 0, len(widths))
	for _, width := range widths {
		part := strings.Repeat("─", width+2)
		displayParts = append(displayParts, markdownTableBorderStyle.Render(part))
		copyParts = append(copyParts, strings.Repeat("-", width+2))
	}
	return markdownRenderLine{
		Display: markdownTableBorderStyle.Render(left) + strings.Join(displayParts, markdownTableBorderStyle.Render(middle)) + markdownTableBorderStyle.Render(right),
		Copy:    leftCopy + strings.Join(copyParts, middleCopy) + rightCopy,
	}
}

func renderStackedMarkdownTable(header [][]markdownSpan, rows [][]markdownTableCell, width int) []markdownRenderLine {
	lines := make([]markdownRenderLine, 0, len(rows)*3)
	for rowIndex, row := range rows {
		if rowIndex > 0 {
			lines = append(lines, markdownRenderLine{})
		}
		for col := range header {
			label := strings.TrimSpace(plainTextFromSpans(header[col]))
			if label == "" {
				label = fmt.Sprintf("Column %d", col+1)
			}
			prefixText := label + ": "
			prefix := markdownLinePrefix{
				Display: markdownTableHeaderStyle.Render(prefixText),
				Copy:    prefixText,
				Width:   runewidth.StringWidth(prefixText),
			}
			rest := markdownLinePrefix{
				Display: strings.Repeat(" ", prefix.Width),
				Copy:    strings.Repeat(" ", prefix.Width),
				Width:   prefix.Width,
			}
			valueLines := renderWrappedSpans(cellSpansAt(row, col), width, prefix, rest, markdownBodyStyle)
			if len(valueLines) == 0 {
				valueLines = []markdownRenderLine{{
					Display: prefix.Display,
					Copy:    prefix.Copy,
				}}
			}
			lines = append(lines, valueLines...)
		}
	}
	return lines
}

func cellSpansAt[T interface{ ~[]markdownTableCell }](cells T, index int) []markdownSpan {
	if index < 0 || index >= len(cells) {
		return nil
	}
	return cells[index].Spans
}

func renderWrappedSpans(spans []markdownSpan, width int, firstPrefix, restPrefix markdownLinePrefix, defaultStyle lipgloss.Style) []markdownRenderLine {
	chunks := spansToChunks(spans, defaultStyle)
	if len(chunks) == 0 {
		return nil
	}
	return wrapMarkdownChunks(chunks, width, firstPrefix, restPrefix)
}

func spansToChunks(spans []markdownSpan, defaultStyle lipgloss.Style) []markdownChunk {
	chunks := make([]markdownChunk, 0, len(spans)*4)
	for _, span := range spans {
		if span.Text == "" {
			continue
		}
		runes := []rune(span.Text)
		for _, r := range runes {
			text := string(r)
			display := text
			switch {
			case span.Styled:
				display = span.Style.Render(text)
			case isStyledLipgloss(defaultStyle):
				display = defaultStyle.Render(text)
			}
			width := runewidth.RuneWidth(r)
			if width < 0 {
				width = 0
			}
			chunks = append(chunks, markdownChunk{
				Display: display,
				Copy:    text,
				Width:   width,
				IsSpace: unicode.IsSpace(r),
			})
		}
	}
	return chunks
}

func sameRenderedStyle(a, b lipgloss.Style) bool {
	return a.Render("x") == b.Render("x")
}

func isStyledLipgloss(style lipgloss.Style) bool {
	return style.Render("x") != "x"
}

func wrapMarkdownChunks(chunks []markdownChunk, width int, firstPrefix, restPrefix markdownLinePrefix) []markdownRenderLine {
	if len(chunks) == 0 {
		return nil
	}
	if width <= 0 {
		width = 1
	}

	lines := make([]markdownRenderLine, 0, 4)
	start := 0
	lineIndex := 0
	for start < len(chunks) {
		prefix := restPrefix
		if lineIndex == 0 {
			prefix = firstPrefix
		}
		available := max(1, width-prefix.Width)
		curWidth := 0
		end := start
		lastSpaceEnd := -1
		for i := start; i < len(chunks); i++ {
			chunkWidth := max(0, chunks[i].Width)
			if end > start && curWidth+chunkWidth > available {
				break
			}
			curWidth += chunkWidth
			end = i + 1
			if chunks[i].IsSpace {
				lastSpaceEnd = i + 1
			}
			if curWidth >= available {
				break
			}
		}

		if end == start {
			end = start + 1
		} else if lastSpaceEnd > start && end < len(chunks) {
			end = lastSpaceEnd
		}

		trimEnd := end
		for trimEnd > start && chunks[trimEnd-1].IsSpace {
			trimEnd--
		}
		if trimEnd == start {
			trimEnd = end
		}

		line := markdownRenderLine{
			Display: prefix.Display + joinChunkDisplay(chunks[start:trimEnd]),
			Copy:    prefix.Copy + joinChunkCopy(chunks[start:trimEnd]),
		}
		lines = append(lines, line)

		start = end
		for start < len(chunks) && chunks[start].IsSpace {
			start++
		}
		lineIndex++
	}
	return lines
}

func joinChunkDisplay(chunks []markdownChunk) string {
	var b strings.Builder
	for _, chunk := range chunks {
		b.WriteString(chunk.Display)
	}
	return b.String()
}

func joinChunkCopy(chunks []markdownChunk) string {
	var b strings.Builder
	for _, chunk := range chunks {
		b.WriteString(chunk.Copy)
	}
	return b.String()
}

func markdownResultFromLines(lines []markdownRenderLine) MarkdownRenderResult {
	displayLines := make([]string, 0, len(lines))
	copyLines := make([]string, 0, len(lines))
	for _, line := range lines {
		displayLines = append(displayLines, line.Display)
		copyLines = append(copyLines, line.Copy)
	}
	return MarkdownRenderResult{
		Display: strings.Join(displayLines, "\n"),
		Copy:    strings.Join(copyLines, "\n"),
		Lines:   lines,
	}
}

func styleAssistantStatusLines(result MarkdownRenderResult) MarkdownRenderResult {
	if len(result.Lines) == 0 {
		return result
	}
	lines := make([]markdownRenderLine, len(result.Lines))
	copy(lines, result.Lines)

	changed := false
	statusStyle := mutedStyle.Copy().Faint(true)
	for i := range lines {
		copyText := strings.TrimSpace(lines[i].Copy)
		if strings.HasPrefix(copyText, "Processed for ") {
			lines[i].Display = statusStyle.Render(lines[i].Copy)
			changed = true
		}
	}
	if !changed {
		return result
	}
	return markdownResultFromLines(lines)
}

func trimMarkdownBlankLines(lines []markdownRenderLine) []markdownRenderLine {
	start := 0
	for start < len(lines) && lines[start].Display == "" && lines[start].Copy == "" {
		start++
	}
	end := len(lines)
	for end > start && lines[end-1].Display == "" && lines[end-1].Copy == "" {
		end--
	}
	if start >= end {
		return nil
	}
	out := make([]markdownRenderLine, 0, end-start)
	prevBlank := false
	for _, line := range lines[start:end] {
		isBlank := line.Display == "" && line.Copy == ""
		if isBlank && prevBlank {
			continue
		}
		out = append(out, line)
		prevBlank = isBlank
	}
	return out
}

func markdownHeadingStyle(level int) lipgloss.Style {
	switch level {
	case 1:
		return markdownHeading1Style
	case 2:
		return markdownHeading2Style
	case 3:
		return markdownHeading3Style
	default:
		return markdownHeading4Style
	}
}

func markdownHeadingPrefixes(level int) (markdownLinePrefix, markdownLinePrefix) {
	prefixText := ""
	switch level {
	case 1:
		prefixText = "█ "
	case 2:
		prefixText = "◆ "
	case 3:
		prefixText = "• "
	default:
		prefixText = "· "
	}
	styled := markdownHeadingStyle(level).Render(prefixText)
	first := markdownLinePrefix{
		Display: styled,
		Copy:    "",
		Width:   runewidth.StringWidth(prefixText),
	}
	rest := markdownLinePrefix{
		Display: strings.Repeat(" ", first.Width),
		Copy:    "",
		Width:   first.Width,
	}
	return first, rest
}

func renderCodeFrameTop(lang string, width int) (string, string) {
	if width <= 0 {
		width = 12
	}
	if lang != "" {
		label := markdownCodeLangStyle.Render("[" + lang + "]")
		copyLabel := "[" + lang + "]"
		ruleWidth := max(4, width-runewidth.StringWidth(copyLabel)-3)
		rule := strings.Repeat("─", ruleWidth)
		return markdownTableBorderStyle.Render("╭") + label + markdownTableBorderStyle.Render("─"+rule), copyLabel + " " + strings.Repeat("-", ruleWidth)
	}
	rule := strings.Repeat("─", max(8, width))
	return markdownTableBorderStyle.Render("╭" + rule), strings.Repeat("-", max(8, width))
}

func restoreSemanticTagsPlain(rendered string, records map[string]semanticTagRecord) string {
	return tagTokenRegex.ReplaceAllStringFunc(rendered, func(token string) string {
		sub := tagTokenRegex.FindStringSubmatch(token)
		if len(sub) != 3 {
			return token
		}
		record, ok := records[sub[2]]
		if !ok {
			return token
		}
		return record.Content
	})
}

func stripANSIForCopy(s string) string {
	return stripANSI(s)
}

func stripANSI(s string) string {
	var out bytes.Buffer
	inEscape := false
	for _, r := range s {
		switch {
		case r == '\x1b':
			inEscape = true
		case inEscape && r == 'm':
			inEscape = false
		case !inEscape:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func padPlainRight(s string, width int) string {
	padding := width - runewidth.StringWidth(s)
	if padding <= 0 {
		return s
	}
	return s + strings.Repeat(" ", padding)
}

func padStyledLineRight(display, copy string, width int) string {
	padding := width - runewidth.StringWidth(copy)
	if padding <= 0 {
		return display
	}
	return display + strings.Repeat(" ", padding)
}

func textpkgNewReader(source []byte) text.Reader {
	return text.NewReader(source)
}
