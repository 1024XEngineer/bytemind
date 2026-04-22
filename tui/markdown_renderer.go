package tui

import (
	"bytes"
	"fmt"
	"hash/fnv"
	"regexp"
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
	textpkg "github.com/yuin/goldmark/text"
)

type markdownSurface string

const (
	markdownSurfaceAssistant markdownSurface = "assistant"
	markdownSurfaceHelp      markdownSurface = "help"
	markdownSurfaceTool      markdownSurface = "tool"
)

const (
	markdownHeading1Glyph  = "\u25c6 "
	markdownHeading2Glyph  = "\u25c7 "
	markdownHeading3Glyph  = "\u2022 "
	markdownHeading4Glyph  = "\u00b7 "
	markdownQuoteGlyph     = "\u258e "
	markdownBulletGlyph    = "\u2022 "
	markdownTaskDoneGlyph  = "\u2713 "
	markdownRuleGlyph      = "\u2500"
	markdownCodeTopLeft    = "\u256d"
	markdownCodeBottomLeft = "\u2570"
	markdownCodeSideGlyph  = "\u2502 "

	// Enhanced icon set for richer visual representation
	markdownBulletAltGlyph     = "\u25e6 " // White bullet
	markdownBulletSquareGlyph  = "\u25a0 " // Square bullet
	markdownBulletDiamondGlyph = "\u25c6 " // Diamond bullet
	markdownBulletStarGlyph    = "\u2605 " // Star bullet
	markdownBulletArrowGlyph   = "\u279c " // Arrow bullet
	markdownBulletPlusGlyph    = "\u2795 " // Plus bullet
	markdownBulletCheckGlyph   = "\u2713 " // Check bullet

	markdownCodeInfoGlyph    = "\u2139 " // Information icon
	markdownCodeWarnGlyph    = "\u26a0 " // Warning icon
	markdownCodeErrorGlyph   = "\u274c " // Error icon
	markdownCodeSuccessGlyph = "\u2705 " // Success icon
	markdownCodeTipGlyph     = "💡 "      // Tip icon

	markdownListRomanNumeral = "I." // Roman numeral for ordered lists
	markdownListLetterUpper  = "A." // Uppercase letter for ordered lists
	markdownListLetterLower  = "a." // Lowercase letter for ordered lists

	markdownTableCornerTopLeft     = "\u250c" // Table corner
	markdownTableCornerTopRight    = "\u2510" // Table corner
	markdownTableCornerBottomLeft  = "\u2514" // Table corner
	markdownTableCornerBottomRight = "\u2518" // Table corner
	markdownTableCross             = "\u253c" // Table cross
	markdownTableTeeTop            = "\u252c" // Table tee
	markdownTableTeeBottom         = "\u2534" // Table tee
	markdownTableTeeLeft           = "\u251c" // Table tee
	markdownTableTeeRight          = "\u2524" // Table tee
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
				Foreground(semanticColors.TextBase)

	markdownHeading1Style = lipgloss.NewStyle().
				Foreground(semanticColors.TextStrong).
				Bold(true)

	markdownHeading2Style = lipgloss.NewStyle().
				Foreground(semanticColors.Accent).
				Bold(true)

	markdownHeading3Style = lipgloss.NewStyle().
				Foreground(semanticColors.AccentSoft).
				Bold(true)

	markdownHeading4Style = lipgloss.NewStyle().
				Foreground(semanticColors.TextMuted).
				Bold(true)

	markdownStrongStyle = lipgloss.NewStyle().
				Foreground(semanticColors.TextStrong).
				Bold(true)

	markdownEmStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C7F0FF")).
			Italic(true)

	markdownStrikeStyle = lipgloss.NewStyle().
				Foreground(semanticColors.TextMuted).
				Faint(true)

	markdownInlineCodeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFF2A8")).
				Background(semanticColors.CodeInlineBg).
				Bold(true)

	markdownLinkLabelStyle = lipgloss.NewStyle().
				Foreground(semanticColors.ToolSoft).
				Underline(true)

	markdownLinkURLStyle = lipgloss.NewStyle().
				Foreground(semanticColors.TextMuted).
				Faint(true)

	markdownQuotePrefixStyle = lipgloss.NewStyle().
					Foreground(semanticColors.Quote).
					Bold(true)

	markdownQuoteTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E7D5B4"))

	markdownRuleStyle = lipgloss.NewStyle().
				Foreground(semanticColors.TextMuted)

	markdownCodeBaseStyle = lipgloss.NewStyle().
				Foreground(semanticColors.TextBase).
				Background(semanticColors.CodeBg)

	markdownCodeFrameStyle = lipgloss.NewStyle().
				Foreground(semanticColors.CodeBorder)

	markdownCodeLangStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#D8E4F2")).
				Background(lipgloss.Color("#21415F")).
				Padding(0, 1).
				Bold(true)

	markdownTableHeaderStyle = lipgloss.NewStyle().
					Foreground(semanticColors.TextStrong).
					Bold(true)

	markdownTableBorderStyle = lipgloss.NewStyle().
					Foreground(semanticColors.TableBorder).
					Bold(true)

	markdownListBulletStyle = lipgloss.NewStyle().
				Foreground(semanticColors.Accent).
				Bold(true)

	markdownListOrderedStyle = lipgloss.NewStyle().
					Foreground(semanticColors.AccentSoft).
					Bold(true)

	markdownListDoneStyle = lipgloss.NewStyle().
				Foreground(semanticColors.Success).
				Bold(true)

	markdownListTodoStyle = lipgloss.NewStyle().
				Foreground(semanticColors.Warning).
				Bold(true)

	// Enhanced markdown styles for better visual presentation
	markdownHighlightYellowStyle = lipgloss.NewStyle().
					Background(semanticColors.HighlightYellow).
					Foreground(lipgloss.Color("#856404"))

	markdownHighlightBlueStyle = lipgloss.NewStyle().
					Background(semanticColors.HighlightBlue).
					Foreground(lipgloss.Color("#004085"))

	markdownHighlightGreenStyle = lipgloss.NewStyle().
					Background(semanticColors.HighlightGreen).
					Foreground(lipgloss.Color("#155724"))

	markdownHighlightRedStyle = lipgloss.NewStyle().
					Background(semanticColors.HighlightRed).
					Foreground(lipgloss.Color("#721c24"))

	markdownHighlightPurpleStyle = lipgloss.NewStyle().
					Background(semanticColors.HighlightPurple).
					Foreground(lipgloss.Color("#383d41"))

	markdownHighlightOrangeStyle = lipgloss.NewStyle().
					Background(semanticColors.HighlightOrange).
					Foreground(lipgloss.Color("#856404"))

	// Enhanced table styles
	markdownTableAltRowStyle = lipgloss.NewStyle().
					Background(lipgloss.Color("#1a1f29"))

	markdownTableBorderStyleLight = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#4a5568"))

	// Enhanced code block styles
	markdownCodeLightStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#24292e")).
				Background(semanticColors.CodeThemeLight)

	markdownCodeDarkStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#d4d4d4")).
				Background(semanticColors.CodeThemeDark)

	markdownCodeOceanStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#c0c5ce")).
				Background(semanticColors.CodeThemeOcean)

	markdownCodeForestStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#d8dee9")).
				Background(semanticColors.CodeThemeForest)

	// Enhanced heading styles with more visual hierarchy
	markdownHeading1AltStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#E2E8F0")).
					Bold(true).
					Underline(true)

	markdownHeading2AltStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#CBD5E0")).
					Bold(true).
					BorderBottom(true)

	markdownHeading3AltStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#A0AEC0")).
					Bold(true).
					Italic(true)
)

func renderStructuredMarkdown(surface markdownSurface, text string, width int) MarkdownRenderResult {
	result, err := renderStructuredMarkdownPrepared(surface, text, width)
	if err == nil {
		return result
	}
	renderer := NewSimpleMarkdownRenderer(width)
	display := renderer.Render(text)

	return MarkdownRenderResult{
		Display: display,
		Copy:    strings.TrimRight(stripANSI(display), "\n"),
	}
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
	lines := renderMarkdownBlocks(surface, blocks, wrapWidth)
	return markdownResultFromLines(lines), nil
}

func markdownLegacyResult(surface markdownSurface, text string, width int) MarkdownRenderResult {
	switch surface {
	case markdownSurfaceHelp:
		display := renderHelpMarkdownLegacy(text, width)
		return MarkdownRenderResult{Display: display, Copy: stripANSIForCopy(display)}
	case markdownSurfaceTool:
		display := renderToolBodyLegacy(text, width)
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
	case *gast.TextBlock:
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

// Process highlights in paragraph text
func processParagraphHighlights(spans []markdownSpan) []markdownSpan {
	// Combine all text from spans
	combinedText := plainTextFromSpans(spans)

	// Process highlights in the combined text
	highlightedSpans := processHighlights(combinedText)

	return highlightedSpans
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

func renderMarkdownBlocks(surface markdownSurface, blocks []markdownBlock, width int) []markdownRenderLine {
	lines := make([]markdownRenderLine, 0, len(blocks)*3)
	for i, block := range blocks {
		if i > 0 && shouldSeparateMarkdownBlocks(blocks[i-1], block) {
			lines = append(lines, markdownRenderLine{})
		}
		lines = append(lines, renderMarkdownBlock(surface, block, width)...)
	}
	return trimMarkdownBlankLines(lines)
}

func shouldSeparateMarkdownBlocks(prev, next markdownBlock) bool {
	if prev.Kind == markdownBlockHeading {
		return false
	}
	return true
}

func markdownParagraphStyle(surface markdownSurface, spans []markdownSpan) lipgloss.Style {
	text := strings.TrimSpace(plainTextFromSpans(spans))
	switch surface {
	case markdownSurfaceAssistant:
		if strings.HasPrefix(text, "Processed for ") {
			return mutedStyle.Copy().Faint(true)
		}
	case markdownSurfaceTool:
		switch {
		case isToolSearchSummaryLine(text):
			return toolSearchSummaryStyle
		case isToolSearchMatchLine(text):
			return toolSearchMatchStyle
		default:
			return toolBodyStyle
		}
	}
	return markdownBodyStyle
}

func renderMarkdownBlock(surface markdownSurface, block markdownBlock, width int) []markdownRenderLine {
	switch block.Kind {
	case markdownBlockHeading:
		style := markdownHeadingStyle(block.Level)
		first, rest := markdownHeadingPrefixes(block.Level)
		return renderWrappedSpans(block.Spans, width, first, rest, style)
	case markdownBlockParagraph:
		// Process highlights and annotations in paragraph text
		highlightedSpans, annotations := processAnnotationsWithHighlights(block.Spans)
		renderedLines := renderWrappedSpans(highlightedSpans, width, markdownLinePrefix{}, markdownLinePrefix{}, markdownParagraphStyle(surface, highlightedSpans))

		// Add annotation summaries after the paragraph if there are any
		if len(annotations) > 0 {
			// Add a blank line for separation
			renderedLines = append(renderedLines, markdownRenderLine{})

			// Create annotation summary
			summaryText := "Annotations: "
			for i, ann := range annotations {
				if i > 0 {
					summaryText += ", "
				}
				summaryText += annotationIcons[ann.Type] + " " + ann.Label
			}

			// Style the summary based on the most important annotation type
			summaryStyle := markdownBodyStyle
			if len(annotations) > 0 {
				// Use the style of the first annotation
				summaryStyle = annotationStyles[annotations[0].Type]
			}

			// Render the summary
			summaryLines := strings.Split(wrapPlainText(summaryText, width), "\n")
			for _, line := range summaryLines {
				renderedLines = append(renderedLines, markdownRenderLine{
					Display: summaryStyle.Render(line),
					Copy:    line,
				})
			}
		}

		return renderedLines
	case markdownBlockQuote:
		return renderQuotedBlocks(surface, block.Children, width)
	case markdownBlockList:
		return renderMarkdownList(surface, block.List, width)
	case markdownBlockCode:
		return renderMarkdownCodeBlock(block.Code, width)
	case markdownBlockRule:
		rule := strings.Repeat("─", max(12, min(width, 26)))
		return []markdownRenderLine{{
			Display: markdownRuleStyle.Render(rule),
			Copy:    rule,
		}}
	case markdownBlockTable:
		return renderMarkdownTable(surface, block.Table, width)
	default:
		return nil
	}
}

func renderQuotedBlocks(surface markdownSurface, blocks []markdownBlock, width int) []markdownRenderLine {
	prefix := markdownLinePrefix{
		Display: markdownQuotePrefixStyle.Render("▍ "),
		Copy:    "> ",
		Width:   2,
	}
	innerWidth := max(8, width-prefix.Width)
	childLines := renderMarkdownBlocks(surface, blocks, innerWidth)
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

func renderMarkdownList(surface markdownSurface, list markdownListBlock, width int) []markdownRenderLine {
	out := make([]markdownRenderLine, 0, len(list.Items)*2)
	index := max(1, list.Start)

	// Determine list depth for enhanced styling (assuming we track depth somewhere)
	// For now, we'll use a simple heuristic based on width
	listDepth := calculateListDepth(width)

	for itemIndex, item := range list.Items {
		if itemIndex > 0 {
			// Add slightly less spacing for compact appearance
			out = append(out, markdownRenderLine{})
		}

		var markerText string
		var markerDisplay string

		// Enhanced marker selection based on list type and depth
		switch item.TaskState {
		case "done":
			markerText = "[x] "
			markerDisplay = markdownListDoneStyle.Render("[✓] ")
		case "todo":
			markerText = "[ ] "
			markerDisplay = markdownListTodoStyle.Render("[ ] ")
		case "in-progress":
			markerText = "[~] "
			markerDisplay = markdownListTodoStyle.Copy().Foreground(semanticColors.Warning).Render("[~] ")
		case "":
			if list.Ordered {
				// Use enhanced ordered list markers based on depth
				markerText = selectOrderedMarker(itemIndex, listDepth, list.Start)
				markerDisplay = markdownListOrderedStyle.Render(markerText)
			} else {
				// Use enhanced bullet styles based on depth
				bulletIcon := selectBulletIcon(itemIndex, listDepth)
				markerDisplay = markdownListBulletStyle.Render(bulletIcon)
				markerText = bulletIcon
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
			prefixForBlock := restPrefix
			if blockIndex == 0 {
				prefixForBlock = firstPrefix
			}
			blockLines := renderMarkdownBlock(surface, block, max(8, width-prefixForBlock.Width))
			if len(blockLines) == 0 {
				continue
			}
			if blockIndex == 0 {
				blockLines = applyLinePrefixes(blockLines, firstPrefix, restPrefix)
			} else {
				blockLines = applyLinePrefixes(blockLines, restPrefix, restPrefix)
			}
			out = append(out, blockLines...)
		}
		index++
	}
	return out
}

// Calculate list depth based on available width (heuristic)
func calculateListDepth(width int) int {
	// Simple heuristic: deeper nesting in wider windows
	if width < 60 {
		return 0
	} else if width < 80 {
		return 1
	} else {
		return 2
	}
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

	// Select appropriate code theme based on language or type
	codeStyle := selectCodeTheme(lang)

	// Add special icon if the code block represents something special
	codeIcon := selectCodeIcon(lang)
	if codeIcon != "" {
		lang = codeIcon + " " + lang
	}

	topDisplay, topCopy := renderCodeFrameTop(lang, borderWidth)
	out = append(out, markdownRenderLine{Display: topDisplay, Copy: topCopy})

	codeLines := tokenizeCodeLines(block.Text, block.Language)
	if len(codeLines) == 0 {
		codeLines = [][]markdownSpan{{}}
	}

	// Calculate line number width for alignment
	lineNumWidth := len(fmt.Sprintf("%d", len(codeLines)))

	// Enhanced line prefix with line numbers
	for i, line := range codeLines {
		lineNum := i + 1
		lineNumStr := fmt.Sprintf("%*d", lineNumWidth, lineNum)

		// Different styles for line numbers based on context
		lineNumStyle := markdownCodeBaseStyle
		if i%5 == 0 { // Every 5th line gets a slightly different style
			lineNumStyle = markdownCodeBaseStyle.Copy().Foreground(semanticColors.AccentSoft)
		}

		// Create enhanced line prefix with line number
		linePrefix := markdownLinePrefix{
			Display: markdownTableBorderStyle.Render("│ ") + lineNumStyle.Render(lineNumStr+" "),
			Copy:    fmt.Sprintf("%s%*d ", "  ", lineNumWidth, lineNum),
			Width:   2 + lineNumWidth + 1, // border + space + line number + space
		}

		// Adjust width to account for line numbers
		contentWidth := max(8, width-linePrefix.Width)

		if len(line) == 0 {
			// Empty line with just line number
			out = append(out, markdownRenderLine{
				Display: linePrefix.Display,
				Copy:    linePrefix.Copy,
			})
			continue
		}

		// Render the line content with the selected theme
		out = append(out, renderWrappedSpans(line, contentWidth, linePrefix, linePrefix, codeStyle)...)
	}

	// Enhanced bottom border with decorative elements
	bottomRule := strings.Repeat("─", max(8, borderWidth))
	borderEnd := "╰"
	if lang == "error" {
		borderEnd = "╭" // Different border for error blocks
	}
	out = append(out, markdownRenderLine{
		Display: markdownTableBorderStyle.Render(borderEnd + bottomRule),
		Copy:    strings.Repeat("-", max(8, borderWidth)),
	})
	return out
}

// Select appropriate code theme based on language or type
func selectCodeTheme(language string) lipgloss.Style {
	language = strings.ToLower(language)

	// Select theme based on language or special keywords
	switch {
	case strings.Contains(language, "light"):
		return markdownCodeLightStyle
	case strings.Contains(language, "dark"):
		return markdownCodeDarkStyle
	case strings.Contains(language, "ocean"):
		return markdownCodeOceanStyle
	case strings.Contains(language, "forest"):
		return markdownCodeForestStyle
	case strings.Contains(language, "error"):
		return markdownCodeDarkStyle.Copy().Background(lipgloss.Color("#2D1B1B"))
	case strings.Contains(language, "success"):
		return markdownCodeDarkStyle.Copy().Background(lipgloss.Color("#1B2D1B"))
	case strings.Contains(language, "warning"):
		return markdownCodeDarkStyle.Copy().Background(lipgloss.Color("#2D2A1B"))
	default:
		return markdownCodeBaseStyle
	}
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

func renderMarkdownTable(surface markdownSurface, table markdownTableBlock, width int) []markdownRenderLine {
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
		return renderStackedMarkdownTable(surface, headerTexts, table.Rows, width)
	}

	return renderGridMarkdownTable(surface, headerTexts, table.Rows, gridWidths)
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

func renderGridMarkdownTable(surface markdownSurface, header [][]markdownSpan, rows [][]markdownTableCell, widths []int) []markdownRenderLine {
	lines := make([]markdownRenderLine, 0, len(rows)*3+4)

	// Enhanced table border styles
	lines = append(lines, renderEnhancedTableBorderLine(widths, markdownTableCornerTopLeft, "┬", markdownTableCornerTopRight, "+", "+", "+"))

	// Render header with enhanced style
	lines = append(lines, renderTableGridRow(header, widths, markdownTableHeaderStyle)...)

	// Enhanced table separator
	lines = append(lines, renderEnhancedTableBorderLine(widths, "├", markdownTableCross, "┤", "+", "+", "+"))

	// Render table rows with alternating styles
	for rowIndex, row := range rows {
		cells := make([][]markdownSpan, len(widths))
		for i := range widths {
			cells[i] = cellSpansAt(row, i)
		}

		// Use alternating row styles for better readability
		rowStyle := markdownBodyStyle
		if rowIndex%2 == 1 {
			rowStyle = markdownTableAltRowStyle
		}

		lines = append(lines, renderTableGridRow(cells, widths, rowStyle)...)
	}

	// Enhanced table bottom border
	lines = append(lines, renderEnhancedTableBorderLine(widths, markdownTableCornerBottomLeft, "┴", markdownTableCornerBottomRight, "+", "+", "+"))
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

func renderEnhancedTableBorderLine(widths []int, left, middle, right, leftCopy, middleCopy, rightCopy string) markdownRenderLine {
	displayParts := make([]string, 0, len(widths))
	copyParts := make([]string, 0, len(widths))

	for i, width := range widths {
		if i == 0 {
			displayParts = append(displayParts, strings.Repeat("─", width))
			copyParts = append(copyParts, strings.Repeat("-", width))
		} else {
			displayParts = append(displayParts, strings.Repeat("─", width))
			copyParts = append(copyParts, strings.Repeat("-", width))
		}
	}

	return markdownRenderLine{
		Display: markdownTableBorderStyle.Render(left + strings.Join(displayParts, markdownTableBorderStyle.Render(middle)) + right),
		Copy:    leftCopy + strings.Join(copyParts, middleCopy) + rightCopy,
	}
}

func renderTableBorderLine(widths []int, left, middle, right, leftCopy, middleCopy, rightCopy string) markdownRenderLine {
	displayParts := make([]string, 0, len(widths))
	copyParts := make([]string, 0, len(widths))
	for i, width := range widths {
		if i == 0 {
			displayParts = append(displayParts, strings.Repeat("─", width))
			copyParts = append(copyParts, strings.Repeat("-", width))
		} else {
			displayParts = append(displayParts, strings.Repeat("─", width))
			copyParts = append(copyParts, strings.Repeat("-", width))
		}
	}
	return markdownRenderLine{
		Display: markdownTableBorderStyle.Render(left) + strings.Join(displayParts, markdownTableBorderStyle.Render(middle)) + markdownTableBorderStyle.Render(right),
		Copy:    leftCopy + strings.Join(copyParts, middleCopy) + rightCopy,
	}
}

func renderStackedMarkdownTable(surface markdownSurface, header [][]markdownSpan, rows [][]markdownTableCell, width int) []markdownRenderLine {
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
	// Enhanced prefixes with more visual variety
	var prefixText string
	var prefixChars []string

	switch level {
	case 1:
		prefixChars = []string{"█ ", "▓ ", "▒ ", "░ ", "◆ ", "◇ "}
		prefixText = prefixChars[0]
	case 2:
		prefixChars = []string{"◆ ", "◇ ", "▸ ", "▹ ", "▪ ", "▫ "}
		prefixText = prefixChars[0]
	case 3:
		prefixChars = []string{"• ", "◦ ", "‣ ", "⁃ ", "∙ ", "○ "}
		prefixText = prefixChars[0]
	default:
		prefixChars = []string{"· ", "∙ ", "• ", "· "}
		prefixText = prefixChars[0]
	}

	styled := markdownHeadingStyle(level).Render(prefixText)
	first := markdownLinePrefix{
		Display: styled,
		Copy:    "",
		Width:   runewidth.StringWidth(prefixText),
	}

	// Add some spacing after the prefix for better visual separation
	spacer := " "
	if level == 1 {
		spacer = "  " // More space for H1
	}

	rest := markdownLinePrefix{
		Display: strings.Repeat(" ", first.Width) + spacer,
		Copy:    spacer,
		Width:   first.Width + len(spacer),
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

// Layout and spacing constants for better visual rhythm
const (
	// Standard spacing units
	markdownSpacingUnit   = 1
	markdownSmallSpacing  = markdownSpacingUnit
	markdownMediumSpacing = markdownSpacingUnit * 2
	markdownLargeSpacing  = markdownSpacingUnit * 3

	// Element-specific spacing
	markdownParagraphSpacing = markdownSmallSpacing
	markdownHeadingSpacing   = markdownMediumSpacing
	markdownListItemSpacing  = markdownSmallSpacing
	markdownCodeBlockSpacing = markdownMediumSpacing
	markdownTableSpacing     = markdownMediumSpacing
	markdownListIndentation  = 2
	markdownQuoteIndentation = 2
)

// Enhanced layout functions for better visual presentation
func calculateOptimalSpacing(prevBlock, currBlock markdownBlockKind) int {
	// Define spacing rules between different block types
	spacingMatrix := map[[2]markdownBlockKind]int{
		{markdownBlockParagraph, markdownBlockParagraph}: markdownParagraphSpacing,
		{markdownBlockParagraph, markdownBlockHeading}:   markdownHeadingSpacing,
		{markdownBlockParagraph, markdownBlockList}:      markdownSmallSpacing,
		{markdownBlockParagraph, markdownBlockQuote}:     markdownSmallSpacing,
		{markdownBlockParagraph, markdownBlockCode}:      markdownCodeBlockSpacing,
		{markdownBlockParagraph, markdownBlockTable}:     markdownTableSpacing,
		{markdownBlockParagraph, markdownBlockRule}:      markdownMediumSpacing,

		{markdownBlockHeading, markdownBlockParagraph}: markdownMediumSpacing,
		{markdownBlockHeading, markdownBlockHeading}:   markdownHeadingSpacing,
		{markdownBlockHeading, markdownBlockList}:      markdownMediumSpacing,
		{markdownBlockHeading, markdownBlockQuote}:     markdownMediumSpacing,
		{markdownBlockHeading, markdownBlockCode}:      markdownMediumSpacing,
		{markdownBlockHeading, markdownBlockTable}:     markdownMediumSpacing,
		{markdownBlockHeading, markdownBlockRule}:      markdownMediumSpacing,

		{markdownBlockList, markdownBlockParagraph}: markdownSmallSpacing,
		{markdownBlockList, markdownBlockHeading}:   markdownMediumSpacing,
		{markdownBlockList, markdownBlockList}:      markdownListItemSpacing,
		{markdownBlockList, markdownBlockQuote}:     markdownSmallSpacing,
		{markdownBlockList, markdownBlockCode}:      markdownSmallSpacing,
		{markdownBlockList, markdownBlockTable}:     markdownTableSpacing,
		{markdownBlockList, markdownBlockRule}:      markdownMediumSpacing,

		{markdownBlockQuote, markdownBlockParagraph}: markdownSmallSpacing,
		{markdownBlockQuote, markdownBlockHeading}:   markdownMediumSpacing,
		{markdownBlockQuote, markdownBlockList}:      markdownSmallSpacing,
		{markdownBlockQuote, markdownBlockQuote}:     markdownSmallSpacing,
		{markdownBlockQuote, markdownBlockCode}:      markdownSmallSpacing,
		{markdownBlockQuote, markdownBlockTable}:     markdownTableSpacing,
		{markdownBlockQuote, markdownBlockRule}:      markdownMediumSpacing,

		{markdownBlockCode, markdownBlockParagraph}: markdownMediumSpacing,
		{markdownBlockCode, markdownBlockHeading}:   markdownMediumSpacing,
		{markdownBlockCode, markdownBlockList}:      markdownSmallSpacing,
		{markdownBlockCode, markdownBlockQuote}:     markdownSmallSpacing,
		{markdownBlockCode, markdownBlockCode}:      markdownCodeBlockSpacing,
		{markdownBlockCode, markdownBlockTable}:     markdownTableSpacing,
		{markdownBlockCode, markdownBlockRule}:      markdownMediumSpacing,

		{markdownBlockTable, markdownBlockParagraph}: markdownMediumSpacing,
		{markdownBlockTable, markdownBlockHeading}:   markdownMediumSpacing,
		{markdownBlockTable, markdownBlockList}:      markdownSmallSpacing,
		{markdownBlockTable, markdownBlockQuote}:     markdownSmallSpacing,
		{markdownBlockTable, markdownBlockCode}:      markdownSmallSpacing,
		{markdownBlockTable, markdownBlockTable}:     markdownTableSpacing,
		{markdownBlockTable, markdownBlockRule}:      markdownMediumSpacing,

		{markdownBlockRule, markdownBlockParagraph}: markdownMediumSpacing,
		{markdownBlockRule, markdownBlockHeading}:   markdownMediumSpacing,
		{markdownBlockRule, markdownBlockList}:      markdownMediumSpacing,
		{markdownBlockRule, markdownBlockQuote}:     markdownMediumSpacing,
		{markdownBlockRule, markdownBlockCode}:      markdownMediumSpacing,
		{markdownBlockRule, markdownBlockTable}:     markdownMediumSpacing,
		{markdownBlockRule, markdownBlockRule}:      markdownMediumSpacing,
	}

	if spacing, ok := spacingMatrix[[2]markdownBlockKind{prevBlock, currBlock}]; ok {
		return spacing
	}

	// Default spacing for undefined combinations
	return markdownSmallSpacing
}

// Icon selection functions for enhanced visual representation
func selectBulletIcon(listIndex int, listDepth int) string {
	// Different bullet styles based on depth and index
	bulletStyles := []string{
		markdownBulletGlyph,        // Default bullet
		markdownBulletAltGlyph,     // White bullet
		markdownBulletSquareGlyph,  // Square bullet
		markdownBulletDiamondGlyph, // Diamond bullet
		markdownBulletStarGlyph,    // Star bullet
		markdownBulletArrowGlyph,   // Arrow bullet
	}

	// Rotate through styles based on depth
	styleIndex := listDepth % len(bulletStyles)
	return bulletStyles[styleIndex]
}

func selectOrderedMarker(listIndex int, listDepth int, startIndex int) string {
	return fmt.Sprintf("%d. ", startIndex+listIndex)
}

func selectCodeIcon(language string) string {
	// Different icons based on code language or type
	language = strings.ToLower(language)
	switch {
	case strings.Contains(language, "error") || strings.Contains(language, "err"):
		return markdownCodeErrorGlyph
	case strings.Contains(language, "warn") || strings.Contains(language, "warning"):
		return markdownCodeWarnGlyph
	case strings.Contains(language, "info") || strings.Contains(language, "note"):
		return markdownCodeInfoGlyph
	case strings.Contains(language, "success") || strings.Contains(language, "ok"):
		return markdownCodeSuccessGlyph
	case strings.Contains(language, "tip") || strings.Contains(language, "hint"):
		return markdownCodeTipGlyph
	default:
		return "" // No special icon for regular code blocks
	}
}

// Highlight patterns and styles for enhanced markdown rendering
var (
	highlightPatterns = map[string]*regexp.Regexp{
		"yellow": regexp.MustCompile(`==([^=]+)==`),     // ==text== for yellow highlight
		"blue":   regexp.MustCompile(`\^\^([^^]+)\^\^`), // ^^text^^ for blue highlight
		"green":  regexp.MustCompile(`__([^_]+)__`),     // __text__ for green highlight
		"red":    regexp.MustCompile(`~~([^^]+)~~`),     // ~~text~~ for red highlight (strikethrough alternative)
		"purple": regexp.MustCompile(`##([^#]+)##`),     // ##text## for purple highlight
		"orange": regexp.MustCompile(`\*\*([^*]+)\*\*`), // **text** for orange highlight (bold alternative)
	}

	highlightStyles = map[string]lipgloss.Style{
		"yellow": markdownHighlightYellowStyle,
		"blue":   markdownHighlightBlueStyle,
		"green":  markdownHighlightGreenStyle,
		"red":    markdownHighlightRedStyle,
		"purple": markdownHighlightPurpleStyle,
		"orange": markdownHighlightOrangeStyle,
	}
)

// Process highlights in text and return styled spans
func processHighlights(text string) []markdownSpan {
	spans := []markdownSpan{{Text: text}}

	// Process each highlight type
	for color, pattern := range highlightPatterns {
		newSpans := []markdownSpan{}

		for _, span := range spans {
			if span.Styled {
				// Skip spans that already have styling
				newSpans = append(newSpans, span)
				continue
			}

			matches := pattern.FindAllStringSubmatchIndex(span.Text, -1)
			if len(matches) == 0 {
				// No matches in this span, keep it as is
				newSpans = append(newSpans, span)
				continue
			}

			// Process matches and create new spans
			lastEnd := 0
			for _, match := range matches {
				// Add text before the match
				if match[0] > lastEnd {
					newSpans = append(newSpans, markdownSpan{
						Text: span.Text[lastEnd:match[0]],
					})
				}

				// Add highlighted text
				highlightedContent := span.Text[match[2]:match[3]]
				newSpans = append(newSpans, markdownSpan{
					Text:   highlightedContent,
					Style:  highlightStyles[color],
					Styled: true,
				})

				lastEnd = match[1]
			}

			// Add remaining text after the last match
			if lastEnd < len(span.Text) {
				newSpans = append(newSpans, markdownSpan{
					Text: span.Text[lastEnd:],
				})
			}
		}

		spans = newSpans
	}

	return spans
}

// Annotation patterns and styles for enhanced markdown rendering
var (
	annotationPatterns = map[string]*regexp.Regexp{
		"note":      regexp.MustCompile(`::note\(([^)]+)\)::\s*([^:]+)`),      // ::note(text):: content
		"important": regexp.MustCompile(`::important\(([^)]+)\)::\s*([^:]+)`), // ::important(text):: content
		"todo":      regexp.MustCompile(`::todo\(([^)]+)\)::\s*([^:]+)`),      // ::todo(text):: content
		"question":  regexp.MustCompile(`::question\(([^)]+)\)::\s*([^:]+)`),  // ::question(text):: content
		"warning":   regexp.MustCompile(`::warning\(([^)]+)\)::\s*([^:]+)`),   // ::warning(text):: content
	}

	annotationIcons = map[string]string{
		"note":      "📝",
		"important": "❗",
		"todo":      "📋",
		"question":  "❓",
		"warning":   "⚠️",
	}

	annotationStyles = map[string]lipgloss.Style{
		"note":      markdownHighlightBlueStyle,
		"important": markdownHighlightRedStyle,
		"todo":      markdownHighlightOrangeStyle,
		"question":  markdownHighlightPurpleStyle,
		"warning":   markdownHighlightYellowStyle,
	}
)

// Annotation represents a structured annotation with type, label, and content
type Annotation struct {
	Type    string
	Label   string
	Content string
}

// Parse annotations in text and return both annotations and cleaned text
func parseAnnotations(text string) ([]Annotation, string) {
	annotations := []Annotation{}
	cleanedText := text

	// Process each annotation type
	for annType, pattern := range annotationPatterns {
		matches := pattern.FindAllStringSubmatchIndex(cleanedText, -1)

		// Process matches in reverse order to avoid index shifting
		for i := len(matches) - 1; i >= 0; i-- {
			match := matches[i]

			// Extract annotation components
			label := cleanedText[match[2]:match[3]]
			content := cleanedText[match[4]:match[5]]

			// Create annotation
			annotation := Annotation{
				Type:    annType,
				Label:   label,
				Content: content,
			}
			annotations = append(annotations, annotation)

			// Remove annotation from text
			cleanedText = cleanedText[:match[0]] + content + cleanedText[match[1]:]
		}
	}

	return annotations, cleanedText
}

// Process annotations in paragraph text and return styled spans with annotations
func processAnnotationsWithHighlights(spans []markdownSpan) ([]markdownSpan, []Annotation) {
	// Combine all text from spans
	combinedText := plainTextFromSpans(spans)

	// Parse annotations
	annotations, cleanedText := parseAnnotations(combinedText)

	// Process highlights in the cleaned text
	highlightedSpans := processHighlights(cleanedText)

	return highlightedSpans, annotations
}

func textpkgNewReader(source []byte) textpkg.Reader {
	return textpkg.NewReader(source)
}
