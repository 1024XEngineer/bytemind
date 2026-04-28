package tui

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"
	zone "github.com/lrstanley/bubblezone"
)

var landingShortcutHints = []footerShortcutHint{
	{Key: "/", Label: "commands"},
	{Key: "Ctrl+L", Label: "sessions"},
	{Key: "Ctrl+C", Label: "quit"},
}

func (m model) renderLandingHero() string {
	innerWidth := m.landingPromptHeroWidth()
	borderStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#1E334C"))
	headerBgStyle := lipgloss.NewStyle().Background(lipgloss.Color("#020A14"))
	headerHostStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64DF69"))
	promptSigilStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64DF69")).Bold(true)
	promptLabelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#7FA4CC"))
	brandStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#E6F2FF")).Bold(true)
	pixelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#2BE8FF"))
	pixelGlowStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#91CFD5"))
	dotMutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#455C71"))
	dotActiveStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64DF69"))

	headerHost := headerHostStyle.Render("bytemind@localhost:~")
	dots := strings.Join([]string{
		dotMutedStyle.Render("●"),
		dotMutedStyle.Render("●"),
		dotActiveStyle.Render("●"),
	}, " ")
	headerGap := max(1, innerWidth-lipgloss.Width(headerHost)-lipgloss.Width(dots))
	headerRow := headerBgStyle.Render(padLandingANSI(headerHost+strings.Repeat(" ", headerGap)+dots, innerWidth))

	promptRow := padLandingANSI("  "+promptSigilStyle.Render(">_")+"  "+promptLabelStyle.Render("launching bytemind"), innerWidth)
	pixelRows := landingPixelLogoRows("BYTEMIND", pixelStyle, pixelGlowStyle, m.landingGlowStep, innerWidth-2)
	logoRows := make([]string, 0, len(pixelRows))
	for _, row := range pixelRows {
		left := max(0, (innerWidth-lipgloss.Width(row))/2)
		logoRows = append(logoRows, padLandingANSI(strings.Repeat(" ", left)+row, innerWidth))
	}
	if len(logoRows) == 0 || lipgloss.Width(pixelRows[0])+2 > innerWidth {
		logoRows = []string{padLandingANSI("  "+brandStyle.Render("Bytemind"), innerWidth)}
	}
	frameRows := []string{
		borderStyle.Render("┌" + strings.Repeat("─", innerWidth) + "┐"),
		borderStyle.Render("│") + headerRow + borderStyle.Render("│"),
		borderStyle.Render("├" + strings.Repeat("─", innerWidth) + "┤"),
		borderStyle.Render("│") + promptRow + borderStyle.Render("│"),
		borderStyle.Render("│") + strings.Repeat(" ", innerWidth) + borderStyle.Render("│"),
	}
	for _, row := range logoRows {
		frameRows = append(frameRows, borderStyle.Render("│")+row+borderStyle.Render("│"))
	}
	frameRows = append(
		frameRows,
		borderStyle.Render("└"+strings.Repeat("─", innerWidth)+"┘"),
	)
	frame := strings.Join(frameRows, "\n")

	subtitle := landingSubtitleStyle.Render("Your AI assistant")
	return frame + "\n\n" + subtitle
}

func (m model) landingPromptHeroWidth() int {
	if m.width <= 0 {
		return 74
	}
	maxFit := max(24, m.width-8)
	preferred := min(118, max(74, (m.width*5)/6))
	return clamp(preferred, 62, maxFit)
}

func padLandingANSI(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if w := lipgloss.Width(text); w < width {
		return text + strings.Repeat(" ", width-w)
	}
	return xansi.Cut(text, 0, width)
}

var landingPixelGlyphs = map[rune][]string{
	'B': {
		"11110",
		"10001",
		"10001",
		"11110",
		"10001",
		"10001",
		"11110",
	},
	'Y': {
		"10001",
		"10001",
		"01010",
		"00100",
		"00100",
		"00100",
		"01110",
	},
	'T': {
		"11111",
		"00100",
		"00100",
		"00100",
		"00100",
		"00100",
		"00100",
	},
	'E': {
		"11111",
		"10000",
		"10000",
		"11110",
		"10000",
		"10000",
		"11111",
	},
	'M': {
		"10001",
		"11011",
		"10101",
		"10001",
		"10001",
		"10001",
		"10001",
	},
	'I': {
		"11111",
		"00100",
		"00100",
		"00100",
		"00100",
		"00100",
		"11111",
	},
	'N': {
		"10001",
		"11001",
		"10101",
		"10011",
		"10001",
		"10001",
		"10001",
	},
	'D': {
		"11110",
		"10001",
		"10001",
		"10001",
		"10001",
		"10001",
		"11110",
	},
	' ': {
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
		"00000",
	},
}

func landingPixelLogoRows(text string, onStyle lipgloss.Style, glowStyle lipgloss.Style, glowStep int, maxWidth int) []string {
	const glyphHeight = 7
	runes := []rune(text)
	if len(runes) == 0 {
		return nil
	}
	renderWithCells := func(onCell, offCell, letterGap string, letterGapCols int) []string {
		prefixCols := make([]int, len(runes))
		totalCols := 0
		for i, r := range runes {
			glyph, ok := landingPixelGlyphs[unicode.ToUpper(r)]
			if !ok {
				glyph = landingPixelGlyphs[' ']
			}
			prefixCols[i] = totalCols
			totalCols += len(glyph[0])
			if i < len(runes)-1 {
				totalCols += letterGapCols
			}
		}
		glowWidth := max(2, totalCols/10)
		glowSpan := totalCols + glowWidth + 6
		glowHead := 0
		if glowSpan > 0 {
			glowHead = glowStep % glowSpan
		}
		glowStart := glowHead - glowWidth

		rowBuilders := make([]strings.Builder, glyphHeight)
		for i, r := range runes {
			glyph, ok := landingPixelGlyphs[unicode.ToUpper(r)]
			if !ok {
				glyph = landingPixelGlyphs[' ']
			}
			for rowIdx := 0; rowIdx < glyphHeight; rowIdx++ {
				rowPattern := glyph[rowIdx]
				colCursor := prefixCols[i]
				for _, cell := range rowPattern {
					if cell == '1' {
						if colCursor >= glowStart && colCursor < glowHead {
							rowBuilders[rowIdx].WriteString(glowStyle.Render(onCell))
						} else {
							rowBuilders[rowIdx].WriteString(onStyle.Render(onCell))
						}
					} else {
						rowBuilders[rowIdx].WriteString(offCell)
					}
					colCursor++
				}
				if i < len(runes)-1 {
					rowBuilders[rowIdx].WriteString(letterGap)
				}
			}
		}
		rows := make([]string, glyphHeight)
		for i := range rowBuilders {
			rows[i] = rowBuilders[i].String()
		}
		return rows
	}

	configs := []struct {
		onCell    string
		offCell   string
		letterGap string
		gapCols   int
	}{
		{onCell: "██", offCell: "  ", letterGap: "  ", gapCols: 2},
		{onCell: "█", offCell: " ", letterGap: " ", gapCols: 1},
		{onCell: "█", offCell: " ", letterGap: "", gapCols: 0},
	}

	lastRows := []string{}
	for _, cfg := range configs {
		rows := renderWithCells(cfg.onCell, cfg.offCell, cfg.letterGap, cfg.gapCols)
		lastRows = rows
		if maxWidth <= 0 || lipgloss.Width(rows[0]) <= maxWidth {
			return rows
		}
	}
	return lastRows
}

func (m model) renderLandingOverlayPanel() string {
	switch {
	case m.startupGuide.Active:
		return m.renderStartupGuidePanel()
	case m.promptSearchOpen:
		return m.renderPromptSearchPalette()
	case m.mentionOpen:
		return m.renderMentionPalette()
	case m.commandOpen:
		return m.renderCommandPalette()
	default:
		return ""
	}
}

func (m model) renderLandingInputBox(markZone bool) string {
	editor := m.renderInputEditorView()
	editor = strings.TrimRight(editor, "\n")
	if editor == "" {
		editor = " "
	}
	editor = ensureMinRows(editor, 2)
	editor = landingInputEditorSurfaceStyle.Copy().
		Width(m.landingInputContentWidth()).
		Render(editor)
	if markZone {
		editor = zone.Mark(inputEditorZoneID, editor)
	}
	return landingInputStyle.Copy().
		BorderForeground(m.modeAccentColor()).
		Width(m.landingInputShellWidth()).
		Render(editor)
}

func ensureMinRows(text string, rows int) string {
	if rows <= 1 {
		return text
	}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return strings.Join(lines, "\n")
}

func (m model) renderLandingInputActions() string {
	actions := landingActionKeyStyle.Render("Enter") + " " + landingActionLabelStyle.Render("send") +
		landingActionDividerStyle.Render(" | ") +
		landingActionKeyStyle.Render("Shift+Enter") + " " + landingActionLabelStyle.Render("newline")
	return landingHintStyle.Render(actions)
}

func (m model) renderLandingModeTabs() string {
	buildStyle := landingModeInactiveStyle
	planStyle := landingModeInactiveStyle
	if m.mode == modeBuild {
		buildStyle = landingModeBuildActiveStyle
	} else {
		planStyle = landingModePlanActiveStyle
	}
	sep := landingModeInactiveStyle.Render("   ")
	return buildStyle.Render("Build") +
		sep +
		planStyle.Render("Plan")
}

func renderLandingShortcutHints() string {
	parts := make([]string, 0, len(landingShortcutHints))
	for _, hint := range landingShortcutHints {
		parts = append(parts, landingShortcutKeyStyle.Render(hint.Key)+" "+landingShortcutLabelStyle.Render(hint.Label))
	}
	return strings.Join(parts, landingShortcutDividerStyle.Render("  |  "))
}

func (m model) renderLandingContent(markInputZone bool) string {
	parts := []string{
		m.renderLandingHero(),
		"",
		m.renderLandingModeTabs(),
	}
	if overlay := strings.TrimSpace(m.renderLandingOverlayPanel()); overlay != "" {
		parts = append(parts, "", overlay)
	}
	parts = append(
		parts,
		"",
		m.renderLandingInputBox(markInputZone),
		m.renderLandingInputActions(),
		"",
		renderLandingShortcutHints(),
	)
	return strings.Join(parts, "\n")
}

func (m model) landingContentHeight() int {
	return lipgloss.Height(m.renderLandingContent(false))
}

func (m model) landingContentTop(contentHeight int) int {
	return max(0, (m.height-contentHeight)/2+1)
}

func (m model) landingInputTop(contentTop int) int {
	top := contentTop + lipgloss.Height(m.renderLandingHero()) + 1 + lipgloss.Height(m.renderLandingModeTabs())
	if overlay := strings.TrimSpace(m.renderLandingOverlayPanel()); overlay != "" {
		top += 1 + lipgloss.Height(overlay)
	}
	return top + 1
}

func (m model) renderLandingCanvas(content string) string {
	if m.width <= 0 || m.height <= 0 {
		return landingCanvasStyle.Render(content)
	}
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	rows := make([]string, 0, m.height)
	topPad := m.landingContentTop(len(lines))
	for i := 0; i < topPad && len(rows) < m.height; i++ {
		rows = append(rows, m.renderLandingCanvasRow("", len(rows)))
	}
	for _, line := range lines {
		if len(rows) >= m.height {
			break
		}
		rows = append(rows, m.renderLandingCanvasRow(line, len(rows)))
	}
	for len(rows) < m.height {
		rows = append(rows, m.renderLandingCanvasRow("", len(rows)))
	}
	return strings.Join(rows, "\n")
}

func (m model) renderLandingCanvasRow(line string, row int) string {
	rowStyle := lipgloss.NewStyle().Background(m.landingGradientColor(row))
	if strings.TrimSpace(line) == "" {
		return rowStyle.Width(m.width).Render("")
	}
	lineWidth := lipgloss.Width(line)
	if lineWidth >= m.width {
		return xansi.Cut(line, 0, m.width)
	}
	left := max(0, (m.width-lineWidth)/2)
	right := max(0, m.width-left-lineWidth)
	return rowStyle.Width(left).Render("") + rowStyle.Render(line) + rowStyle.Width(right).Render("")
}

func (m model) landingGradientColor(row int) lipgloss.Color {
	return colorLandingPanel
}
