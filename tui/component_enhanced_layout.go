package tui

func RenderMessageSeparator() string {
	return messageSeparatorStyle.Render("")
}

func RenderSectionDivider() string {
	return subtleSeparatorStyle.Render("")
}

func RenderStatusIndicator(status string) string {
	return RenderStatus(status, status)
}

func RenderBadge(text, badgeType string) string {
	return renderStatusBadge(text, badgeType)
}

func RenderTimelineNode(completed bool) string {
	if completed {
		return doneStyle.Render("*")
	}
	return accentStyle.Render("*")
}

func RenderTimelineLine() string {
	return mutedStyle.Render("|")
}

func RenderShadowContainer(content string) string {
	return panelStyle.Render(content)
}

func RenderEnhancedCodeBlock(content string) string {
	return enhancedCodeStyle.Render(content)
}

func RenderEnhancedHeader(text string) string {
	return gradientTitleStyle.Render(text)
}

func renderStatusBadge(text, badgeType string) string {
	switch badgeType {
	case "success":
		return doneStyle.Render(text)
	case "warning":
		return warnStyle.Render(text)
	case "error":
		return errorStyle.Render(text)
	default:
		return accentStyle.Render(text)
	}
}
