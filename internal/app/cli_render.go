package app

import (
	"fmt"
	"io"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/session"
	subagentspkg "github.com/1024XEngineer/bytemind/internal/subagents"
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
	ansiGray  = "\x1b[90m"
)

func RenderHelp(w io.Writer) {
	for _, line := range DefaultHelpLines() {
		fmt.Fprintf(w, "%-42s %s\n", line.Usage, line.Description)
	}
}

func RenderCurrentSession(w io.Writer, sess *session.Session) {
	view := BuildSessionSnapshotView(sess, nil)
	fmt.Fprintf(w, "%ssession%s %s\n", ansiDim, ansiReset, view.ID)
	fmt.Fprintf(w, "%sworkspace%s %s\n", ansiDim, ansiReset, view.Workspace)
	fmt.Fprintf(w, "%supdated%s %s\n", ansiDim, ansiReset, view.Updated)
}

func RenderSessionsView(w io.Writer, currentID string, summaries []session.Summary, warnings []string) {
	if len(summaries) == 0 {
		fmt.Fprintln(w, "No saved sessions.")
	} else {
		views := BuildSessionSummaryViews(summaries, currentID, nil)
		fmt.Fprintf(w, "%srecent sessions%s\n", ansiBold, ansiReset)
		for _, item := range views {
			fmt.Fprintf(w, "%s %s  %s  %2d msgs  %s\n", item.Marker, item.ID, item.Updated, item.MessageCount, item.Preview)
			fmt.Fprintf(w, "%s    %s%s\n", ansiGray, item.Workspace, ansiReset)
		}
	}

	if len(warnings) > 0 {
		if len(summaries) > 0 {
			fmt.Fprintln(w)
		}
		for _, warning := range warnings {
			fmt.Fprintf(w, "%swarning%s %s\n", ansiDim, ansiReset, warning)
		}
	}
}

func RenderUsage(w io.Writer) {
	for _, line := range DefaultUsageLines() {
		fmt.Fprintln(w, line)
	}
}

func RenderCommandSuggestions(w io.Writer, input string, suggestions []string) {
	fmt.Fprintf(w, "%smatches%s for %s:\n", ansiDim, ansiReset, input)
	for _, suggestion := range suggestions {
		fmt.Fprintf(w, "  %s\n", suggestion)
	}
}

func RenderSubAgentsView(w io.Writer, agents []subagentspkg.Agent) {
	if len(agents) == 0 {
		fmt.Fprintln(w, "No subagents available.")
		return
	}
	fmt.Fprintf(w, "%savailable subagents%s\n", ansiBold, ansiReset)
	for _, agent := range agents {
		description := strings.TrimSpace(agent.Description)
		if description == "" {
			description = "No description provided."
		}
		fmt.Fprintf(w, "- %s [%s]: %s\n", agent.Name, agent.Scope, description)
	}
}

func RenderSubAgentDetail(w io.Writer, agent subagentspkg.Agent) {
	fmt.Fprintf(w, "%ssubagent%s %s\n", ansiDim, ansiReset, agent.Name)
	fmt.Fprintf(w, "%sscope%s %s\n", ansiDim, ansiReset, agent.Scope)
	fmt.Fprintf(w, "%sentry%s %s\n", ansiDim, ansiReset, agent.Entry)
	if strings.TrimSpace(agent.Mode) != "" {
		fmt.Fprintf(w, "%smode%s %s\n", ansiDim, ansiReset, agent.Mode)
	}
	if strings.TrimSpace(agent.Output) != "" {
		fmt.Fprintf(w, "%soutput%s %s\n", ansiDim, ansiReset, agent.Output)
	}
	if strings.TrimSpace(agent.Isolation) != "" {
		fmt.Fprintf(w, "%sisolation%s %s\n", ansiDim, ansiReset, agent.Isolation)
	}
	if len(agent.Tools) > 0 {
		fmt.Fprintf(w, "%stools%s %s\n", ansiDim, ansiReset, strings.Join(agent.Tools, ", "))
	}
	if len(agent.DisallowedTools) > 0 {
		fmt.Fprintf(w, "%sdisallowed%s %s\n", ansiDim, ansiReset, strings.Join(agent.DisallowedTools, ", "))
	}
	if source := strings.TrimSpace(agent.SourcePath); source != "" {
		fmt.Fprintf(w, "%ssource%s %s\n", ansiDim, ansiReset, source)
	}
	if description := strings.TrimSpace(agent.Description); description != "" {
		fmt.Fprintf(w, "%sdescription%s %s\n", ansiDim, ansiReset, description)
	}
}
