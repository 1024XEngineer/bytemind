package plan

import "testing"

func TestDerivePhaseInPlanModeUsesClarifyWhenDecisionGapsRemain(t *testing.T) {
	state := State{
		Steps:        []Step{{Title: "Inspect prompt flow", Status: StepPending}},
		DecisionGaps: []string{"Choose the execution trigger copy"},
	}
	if got := DerivePhase(ModePlan, state); got != PhaseClarify {
		t.Fatalf("expected clarify phase, got %q", got)
	}
}

func TestDerivePhaseInPlanModeUsesConvergeReadyWhenChecklistIsComplete(t *testing.T) {
	state := State{
		Steps:               []Step{{Title: "Inspect prompt flow", Status: StepPending}},
		Risks:               []string{"Mode-switch regression"},
		Verification:        []string{"go test ./internal/plan -v"},
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
	}
	if got := DerivePhase(ModePlan, state); got != PhaseConvergeReady {
		t.Fatalf("expected converge_ready phase, got %q", got)
	}
}

func TestValidateStateRejectsConvergedPlanWithoutReadiness(t *testing.T) {
	state := State{
		Phase:        PhaseConvergeReady,
		Steps:        []Step{{Title: "Inspect prompt flow", Status: StepPending}},
		DecisionGaps: nil,
	}
	result := ValidateState(state)
	if result.OK {
		t.Fatalf("expected converged plan without readiness to fail validation")
	}
}

func TestCanStartExecutionRequiresConvergedPhaseAndNoDecisionGaps(t *testing.T) {
	state := State{
		Phase:        PhaseConvergeReady,
		Steps:        []Step{{Title: "Inspect prompt flow", Status: StepPending}},
		DecisionGaps: []string{"Pick the trigger phrase"},
	}
	if CanStartExecution(state) {
		t.Fatalf("expected unresolved decision gaps to block execution")
	}

	state.DecisionGaps = nil
	if !CanStartExecution(state) {
		t.Fatalf("expected converged plan without decision gaps to allow execution")
	}
}

func TestShouldRenderStructuredPlanBlockHidesClarifyTurns(t *testing.T) {
	state := State{
		Phase:        PhaseClarify,
		Steps:        []Step{{Title: "Inspect prompt flow", Status: StepPending}},
		DecisionGaps: []string{"Pick the trigger phrase"},
	}
	if ShouldRenderStructuredPlanBlock(state) {
		t.Fatalf("expected clarify turn with open decision gap not to render proposed plan")
	}
}

func TestShouldRenderStructuredPlanBlockShowsConvergedPlan(t *testing.T) {
	state := State{
		Phase:               PhaseConvergeReady,
		Steps:               []Step{{Title: "Inspect prompt flow", Status: StepPending}},
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
	}
	if !ShouldRenderStructuredPlanBlock(state) {
		t.Fatalf("expected converged plan to render proposed plan block")
	}
}

func TestNormalizeStateKeepsClarifyChoiceWhenConvergedStateStillContainsIt(t *testing.T) {
	state := NormalizeState(State{
		Phase:        PhaseConvergeReady,
		Steps:        []Step{{Title: "Inspect prompt flow", Status: StepPending}},
		DecisionGaps: nil,
		ActiveChoice: &ActiveChoice{
			ID:       "frontend_stack",
			Question: "Pick the frontend stack",
			Options: []ChoiceOption{
				{ID: "a", Shortcut: "A", Title: "FastAPI + Jinja2"},
				{ID: "b", Shortcut: "B", Title: "Flask + Jinja2"},
			},
		},
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
	})
	if state.ActiveChoice == nil {
		t.Fatalf("expected active choice to survive inconsistent convergence state, got %#v", state)
	}
	if state.Phase != PhaseClarify {
		t.Fatalf("expected phase to fall back to clarify, got %q", state.Phase)
	}
	if len(state.DecisionGaps) != 1 || state.DecisionGaps[0] != "Pick the frontend stack" {
		t.Fatalf("expected decision gap to be restored from active choice, got %#v", state.DecisionGaps)
	}
}

func TestValidateStateRejectsActiveChoiceWithoutEnoughOptions(t *testing.T) {
	result := ValidateState(State{
		Phase:        PhaseClarify,
		Steps:        []Step{{Title: "Inspect prompt flow", Status: StepPending}},
		DecisionGaps: []string{"Choose the frontend stack"},
		ActiveChoice: &ActiveChoice{
			ID:       "frontend_stack",
			Question: "Pick the frontend stack",
			Options: []ChoiceOption{
				{ID: "a", Shortcut: "A", Title: "FastAPI + Jinja2"},
			},
		},
	})
	if result.OK {
		t.Fatalf("expected invalid active choice to fail validation")
	}
}
