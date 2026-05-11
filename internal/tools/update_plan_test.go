package tools

import (
	"context"
	"encoding/json"
	"testing"

	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	"github.com/1024XEngineer/bytemind/internal/session"
)

func TestUpdatePlanToolUpdatesSessionPlan(t *testing.T) {
	workspace := t.TempDir()
	sess := session.New(workspace)
	tool := UpdatePlanTool{}
	payload, _ := json.Marshal(map[string]any{
		"explanation": "starting work",
		"plan": []map[string]any{
			{"step": "Inspect provider", "status": "completed"},
			{"step": "Add tests", "status": "in_progress"},
		},
	})
	result, err := tool.Run(context.Background(), payload, &ExecutionContext{Session: sess})
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Plan.Steps) != 2 || sess.Plan.Steps[1].Title != "Add tests" || sess.Plan.Steps[1].Status != planpkg.StepInProgress {
		t.Fatalf("unexpected session plan %#v", sess.Plan)
	}

	var parsed struct {
		Explanation string        `json:"explanation"`
		Plan        planpkg.State `json:"plan"`
	}
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Explanation != "starting work" || len(parsed.Plan.Steps) != 2 {
		t.Fatalf("unexpected result %#v", parsed)
	}
}

func TestUpdatePlanToolRequiresSession(t *testing.T) {
	tool := UpdatePlanTool{}
	payload := []byte(`{"plan":[{"step":"x","status":"pending"}]}`)
	_, err := tool.Run(context.Background(), payload, &ExecutionContext{})
	if err == nil {
		t.Fatal("expected missing session error")
	}
}

func TestUpdatePlanToolRejectsInvalidPlanShapes(t *testing.T) {
	tests := []struct {
		name    string
		payload string
	}{
		{
			name:    "empty plan",
			payload: `{"plan":[]}`,
		},
		{
			name:    "empty step",
			payload: `{"plan":[{"step":" ","status":"pending"}]}`,
		},
		{
			name:    "multiple in progress",
			payload: `{"plan":[{"step":"x","status":"in_progress"},{"step":"y","status":"in_progress"}]}`,
		},
	}

	tool := UpdatePlanTool{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sess := session.New(t.TempDir())
			_, err := tool.Run(context.Background(), []byte(tt.payload), &ExecutionContext{Session: sess})
			if err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestUpdatePlanToolStoresConvergenceFields(t *testing.T) {
	sess := session.New(t.TempDir())
	tool := UpdatePlanTool{}
	payload, _ := json.Marshal(map[string]any{
		"phase":                     "converge_ready",
		"scope_defined":             true,
		"risk_and_rollback_defined": true,
		"verification_defined":      true,
		"decision_log":              []map[string]any{{"decision": "Keep plan state provider-agnostic", "reason": "Matches prompt architecture"}},
		"risks":                     []string{"Mode switch regression"},
		"verification":              []string{"go test ./internal/plan -v"},
		"plan":                      []map[string]any{{"step": "Rewrite prompt", "status": "pending"}},
	})

	if _, err := tool.Run(context.Background(), payload, &ExecutionContext{Session: sess, Mode: planpkg.ModePlan}); err != nil {
		t.Fatal(err)
	}
	if sess.Plan.Phase != planpkg.PhaseConvergeReady {
		t.Fatalf("expected converge_ready phase, got %q", sess.Plan.Phase)
	}
	if len(sess.Plan.DecisionLog) != 1 || sess.Plan.DecisionLog[0].Decision == "" {
		t.Fatalf("expected decision log to be stored, got %#v", sess.Plan.DecisionLog)
	}
	if !sess.Plan.ScopeDefined || !sess.Plan.RiskRollbackDefined || !sess.Plan.VerificationDefined {
		t.Fatalf("expected readiness flags to be stored, got %#v", sess.Plan)
	}
}

func TestUpdatePlanToolStoresActiveChoice(t *testing.T) {
	sess := session.New(t.TempDir())
	tool := UpdatePlanTool{}
	payload, _ := json.Marshal(map[string]any{
		"phase":         "clarify",
		"decision_gaps": []string{"Choose the frontend stack"},
		"active_choice": map[string]any{
			"id":       "frontend_stack",
			"kind":     "clarify",
			"question": "前端希望走哪条路线？",
			"options": []map[string]any{
				{
					"id":          "fastapi_jinja2",
					"shortcut":    "A",
					"title":       "FastAPI + Jinja2 HTML",
					"description": "贴近现有 HTML GUI 方向",
					"recommended": true,
				},
				{
					"id":          "other",
					"shortcut":    "B",
					"title":       "Other",
					"description": "输入自定义方案",
					"freeform":    true,
				},
			},
		},
		"plan": []map[string]any{
			{"step": "Define the stack", "status": "pending"},
		},
	})

	if _, err := tool.Run(context.Background(), payload, &ExecutionContext{Session: sess, Mode: planpkg.ModePlan}); err != nil {
		t.Fatal(err)
	}
	if sess.Plan.ActiveChoice == nil {
		t.Fatal("expected active choice to be stored")
	}
	if got := sess.Plan.ActiveChoice.Question; got != "前端希望走哪条路线？" {
		t.Fatalf("unexpected active choice question %q", got)
	}
	if len(sess.Plan.ActiveChoice.Options) != 2 || !sess.Plan.ActiveChoice.Options[1].Freeform {
		t.Fatalf("unexpected active choice %#v", sess.Plan.ActiveChoice)
	}
}
