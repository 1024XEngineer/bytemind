package plan

import "testing"

func TestSeedForRunNoopOutsidePlanMode(t *testing.T) {
	state := State{}
	SeedForRun(&state, ModeBuild, "build task", "fallback")
	if state.Goal != "" || state.Phase != "" {
		t.Fatalf("expected no change outside plan mode, got %#v", state)
	}
}

func TestSeedForRunSetsGoalAndDraftingPhase(t *testing.T) {
	state := State{Phase: PhaseNone}
	SeedForRun(&state, ModePlan, "plan task", "fallback")
	if state.Goal != "plan task" {
		t.Fatalf("expected goal from user input, got %q", state.Goal)
	}
	if state.Phase != PhaseDrafting {
		t.Fatalf("expected drafting phase, got %q", state.Phase)
	}
}

func TestSeedForRunUsesFallbackWhenUserInputEmpty(t *testing.T) {
	state := State{Phase: PhaseNone}
	SeedForRun(&state, ModePlan, "   ", "from structured message")
	if state.Goal != "from structured message" {
		t.Fatalf("expected fallback goal, got %q", state.Goal)
	}
}

func TestSeedForRunPreservesExistingGoal(t *testing.T) {
	state := State{Goal: "existing"}
	SeedForRun(&state, ModePlan, "new goal", "fallback")
	if state.Goal != "existing" {
		t.Fatalf("expected existing goal to be preserved, got %q", state.Goal)
	}
}

func TestSeedForRunPreservesNonNonePhase(t *testing.T) {
	state := State{Phase: PhaseReady}
	SeedForRun(&state, ModePlan, "goal", "fallback")
	if state.Phase != PhaseReady {
		t.Fatalf("expected existing phase to be preserved, got %q", state.Phase)
	}
}
