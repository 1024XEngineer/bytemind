package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	"github.com/1024XEngineer/bytemind/internal/session"
	subagentspkg "github.com/1024XEngineer/bytemind/internal/subagents"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

const (
	defaultSubAgentMaxIterations = 8

	subAgentResultPolicyCompressed = `Return your final answer as a single JSON object (no markdown fences). Schema:
{"summary":"<one-paragraph overview>"}
Do not include full tool logs.`
)

// SubAgentExecutor runs a subagent task to completion and returns a structured result.
// It encapsulates: child runner creation, child session creation, prompt building,
// engine loop execution, and result extraction.
type SubAgentExecutor interface {
	Execute(ctx context.Context, input SubAgentExecutionInput) (tools.DelegateSubAgentResult, error)
}

// SubAgentExecutionInput carries all resolved parameters needed to execute a subagent task.
type SubAgentExecutionInput struct {
	Request      tools.DelegateSubAgentRequest
	Preflight    subagentspkg.PreflightResult
	InvocationID string
	Agent        string
	RunMode      planpkg.AgentMode
	ExecCtx      *tools.ExecutionContext
	Observer     Observer    // optional: receives streaming events from the child runner
	Store        SessionStore // optional: used to persist child session transcript
}

type defaultSubAgentExecutor struct {
	runner *Runner
}

// NewSubAgentExecutor creates a SubAgentExecutor backed by the given Runner.
func NewSubAgentExecutor(runner *Runner) SubAgentExecutor {
	return &defaultSubAgentExecutor{runner: runner}
}

func (e *defaultSubAgentExecutor) Execute(
	ctx context.Context,
	input SubAgentExecutionInput,
) (tools.DelegateSubAgentResult, error) {
	r := e.runner
	workspace := strings.TrimSpace(r.workspace)
	if input.ExecCtx != nil {
		if scopedWorkspace := strings.TrimSpace(input.ExecCtx.Workspace); scopedWorkspace != "" {
			workspace = scopedWorkspace
		}
	}
	parentSessionID := ""
	if input.ExecCtx != nil && input.ExecCtx.Session != nil {
		parentSessionID = strings.TrimSpace(input.ExecCtx.Session.ID)
	}

	childRunner := e.newSubAgentChildRunner(workspace, input.Preflight.Definition.MaxTurns, input.Observer)
	if childRunner == nil || childRunner.client == nil {
		return subAgentFailureResult(
			input.InvocationID,
			input.Agent,
			subAgentErrorCodeRuntimeUnavailable,
			"llm client is unavailable for subagent execution",
			true,
		), nil
	}

	// Resume path: load existing child session instead of creating a new one.
	var childSession *session.Session
	if resumeID := strings.TrimSpace(input.Request.ResumeSessionID); resumeID != "" && input.Store != nil {
		flattenedID := session.FlattenSubAgentSessionID(resumeID)
		loaded, loadErr := input.Store.Load(flattenedID)
		if loadErr != nil {
			return subAgentFailureResult(input.InvocationID, input.Agent, subAgentErrorCodeExecutionFailed,
				fmt.Sprintf("failed to load subagent session %s: %v", resumeID, loadErr), false), nil
		}
		childSession = loaded
		childSession.ID = resumeID
		// Append the new task as a continuation user message.
		childSession.Messages = append(childSession.Messages, llm.NewUserTextMessage(buildSubAgentTaskInput(input.Request)))
	} else {
		childSession = newSubAgentSession(workspace, parentSessionID, input.InvocationID, input.RunMode)
	}
	defer childRunner.clearSessionSkillBridges(childSession)

	if err := childRunner.syncExtensionTools(ctx, false); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return subAgentFailureResult(input.InvocationID, input.Agent, subAgentErrorCodeExecutionFailed, err.Error(), true), nil
		}
	}

	userInput := buildSubAgentTaskInput(input.Request)
	setup, err := childRunner.prepareRunPrompt(childSession, RunPromptInput{
		UserMessage: llm.NewUserTextMessage(userInput),
		DisplayText: userInput,
		SubAgent:    buildSubAgentPromptInput(input.Request, input.Preflight),
	}, string(input.RunMode))
	if err != nil {
		return subAgentFailureResult(input.InvocationID, input.Agent, subAgentErrorCodeExecutionFailed, err.Error(), true), nil
	}
	applySubAgentPreflightSetup(&setup, input.Preflight)

	answer, runErr := (&defaultEngine{runner: childRunner}).runPromptTurns(ctx, childSession, setup, nil)
	if runErr != nil {
		return subAgentFailureResult(input.InvocationID, input.Agent, subAgentErrorCodeExecutionFailed, runErr.Error(), true), nil
	}

	result := buildSubAgentResultFromAnswer(answer, input.InvocationID, input.Agent, childSession.Messages)

	// Persist child session transcript (best-effort).
	if input.Store != nil && childSession != nil {
		persistedID := session.FlattenSubAgentSessionID(childSession.ID)
		clone := cloneSessionForPersist(childSession, persistedID, workspace)
		_ = input.Store.Save(clone)
		result.TranscriptSessionID = persistedID
	}

	return result, nil
}

func (e *defaultSubAgentExecutor) newSubAgentChildRunner(workspace string, maxTurns int, streamObserver Observer) *Runner {
	r := e.runner
	cfg := r.config
	cfg.MaxIterations = resolveSubAgentMaxIterations(cfg.MaxIterations, maxTurns)
	// Deep-copy mutable config slices to prevent parent/child aliasing.
	cfg.WritableRoots = append([]string(nil), cfg.WritableRoots...)
	cfg.ExecAllowlist = append([]config.ExecAllowRule(nil), cfg.ExecAllowlist...)
	cfg.NetworkAllowlist = append([]config.NetworkAllowRule(nil), cfg.NetworkAllowlist...)

	childObserver := Observer(&noOpObserver{})
	if streamObserver != nil {
		childObserver = streamObserver
	}

	return NewRunner(Options{
		Workspace:       workspace,
		Config:          cfg,
		Client:          r.GetClient(),
		Registry:        r.registry,
		Executor:        r.executor,
		PolicyGateway:   r.policyGateway,
		TaskManager:     r.taskManager,
		Runtime:         r.runtime,
		Extensions:      r.extensions,
		SubAgentManager: r.subAgentManager,
		TokenManager:    r.tokenManager,
		AuditStore:      r.auditStore,
		PromptStore:     r.promptStore,
		Observer:        childObserver,
		Approval:        nonInteractiveApproval(),
		Stdin:           nil,
		Stdout:          subAgentStdout(),
	})
}

func newSubAgentSession(workspace, parentSessionID, invocationID string, runMode planpkg.AgentMode) *session.Session {
	child := session.New(workspace)
	base := strings.TrimSpace(parentSessionID)
	if base == "" {
		base = "session"
	}
	child.ID = fmt.Sprintf("%s/subagent/%s", base, strings.TrimSpace(invocationID))
	child.Mode = runMode
	child.ActiveSkill = nil
	return child
}

func buildSubAgentTaskInput(request tools.DelegateSubAgentRequest) string {
	task := strings.TrimSpace(request.Task)
	if task == "" {
		task = "Complete the delegated subagent task."
	}
	return task
}

func buildSubAgentPromptInput(request tools.DelegateSubAgentRequest, preflight subagentspkg.PreflightResult) *SubAgentPromptInput {
	return &SubAgentPromptInput{
		Name:           strings.TrimSpace(preflight.Definition.Name),
		Task:           strings.TrimSpace(request.Task),
		ScopePaths:     normalizeUniqueStrings(request.Scope.Paths),
		ScopeSymbols:   normalizeUniqueStrings(request.Scope.Symbols),
		AllowedTools:   append([]string(nil), preflight.EffectiveTools...),
		Isolation:      strings.TrimSpace(preflight.Isolation),
		ResultPolicy:   subAgentResultPolicyCompressed,
		DefinitionBody: strings.TrimSpace(preflight.Definition.Instruction),
	}
}

func applySubAgentPreflightSetup(setup *runPromptSetup, preflight subagentspkg.PreflightResult) {
	if setup == nil {
		return
	}
	setup.AllowedTools = cloneToolSet(preflight.AllowedTools)
	setup.DeniedTools = cloneToolSet(preflight.DeniedTools)
	setup.AllowedToolNames = append([]string(nil), preflight.EffectiveTools...)
	setup.DeniedToolNames = sortedToolSetNames(preflight.DeniedTools)
	setup.AvailableTools = append([]string(nil), preflight.EffectiveTools...)
	setup.AvailableSubAgents = nil
	setup.ActiveSkill = nil
}

func sortedToolSetNames(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	names := make([]string, 0, len(set))
	for name := range set {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		names = append(names, trimmed)
	}
	if len(names) == 0 {
		return nil
	}
	slices.Sort(names)
	return names
}

func buildSubAgentResultFromAnswer(answer, invocationID, agent string, messages []llm.Message) tools.DelegateSubAgentResult {
	trimmed := strings.TrimSpace(answer)
	if trimmed == "" {
		trimmed = "SubAgent task completed."
	}
	base := tools.DelegateSubAgentResult{
		OK:           true,
		Status:       subAgentResultStatusCompleted,
		InvocationID: strings.TrimSpace(invocationID),
		Agent:        strings.TrimSpace(agent),
	}

	jsonStr := extractJSONFromAnswer(trimmed)
	if jsonStr == "" {
		base.Summary = trimmed
		base.Transcript = messagesToTranscript(messages)
		return base
	}

	var parsed tools.DelegateSubAgentResult
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		base.Summary = trimmed
		base.Transcript = messagesToTranscript(messages)
		return base
	}

	hasStructuredData := strings.TrimSpace(parsed.Summary) != ""
	if !hasStructuredData {
		base.Summary = trimmed
		base.Transcript = messagesToTranscript(messages)
		return base
	}

	base.Summary = strings.TrimSpace(parsed.Summary)
	if base.Summary == "" {
		base.Summary = trimmed
	}
	base.Transcript = messagesToTranscript(messages)
	return base
}

func extractJSONFromAnswer(answer string) string {
	if json.Valid([]byte(answer)) {
		return answer
	}

	if idx := strings.Index(answer, "```"); idx >= 0 {
		fenceEnd := strings.Index(answer[idx+3:], "\n")
		if fenceEnd >= 0 {
			codeStart := idx + 3 + fenceEnd + 1
			codeEnd := strings.Index(answer[codeStart:], "```")
			if codeEnd >= 0 {
				candidate := strings.TrimSpace(answer[codeStart : codeStart+codeEnd])
				if json.Valid([]byte(candidate)) {
					return candidate
				}
			}
		}
	}

	first := strings.Index(answer, "{")
	if first < 0 {
		return ""
	}
	depth := 0
	for i := first; i < len(answer); i++ {
		switch answer[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				candidate := answer[first : i+1]
				if json.Valid([]byte(candidate)) {
					return candidate
				}
			}
		}
	}
	return ""
}

func resolveSubAgentMaxIterations(parentMaxIterations int, requestedMaxTurns int) int {
	effectiveParent := parentMaxIterations
	if effectiveParent <= 0 {
		effectiveParent = defaultSubAgentMaxIterations
	}
	if requestedMaxTurns > 0 && requestedMaxTurns < effectiveParent {
		return requestedMaxTurns
	}
	return effectiveParent
}

func subAgentFailureResult(invocationID, agent, code, message string, retryable bool) tools.DelegateSubAgentResult {
	return tools.DelegateSubAgentResult{
		OK:           false,
		Status:       subAgentResultStatusFailed,
		InvocationID: strings.TrimSpace(invocationID),
		Agent:        strings.TrimSpace(agent),
		Error: &tools.DelegateSubAgentError{
			Code:      strings.TrimSpace(code),
			Message:   strings.TrimSpace(message),
			Retryable: retryable,
		},
	}
}

func messagesToTranscript(messages []llm.Message) []tools.TranscriptMessage {
	result := make([]tools.TranscriptMessage, 0, len(messages))
	for _, msg := range messages {
		content := strings.TrimSpace(msg.Text())
		if content == "" {
			continue
		}
		result = append(result, tools.TranscriptMessage{
			Role:    string(msg.Role),
			Content: content,
		})
	}
	return result
}

func cloneSessionForPersist(src *session.Session, flattenedID, workspace string) *session.Session {
	data, err := json.Marshal(src)
	if err != nil {
		return src
	}
	var clone session.Session
	if err := json.Unmarshal(data, &clone); err != nil {
		return src
	}
	clone.ID = flattenedID
	clone.Workspace = workspace
	return &clone
}
