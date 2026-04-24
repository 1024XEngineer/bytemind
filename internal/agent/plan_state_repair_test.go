package agent

import (
	"context"
	"io"
	"strings"
	"testing"

	"bytemind/internal/config"
	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
	"bytemind/internal/session"
	"bytemind/internal/tools"
)

func TestLooksLikeInlineClarifyChoicePromptMatchesCompactChineseChoiceSentence(t *testing.T) {
	text := "\u8bf7\u76f4\u63a5\u9009\u4e00\u4e2a\u65b9\u6848\uff1aA\uff08\u63a8\u8350\uff0c\u96f6\u4f9d\u8d56\u6807\u51c6\u5e93\uff09\u3001B\uff08Flask\uff09\uff0c\u6216 C\uff08\u4f60\u81ea\u5b9a\u4e49\u7ea6\u675f\uff09\u3002"
	if !looksLikeInlineClarifyChoicePrompt(text) {
		t.Fatalf("expected compact Chinese clarify prompt to be detected, got %q", text)
	}
}

func TestShouldRepairPlanClarifyTurnIgnoresExecutionActionChoices(t *testing.T) {
	state := planpkg.State{
		Goal:                "Ship the feature",
		Phase:               planpkg.PhaseConvergeReady,
		Steps:               []planpkg.Step{{Title: "Implement the feature", Status: planpkg.StepPending}},
		ScopeDefined:        true,
		RiskRollbackDefined: true,
		VerificationDefined: true,
	}
	reply := llm.Message{
		Role:    llm.RoleAssistant,
		Content: "A. Start execution\nB. Adjust plan",
	}
	if shouldRepairPlanClarifyTurn(planpkg.ModePlan, state, turnIntentAskUser, reply) {
		t.Fatalf("expected execution action choices not to trigger clarify repair")
	}
}

func TestRunPromptRepairsInlineClarifyChoiceWhenDecisionGapsWereDropped(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.Mode = planpkg.ModePlan

	client := &fakeClient{replies: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name: "update_plan",
					Arguments: `{
						"summary":"Need to lock the frontend stack before the plan converges.",
						"phase":"clarify",
						"decision_gaps":[],
						"active_choice":{
							"id":"frontend_stack",
							"kind":"clarify",
							"question":"Choose the frontend stack.",
							"options":[
								{"id":"stdlib","shortcut":"A","title":"Standard library only","recommended":true},
								{"id":"flask","shortcut":"B","title":"Flask"},
								{"id":"custom","shortcut":"C","title":"Custom constraints","freeform":true}
							]
						},
						"plan":[
							{"step":"Choose the frontend stack","status":"pending"},
							{"step":"Draft the implementation plan","status":"pending"},
							{"step":"Define the verification path","status":"pending"}
						]
					}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "\u8bf7\u76f4\u63a5\u9009\u4e00\u4e2a\u65b9\u6848\uff1aA\uff08\u63a8\u8350\uff0c\u96f6\u4f9d\u8d56\u6807\u51c6\u5e93\uff09\u3001B\uff08Flask\uff09\uff0c\u6216 C\uff08\u4f60\u81ea\u5b9a\u4e49\u7ea6\u675f\uff09\u3002",
		},
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "call-2",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name: "update_plan",
					Arguments: `{
						"summary":"The frontend stack choice is waiting on the user.",
						"phase":"clarify",
						"decision_gaps":["Choose the frontend stack."],
						"active_choice":{
							"id":"frontend_stack",
							"kind":"clarify",
							"question":"Choose the frontend stack.",
							"gap_key":"Choose the frontend stack.",
							"options":[
								{"id":"stdlib","shortcut":"A","title":"Standard library only","description":"Recommended for the lowest dependency footprint.","recommended":true},
								{"id":"flask","shortcut":"B","title":"Flask","description":"Use Flask for faster templated delivery."},
								{"id":"custom","shortcut":"C","title":"Custom constraints","description":"Capture a different path or constraint.","freeform":true}
							]
						},
						"plan":[
							{"step":"Choose the frontend stack","status":"pending"},
							{"step":"Draft the implementation plan","status":"pending"},
							{"step":"Define the verification path","status":"pending"}
						]
					}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "<turn_intent>ask_user</turn_intent>Please use the on-screen picker below.",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 8,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "continue planning", "plan", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(client.requests) != 4 {
		t.Fatalf("expected four requests (initial plan + bad clarify + repair + ask_user), got %d", len(client.requests))
	}

	repairTurnMessages := client.requests[2].Messages
	last := repairTurnMessages[len(repairTurnMessages)-1]
	if last.Role != llm.RoleUser || !strings.Contains(strings.ToLower(last.Text()), "without storing active_choice first") {
		t.Fatalf("expected clarify repair note to be appended as a user message, got %#v", repairTurnMessages)
	}

	if sess.Plan.ActiveChoice == nil {
		t.Fatalf("expected session plan to store active_choice after repair, got %#v", sess.Plan)
	}
	if len(sess.Plan.DecisionGaps) != 1 || sess.Plan.DecisionGaps[0] != "Choose the frontend stack." {
		t.Fatalf("expected repaired plan to restore the missing decision gap, got %#v", sess.Plan.DecisionGaps)
	}
	if !strings.Contains(answer, "on-screen picker") {
		t.Fatalf("expected repaired answer to keep a short picker lead sentence, got %q", answer)
	}
	if strings.Contains(strings.ToLower(answer), "flask") || strings.Contains(strings.ToLower(answer), "standard library only") {
		t.Fatalf("expected repaired answer to avoid inlining choice text once the picker can render it, got %q", answer)
	}
}
