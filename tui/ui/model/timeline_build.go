package tui

import (
	"fmt"
	"strings"
	"time"

	"bytemind/internal/session"

	tea "github.com/charmbracelet/bubbletea"
)

func waitForAsync(ch <-chan tea.Msg) tea.Cmd {
	return func() tea.Msg {
		return <-ch
	}
}

func rebuildSessionTimeline(sess *session.Session) ([]chatEntry, []toolRun) {
	items := make([]chatEntry, 0, len(sess.Messages))
	runs := make([]toolRun, 0, 8)
	callNames := map[string]string{}

	for _, message := range sess.Messages {
		message.Normalize()
		switch message.Role {
		case "user":
			userTextParts := make([]string, 0, len(message.Parts))
			for _, part := range message.Parts {
				if part.Text != nil {
					userTextParts = append(userTextParts, part.Text.Value)
				}
				if part.ToolResult == nil {
					continue
				}
				name := callNames[part.ToolResult.ToolUseID]
				if name == "" {
					name = "tool"
				}
				summary, lines, status := summarizeTool(name, part.ToolResult.Content)
				items = append(items, chatEntry{
					Kind:   "tool",
					Title:  "Tool Call | " + name,
					Body:   joinSummary(summary, lines),
					Status: status,
				})
				runs = append(runs, toolRun{Name: name, Summary: summary, Lines: lines, Status: status})
			}
			userText := strings.Join(userTextParts, "")
			if strings.TrimSpace(userText) != "" {
				items = append(items, chatEntry{Kind: "user", Title: "You", Body: userText, Status: "final"})
			}
		case "assistant":
			for _, call := range message.ToolCalls {
				callNames[call.ID] = call.Function.Name
			}
			if strings.TrimSpace(message.Text()) != "" {
				items = append(items, chatEntry{Kind: "assistant", Title: assistantLabel, Body: message.Text(), Status: "final"})
			}
		case "tool":
			name := callNames[message.ToolCallID]
			if name == "" {
				name = "tool"
			}
			summary, lines, status := summarizeTool(name, message.Content)
			items = append(items, chatEntry{
				Kind:   "tool",
				Title:  "Tool Call | " + name,
				Body:   joinSummary(summary, lines),
				Status: status,
			})
			runs = append(runs, toolRun{Name: name, Summary: summary, Lines: lines, Status: status})
		}
	}
	return items, runs
}

func chatBubbleWidth(item chatEntry, width int) int {
	if width <= 28 {
		return width
	}
	return width
}

func (m model) thinkingText() string {
	elapsed := ""
	if !m.thinkingStartedAt.IsZero() {
		seconds := int(time.Since(m.thinkingStartedAt).Round(time.Second).Seconds())
		if seconds < 0 {
			seconds = 0
		}
		elapsed = fmt.Sprintf(" (%ds)", seconds)
	}
	return fmt.Sprintf("%s Thinking%s\nTip: Use /btw to ask a quick side question without interrupting the current work.", m.spinner.View(), elapsed)
}

func (m model) thinkingDoneText() string {
	elapsed := ""
	if !m.thinkingStartedAt.IsZero() {
		seconds := int(time.Since(m.thinkingStartedAt).Round(time.Second).Seconds())
		if seconds < 0 {
			seconds = 0
		}
		elapsed = fmt.Sprintf(" (%ds)", seconds)
	}
	return fmt.Sprintf("Thinking%s\nTip: Use /btw to ask a quick side question without interrupting the current work.", elapsed)
}

func shouldExecuteFromPalette(item commandItem) bool {
	if item.Kind == "skill" {
		return true
	}
	switch item.Name {
	case "/help", "/session", "/skills", "/skill clear", "/new", "/compact", "/quit":
		return true
	default:
		return false
	}
}
