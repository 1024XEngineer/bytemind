package tui

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/fatih/color"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

type semanticTagRecord struct {
	Tag     string
	Content string
}

var (
	tagTokenRegex = regexp.MustCompile(`ZZBYTEMINDTAG([A-Z]+)X([0-9]+)ZZ`)

	semanticTagOrder = []string{"info", "tool", "success", "error", "warning", "dim"}

	semanticTagColors = map[string]*color.Color{
		"info":    color.New(color.FgHiCyan),
		"tool":    color.New(color.FgHiCyan),
		"success": color.New(color.FgHiGreen),
		"error":   color.New(color.FgRed),
		"warning": color.New(color.FgYellow),
		"dim":     color.New(color.Faint, color.FgHiBlack),
	}

	glamourRenderers sync.Map // map[string]*glamour.TermRenderer
)

func renderAssistantBody(text string, width int) string {
	result := renderStructuredMarkdown(markdownSurfaceAssistant, text, width)
	if strings.TrimSpace(result.Display) != "" {
		return result.Display
	}
	return renderAssistantBodyLegacy(text, width)
}

func renderAssistantBodyWithMarkdownAndTags(text string, width int) (string, error) {
	prepared, records := preprocessSemanticTags(text)
	renderer, err := getBlueGlamourRenderer(width)
	if err != nil {
		return "", err
	}

	rendered, err := renderer.Render(prepared)
	if err != nil {
		return "", err
	}

	rendered = strings.Trim(rendered, "\n")
	rendered = postprocessSemanticTags(rendered, records)
	rendered = trimRenderedTrailingSpaces(rendered)
	return rendered, nil
}

func preprocessSemanticTags(raw string) (string, map[string]semanticTagRecord) {
	current := raw
	records := make(map[string]semanticTagRecord)
	counter := 0

	for _, tag := range semanticTagOrder {
		re := regexp.MustCompile(`(?s)<` + tag + `>(.*?)</` + tag + `>`)
		current = re.ReplaceAllStringFunc(current, func(match string) string {
			sub := re.FindStringSubmatch(match)
			if len(sub) < 2 {
				return match
			}
			id := fmt.Sprintf("%d", counter)
			counter++
			records[id] = semanticTagRecord{Tag: tag, Content: sub[1]}
			return "ZZBYTEMINDTAG" + strings.ToUpper(tag) + "X" + id + "ZZ"
		})
	}

	return current, records
}

func postprocessSemanticTags(rendered string, records map[string]semanticTagRecord) string {
	return tagTokenRegex.ReplaceAllStringFunc(rendered, func(token string) string {
		sub := tagTokenRegex.FindStringSubmatch(token)
		if len(sub) != 3 {
			return token
		}

		tag := strings.ToLower(sub[1])
		id := sub[2]
		record, ok := records[id]
		if !ok || record.Tag != tag {
			return token
		}

		painter, ok := semanticTagColors[tag]
		if !ok {
			return record.Content
		}
		return painter.Sprint(record.Content)
	})
}

func getBlueGlamourRenderer(width int) (*glamour.TermRenderer, error) {
	wrapWidth := width
	if wrapWidth <= 0 {
		wrapWidth = 100
	}
	if wrapWidth < 40 {
		wrapWidth = 40
	}

	profile, profileKey := renderColorProfile()
	cacheKey := fmt.Sprintf("%s-%d", profileKey, wrapWidth)

	if cached, ok := glamourRenderers.Load(cacheKey); ok {
		if renderer, ok := cached.(*glamour.TermRenderer); ok {
			return renderer, nil
		}
	}

	renderer, err := glamour.NewTermRenderer(
		glamour.WithWordWrap(wrapWidth),
		glamour.WithColorProfile(profile),
		glamour.WithStyles(blueGlamourStyle()),
	)
	if err != nil {
		return nil, err
	}
	glamourRenderers.Store(cacheKey, renderer)
	return renderer, nil
}

func blueGlamourStyle() ansi.StyleConfig {
	cfg := styles.DarkStyleConfig

	bodyText := "#D8E4F0"
	mutedText := "#7D8FA6"
	structureBlue := "#6CB6FF"
	headingBlue := "#EAF4FF"
	secondaryHeading := "#B8DDFF"
	linkCyan := "#7FE6FF"
	quoteGold := "#E7C27D"
	inlineCodeText := "#E6B873"
	strongText := "#F6FBFF"
	emphasisCyan := "#8FDFFF"
	codeText := "#D7E3F4"
	codeKeyword := "#7CB8FF"
	codeType := "#9B8CFF"
	codeString := "#E6B873"
	codeFunction := "#7EE0B5"
	codeNumber := "#8DE1C5"
	codeOperator := "#FF9CAC"
	codeComment := "#6F8197"
	codeBackground := "#000000"

	cfg.Document.BlockPrefix = ""
	cfg.Document.BlockSuffix = ""
	cfg.Document.Margin = uintPtr(0)
	cfg.Document.Color = strPtr(bodyText)
	cfg.Paragraph.Color = strPtr(bodyText)

	cfg.BlockQuote.IndentToken = strPtr("\u258e ")
	cfg.BlockQuote.Color = strPtr(quoteGold)
	cfg.BlockQuote.Indent = uintPtr(0)

	cfg.Strong.Color = strPtr(strongText)
	cfg.Strong.Bold = boolPtr(true)
	cfg.Emph.Color = strPtr(emphasisCyan)
	cfg.Emph.Italic = boolPtr(true)
	cfg.Link.Color = strPtr(linkCyan)
	cfg.Link.Underline = boolPtr(true)
	cfg.LinkText.Color = strPtr(linkCyan)
	cfg.LinkText.Bold = boolPtr(true)
	cfg.HorizontalRule.Color = strPtr(mutedText)
	cfg.HorizontalRule.Format = "\n\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\u2500\n"

	cfg.Code.Color = strPtr(inlineCodeText)
	cfg.Code.BackgroundColor = nil
	cfg.Code.Bold = boolPtr(true)
	cfg.Code.Prefix = " "
	cfg.Code.Suffix = " "
	cfg.CodeBlock.Color = strPtr(codeText)
	cfg.CodeBlock.BackgroundColor = strPtr(codeBackground)
	cfg.CodeBlock.Margin = uintPtr(0)
	cfg.CodeBlock.Chroma = &ansi.Chroma{
		Text: ansi.StylePrimitive{
			Color: strPtr(codeText),
		},
		Comment: ansi.StylePrimitive{
			Color: strPtr(codeComment),
		},
		CommentPreproc: ansi.StylePrimitive{
			Color: strPtr(codeOperator),
		},
		Keyword: ansi.StylePrimitive{
			Color: strPtr(codeKeyword),
		},
		KeywordReserved: ansi.StylePrimitive{
			Color: strPtr(codeOperator),
		},
		KeywordNamespace: ansi.StylePrimitive{
			Color: strPtr(codeOperator),
		},
		KeywordType: ansi.StylePrimitive{
			Color: strPtr(codeType),
		},
		Operator: ansi.StylePrimitive{
			Color: strPtr(codeOperator),
		},
		Punctuation: ansi.StylePrimitive{
			Color: strPtr("#AFC4DC"),
		},
		Name: ansi.StylePrimitive{
			Color: strPtr(codeText),
		},
		NameBuiltin: ansi.StylePrimitive{
			Color: strPtr(codeOperator),
		},
		NameTag: ansi.StylePrimitive{
			Color: strPtr(codeKeyword),
		},
		NameAttribute: ansi.StylePrimitive{
			Color: strPtr(secondaryHeading),
		},
		NameClass: ansi.StylePrimitive{
			Color: strPtr(strongText),
			Bold:  boolPtr(true),
		},
		NameDecorator: ansi.StylePrimitive{
			Color: strPtr(quoteGold),
		},
		NameFunction: ansi.StylePrimitive{
			Color: strPtr(codeFunction),
		},
		LiteralNumber: ansi.StylePrimitive{
			Color: strPtr(codeNumber),
		},
		LiteralString: ansi.StylePrimitive{
			Color: strPtr(codeString),
		},
		LiteralStringEscape: ansi.StylePrimitive{
			Color: strPtr(linkCyan),
		},
		GenericDeleted: ansi.StylePrimitive{
			Color: strPtr("#FF8F8F"),
		},
		GenericEmph: ansi.StylePrimitive{
			Italic: boolPtr(true),
		},
		GenericInserted: ansi.StylePrimitive{
			Color: strPtr(codeFunction),
		},
		GenericStrong: ansi.StylePrimitive{
			Bold: boolPtr(true),
		},
		GenericSubheading: ansi.StylePrimitive{
			Color: strPtr(mutedText),
		},
		Background: ansi.StylePrimitive{
			BackgroundColor: strPtr(codeBackground),
		},
	}

	cfg.Item.BlockPrefix = "\u2022 "
	cfg.Item.Color = strPtr(structureBlue)
	cfg.Enumeration.Color = strPtr(structureBlue)
	cfg.Task.Color = strPtr(structureBlue)
	cfg.Task.Ticked = "[\u2713] "
	cfg.Task.Unticked = "[ ] "

	cfg.Heading.Color = strPtr(structureBlue)
	cfg.Heading.Bold = boolPtr(true)
	cfg.H1.Color = strPtr(headingBlue)
	cfg.H1.BackgroundColor = nil
	cfg.H1.Prefix = "\u258d "
	cfg.H1.Suffix = ""
	cfg.H2.Color = strPtr(structureBlue)
	cfg.H2.Prefix = "\u25c6 "
	cfg.H3.Color = strPtr(secondaryHeading)
	cfg.H3.Prefix = "\u25b8 "
	cfg.H4.Color = strPtr(secondaryHeading)
	cfg.H4.Prefix = "\u2022 "
	cfg.H5.Color = strPtr(mutedText)
	cfg.H5.Prefix = "\u00b7 "
	cfg.H6.Color = strPtr(mutedText)
	cfg.H6.Prefix = "\u00b7 "
	cfg.H6.Bold = boolPtr(false)
	cfg.Table.Color = strPtr(bodyText)
	cfg.Table.CenterSeparator = strPtr("\u253c")
	cfg.Table.ColumnSeparator = strPtr("\u2502")
	cfg.Table.RowSeparator = strPtr("\u2500")

	return cfg
}

func strPtr(v string) *string { return &v }
func boolPtr(v bool) *bool    { return &v }
func uintPtr(v uint) *uint    { return &v }

func renderColorProfile() (termenv.Profile, string) {
	if isTerminalOutput() {
		return termenv.TrueColor, "color"
	}
	return termenv.Ascii, "ascii"
}

func isTerminalOutput() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

func trimRenderedTrailingSpaces(rendered string) string {
	if rendered == "" {
		return ""
	}
	lines := strings.Split(rendered, "\n")
	for i := range lines {
		lines[i] = strings.TrimRight(lines[i], " \t")
	}
	return strings.Join(lines, "\n")
}
