package agent

import (
	"fmt"
	"strings"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
)

const defaultRecentToolLimit = 4

type StopSummaryInput struct {
	SessionID         corepkg.SessionID
	Reason            string
	ExecutedTools     []string
	TaskReport        *TaskReport
	IncludeTaskReport bool
	RecentToolLimit   int
}

func BuildStopSummary(in StopSummaryInput) string {
	var builder strings.Builder
	builder.WriteString("Paused before a final answer.\n")
	builder.WriteString(in.Reason)

	recentLimit := in.RecentToolLimit
	if recentLimit <= 0 {
		recentLimit = defaultRecentToolLimit
	}
	recentTools := RecentUniqueToolNames(in.ExecutedTools, recentLimit)
	if len(recentTools) > 0 {
		builder.WriteString("\nRecent tool activity:\n")
		for _, toolName := range recentTools {
			fmt.Fprintf(&builder, "- %s\n", toolName)
		}
	}
	if in.IncludeTaskReport && in.TaskReport != nil && !in.TaskReport.IsEmpty() {
		builder.WriteString("\nTask report summary:\n")
		if human := strings.TrimSpace(in.TaskReport.HumanSummary()); human != "" {
			builder.WriteString(human)
			builder.WriteString("\n")
		}
		builder.WriteString("Task report (json):\n")
		builder.WriteString(in.TaskReport.JSON())
		builder.WriteString("\n")
	}

	fmt.Fprintf(&builder, "\nYou can continue by reusing session %s with -session %s, or raise the budget with -max-iterations <n>.", in.SessionID, in.SessionID)
	return builder.String()
}

func RecentUniqueToolNames(names []string, limit int) []string {
	if limit <= 0 || len(names) == 0 {
		return nil
	}
	result := make([]string, 0, limit)
	seen := map[string]struct{}{}
	for i := len(names) - 1; i >= 0 && len(result) < limit; i-- {
		name := names[i]
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}
	return result
}
