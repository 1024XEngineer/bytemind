package tui

import "github.com/charmbracelet/lipgloss"

var (
	colorPanel   = lipgloss.Color("#0F1720")
	colorBorder  = lipgloss.Color("#314156")
	colorAccent  = lipgloss.Color("#7DD3FC")
	colorUser    = lipgloss.Color("#F6AD7B")
	colorTool    = lipgloss.Color("#F9D67A")
	colorMuted   = lipgloss.Color("#93A4B8")
	colorDanger  = lipgloss.Color("#F7A8A8")
	colorSuccess = lipgloss.Color("#8EE6A0")
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#E5EEF8"))

	tagStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#D6E4F5")).
			Background(lipgloss.Color("#172434")).
			Padding(0, 1).
			MarginLeft(1)

	statusTagStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#E6F7FF")).
			Background(lipgloss.Color("#15303B")).
			Padding(0, 1).
			MarginLeft(1)

	subtleBorderStyle = lipgloss.NewStyle().
				Foreground(colorMuted).
				BorderStyle(lipgloss.NormalBorder()).
				BorderTop(false).
				BorderLeft(false).
				BorderRight(false).
				BorderBottom(true).
				BorderForeground(colorBorder).
				Padding(0, 1)

	panelStyle = lipgloss.NewStyle().
			Background(colorPanel).
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1)

	landingLogoStyle = lipgloss.NewStyle().
				Foreground(colorAccent).
				Bold(true)

	landingTitleStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#E2E8F0")).
				Bold(true)

	landingInputStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(colorAccent).
				Padding(0, 1).
				Background(colorPanel)

	sectionTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#E2E8F0"))

	inputStyle = lipgloss.NewStyle().
			BorderStyle(lipgloss.NormalBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1).
			Background(colorPanel)

	modeTabStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(lipgloss.Color("#132030")).
			BorderStyle(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1).
			MarginRight(1).
			MarginBottom(1)

	cardTitleStyle = lipgloss.NewStyle().Bold(true)

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
				BorderLeft(true).
				BorderForeground(colorAccent).
				Padding(0, 1).
				Background(lipgloss.Color("#111C27"))

	chatUserStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(colorUser).
			Padding(0, 1).
			Background(lipgloss.Color("#241B16"))

	chatToolStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(colorTool).
			Padding(0, 1).
			Background(lipgloss.Color("#272317"))

	chatSystemStyle = lipgloss.NewStyle().
			BorderLeft(true).
			BorderForeground(colorMuted).
			Padding(0, 1).
			Background(lipgloss.Color("#17202B"))

	approvalBannerStyle = lipgloss.NewStyle().
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(colorTool).
				Background(lipgloss.Color("#17140D")).
				Padding(0, 1)

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
