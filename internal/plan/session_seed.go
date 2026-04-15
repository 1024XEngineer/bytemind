package plan

import "strings"

// SeedForRun initializes plan state when entering plan mode.
func SeedForRun(state *State, runMode AgentMode, userInput, fallbackUserText string) {
	if state == nil || runMode != ModePlan {
		return
	}

	goalText := strings.TrimSpace(userInput)
	if goalText == "" {
		goalText = strings.TrimSpace(fallbackUserText)
	}
	if strings.TrimSpace(state.Goal) == "" {
		state.Goal = goalText
	}
	if state.Phase == PhaseNone {
		state.Phase = PhaseDrafting
	}
}
