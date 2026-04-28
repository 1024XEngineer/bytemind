package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func (m model) renderPromptSearchPalette() string {
	width := m.commandPaletteWidth()
	items := m.promptSearchMatches
	modeLabel := "search"
	if m.promptSearchMode == promptSearchModePanel {
		modeLabel = "panel"
	}
	if len(items) == 0 {
		query := strings.TrimSpace(m.promptSearchQuery)
		if query == "" {
			query = "(all)"
		}
		content := []string{
			commandPaletteMetaStyle.Render("Prompt history " + modeLabel),
			commandPaletteMetaStyle.Render("query: "+query+"  (filters: ") + renderInlineShortcutHints(promptSearchFilterHints) + commandPaletteMetaStyle.Render(")"),
			commandPaletteMetaStyle.Render("No matching prompts."),
			commandPaletteMetaStyle.Render("Type to filter  ") +
				renderInlineShortcutHints([]footerShortcutHint{
					{Key: "PgUp/PgDn", Label: "page"},
					{Key: "Enter", Label: "apply"},
					{Key: "Esc", Label: "close"},
				}),
		}
		return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, content...))
	}

	selected, _ := m.selectedPromptSearchEntry()
	rowWidth := max(1, width-commandPaletteStyle.GetHorizontalFrameSize())
	rows := make([]string, 0, promptSearchPageSize+3)
	for _, item := range m.visiblePromptSearchEntriesPage() {
		rowStyle := commandPaletteRowStyle
		textStyle := commandPaletteDescStyle
		if item.Timestamp.Equal(selected.Timestamp) && item.SessionID == selected.SessionID && item.Prompt == selected.Prompt {
			rowStyle = commandPaletteSelectedRowStyle
			textStyle = commandPaletteSelectedDescStyle
		}
		workspaceName := filepath.Base(strings.TrimSpace(item.Workspace))
		if workspaceName == "" || workspaceName == "." {
			workspaceName = strings.TrimSpace(item.Workspace)
		}
		if workspaceName == "" {
			workspaceName = "-"
		}
		meta := fmt.Sprintf("%s  ws:%s  sid:%s", item.Timestamp.Local().Format("01-02 15:04"), compact(workspaceName, 16), compact(item.SessionID, 12))
		rowText := compact(strings.TrimSpace(item.Prompt), max(12, rowWidth-2))
		rows = append(rows, rowStyle.Width(rowWidth).Render(textStyle.Render(rowText)))
		rows = append(rows, rowStyle.Width(rowWidth).Render(commandPaletteMetaStyle.Render(compact(meta, max(12, rowWidth-2)))))
	}
	for len(rows) < promptSearchPageSize*2 {
		rows = append(rows, commandPaletteRowStyle.Width(rowWidth).Render(""))
	}

	query := strings.TrimSpace(m.promptSearchQuery)
	if query == "" {
		query = "(all)"
	}
	meta := commandPaletteMetaStyle.Render(fmt.Sprintf("%s  query:%s", modeLabel, compact(query, 24))) +
		footerHintDividerStyle.Render("  |  ") +
		renderInlineShortcutHints(promptSearchFilterHints) +
		footerHintDividerStyle.Render("  |  ") +
		renderInlineShortcutHints(promptSearchActionHints)
	rows = append(rows, meta)
	return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m model) renderCommandPalette() string {
	width := m.commandPaletteWidth()
	items := m.filteredCommands()
	if len(items) == 0 {
		return commandPaletteStyle.Width(width).Render(
			commandPaletteMetaStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render("No matching commands."),
		)
	}

	selected, _ := m.selectedCommandItem()
	nameWidth := min(22, max(12, width/5))
	descWidth := max(12, width-commandPaletteStyle.GetHorizontalFrameSize()-nameWidth-3)
	rows := make([]string, 0, commandPageSize+1)
	for _, item := range m.visibleCommandItemsPage() {
		rowStyle := commandPaletteRowStyle
		nameStyle := commandPaletteNameStyle
		descStyle := commandPaletteDescStyle
		if item.Name == selected.Name {
			rowStyle = commandPaletteSelectedRowStyle
			nameStyle = commandPaletteSelectedNameStyle
			descStyle = commandPaletteSelectedDescStyle
		}

		name := nameStyle.Width(nameWidth).Render(item.Usage)
		desc := descStyle.Width(descWidth).Render(compact(item.Description, max(12, descWidth)))
		rows = append(rows, rowStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render(
			lipgloss.JoinHorizontal(lipgloss.Top, name, " ", desc),
		))
	}
	for len(rows) < commandPageSize {
		rows = append(rows, commandPaletteRowStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render(""))
	}
	rows = append(rows, commandPaletteMetaStyle.Render("Up/Down move  PgUp/PgDn page  Enter run  Esc close"))
	return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}

func (m model) renderMentionPalette() string {
	width := m.commandPaletteWidth()
	items := m.mentionResults
	if len(items) == 0 {
		return commandPaletteStyle.Width(width).Render(
			commandPaletteMetaStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render("No matching files in workspace."),
		)
	}

	selected, _ := m.selectedMentionCandidate()
	nameWidth := min(26, max(12, width/4))
	descWidth := max(12, width-commandPaletteStyle.GetHorizontalFrameSize()-nameWidth-4)
	rows := make([]string, 0, mentionPageSize+1)
	for _, item := range m.visibleMentionItemsPage() {
		rowStyle := commandPaletteRowStyle
		nameStyle := commandPaletteNameStyle
		descStyle := commandPaletteDescStyle
		if item.Path == selected.Path {
			rowStyle = commandPaletteSelectedRowStyle
			nameStyle = commandPaletteSelectedNameStyle
			descStyle = commandPaletteSelectedDescStyle
		}

		nameText := item.BaseName
		if tag := strings.TrimSpace(item.TypeTag); tag != "" {
			nameText = "[" + tag + "] " + nameText
		}
		if m.hasRecentMention(item.Path) {
			nameText = "* " + nameText
		} else {
			nameText = "  " + nameText
		}

		name := nameStyle.Width(nameWidth).Render(compact(nameText, max(12, nameWidth)))
		desc := descStyle.Width(descWidth).Render(compact(item.Path, max(12, descWidth)))
		rows = append(rows, rowStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render(
			lipgloss.JoinHorizontal(lipgloss.Top, name, "  ", desc),
		))
	}
	for len(rows) < mentionPageSize {
		rows = append(rows, commandPaletteRowStyle.Width(max(1, width-commandPaletteStyle.GetHorizontalFrameSize())).Render(""))
	}
	metaText := "* recent  Type @query to search  Up/Down move  Enter/Tab insert  Esc close"
	if m.mentionIndex != nil {
		stats := m.mentionIndex.Stats()
		if stats.Truncated && stats.MaxFiles > 0 {
			metaText = fmt.Sprintf("* recent  indexed first %d files  Enter/Tab insert  Esc close", stats.MaxFiles)
		}
	}
	rows = append(rows, commandPaletteMetaStyle.Render(metaText))
	return commandPaletteStyle.Width(width).Render(lipgloss.JoinVertical(lipgloss.Left, rows...))
}
