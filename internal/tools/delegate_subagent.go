package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"bytemind/internal/llm"
	planpkg "bytemind/internal/plan"
)

type DelegateSubAgentScope struct {
	Paths   []string `json:"paths"`
	Symbols []string `json:"symbols"`
}

type DelegateSubAgentRequest struct {
	Agent           string                `json:"agent"`
	Task            string                `json:"task"`
	Scope           DelegateSubAgentScope `json:"scope"`
	Timeout         string                `json:"timeout"`
	Isolation       string                `json:"isolation"`
	Output          string                `json:"output"`
	RunInBackground bool                  `json:"run_in_background"`
}

type DelegateSubAgentFinding struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

type DelegateSubAgentReference struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Note string `json:"note"`
}

type DelegateSubAgentError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

type DelegateSubAgentResult struct {
	OK           bool                        `json:"ok"`
	Status       string                      `json:"status,omitempty"`
	InvocationID string                      `json:"invocation_id"`
	Agent        string                      `json:"agent"`
	TaskID       string                      `json:"task_id,omitempty"`
	Summary      string                      `json:"summary,omitempty"`
	Findings     []DelegateSubAgentFinding   `json:"findings"`
	References   []DelegateSubAgentReference `json:"references"`
	Error        *DelegateSubAgentError      `json:"error,omitempty"`
}

type DelegateSubAgentHandler func(context.Context, DelegateSubAgentRequest, *ExecutionContext) (DelegateSubAgentResult, error)

type DelegateSubAgentTool struct{}

func (DelegateSubAgentTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "delegate_subagent",
			Description: "Delegate a clear, bounded subtask to a registered SubAgent and return a structured result.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"agent": map[string]any{
						"type":        "string",
						"description": "SubAgent name or alias, for example explorer or review.",
					},
					"task": map[string]any{
						"type":        "string",
						"description": "Independent, verifiable subtask statement.",
					},
					"scope": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"paths": map[string]any{
								"type":  "array",
								"items": map[string]any{"type": "string"},
							},
							"symbols": map[string]any{
								"type":  "array",
								"items": map[string]any{"type": "string"},
							},
						},
					},
					"timeout": map[string]any{
						"type":        "string",
						"description": "Optional timeout duration such as 90s.",
					},
					"isolation": map[string]any{
						"type":        "string",
						"description": "Optional isolation override such as worktree.",
					},
					"output": map[string]any{
						"type":        "string",
						"description": "Optional preferred result format such as findings.",
					},
					"run_in_background": map[string]any{
						"type":        "boolean",
						"description": "When true, launch asynchronously and return a task handle.",
					},
				},
				"required": []string{"agent", "task"},
			},
		},
	}
}

func (DelegateSubAgentTool) Spec() ToolSpec {
	return ToolSpec{
		Name:            "delegate_subagent",
		ReadOnly:        false,
		ConcurrencySafe: true,
		Destructive:     false,
		SafetyClass:     SafetyClassSensitive,
		StrictArgs:      true,
		AllowedModes:    []planpkg.AgentMode{planpkg.ModeBuild, planpkg.ModePlan},
		DefaultTimeoutS: 120,
		MaxTimeoutS:     900,
		MaxResultChars:  64 * 1024,
	}
}

func (DelegateSubAgentTool) Run(ctx context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	if execCtx == nil || execCtx.DelegateSubAgent == nil {
		return "", NewToolExecError(ToolErrorPermissionDenied, "delegate_subagent handler is unavailable", false, nil)
	}

	var req DelegateSubAgentRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return "", NewToolExecError(ToolErrorInvalidArgs, err.Error(), false, err)
	}
	req.Agent = strings.TrimSpace(req.Agent)
	req.Task = strings.TrimSpace(req.Task)
	if req.Agent == "" {
		return "", NewToolExecError(ToolErrorInvalidArgs, "agent is required", false, nil)
	}
	if req.Task == "" {
		return "", NewToolExecError(ToolErrorInvalidArgs, "task is required", false, nil)
	}

	result, err := execCtx.DelegateSubAgent(ctx, req, execCtx)
	if err != nil {
		return "", err
	}
	if result.Findings == nil {
		result.Findings = []DelegateSubAgentFinding{}
	}
	if result.References == nil {
		result.References = []DelegateSubAgentReference{}
	}
	output, marshalErr := toJSON(result)
	if marshalErr != nil {
		return "", NewToolExecError(ToolErrorInternal, fmt.Sprintf("failed to marshal delegate_subagent result: %v", marshalErr), false, marshalErr)
	}
	return output, nil
}
