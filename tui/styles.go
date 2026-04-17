package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPanel        = lipgloss.Color("#0B0D12")
	colorBorder       = lipgloss.Color("#314156")
	colorAccent       = lipgloss.Color("#6CB6FF")
	colorThinkingBlue = lipgloss.Color("#6CB6FF")
	colorThinkingDone = lipgloss.Color("#93A4B8")
	colorCard         = lipgloss.Color("#171717")
	colorHotPink      = lipgloss.Color("#F05AA6")
	colorThinking     = lipgloss.Color("#9D8AC8")
	colorUser         = lipgloss.Color("#F59E0B")
	colorTool         = lipgloss.Color("#E7C27D")
	colorMuted        = lipgloss.Color("#93A4B8")
	colorDanger       = lipgloss.Color("#FF8F8F")
	colorSuccess      = lipgloss.Color("#7EE0B5")
	colorStreamBorder = lipgloss.Color("#3A5F86")

	colorUserWarm      = lipgloss.Color("#FFD39B")
	colorAICool        = lipgloss.Color("#E6EAF2")
	colorToolNeutral   = lipgloss.Color("#7FE6FF")
	colorCodeBg        = lipgloss.Color("#000000")
	colorCodeBorder    = lipgloss.Color("#2C3644")
	colorGradientStart = lipgloss.Color("#1E344A")
	colorGradientEnd   = lipgloss.Color("#294B68")
	colorSubtleBg      = lipgloss.Color("#0D1117")
	colorHighlight     = lipgloss.Color("#2E4C7D")
	colorSuccessBright = lipgloss.Color("#7EE0B5")
	colorWarningBright = lipgloss.Color("#E7C27D")
)

var (
	panelStyle = lipgloss.NewStyle().
			Background(colorPanel).
			Padding(0, 0)

	landingCanvasStyle = lipgloss.NewStyle().
				Background(colorPanel)

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
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(lipgloss.Color("#2B313C")).
				Background(lipgloss.Color("#11141B")).
				Padding(0, 2)

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
				Foreground(lipgloss.AdaptiveColor{Light: "246", Dark: "240"})

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
			Foreground(lipgloss.Color("#F1F5F9")).
			Background(lipgloss.Color("#101010")).
			Padding(0, 1)

	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderBottom(true).
			BorderLeft(false).
			BorderRight(false).
			BorderForeground(lipgloss.Color("#2E3440")).
			Background(lipgloss.Color("#11141B")).
			Padding(0, 1)

	modeTabStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1).
			MarginRight(1)

	cardTitleStyle = lipgloss.NewStyle().Bold(true)

	chatBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E6EAF2"))

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
				Foreground(lipgloss.Color("#6CB6FF"))

	assistantHeading3Style = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#B8DDFF"))

	listMarkerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#6CB6FF"))

	quoteLineStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(lipgloss.Color("#E7C27D")).
			PaddingLeft(1).
			Foreground(lipgloss.Color("#C6D2E3"))

	tableLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#C6D2E3")).
			Background(colorPanel)

	chatAssistantStyle = lipgloss.NewStyle().
				Padding(0, 0)

	chatStreamingStyle = lipgloss.NewStyle().
				BorderLeft(true).
				BorderForeground(colorStreamBorder).
				PaddingLeft(1)

	chatSettlingStyle = lipgloss.NewStyle().
				BorderLeft(true).
				BorderForeground(colorStreamBorder).
				PaddingLeft(1)

	chatThinkingStyle = lipgloss.NewStyle().
				BorderLeft(true).
				BorderForeground(lipgloss.Color("#314156")).
				PaddingLeft(1)

	chatUserStyle = lipgloss.NewStyle().
			Background(lipgloss.Color("#171B23")).
			Padding(0, 1)

	chatToolStyle = lipgloss.NewStyle().
			Padding(0, 0)

	toolBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A1ADBF"))

	toolCallTitleStyle = cardTitleStyle.Copy().
				Foreground(lipgloss.Color("#8FD8FF")).
				Faint(true)

	toolResultTitleStyle = cardTitleStyle.Copy().
				Foreground(lipgloss.Color("#8FD8FF"))

	toolSearchSummaryStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#DDF3FF")).
				Bold(true)

	toolSearchMatchStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#8FD8FF"))

	chatSystemStyle = lipgloss.NewStyle().
			Padding(0, 1)

	thinkingBodyStyle = lipgloss.NewStyle().
				Foreground(colorThinkingBlue)

	thinkingDoneBodyStyle = lipgloss.NewStyle().
				Foreground(colorThinkingDone)

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

	runIndicatorStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Faint(true)

	thinkingIndicatorStyle = lipgloss.NewStyle().
				Foreground(colorThinkingBlue).
				Bold(true)

	thinkingDetailStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	thinkingDetailPrefixStyle = lipgloss.NewStyle().
					Foreground(colorBorder)

	thinkingPanelStyle = lipgloss.NewStyle().
				BorderLeft(true).
				BorderForeground(lipgloss.Color("#314156")).
				PaddingLeft(1)

	thinkingDividerStyle = lipgloss.NewStyle().
				Foreground(colorBorder).
				SetString("----------------------------------------------------------------")

	thinkingPromptStyle = lipgloss.NewStyle().
				Foreground(colorThinkingBlue).
				Bold(true)

	thinkingCursorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E8F1FF")).
				Background(lipgloss.Color("#E8F1FF"))

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
			Foreground(lipgloss.Color("#C6D2E3")).
			Background(colorCodeBg).
			Padding(0, 1).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorCodeBorder)

	enhancedCodeStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#C6D2E3")).
				Background(colorCodeBg).
				Padding(1, 2).
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(colorCodeBorder).
				MarginTop(1).
				MarginBottom(1)

	mutedStyle  = lipgloss.NewStyle().Foreground(colorMuted)
	accentStyle = lipgloss.NewStyle().Foreground(colorAccent)
	infoStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#7FE6FF"))
	doneStyle   = lipgloss.NewStyle().Foreground(colorSuccess)
	warnStyle   = lipgloss.NewStyle().Foreground(colorTool)
	errorStyle  = lipgloss.NewStyle().Foreground(colorDanger)
	strongStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F6FBFF")).Bold(true)
	emStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#8FDFFF")).Italic(true)

	userMessageStyle = lipgloss.NewStyle().
				Foreground(colorUserWarm).
				Bold(true).
				Padding(0, 1)

	assistantMessageStyle = lipgloss.NewStyle().
				Foreground(colorAICool).
				Padding(0, 0)

	assistantStreamingTitleStyle = lipgloss.NewStyle().
					Foreground(colorThinkingBlue).
					Bold(true)

	assistantSettlingTitleStyle = lipgloss.NewStyle().
					Foreground(colorThinkingBlue).
					Bold(true)

	assistantFinalTitleStyle = lipgloss.NewStyle().
					Foreground(colorAICool).
					Bold(true)

	statusGeneratingStyle = lipgloss.NewStyle().
				Foreground(colorThinkingBlue).
				Bold(true)

	statusSettlingStyle = lipgloss.NewStyle().
				Foreground(colorThinkingBlue).
				Bold(true)

	statusFinalStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Bold(true)

	toolMessageStyle = lipgloss.NewStyle().
				Foreground(colorToolNeutral).
				Padding(0, 1)

	gradientTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorGradientStart).
				Background(colorGradientEnd)

	subtleSeparatorStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Faint(true).
				SetString("-----")

	statusGoodStyle = lipgloss.NewStyle().
			Foreground(colorSuccessBright).
			Bold(true)

	statusWarningStyle = lipgloss.NewStyle().
				Foreground(colorWarningBright).
				Bold(true)

	messageSeparatorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#4A5568")).
				Faint(true).
				SetString("")
)

func spacer(width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Width(width).Render("")
}
