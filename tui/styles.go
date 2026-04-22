package tui

import "github.com/charmbracelet/lipgloss"

type semanticColorTokens struct {
	Panel         lipgloss.Color
	PanelMuted    lipgloss.Color
	Surface       lipgloss.Color
	SurfaceAlt    lipgloss.Color
	Border        lipgloss.Color
	BorderStrong  lipgloss.Color
	TextBase      lipgloss.Color
	TextMuted     lipgloss.Color
	TextStrong    lipgloss.Color
	Accent        lipgloss.Color
	AccentSoft    lipgloss.Color
	User          lipgloss.Color
	UserSoft      lipgloss.Color
	Tool          lipgloss.Color
	ToolSoft      lipgloss.Color
	Success       lipgloss.Color
	SuccessSoft   lipgloss.Color
	Warning       lipgloss.Color
	WarningSoft   lipgloss.Color
	Danger        lipgloss.Color
	DangerSoft    lipgloss.Color
	CodeBg        lipgloss.Color
	CodeBorder    lipgloss.Color
	CodeInlineBg  lipgloss.Color
	Quote         lipgloss.Color
	TableBorder   lipgloss.Color
	Highlight     lipgloss.Color
	Thinking      lipgloss.Color
	ThinkingDone  lipgloss.Color
	StreamBorder  lipgloss.Color
	GradientStart lipgloss.Color
	GradientEnd   lipgloss.Color
	// Extended colors for enhanced markdown rendering
	HighlightYellow lipgloss.Color
	HighlightBlue   lipgloss.Color
	HighlightGreen  lipgloss.Color
	HighlightRed    lipgloss.Color
	HighlightPurple lipgloss.Color
	HighlightOrange lipgloss.Color
	CodeThemeLight  lipgloss.Color
	CodeThemeDark   lipgloss.Color
	CodeThemeOcean  lipgloss.Color
	CodeThemeForest lipgloss.Color
}

var semanticColors = semanticColorTokens{
	Panel:         lipgloss.Color("#0B0D12"),
	PanelMuted:    lipgloss.Color("#0D1117"),
	Surface:       lipgloss.Color("#11141B"),
	SurfaceAlt:    lipgloss.Color("#171B23"),
	Border:        lipgloss.Color("#1B2332"),
	BorderStrong:  lipgloss.Color("#2B3848"),
	TextBase:      lipgloss.Color("#EAEAEA"),
	TextMuted:     lipgloss.Color("#A1ADBF"),
	TextStrong:    lipgloss.Color("#FFFFFF"),
	Accent:        lipgloss.Color("#6CB6FF"),
	AccentSoft:    lipgloss.Color("#8FD8FF"),
	User:          lipgloss.Color("#F59E0B"),
	UserSoft:      lipgloss.Color("#FFD39B"),
	Tool:          lipgloss.Color("#E7C27D"),
	ToolSoft:      lipgloss.Color("#7FE6FF"),
	Success:       lipgloss.Color("#7EE0B5"),
	SuccessSoft:   lipgloss.Color("#163423"),
	Warning:       lipgloss.Color("#E7C27D"),
	WarningSoft:   lipgloss.Color("#17140D"),
	Danger:        lipgloss.Color("#FF8F8F"),
	DangerSoft:    lipgloss.Color("#2B1417"),
	CodeBg:        lipgloss.Color("#000000"),
	CodeBorder:    lipgloss.Color("#2C3644"),
	CodeInlineBg:  lipgloss.Color("#1D2430"),
	Quote:         lipgloss.Color("#E7C27D"),
	TableBorder:   lipgloss.Color("#78B7FF"),
	Highlight:     lipgloss.Color("#2E4C7D"),
	Thinking:      lipgloss.Color("#6CB6FF"),
	ThinkingDone:  lipgloss.Color("#93A4B8"),
	StreamBorder:  lipgloss.Color("#3A5F86"),
	GradientStart: lipgloss.Color("#1E344A"),
	GradientEnd:   lipgloss.Color("#294B68"),
	// Extended colors for enhanced markdown rendering
	HighlightYellow: lipgloss.Color("#FFF3CD"),
	HighlightBlue:   lipgloss.Color("#CCE5FF"),
	HighlightGreen:  lipgloss.Color("#D4EDDA"),
	HighlightRed:    lipgloss.Color("#F8D7DA"),
	HighlightPurple: lipgloss.Color("#E2D9F3"),
	HighlightOrange: lipgloss.Color("#FFE5CC"),
	CodeThemeLight:  lipgloss.Color("#F8F9FA"),
	CodeThemeDark:   lipgloss.Color("#282C34"),
	CodeThemeOcean:  lipgloss.Color("#2B303B"),
	CodeThemeForest: lipgloss.Color("#2E3440"),
}

var (
	colorPanel        = semanticColors.Panel
	colorBorder       = semanticColors.Border
	colorAccent       = semanticColors.Accent
	colorThinkingBlue = semanticColors.Thinking
	colorThinkingDone = semanticColors.ThinkingDone
	colorCard         = semanticColors.SurfaceAlt
	colorHotPink      = lipgloss.Color("#F05AA6")
	colorThinking     = lipgloss.Color("#9D8AC8")
	colorUser         = semanticColors.User
	colorTool         = semanticColors.Tool
	colorMuted        = semanticColors.TextMuted
	colorDanger       = semanticColors.Danger
	colorSuccess      = semanticColors.Success
	colorStreamBorder = semanticColors.StreamBorder

	colorUserWarm      = semanticColors.UserSoft
	colorAICool        = semanticColors.TextBase
	colorToolNeutral   = semanticColors.ToolSoft
	colorCodeBg        = semanticColors.CodeBg
	colorCodeBorder    = semanticColors.CodeBorder
	colorGradientStart = semanticColors.GradientStart
	colorGradientEnd   = semanticColors.GradientEnd
	colorSubtleBg      = semanticColors.PanelMuted
	colorHighlight     = semanticColors.Highlight
	colorSuccessBright = semanticColors.Success
	colorWarningBright = semanticColors.Warning
)

var (
	panelStyle = lipgloss.NewStyle().
			Background(semanticColors.Panel).
			Padding(0, 0)

	landingCanvasStyle = lipgloss.NewStyle().
				Background(semanticColors.Panel)

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
				BorderForeground(semanticColors.Border).
				Background(semanticColors.Surface).
				Padding(0, 2)

	landingPlaceholderStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7C7C7C"))

	landingInputValueStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#EAEAEA"))

	landingModeStyle = lipgloss.NewStyle().
				Foreground(semanticColors.Accent).
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
				Foreground(semanticColors.TextStrong).
				Bold(true)

	helpCodeStyle = lipgloss.NewStyle().
			Foreground(semanticColors.TextStrong).
			Background(semanticColors.CodeBg).
			Padding(0, 1)

	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderTop(true).
			BorderBottom(true).
			BorderLeft(false).
			BorderRight(false).
			BorderForeground(semanticColors.Border).
			Background(semanticColors.Surface).
			Padding(0, 1)

	modeTabStyle = lipgloss.NewStyle().
			Foreground(semanticColors.TextMuted).
			Padding(0, 1).
			MarginRight(1)

	cardTitleStyle = lipgloss.NewStyle().Bold(true)

	chatHeaderStyle = lipgloss.NewStyle().
			Foreground(semanticColors.TextMuted)

	chatHeaderMetaStyle = lipgloss.NewStyle().
				Foreground(semanticColors.TextMuted).
				Faint(true)

	chatBodyStyle = lipgloss.NewStyle().
			Foreground(semanticColors.TextBase)

	chatBodyBlockStyle = lipgloss.NewStyle().
				Foreground(semanticColors.TextBase)

	chatMutedBodyBlockStyle = lipgloss.NewStyle().
				Foreground(semanticColors.TextMuted)

	selectionToastStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#DFF7E8")).
				Background(semanticColors.SuccessSoft).
				Padding(0, 1).
				Bold(true)

	selectionHighlightStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#0B1118")).
				Background(lipgloss.Color("#9CCBFF"))

	assistantHeading1Style = lipgloss.NewStyle().
				Bold(true).
				Foreground(semanticColors.TextStrong)

	assistantHeading2Style = lipgloss.NewStyle().
				Bold(true).
				Foreground(semanticColors.Accent)

	assistantHeading3Style = lipgloss.NewStyle().
				Bold(true).
				Foreground(semanticColors.AccentSoft)

	listMarkerStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(semanticColors.Accent)

	quoteLineStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(semanticColors.Quote).
			PaddingLeft(1).
			Foreground(semanticColors.TextBase)

	tableLineStyle = lipgloss.NewStyle().
			Foreground(semanticColors.TextBase).
			Background(semanticColors.Panel)

	chatAssistantStyle = lipgloss.NewStyle().
				Padding(0, 0)

	chatStreamingStyle = lipgloss.NewStyle().
				BorderLeft(true).
				BorderForeground(semanticColors.StreamBorder).
				Background(semanticColors.PanelMuted).
				Padding(0, 1)

	chatSettlingStyle = lipgloss.NewStyle().
				BorderLeft(true).
				BorderForeground(semanticColors.StreamBorder).
				Background(semanticColors.PanelMuted).
				Padding(0, 1)

	chatThinkingStyle = lipgloss.NewStyle().
				BorderLeft(true).
				BorderForeground(semanticColors.Border).
				Background(semanticColors.PanelMuted).
				Padding(0, 1)

	chatUserStyle = lipgloss.NewStyle().
			Background(semanticColors.SurfaceAlt).
			Padding(0, 1)

	chatToolStyle = lipgloss.NewStyle().
			Background(semanticColors.PanelMuted).
			Padding(0, 1)

	runCardStyle = lipgloss.NewStyle().
			Background(semanticColors.PanelMuted).
			Padding(0, 1)

	runCardStreamingStyle = runCardStyle.Copy().
				BorderLeft(true).
				BorderForeground(semanticColors.StreamBorder)

	runCardSettlingStyle = runCardStyle.Copy().
				BorderLeft(true).
				BorderForeground(semanticColors.StreamBorder)

	toolBodyStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A1ADBF"))

	toolErrorBodyStyle = lipgloss.NewStyle().
				Foreground(semanticColors.Danger)

	toolCallTitleStyle = cardTitleStyle.Copy().
				Foreground(semanticColors.TextMuted)

	toolResultTitleStyle = cardTitleStyle.Copy().
				Foreground(semanticColors.TextBase)

	toolNameStyle = lipgloss.NewStyle().
			Foreground(semanticColors.TextBase).
			Bold(true)

	toolMetaStyle = lipgloss.NewStyle().
			Foreground(semanticColors.TextMuted)

	toolSummaryStyle = lipgloss.NewStyle().
				Foreground(semanticColors.TextStrong).
				Bold(true)

	toolSearchSummaryStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#DDF3FF")).
				Bold(true)

	toolSearchMatchStyle = lipgloss.NewStyle().
				Foreground(semanticColors.AccentSoft)

	toolDetailStyle = lipgloss.NewStyle().
			Foreground(semanticColors.TextMuted)

	toolErrorSummaryStyle = lipgloss.NewStyle().
				Foreground(semanticColors.Danger).
				Bold(true)

	toolErrorDetailStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#FFC9C9"))

	runSectionDividerStyle = lipgloss.NewStyle().
				Foreground(semanticColors.Border).
				Faint(true)

	runToolSectionStyle = lipgloss.NewStyle().
				Background(semanticColors.Surface).
				BorderLeft(true).
				BorderForeground(semanticColors.Border).
				Padding(0, 1)

	runToolSuccessSectionStyle = runToolSectionStyle.Copy().
					BorderForeground(semanticColors.Success)

	runToolWarningSectionStyle = runToolSectionStyle.Copy().
					BorderForeground(semanticColors.Warning)

	runToolErrorSectionStyle = runToolSectionStyle.Copy().
					BorderForeground(semanticColors.Danger)

	runAnswerSectionStyle = lipgloss.NewStyle().
				Padding(0, 0)

	chatSystemStyle = lipgloss.NewStyle().
			Padding(0, 1)

	thinkingBodyStyle = lipgloss.NewStyle().
				Foreground(colorThinkingBlue)

	thinkingDoneBodyStyle = lipgloss.NewStyle().
				Foreground(colorThinkingDone)

	approvalBannerStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(semanticColors.Warning).
				Background(semanticColors.WarningSoft).
				Padding(0, 1)

	activeSkillBannerStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(semanticColors.Accent).
				Background(lipgloss.Color("#0F1A28")).
				Padding(0, 1)

	runIndicatorStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				Faint(true)

	thinkingIndicatorStyle = lipgloss.NewStyle().
				Foreground(semanticColors.Thinking).
				Bold(true)

	thinkingDetailStyle = lipgloss.NewStyle().
				Foreground(colorMuted)

	thinkingDetailPrefixStyle = lipgloss.NewStyle().
					Foreground(semanticColors.Border)

	thinkingPanelStyle = lipgloss.NewStyle().
				BorderLeft(true).
				BorderForeground(semanticColors.Border).
				Background(semanticColors.PanelMuted).
				Padding(0, 1)

	thinkingDividerStyle = lipgloss.NewStyle().
				Foreground(semanticColors.Border).
				SetString("----------------------------------------------------------------")

	thinkingPromptStyle = lipgloss.NewStyle().
				Foreground(colorThinkingBlue).
				Bold(true)

	thinkingCursorStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E8F1FF")).
				Background(lipgloss.Color("#E8F1FF"))

	statusBarStyle = lipgloss.NewStyle().
			Foreground(semanticColors.TextMuted).
			Faint(true)

	commandPaletteStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderTop(true).
				BorderLeft(true).
				BorderRight(true).
				BorderBottom(false).
				BorderForeground(semanticColors.Border).
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
			Background(semanticColors.Panel).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(semanticColors.Accent).
			Padding(0, 1)

	modalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(semanticColors.Accent)

	codeStyle = lipgloss.NewStyle().
			Foreground(semanticColors.TextBase).
			Background(semanticColors.CodeBg).
			Padding(0, 1).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(semanticColors.CodeBorder)

	enhancedCodeStyle = lipgloss.NewStyle().
				Foreground(semanticColors.TextBase).
				Background(semanticColors.CodeBg).
				Padding(1, 2).
				BorderStyle(lipgloss.RoundedBorder()).
				BorderForeground(semanticColors.CodeBorder).
				MarginTop(1).
				MarginBottom(1)

	badgeBaseStyle = lipgloss.NewStyle().
			Bold(true).
			Padding(0, 1)

	statusBadgeNeutralStyle = badgeBaseStyle.Copy().
				Foreground(semanticColors.TextMuted).
				Background(lipgloss.Color("#151D27"))

	statusBadgeAccentStyle = badgeBaseStyle.Copy().
				Foreground(lipgloss.Color("#DFF3FF")).
				Background(lipgloss.Color("#19344D"))

	statusBadgeSuccessStyle = badgeBaseStyle.Copy().
				Foreground(lipgloss.Color("#DFF7E8")).
				Background(lipgloss.Color("#163423"))

	statusBadgeWarningStyle = badgeBaseStyle.Copy().
				Foreground(lipgloss.Color("#FFF1D6")).
				Background(lipgloss.Color("#3A2611"))

	statusBadgeDangerStyle = badgeBaseStyle.Copy().
				Foreground(lipgloss.Color("#FFE5E5")).
				Background(lipgloss.Color("#3A1717"))

	mutedStyle  = lipgloss.NewStyle().Foreground(semanticColors.TextMuted)
	accentStyle = lipgloss.NewStyle().Foreground(semanticColors.Accent)
	infoStyle   = lipgloss.NewStyle().Foreground(semanticColors.ToolSoft)
	doneStyle   = lipgloss.NewStyle().Foreground(semanticColors.Success)
	warnStyle   = lipgloss.NewStyle().Foreground(semanticColors.Warning)
	errorStyle  = lipgloss.NewStyle().Foreground(semanticColors.Danger)
	strongStyle = lipgloss.NewStyle().Foreground(semanticColors.TextStrong).Bold(true)
	emStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#8FDFFF")).Italic(true)

	userMessageStyle = lipgloss.NewStyle().
				Foreground(semanticColors.UserSoft).
				Bold(true).
				Padding(0, 1)

	assistantMessageStyle = lipgloss.NewStyle().
				Foreground(semanticColors.TextBase).
				Padding(0, 0)

	assistantStreamingTitleStyle = lipgloss.NewStyle().
					Foreground(semanticColors.Thinking).
					Bold(true)

	assistantSettlingTitleStyle = lipgloss.NewStyle().
					Foreground(semanticColors.Thinking).
					Bold(true)

	assistantFinalTitleStyle = lipgloss.NewStyle().
					Foreground(semanticColors.TextBase).
					Bold(true)

	statusGeneratingStyle = statusBadgeAccentStyle
	statusSettlingStyle   = statusBadgeNeutralStyle.Copy().Foreground(lipgloss.Color("#CFE7FF"))
	statusFinalStyle      = statusBadgeNeutralStyle

	toolMessageStyle = lipgloss.NewStyle().
				Foreground(semanticColors.ToolSoft).
				Padding(0, 1)

	gradientTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(colorGradientStart).
				Background(colorGradientEnd)

	subtleSeparatorStyle = lipgloss.NewStyle().
				Foreground(semanticColors.TextMuted).
				Faint(true).
				SetString("-----")

	statusGoodStyle = lipgloss.NewStyle().
			Foreground(semanticColors.Success).
			Bold(true)

	statusWarningStyle = lipgloss.NewStyle().
				Foreground(semanticColors.Warning).
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

func statusBadgeStyle(badgeType string) lipgloss.Style {
	switch badgeType {
	case "active", "running", "accent", "info":
		return statusBadgeAccentStyle
	case "success", "done":
		return statusBadgeSuccessStyle
	case "warning", "pending":
		return statusBadgeWarningStyle
	case "error", "failed", "danger", "warn":
		return statusBadgeDangerStyle
	default:
		return statusBadgeNeutralStyle
	}
}

func renderPillBadge(text, badgeType string) string {
	return statusBadgeStyle(badgeType).Render(text)
}
