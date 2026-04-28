package plan

import (
	"strings"
	"testing"
)

func TestFinalizeAssistantAnswerBuildModeUnchanged(t *testing.T) {
	answer := "normal build answer"
	got := FinalizeAssistantAnswer(ModeBuild, State{}, answer)
	if got != answer {
		t.Fatalf("expected unchanged answer, got %q", got)
	}
}

func TestFinalizeAssistantAnswerPlanModeWithOpenDecisionGapKeepsQuestionOnly(t *testing.T) {
	answer := "plan answer"
	got := FinalizeAssistantAnswer(ModePlan, State{
		Goal:         "Ship plan loop",
		Phase:        PhaseClarify,
		Steps:        []Step{{Title: "step1", Status: StepPending}},
		DecisionGaps: []string{"Choose the execution trigger wording"},
	}, answer)
	if got != answer {
		t.Fatalf("expected clarify answer to stay question-only, got %q", got)
	}
}

func TestFinalizeAssistantAnswerPlanModeWithoutDecisionGapAppendsCanonicalBlock(t *testing.T) {
	answer := "plan answer"
	got := FinalizeAssistantAnswer(ModePlan, State{
		Goal:                "Ship plan loop",
		ImplementationBrief: "Objective: ship the plan loop.\nDeliverable: prompt + finalize behavior.",
		Phase:               PhaseConvergeReady,
		Steps:               []Step{{Title: "step1", Status: StepPending}},
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
	}, answer)
	for _, want := range []string{
		"plan answer",
		"<proposed_plan>",
		"## Implementation Brief",
		"### Objective",
		"ship the plan loop.",
		"## Goal",
		"1. [pending] step1",
		"- [x] Scope defined",
		"</proposed_plan>",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected finalized answer to include %q, got %q", want, got)
		}
	}
}

func TestFinalizeAssistantAnswerMovesActionChoicesAfterProposedPlan(t *testing.T) {
	answer := strings.Join([]string{
		"计划已收敛，可以执行。",
		"",
		"请选择下一步：",
		"- Start execution",
		"- Adjust plan",
		"",
		"你输入 start execution / continue execution / 开始执行 / 继续执行 都会切换到 Build mode。",
	}, "\n")
	got := FinalizeAssistantAnswer(ModePlan, State{
		Goal:                "Ship plan loop",
		ImplementationBrief: "Objective: ship the plan loop.\nDeliverable: prompt + finalize behavior.",
		Phase:               PhaseConvergeReady,
		Steps:               []Step{{Title: "step1", Status: StepPending}},
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
	}, answer)

	actionIndex := strings.Index(got, "请选择下一步")
	planIndex := strings.Index(got, "<proposed_plan>")
	if actionIndex < 0 || planIndex < 0 {
		t.Fatalf("expected both action block and proposed plan, got %q", got)
	}
	if actionIndex < planIndex {
		t.Fatalf("expected action block after proposed plan, got %q", got)
	}
	if !strings.HasPrefix(got, "计划已收敛，可以执行。") {
		t.Fatalf("expected summary to stay before proposed plan, got %q", got)
	}
}

func TestFinalizeAssistantAnswerMovesNumberedActionChoicesAfterProposedPlan(t *testing.T) {
	answer := strings.Join([]string{
		"Plan is converged and ready to execute.",
		"",
		"Choose next step:",
		"1. Start execution",
		"2. Adjust plan",
		"",
		"Reply with 1 / A / start execution / continue execution to enter Build mode.",
	}, "\n")
	got := FinalizeAssistantAnswer(ModePlan, State{
		Goal:                "Ship plan loop",
		ImplementationBrief: "Objective: ship the plan loop.\nDeliverable: prompt + finalize behavior.",
		Phase:               PhaseConvergeReady,
		Steps:               []Step{{Title: "step1", Status: StepPending}},
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
	}, answer)

	actionIndex := strings.Index(got, "Choose next step:")
	planIndex := strings.Index(got, "<proposed_plan>")
	if actionIndex < 0 || planIndex < 0 {
		t.Fatalf("expected both action block and proposed plan, got %q", got)
	}
	if actionIndex < planIndex {
		t.Fatalf("expected numbered action block after proposed plan, got %q", got)
	}
}

func TestFinalizeAssistantAnswerPlanModeWithoutStructuredPlanAppendsReminder(t *testing.T) {
	answer := "drafted plan"
	got := FinalizeAssistantAnswer(ModePlan, State{}, answer)
	want := answer + "\n\n" + StructuredPlanReminder
	if got != want {
		t.Fatalf("unexpected finalized answer: got=%q want=%q", got, want)
	}
}

func TestFinalizeAssistantAnswerPlanModeWithoutStructuredPlanHandlesEmptyAnswer(t *testing.T) {
	got := FinalizeAssistantAnswer(ModePlan, State{}, "   ")
	if got != StructuredPlanReminder {
		t.Fatalf("expected reminder-only answer, got %q", got)
	}
}
