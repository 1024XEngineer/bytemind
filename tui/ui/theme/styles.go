package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPanel        = lipgloss.Color("#000000")
	colorBorder       = lipgloss.Color("#314156")
	colorAccent       = lipgloss.Color("#58A6FF")
	colorCard         = lipgloss.Color("#171717")
	colorHotPink      = lipgloss.Color("#F05AA6")
	colorThinking     = lipgloss.Color("#8FD3FF")
	colorThinkingDone = lipgloss.Color("#9AA6B2")
	colorUser         = lipgloss.Color("#F59E0B")
	colorTool         = lipgloss.Color("#BEA15A")
	colorMuted        = lipgloss.Color("#8B949E")
	colorDanger       = lipgloss.Color("#F85149")
	colorSuccess      = lipgloss.Color("#8EE6A0")
	colorCodeBg       = lipgloss.Color("#1E1E1E")
	colorCodeFg       = lipgloss.Color("#D4D4D4")
	colorCodeKeyword  = lipgloss.Color("#569CD6")
	colorCodeString   = lipgloss.Color("#CE9178")
	colorCodeComment  = lipgloss.Color("#6A9955")
	colorCodeNumber   = lipgloss.Color("#B5CEA8")
	colorErrorBg      = lipgloss.Color("#7F1D1D")
	colorErrorFg      = lipgloss.Color("#FFFFFF")
	colorFinalBorder  = lipgloss.Color("#3E5F83")
	colorFinalTitle   = lipgloss.Color("#7FB2E6")
)

var (
	panelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("237")).
			Background(lipgloss.Color("#000000")).
			Padding(0, 1)

	landingCanvasStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#000000"))

	landingLogoByteStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#7A7A7A", Dark: "#8A8A8A"}).
				Bold(false).
				Align(lipgloss.Center)

	landingLogoMindStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFFFFF")).
				Bold(true).
				Align(lipgloss.Center)

	landingLogoStyle = landingLogoMindStyle

	landingInputStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#1A1A1A")).
				Padding(1, 4)

	landingPlaceholderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7C7C7C"))

	landingInputValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EAEAEA"))

	landingModeStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	landingModelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#B9B9B9"))

	landingHintStyle = lipgloss.NewStyle().
				Foreground(lipgloss.AdaptiveColor{Light: "#6C6C6C", Dark: "#565656"})

	landingTipDotStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F59E0B"))

	landingTipLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#BEBEBE")).
				Bold(true)

	landingTipTextStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A4A4A4"))

	footerHintStyle = lipgloss.NewStyle().
			Foreground(lipgloss.AdaptiveColor{Light: "246", Dark: "240"})

	footerHintKeyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#F5F5F5")).
				Bold(true)

	footerHintLabelStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8D97A6"))

	footerHintDividerStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#5C6675"))

	modeBuildActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6CB6FF")).
				Bold(true)

	modePlanActiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A78BFA")).
				Bold(true)

	modeInactiveStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#6B7280"))

	helpHeadingStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#D9E6F2")).
				Bold(true)

	helpCodeStyle = lipgloss.NewStyle().
			Foreground(colorCodeFg).
			Background(colorCodeBg).
			Padding(0, 1)

	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	modeTabStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1).
			MarginRight(1)

	cardTitleStyle = lipgloss.NewStyle().Bold(true)

	chatBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D8D8D8"))

	selectionToastStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#DFF7E8")).
				Background(lipgloss.Color("#163423")).
				Padding(0, 1).
				Bold(true)

	selectionHighlightStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#0B1118")).
				Background(lipgloss.Color("#9CCBFF"))

	assistantHeading1Style = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#EAF6FF"))

	assistantHeading2Style = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#CDEBFF"))

	assistantHeading3Style = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#A9D8FF"))

	listMarkerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	quoteLineStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(colorMuted).
			PaddingLeft(1).
			Foreground(lipgloss.Color("#C9D1D9"))

	tableLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C9D1D9")).
			Background(lipgloss.Color("#0D1117"))

	chatAssistantStyle = lipgloss.NewStyle().
				Padding(1, 1)

	chatThinkingStyle = lipgloss.NewStyle().
				Padding(1, 1)

	chatFinalStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorFinalBorder).
			Padding(0, 1)

	thinkingBodyStyle = lipgloss.NewStyle().
				Foreground(colorThinking)

	thinkingDoneBodyStyle = lipgloss.NewStyle().
				Foreground(colorThinkingDone)

	chatUserStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#2A2A2A")).
			Padding(1, 1)

	chatToolStyle = lipgloss.NewStyle().
			Padding(1, 1)

	toolBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#8893A1")).
			Faint(true)

	chatSystemStyle = lipgloss.NewStyle().
			Padding(1, 1)

	approvalBannerStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(colorTool).
				Background(lipgloss.Color("#17140D")).
				Padding(0, 1)

	activeSkillBannerStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(colorAccent).
				Background(lipgloss.Color("#0F1A28")).
				Padding(0, 1)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Faint(true)

	commandPaletteStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderTop(true).
				BorderLeft(true).
				BorderRight(true).
				BorderBottom(false).
				BorderForeground(colorBorder).
				Padding(0, 1)

	commandPaletteRowStyle = lipgloss.NewStyle().
				Background(lipgloss.Color("#0B1118")).
				Padding(0, 1)

	commandPaletteSelectedRowStyle = lipgloss.NewStyle().
					Background(lipgloss.Color("#231421")).
					Padding(0, 1)

	commandPaletteNameStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#B7C4FF"))

	commandPaletteSelectedNameStyle = lipgloss.NewStyle().
					Foreground(colorHotPink).
					Bold(true)

	commandPaletteDescStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A9B7C6"))

	commandPaletteSelectedDescStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#F7D9EA"))

	commandPaletteMetaStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	scrollbarTrackStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#1F1F1F"))

	scrollbarThumbIdleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#262626"))

	scrollbarThumbActiveStyle = lipgloss.NewStyle().
					Foreground(lipgloss.Color("#4D4D4D"))

	modalBoxStyle = lipgloss.NewStyle().
			Background(colorPanel).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)

	modalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	codeStyle = lipgloss.NewStyle().
			Foreground(colorCodeFg).
			Background(colorCodeBg).
			Padding(0, 1)

	mutedStyle  = lipgloss.NewStyle().Foreground(colorMuted)
	accentStyle = lipgloss.NewStyle().Foreground(colorAccent)
	doneStyle   = lipgloss.NewStyle().Foreground(colorSuccess)
	warnStyle   = lipgloss.NewStyle().Foreground(colorTool)
	errorStyle  = lipgloss.NewStyle().Foreground(colorErrorFg).Background(colorErrorBg).Padding(0, 1)
	strongStyle = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	emStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#79C0FF")).Italic(true)
)

func spacer(width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Width(width).Render("")
}
