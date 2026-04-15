package plan

import "testing"

func TestFinalizeAssistantAnswerBuildModeUnchanged(t *testing.T) {
	answer := "normal build answer"
	got := FinalizeAssistantAnswer(ModeBuild, State{}, answer)
	if got != answer {
		t.Fatalf("expected unchanged answer, got %q", got)
	}
}

func TestFinalizeAssistantAnswerPlanModeWithStructuredPlanUnchanged(t *testing.T) {
	answer := "plan answer"
	got := FinalizeAssistantAnswer(ModePlan, State{
		Steps: []Step{{Title: "step1", Status: StepPending}},
	}, answer)
	if got != answer {
		t.Fatalf("expected unchanged answer, got %q", got)
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
