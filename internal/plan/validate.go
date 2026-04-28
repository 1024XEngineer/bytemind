package plan

import "strings"

type ValidationResult struct {
	OK             bool
	Warnings       []string
	RequiresReplan bool
}

func CanTransition(from, to Phase) bool {
	switch from {
	case PhaseNone:
		return to == PhaseExplore
	case PhaseExplore:
		return to == PhaseClarify || to == PhaseDraft || to == PhaseConvergeReady || to == PhaseBlocked
	case PhaseClarify:
		return to == PhaseClarify || to == PhaseDraft || to == PhaseConvergeReady || to == PhaseBlocked
	case PhaseDraft:
		return to == PhaseClarify || to == PhaseDraft || to == PhaseConvergeReady || to == PhaseBlocked
	case PhaseConvergeReady:
		return to == PhaseClarify || to == PhaseDraft || to == PhaseApprovedToBuild
	case PhaseApprovedToBuild:
		return to == PhaseExecuting
	case PhaseExecuting:
		return to == PhaseBlocked || to == PhaseCompleted
	case PhaseBlocked:
		return to == PhaseExplore || to == PhaseClarify || to == PhaseDraft || to == PhaseConvergeReady || to == PhaseExecuting
	default:
		return false
	}
}

func ValidateState(state State) ValidationResult {
	result := ValidationResult{OK: true}
	inProgressCount := 0
	blockedCount := 0
	for _, step := range state.Steps {
		switch NormalizeStepStatus(string(step.Status)) {
		case StepInProgress:
			inProgressCount++
		case StepBlocked:
			blockedCount++
		}
	}
	if inProgressCount > 1 {
		result.OK = false
		result.RequiresReplan = true
		result.Warnings = append(result.Warnings, "only one step can be in_progress")
	}
	if blockedCount > 0 && strings.TrimSpace(state.BlockReason) == "" {
		result.OK = false
		result.RequiresReplan = true
		result.Warnings = append(result.Warnings, "blocked plans must include a block reason")
	}

	phase := NormalizePhase(string(state.Phase))
	if phase == PhaseBlocked && strings.TrimSpace(state.BlockReason) == "" {
		result.OK = false
		result.RequiresReplan = true
		result.Warnings = append(result.Warnings, "blocked phase requires a block reason")
	}
	if phase == PhaseConvergeReady || phase == PhaseApprovedToBuild {
		if len(state.DecisionGaps) > 0 {
			result.OK = false
			result.RequiresReplan = true
			result.Warnings = append(result.Warnings, "converged plans cannot keep unresolved decision gaps")
		}
		if !HasExecutionReadiness(state) {
			result.OK = false
			result.RequiresReplan = true
			result.Warnings = append(result.Warnings, "converged plans must confirm scope, risks/rollback, and verification")
		}
	}
	if state.ActiveChoice != nil {
		if strings.TrimSpace(state.ActiveChoice.Question) == "" {
			result.OK = false
			result.RequiresReplan = true
			result.Warnings = append(result.Warnings, "active choice requires a question")
		}
		if len(state.ActiveChoice.Options) < 2 {
			result.OK = false
			result.RequiresReplan = true
			result.Warnings = append(result.Warnings, "active choice requires at least two options")
		}
	}
	return result
}
