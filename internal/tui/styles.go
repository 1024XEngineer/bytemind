package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPanel    = lipgloss.Color("#000000")
	colorBorder   = lipgloss.Color("#314156")
	colorAccent   = lipgloss.Color("#6CB6FF")
	colorCard     = lipgloss.Color("#171717")
	colorHotPink  = lipgloss.Color("#F05AA6")
	colorThinking = lipgloss.Color("#9D8AC8")
	colorUser     = lipgloss.Color("#F59E0B")
	colorTool     = lipgloss.Color("#BEA15A")
	colorMuted    = lipgloss.Color("#93A4B8")
	colorDanger   = lipgloss.Color("#F7A8A8")
	colorSuccess  = lipgloss.Color("#8EE6A0")
)

var (
	panelStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	landingLogoStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true).
				Align(lipgloss.Center)

	landingTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E2E8F0")).
				Bold(true)

	landingInputStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(colorAccent).
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
			Foreground(lipgloss.Color("#D7E3F1"))

	tableLineStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D7E3F1")).
			Background(lipgloss.Color("#101923"))

	chatAssistantStyle = lipgloss.NewStyle().
				Padding(1, 1)

	chatThinkingStyle = lipgloss.NewStyle().
				Padding(1, 1)

	thinkingBodyStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#7F758F")).
				Faint(true)

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

	modalBoxStyle = lipgloss.NewStyle().
			Background(colorPanel).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorAccent).
			Padding(0, 1)

	modalTitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(colorAccent)

	codeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#F8FAFC")).
			Background(lipgloss.Color("#0B1220")).
			Padding(0, 1)

	mutedStyle  = lipgloss.NewStyle().Foreground(colorMuted)
	accentStyle = lipgloss.NewStyle().Foreground(colorAccent)
	doneStyle   = lipgloss.NewStyle().Foreground(colorSuccess)
	warnStyle   = lipgloss.NewStyle().Foreground(colorTool)
	errorStyle  = lipgloss.NewStyle().Foreground(colorDanger)
)

func spacer(width int) string {
	if width <= 0 {
		return ""
	}
	return lipgloss.NewStyle().Width(width).Render("")
}
