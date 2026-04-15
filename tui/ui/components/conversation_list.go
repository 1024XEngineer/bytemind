package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderConversation() string {
	if len(m.chatItems) == 0 {
		return mutedStyle.Render("No messages yet. Start with an instruction like \"analyze this repo\" or \"implement a TUI shell\".")
	}
	width := m.viewport.Width
	if width <= 0 {
		width = m.conversationPanelWidth()
	}
	width = max(24, width)
	blocks := make([]string, 0, len(m.chatItems))
	for i := 0; i < len(m.chatItems); {
		item := m.chatItems[i]
		if item.Kind == "user" {
			blocks = append(blocks, renderChatRow(item, width))
			i++
			continue
		}

		j := i
		for j < len(m.chatItems) && m.chatItems[j].Kind != "user" {
			j++
		}
		blocks = append(blocks, renderBytemindRunRow(m.chatItems[i:j], width))
		i = j
	}
	return lipgloss.JoinVertical(lipgloss.Left, blocks...)
}

func (m model) renderConversationCopy() string {
	if len(m.chatItems) == 0 {
		return "No messages yet. Start with an instruction like \"analyze this repo\" or \"implement a TUI shell\"."
	}
	width := m.viewport.Width
	if width <= 0 {
		width = m.conversationPanelWidth()
	}
	width = max(24, width)
	blocks := make([]string, 0, len(m.chatItems))
	for i := 0; i < len(m.chatItems); {
		item := m.chatItems[i]
		if item.Kind == "user" {
			blocks = append(blocks, renderChatCopySection(item, width))
			i++
			continue
		}

		j := i
		for j < len(m.chatItems) && m.chatItems[j].Kind != "user" {
			j++
		}

		runParts := make([]string, 0, j-i)
		for _, runItem := range m.chatItems[i:j] {
			runParts = append(runParts, renderChatCopySection(runItem, width))
		}
		blocks = append(blocks, strings.Join(runParts, "\n\n"))
		i = j
	}
	return strings.Join(blocks, "\n\n")
}

func renderChatRow(item chatEntry, width int) string {
	bubbleWidth := chatBubbleWidth(item, width)
	card := renderChatCard(item, bubbleWidth)
	if item.Kind == "user" {
		return lipgloss.NewStyle().
			MarginBottom(1).
			Width(width).
			Background(lipgloss.Color("#2A2A2A")).
			Render(card)
	}
	return lipgloss.NewStyle().
		MarginBottom(1).
		Render(lipgloss.PlaceHorizontal(width, lipgloss.Left, card))
}

func renderBytemindRunRow(items []chatEntry, width int) string {
	if len(items) == 0 {
		return ""
	}
	card := renderBytemindRunCard(items, width)
	return lipgloss.NewStyle().
		MarginBottom(1).
		Render(lipgloss.PlaceHorizontal(width, lipgloss.Left, card))
}

func renderBytemindRunCard(items []chatEntry, width int) string {
	outer := chatAssistantStyle
	contentWidth := max(8, width-outer.GetHorizontalFrameSize())
	sections := make([]string, 0, len(items))
	for _, item := range items {
		sections = append(sections, renderChatSection(item, contentWidth))
	}
	return outer.Width(contentWidth).Render(strings.Join(sections, "\n"))
}
