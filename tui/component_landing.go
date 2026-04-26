package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
)

var landingShortcutHints = []footerShortcutHint{
	{Key: "/", Label: "commands"},
	{Key: "Ctrl+L", Label: "sessions"},
	{Key: "Ctrl+C", Label: "quit"},
}

var landingPixelGlyphs = map[rune][]string{
	'B': {"11110", "10001", "11110", "10001", "11110"},
	'Y': {"10001", "10001", "01010", "00100", "00100"},
	'T': {"11111", "00100", "00100", "00100", "00100"},
	'E': {"11111", "10000", "11110", "10000", "11111"},
	'M': {"10001", "11011", "10101", "10001", "10001"},
	'I': {"11111", "00100", "00100", "00100", "11111"},
	'N': {"10001", "11001", "10101", "10011", "10001"},
	'D': {"11110", "10001", "10001", "10001", "11110"},
}

var landingLogoGlyphLines = buildLandingLogoGlyphLines("BYTEMIND")

func buildLandingLogoGlyphLines(text string) []string {
	rows := []strings.Builder{
		{}, {}, {}, {}, {},
	}
	for i, r := range text {
		glyph, ok := landingPixelGlyphs[r]
		if !ok || len(glyph) != len(rows) {
			continue
		}
		for row := range rows {
			rows[row].WriteString(glyph[row])
			if i < len(text)-1 {
				rows[row].WriteByte('0')
			}
		}
	}

	out := make([]string, len(rows))
	for i := range rows {
		out[i] = rows[i].String()
	}
	return out
}

func (m model) renderLandingHero() string {
	logoRows := len(landingLogoGlyphLines)
	logoCols := 0
	if logoRows > 0 {
		logoCols = len(landingLogoGlyphLines[0])
	}
	anim := m.landingAnimationState(logoCols, logoRows)

	pixelRows := make([]string, 0, len(landingLogoGlyphLines))
	for rowIndex, row := range landingLogoGlyphLines {
		pixelRows = append(pixelRows, m.renderLandingPixelRow(row, rowIndex, anim.logoBeam10, anim.logoActive))
	}
	innerWidth := maxLineWidth(pixelRows)
	frameGlow := landingFrameGlowPosition(anim.frameStep, innerWidth, len(pixelRows), anim.frameActive)
	topFrame := renderLandingFrameLine("+"+dashedPattern(innerWidth+2)+"+", frameGlow.topCol)

	lines := make([]string, 0, len(pixelRows)+1)
	lines = append(lines, topFrame)
	for rowIndex, line := range pixelRows {
		pad := max(0, innerWidth-lipgloss.Width(line))
		padded := line + strings.Repeat(" ", pad)
		leftStyle := landingLogoFrameStyle
		if frameGlow.leftRow >= 0 {
			switch absInt(rowIndex - frameGlow.leftRow) {
			case 0:
				leftStyle = landingLogoFrameGlowStyle
			case 1:
				leftStyle = landingLogoFrameSoftStyle
			}
		}
		rightStyle := landingLogoFrameStyle
		if frameGlow.rightRow >= 0 {
			switch absInt(rowIndex - frameGlow.rightRow) {
			case 0:
				rightStyle = landingLogoFrameGlowStyle
			case 1:
				rightStyle = landingLogoFrameSoftStyle
			}
		}
		framed := leftStyle.Render("| ") +
			padded +
			rightStyle.Render(" |")
		lines = append(lines, framed)
	}

	frame := strings.Join(lines, "\n")
	subtitle := landingSubtitleStyle.Render("Your AI assistant")
	return frame + "\n\n" + subtitle
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
	awayStyle := landingModeInactiveStyle
	if m.mode == modeBuild {
		buildStyle = landingModeBuildActiveStyle
	} else {
		planStyle = landingModePlanActiveStyle
	}
	if m.awayEnabled() {
		awayStyle = landingModeAwayActiveStyle
	}
	sep := landingModeInactiveStyle.Render("   ")
	return buildStyle.Render("Build") +
		sep +
		planStyle.Render("Plan") +
		sep +
		awayStyle.Render(m.awayStatusLabel())
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
		return line
	}
	left := max(0, (m.width-lineWidth)/2)
	right := max(0, m.width-left-lineWidth)
	return rowStyle.Width(left).Render("") + rowStyle.Render(line) + rowStyle.Width(right).Render("")
}

func (m model) landingGradientColor(row int) lipgloss.Color {
	return colorLandingPanel
}

type landingAnimationState struct {
	logoActive  bool
	logoBeam10  int
	frameActive bool
	frameStep   int
}

func (m model) landingAnimationState(logoCols, logoRows int) landingAnimationState {
	if logoCols <= 0 || logoRows <= 0 {
		return landingAnimationState{}
	}
	const (
		logoStartCol = -10
		logoAdvance  = 3 // 0.3 col per tick for smoother motion.
	)
	topShift := max(0, (logoRows-1)*2)
	logoEndCol := max(logoStartCol, logoCols-1-topShift)
	logoDistance10 := max(0, (logoEndCol-logoStartCol)*10)
	logoFrames := 1 + ((logoDistance10 + logoAdvance - 1) / logoAdvance)

	innerWidth := logoCols * 2
	frameFrames := landingFrameTravelFrames(innerWidth, logoRows)

	totalFrames := logoFrames + frameFrames
	if totalFrames <= 0 {
		return landingAnimationState{}
	}
	step := m.landingGlowStep % totalFrames
	if step < logoFrames {
		return landingAnimationState{
			logoActive: true,
			logoBeam10: logoStartCol*10 + step*logoAdvance,
		}
	}
	return landingAnimationState{
		logoActive:  false,
		logoBeam10:  logoEndCol * 10,
		frameActive: true,
		frameStep:   step - logoFrames,
	}
}

type landingFrameGlowState struct {
	topCol   int
	leftRow  int
	rightRow int
}

func landingFrameTravelFrames(innerWidth, rowCount int) int {
	if innerWidth <= 0 || rowCount <= 0 {
		return 1
	}
	topFrameLen := innerWidth + 4
	rightDown := rowCount
	rightUp := max(0, rowCount-1)
	leftDown := max(0, rowCount-1)
	return max(1, rightDown+rightUp+topFrameLen+leftDown)
}

func landingFrameGlowPosition(frameStep, innerWidth, rowCount int, active bool) landingFrameGlowState {
	if !active || innerWidth <= 0 || rowCount <= 0 {
		return landingFrameGlowState{topCol: -1, leftRow: -1, rightRow: -1}
	}

	topFrameLen := innerWidth + 4
	rightDown := rowCount
	rightUp := max(0, rowCount-1)
	leftDown := max(0, rowCount-1)
	total := rightDown + rightUp + topFrameLen + leftDown
	if total <= 0 {
		return landingFrameGlowState{topCol: -1, leftRow: -1, rightRow: -1}
	}

	step := frameStep % total
	if step < rightDown {
		return landingFrameGlowState{topCol: -1, leftRow: -1, rightRow: step}
	}
	step -= rightDown

	if step < rightUp {
		return landingFrameGlowState{topCol: -1, leftRow: -1, rightRow: (rowCount - 2) - step}
	}
	step -= rightUp

	if step < topFrameLen {
		return landingFrameGlowState{topCol: topFrameLen - 1 - step, leftRow: -1, rightRow: -1}
	}
	step -= topFrameLen

	if step < leftDown {
		return landingFrameGlowState{topCol: -1, leftRow: step + 1, rightRow: -1}
	}
	return landingFrameGlowState{topCol: -1, leftRow: rowCount - 1, rightRow: -1}
}

func renderLandingFrameLine(line string, glowCol int) string {
	if glowCol < 0 {
		return landingLogoFrameStyle.Render(line)
	}
	var b strings.Builder
	for idx, r := range line {
		ch := string(r)
		switch absInt(idx - glowCol) {
		case 0:
			b.WriteString(landingLogoFrameGlowStyle.Render(ch))
		case 1:
			b.WriteString(landingLogoFrameSoftStyle.Render(ch))
		default:
			b.WriteString(landingLogoFrameStyle.Render(ch))
		}
	}
	return b.String()
}

func (m model) renderLandingPixelRow(pattern string, row int, beamRaw10 int, active bool) string {
	topRow := len(landingLogoGlyphLines) - 1
	beamCol10 := beamRaw10 + max(0, topRow-row)*20

	var b strings.Builder
	for col, ch := range pattern {
		if ch != '1' {
			b.WriteString("  ")
			continue
		}
		if !active {
			b.WriteString(landingLogoPixelStyle.Render("  "))
			continue
		}
		d10 := absInt(col*10 - beamCol10)
		switch {
		case d10 <= 2:
			b.WriteString(landingLogoPixelGlowStyle.Render("  "))
		case d10 <= 11:
			b.WriteString(landingLogoPixelSoftStyle.Render("  "))
		default:
			b.WriteString(landingLogoPixelStyle.Render("  "))
		}
	}
	return b.String()
}

func dashedPattern(width int) string {
	if width <= 0 {
		return ""
	}
	pattern := strings.Repeat("- ", (width/2)+2)
	return pattern[:width]
}

func maxLineWidth(lines []string) int {
	maxWidth := 0
	for _, line := range lines {
		if w := lipgloss.Width(line); w > maxWidth {
			maxWidth = w
		}
	}
	return maxWidth
}

func centeredText(text string, width int) string {
	text = strings.TrimSpace(text)
	if width <= lipgloss.Width(text) {
		return text
	}
	left := (width - lipgloss.Width(text)) / 2
	right := max(0, width-left-lipgloss.Width(text))
	return strings.Repeat(" ", left) + text + strings.Repeat(" ", right)
}

func lerpHexColor(startHex, endHex string, t float64) lipgloss.Color {
	sr, sg, sb := parseHexColor(startHex)
	er, eg, eb := parseHexColor(endHex)
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	r := int(float64(sr) + (float64(er-sr) * t))
	g := int(float64(sg) + (float64(eg-sg) * t))
	b := int(float64(sb) + (float64(eb-sb) * t))
	return lipgloss.Color(formatHexColor(r, g, b))
}

func parseHexColor(hex string) (int, int, int) {
	value := strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(value) != 6 {
		return 0, 0, 0
	}
	parseByte := func(s string) int {
		var v int
		for _, ch := range s {
			v <<= 4
			switch {
			case ch >= '0' && ch <= '9':
				v += int(ch - '0')
			case ch >= 'a' && ch <= 'f':
				v += int(ch-'a') + 10
			case ch >= 'A' && ch <= 'F':
				v += int(ch-'A') + 10
			}
		}
		return v
	}
	return parseByte(value[0:2]), parseByte(value[2:4]), parseByte(value[4:6])
}

func formatHexColor(r, g, b int) string {
	clampChannel := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > 255 {
			return 255
		}
		return v
	}
	const hexDigits = "0123456789ABCDEF"
	r = clampChannel(r)
	g = clampChannel(g)
	b = clampChannel(b)
	buf := []byte{'#', '0', '0', '0', '0', '0', '0'}
	buf[1] = hexDigits[(r>>4)&0xF]
	buf[2] = hexDigits[r&0xF]
	buf[3] = hexDigits[(g>>4)&0xF]
	buf[4] = hexDigits[g&0xF]
	buf[5] = hexDigits[(b>>4)&0xF]
	buf[6] = hexDigits[b&0xF]
	return string(buf)
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
