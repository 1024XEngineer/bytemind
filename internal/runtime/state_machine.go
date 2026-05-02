package runtime

import corepkg "github.com/1024XEngineer/bytemind/internal/core"

var baseAllowedTransitions = map[corepkg.TaskStatus]map[corepkg.TaskStatus]struct{}{
	corepkg.TaskPending: {
		corepkg.TaskRunning: {},
		corepkg.TaskFailed:  {},
		corepkg.TaskKilled:  {},
	},
	corepkg.TaskRunning: {
		corepkg.TaskCompleted: {},
		corepkg.TaskFailed:    {},
		corepkg.TaskKilled:    {},
	},
}

type TransitionOptions struct {
	AllowRetryTransition bool
}

func IsTerminalTaskStatus(status corepkg.TaskStatus) bool {
	switch status {
	case corepkg.TaskCompleted, corepkg.TaskFailed, corepkg.TaskKilled:
		return true
	default:
		return false
	}
}

func ValidateTaskTransition(from, to corepkg.TaskStatus, opts TransitionOptions) error {
	if from == to {
		return nil
	}

	if from == corepkg.TaskFailed && to == corepkg.TaskPending {
		if opts.AllowRetryTransition {
			return nil
		}
		return invalidTransitionError(from, to)
	}

	targets, ok := baseAllowedTransitions[from]
	if !ok {
		return invalidTransitionError(from, to)
	}
	if _, ok := targets[to]; !ok {
		return invalidTransitionError(from, to)
	}
	return nil
}
