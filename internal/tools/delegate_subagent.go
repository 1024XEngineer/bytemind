package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
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
	ResumeSessionID string                `json:"resume_session_id,omitempty"`
}

type DelegateSubAgentError struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// TranscriptMessage represents a single message in a subagent's transcript.
type TranscriptMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// SubAgentToolCallRecord is a JSON-serializable record of a tool call made
// by a subagent. JSON tags match tui.SubAgentToolCall for direct unmarshal.
type SubAgentToolCallRecord struct {
	ToolName    string   `json:"ToolName"`
	ToolCallID  string   `json:"ToolCallID"`
	CompactBody string   `json:"CompactBody"`
	Status      string   `json:"Status"`
	DetailLines []string `json:"DetailLines,omitempty"`
}

type DelegateSubAgentResult struct {
	OK                  bool                   `json:"ok"`
	Status              string                 `json:"status,omitempty"`
	InvocationID        string                 `json:"invocation_id"`
	Agent               string                 `json:"agent"`
	TaskID              string                 `json:"task_id,omitempty"`
	ResultReadTool      string                 `json:"result_read_tool,omitempty"`
	StopTool            string                 `json:"stop_tool,omitempty"`
	Summary             string                 `json:"summary,omitempty"`
	Content             string                 `json:"content,omitempty"`
	Error               *DelegateSubAgentError `json:"error,omitempty"`
	Transcript          []TranscriptMessage    `json:"transcript,omitempty"`
	TranscriptSessionID string                 `json:"transcript_session_id,omitempty"`
	Task                string                 `json:"task,omitempty"`
	ModifiedFiles       []string               `json:"modified_files,omitempty"`
	// ToolCalls carries structured tool call records from the subagent session.
	// Populated by the executor for TUI restoration; excluded from JSON via
	// json:"-" so it never reaches the parent LLM context.
	ToolCalls []SubAgentToolCallRecord `json:"-"`
}

type DelegateSubAgentHandler func(context.Context, DelegateSubAgentRequest, *ExecutionContext) (DelegateSubAgentResult, error)

type AgentInfo struct {
	Name        string
	Description string
}

type DelegateSubAgentTool struct {
	agents []AgentInfo
}

func NewDelegateSubAgentTool(agents []AgentInfo) DelegateSubAgentTool {
	return DelegateSubAgentTool{agents: agents}
}

func (t DelegateSubAgentTool) Definition() llm.ToolDefinition {
	desc := "Delegate a clear, bounded subtask to a registered SubAgent and return a structured result."
	if len(t.agents) > 0 {
		desc += " Available agents:\n"
		for _, a := range t.agents {
			desc += fmt.Sprintf("- %s: %s\n", a.Name, a.Description)
		}
	}
	desc += "\nUse this tool when the user explicitly requests an agent (via @mention) or when a task matches an agent's specialization."
	desc += "\nWrite a detailed, self-contained task description including context, constraints, and expected output format."

	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name:        "delegate_subagent",
			Description: desc,
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
						"description": "Optional preferred result format. Currently only \"summary\" is supported.",
					},
					"run_in_background": map[string]any{
						"type":        "boolean",
						"description": "When true, launch asynchronously and return a task handle.",
					},
					"resume_session_id": map[string]any{
						"type":        "string",
						"description": "Optional session ID of a previously completed subagent to resume. The new task is appended as a continuation.",
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
	output, marshalErr := toJSON(result)
	if marshalErr != nil {
		return "", NewToolExecError(ToolErrorInternal, fmt.Sprintf("failed to marshal delegate_subagent result: %v", marshalErr), false, marshalErr)
	}
	return output, nil
}
