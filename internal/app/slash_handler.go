package app

import (
	"fmt"
	"io"
	"strings"

	"bytemind/internal/session"
)

type SlashHandleResult struct {
	NextSession *session.Session
	ShouldExit  bool
	Handled     bool
}

func HandleSlashCommand(stdout io.Writer, store *session.Store, current *session.Session, input string, commands []SlashCommand) (SlashHandleResult, error) {
	result, err := ExecuteSlashCommand(store, current, input, commands)
	if err != nil {
		return SlashHandleResult{
			NextSession: current,
			Handled:     result.Handled,
		}, err
	}
	if !result.Handled {
		return SlashHandleResult{
			NextSession: current,
		}, nil
	}
	if result.ShouldExit {
		return SlashHandleResult{
			NextSession: result.NextSession,
			Handled:     true,
			ShouldExit:  true,
		}, nil
	}

	switch result.Command {
	case "help":
		RenderHelp(stdout)
	case "session":
		RenderCurrentSession(stdout, result.SessionToDisplay)
	case "sessions":
		RenderSessionsView(stdout, current.ID, result.Summaries, result.Warnings)
	case "resume":
		if strings.TrimSpace(result.UsageHint) != "" {
			fmt.Fprintln(stdout, result.UsageHint)
			return SlashHandleResult{
				NextSession: current,
				Handled:     true,
			}, nil
		}
		next := result.NextSession
		fmt.Fprintf(stdout, "%sresumed%s %s\n", ansiDim, ansiReset, next.ID)
		RenderCurrentSession(stdout, result.SessionToDisplay)
	case "new":
		next := result.NextSession
		fmt.Fprintf(stdout, "%snew session%s %s\n", ansiDim, ansiReset, next.ID)
		RenderCurrentSession(stdout, result.SessionToDisplay)
	default:
		fmt.Fprintf(stdout, "unknown command: %s\n", result.UnknownInput)
		RenderCommandSuggestions(stdout, result.UnknownInput, result.Suggestions)
	}

	return SlashHandleResult{
		NextSession: result.NextSession,
		Handled:     true,
	}, nil
}
