package app

import (
	"fmt"
	"io"

	"bytemind/internal/session"
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
