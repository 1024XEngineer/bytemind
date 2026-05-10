package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

type UpdatePlanTool struct{}

func (UpdatePlanTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "update_plan",
			Description: "Update the task plan for multi-step work. Use it when a task has several meaningful steps.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"goal":    map[string]any{"type": "string"},
					"summary": map[string]any{"type": "string"},
					"phase": map[string]any{"type": "string", "enum": []string{
						"none",
						"explore",
						"clarify",
						"draft",
						"converge_ready",
						"approved_to_build",
						"drafting",
						"ready",
						"approved",
						"executing",
						"blocked",
						"completed",
					}},
					"implementation_brief": map[string]any{"type": "string"},
					"risks": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
					"verification": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
					"decision_log": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"decision": map[string]any{"type": "string"},
								"reason":   map[string]any{"type": "string"},
							},
							"required": []string{"decision"},
						},
					},
					"decision_gaps": map[string]any{
						"type":  "array",
						"items": map[string]any{"type": "string"},
					},
					"active_choice": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"id":       map[string]any{"type": "string"},
							"kind":     map[string]any{"type": "string"},
							"question": map[string]any{"type": "string"},
							"gap_key":  map[string]any{"type": "string"},
							"options": map[string]any{
								"type": "array",
								"items": map[string]any{
									"type": "object",
									"properties": map[string]any{
										"id":          map[string]any{"type": "string"},
										"shortcut":    map[string]any{"type": "string"},
										"title":       map[string]any{"type": "string"},
										"description": map[string]any{"type": "string"},
										"recommended": map[string]any{"type": "boolean"},
										"freeform":    map[string]any{"type": "boolean"},
									},
									"required": []string{"title"},
								},
							},
						},
						"required": []string{"question", "options"},
					},
					"scope_defined":             map[string]any{"type": "boolean"},
					"risk_and_rollback_defined": map[string]any{"type": "boolean"},
					"verification_defined":      map[string]any{"type": "boolean"},
					"next_action":               map[string]any{"type": "string"},
					"block_reason":              map[string]any{"type": "string"},
					"explanation": map[string]any{
						"type":        "string",
						"description": "Optional short explanation of why the plan changed.",
					},
					"plan": map[string]any{
						"type": "array",
						"items": map[string]any{
							"type": "object",
							"properties": map[string]any{
								"id":          map[string]any{"type": "string"},
								"step":        map[string]any{"type": "string"},
								"title":       map[string]any{"type": "string"},
								"description": map[string]any{"type": "string"},
								"status":      map[string]any{"type": "string", "enum": []string{"pending", "in_progress", "completed", "blocked"}},
								"files": map[string]any{
									"type":  "array",
									"items": map[string]any{"type": "string"},
								},
								"verify": map[string]any{
									"type":  "array",
									"items": map[string]any{"type": "string"},
								},
								"risk": map[string]any{"type": "string", "enum": []string{"low", "medium", "high"}},
							},
							"required": []string{"status"},
						},
					},
				},
				"required": []string{"plan"},
			},
		},
	}
}

func (UpdatePlanTool) Run(_ context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	if execCtx.Session == nil {
		return "", errors.New("session is required for update_plan")
	}

	var args struct {
		Goal                string   `json:"goal"`
		Summary             string   `json:"summary"`
		ImplementationBrief string   `json:"implementation_brief"`
		Phase               string   `json:"phase"`
		Risks               []string `json:"risks"`
		Verification        []string `json:"verification"`
		DecisionLog         []struct {
			Decision string `json:"decision"`
			Reason   string `json:"reason"`
		} `json:"decision_log"`
		DecisionGaps []string `json:"decision_gaps"`
		ActiveChoice *struct {
			ID       string `json:"id"`
			Kind     string `json:"kind"`
			Question string `json:"question"`
			GapKey   string `json:"gap_key"`
			Options  []struct {
				ID          string `json:"id"`
				Shortcut    string `json:"shortcut"`
				Title       string `json:"title"`
				Description string `json:"description"`
				Recommended bool   `json:"recommended"`
				Freeform    bool   `json:"freeform"`
			} `json:"options"`
		} `json:"active_choice"`
		ScopeDefined           *bool  `json:"scope_defined"`
		RiskAndRollbackDefined *bool  `json:"risk_and_rollback_defined"`
		VerificationDefined    *bool  `json:"verification_defined"`
		NextAction             string `json:"next_action"`
		BlockReason            string `json:"block_reason"`
		Explanation            string `json:"explanation"`
		Plan                   []struct {
			ID          string   `json:"id"`
			Step        string   `json:"step"`
			Title       string   `json:"title"`
			Description string   `json:"description"`
			Status      string   `json:"status"`
			Files       []string `json:"files"`
			Verify      []string `json:"verify"`
			Risk        string   `json:"risk"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return "", err
	}
	if len(args.Plan) == 0 {
		return "", errors.New("plan must contain at least one step")
	}

	steps := make([]planpkg.Step, 0, len(args.Plan))
	inProgressCount := 0
	for i, item := range args.Plan {
		title := strings.TrimSpace(item.Title)
		if title == "" {
			title = strings.TrimSpace(item.Step)
		}
		status := planpkg.NormalizeStepStatus(item.Status)
		if title == "" {
			return "", errors.New("plan step title cannot be empty")
		}
		if status == planpkg.StepInProgress {
			inProgressCount++
		}
		step := planpkg.Step{
			ID:          strings.TrimSpace(item.ID),
			Title:       title,
			Description: strings.TrimSpace(item.Description),
			Status:      status,
			Files:       trimPlanStrings(item.Files),
			Verify:      trimPlanStrings(item.Verify),
			Risk:        normalizeRisk(item.Risk),
		}
		if step.ID == "" {
			step.ID = fmt.Sprintf("s%d", i+1)
		}
		steps = append(steps, step)
	}
	if inProgressCount > 1 {
		return "", errors.New("only one plan item can be in_progress")
	}

	state := execCtx.Session.Plan
	state.Goal = chooseNonEmpty(strings.TrimSpace(args.Goal), state.Goal)
	state.Summary = chooseNonEmpty(strings.TrimSpace(args.Summary), strings.TrimSpace(args.Explanation), state.Summary)
	state.ImplementationBrief = chooseNonEmpty(strings.TrimSpace(args.ImplementationBrief), state.ImplementationBrief)
	state.NextAction = chooseNonEmpty(strings.TrimSpace(args.NextAction), state.NextAction)
	state.BlockReason = chooseNonEmpty(strings.TrimSpace(args.BlockReason), state.BlockReason)
	state.Steps = steps
	if args.Risks != nil {
		state.Risks = trimPlanStrings(args.Risks)
	}
	if args.Verification != nil {
		state.Verification = trimPlanStrings(args.Verification)
	}
	if args.DecisionLog != nil {
		decisionLog := make([]planpkg.Decision, 0, len(args.DecisionLog))
		for _, entry := range args.DecisionLog {
			decisionLog = append(decisionLog, planpkg.Decision{
				Decision: strings.TrimSpace(entry.Decision),
				Reason:   strings.TrimSpace(entry.Reason),
			})
		}
		state.DecisionLog = decisionLog
	}
	if args.DecisionGaps != nil {
		state.DecisionGaps = trimPlanStrings(args.DecisionGaps)
	}
	if args.ActiveChoice != nil {
		activeChoice := &planpkg.ActiveChoice{
			ID:       strings.TrimSpace(args.ActiveChoice.ID),
			Kind:     strings.TrimSpace(args.ActiveChoice.Kind),
			Question: strings.TrimSpace(args.ActiveChoice.Question),
			GapKey:   strings.TrimSpace(args.ActiveChoice.GapKey),
			Options:  make([]planpkg.ChoiceOption, 0, len(args.ActiveChoice.Options)),
		}
		for _, option := range args.ActiveChoice.Options {
			activeChoice.Options = append(activeChoice.Options, planpkg.ChoiceOption{
				ID:          strings.TrimSpace(option.ID),
				Shortcut:    strings.TrimSpace(option.Shortcut),
				Title:       strings.TrimSpace(option.Title),
				Description: strings.TrimSpace(option.Description),
				Recommended: option.Recommended,
				Freeform:    option.Freeform,
			})
		}
		state.ActiveChoice = activeChoice
	}
	if args.ScopeDefined != nil {
		state.ScopeDefined = *args.ScopeDefined
	}
	if args.RiskAndRollbackDefined != nil {
		state.RiskRollbackDefined = *args.RiskAndRollbackDefined
	}
	if args.VerificationDefined != nil {
		state.VerificationDefined = *args.VerificationDefined
	}
	state.Phase = planpkg.NormalizePhase(args.Phase)
	if state.Phase == planpkg.PhaseNone {
		state.Phase = planpkg.DerivePhase(execCtx.Mode, state)
	}
	state.UpdatedAt = time.Now().UTC()
	state = planpkg.NormalizeState(state)
	if state.NextAction == "" {
		state.NextAction = planpkg.DefaultNextAction(state)
	}

	validation := planpkg.ValidateState(state)
	if !validation.OK {
		return "", errors.New(strings.Join(validation.Warnings, "; "))
	}

	execCtx.Session.Plan = state
	return toJSON(map[string]any{
		"ok":          true,
		"explanation": strings.TrimSpace(args.Explanation),
		"plan":        state,
	})
}

func trimPlanStrings(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func normalizeRisk(raw string) planpkg.RiskLevel {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return planpkg.NormalizeRisk(raw)
}

func chooseNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
