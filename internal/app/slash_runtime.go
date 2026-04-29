package app

import (
	"strings"

	"github.com/1024XEngineer/bytemind/internal/session"
)

type SlashExecution struct {
	NextSession      *session.Session
	SessionToDisplay *session.Session
	Handled          bool
	ShouldExit       bool
	Command          string
	UsageHint        string
	UnknownInput     string
	Suggestions      []string
	Summaries        []session.Summary
	Warnings         []string
}

func ExecuteSlashCommand(store *session.Store, current *session.Session, input string, commands []SlashCommand) (SlashExecution, error) {
	out := SlashExecution{
		NextSession: current,
	}
	fields := strings.Fields(input)
	if len(fields) == 0 {
		return out, nil
	}
	out.Handled = true

	switch fields[0] {
	case "/quit":
		out.Command = "quit"
		out.ShouldExit = true
		return out, nil
	case "/help":
		out.Command = "help"
		return out, nil
	case "/session":
		out.Command = "session"
		out.SessionToDisplay = current
		return out, nil
	case "/sessions":
		out.Command = "sessions"
		limit := DefaultSessionListLimit
		if len(fields) > 1 {
			parsed, err := ParseSessionListLimit(fields[1])
			if err != nil {
				return out, err
			}
			limit = parsed
		}
		summaries, warnings, err := store.List(limit)
		if err != nil {
			return out, err
		}
		out.Summaries = summaries
		out.Warnings = warnings
		return out, nil
	case "/resume":
		out.Command = "resume"
		if len(fields) < 2 {
			out.UsageHint = "usage: /resume <id>"
			return out, nil
		}
		if _, err := store.CleanupZeroMessageSessions(current.Workspace, current.ID); err != nil {
			return out, err
		}
		next, err := ResumeSessionInWorkspace(store, current.Workspace, fields[1])
		if err != nil {
			return out, err
		}
		out.NextSession = next
		out.SessionToDisplay = next
		return out, nil
	case "/new":
		out.Command = "new"
		if _, err := store.CleanupZeroMessageSessions(current.Workspace, current.ID); err != nil {
			return out, err
		}
		next, err := CreateSession(store, current.Workspace)
		if err != nil {
			return out, err
		}
		out.NextSession = next
		out.SessionToDisplay = next
		return out, nil
	default:
		out.Command = "unknown"
		out.UnknownInput = fields[0]
		out.Suggestions = CommandNames(commands)
		return out, nil
	}
}
