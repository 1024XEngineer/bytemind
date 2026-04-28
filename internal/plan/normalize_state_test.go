package plan

import "testing"

func TestNormalizeStateRestoresDecisionGapFromActiveChoice(t *testing.T) {
	state := NormalizeState(State{
		Phase: PhaseClarify,
		Steps: []Step{{Title: "Choose the frontend layout", Status: StepPending}},
		ActiveChoice: &ActiveChoice{
			ID:       "layout",
			Kind:     "clarify",
			Question: "Choose the frontend layout.",
			Options: []ChoiceOption{
				{ID: "a", Shortcut: "A", Title: "Split panes"},
				{ID: "b", Shortcut: "B", Title: "Single column"},
			},
		},
	})

	if len(state.DecisionGaps) != 1 || state.DecisionGaps[0] != "Choose the frontend layout." {
		t.Fatalf("expected decision gap to be restored from active choice, got %#v", state.DecisionGaps)
	}
	if state.ActiveChoice == nil {
		t.Fatalf("expected active choice to be preserved, got %#v", state)
	}
	if state.Phase != PhaseClarify {
		t.Fatalf("expected clarify phase, got %q", state.Phase)
	}
}

func TestNormalizeStateDowngradesPrematureConvergedPhaseWhenActiveChoiceExists(t *testing.T) {
	state := NormalizeState(State{
		Phase:               PhaseConvergeReady,
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
		Steps:               []Step{{Title: "Choose the frontend layout", Status: StepPending}},
		ActiveChoice: &ActiveChoice{
			ID:       "layout",
			Kind:     "clarify",
			GapKey:   "Choose the frontend layout.",
			Question: "Choose the frontend layout.",
			Options: []ChoiceOption{
				{ID: "a", Shortcut: "A", Title: "Split panes"},
				{ID: "b", Shortcut: "B", Title: "Single column"},
			},
		},
	})

	if state.Phase != PhaseClarify {
		t.Fatalf("expected premature converge_ready phase to be downgraded to clarify, got %q", state.Phase)
	}
	if len(state.DecisionGaps) != 1 || state.DecisionGaps[0] != "Choose the frontend layout." {
		t.Fatalf("expected decision gap to be restored from active choice, got %#v", state.DecisionGaps)
	}
	if state.ActiveChoice == nil {
		t.Fatalf("expected active choice to survive normalization, got %#v", state)
	}
}
