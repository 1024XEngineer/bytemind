package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	runtimepkg "github.com/1024XEngineer/bytemind/internal/runtime"
	"github.com/1024XEngineer/bytemind/internal/session"
	subagentspkg "github.com/1024XEngineer/bytemind/internal/subagents"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

const (
	subAgentErrorCodeNotImplemented        = "subagent_not_implemented"
	subAgentErrorCodeRuntimeUnavailable    = "subagent_runtime_unavailable"
	subAgentErrorCodeBackgroundUnavailable = "subagent_background_unavailable"
	subAgentErrorCodeBackgroundWriteDenied = "subagent_background_write_not_allowed"
	subAgentErrorCodeInvalidResult         = "subagent_invalid_result"
	subAgentErrorCodeExecutionFailed       = "subagent_execution_failed"

	subAgentResultStatusCompleted = "completed"
	subAgentResultStatusFailed    = "failed"
	subAgentResultStatusQueued    = "queued"
	subAgentResultStatusRunning   = "running"
	subAgentResultStatusAccepted  = "accepted"

	subAgentRequestedOutputFindings = "findings"
	subAgentRequestedOutputSummary  = "summary"

	defaultSubAgentMaxIterations = 8

	subAgentResultPolicyCompressed = "Return compressed findings only. Do not include full tool logs."

	subAgentTaskOutputTool = "task_output"
	subAgentTaskStopTool   = "task_stop"
)

var subAgentInvocationCounter atomic.Uint64

func (r *Runner) delegateSubAgent(
	ctx context.Context,
	request tools.DelegateSubAgentRequest,
	execCtx *tools.ExecutionContext,
) (tools.DelegateSubAgentResult, error) {
	result := tools.DelegateSubAgentResult{
		OK:           false,
		Status:       subAgentResultStatusFailed,
		InvocationID: newSubAgentInvocationID(),
		Agent:        request.Agent,
		Findings:     []tools.DelegateSubAgentFinding{},
		References:   []tools.DelegateSubAgentReference{},
	}

	if r == nil || r.subAgentManager == nil {
		result.Error = &tools.DelegateSubAgentError{
			Code:      subagentspkg.ErrorCodeSubAgentUnavailable,
			Message:   "subagent manager is unavailable",
			Retryable: true,
		}
		return result, nil
	}

	runMode := planpkg.ModeBuild
	if execCtx != nil {
		runMode = planpkg.NormalizeMode(string(execCtx.Mode))
	}
	parentVisible := []string(nil)
	if r.registry != nil {
		parentVisible = toolNames(r.registry.DefinitionsForMode(runMode))
	}

	gateway := subagentspkg.NewGateway(r.subAgentManager)
	preflight, err := gateway.Preflight(subagentspkg.PreflightRequest{
		Agent:              request.Agent,
		Task:               request.Task,
		Mode:               runMode,
		ParentVisible:      parentVisible,
		ParentAllowed:      cloneToolSet(execCtxGetAllowed(execCtx)),
		ParentDenied:       cloneToolSet(execCtxGetDenied(execCtx)),
		RequestedTimeout:   request.Timeout,
		RequestedOutput:    request.Output,
		RequestedIsolation: request.Isolation,
	})
	if err != nil {
		if gatewayErr, ok := err.(*subagentspkg.GatewayError); ok {
			result.Error = &tools.DelegateSubAgentError{
				Code:      gatewayErr.Code,
				Message:   gatewayErr.Message,
				Retryable: gatewayErr.Retryable,
			}
			return result, nil
		}
		result.Error = &tools.DelegateSubAgentError{
			Code:      subagentspkg.ErrorCodeSubAgentUnavailable,
			Message:   err.Error(),
			Retryable: true,
		}
		return result, nil
	}
	if canonical := strings.TrimSpace(preflight.Definition.Name); canonical != "" {
		result.Agent = canonical
	}

	if r.runtime == nil {
		result.Error = &tools.DelegateSubAgentError{
			Code:      subAgentErrorCodeRuntimeUnavailable,
			Message:   "runtime gateway is unavailable for subagent execution",
			Retryable: true,
		}
		return result, nil
	}

	metadata := map[string]string{
		"invocation_id":          result.InvocationID,
		"agent":                  result.Agent,
		"mode":                   string(runMode),
		"isolation":              preflight.Isolation,
		"effective_tool_count":   strconv.Itoa(len(preflight.EffectiveTools)),
		"effective_toolset_hash": effectiveToolsetHash(preflight.EffectiveTools),
		"effective_tools":        strings.Join(preflight.EffectiveTools, ","),
	}
	if preflight.RequestedTimeout != "" {
		metadata["requested_timeout"] = preflight.RequestedTimeout
		metadata["requested_timeout_ms"] = strconv.FormatInt(preflight.RequestedTimeoutDuration.Milliseconds(), 10)
	}
	if preflight.RequestedOutput != "" {
		metadata["requested_output"] = preflight.RequestedOutput
	}

	runtimeRequest := RuntimeTaskRequest{
		SessionID:  sessionIDFromExecutionContext(execCtx),
		Name:       "delegate_subagent/" + preflightResultName(result.Agent),
		Kind:       "subagent",
		Background: request.RunInBackground,
		Timeout:    preflight.RequestedTimeoutDuration,
		Metadata:   metadata,
		Execute: func(taskCtx context.Context) ([]byte, error) {
			subAgentResult := r.executeSubAgentTask(taskCtx, request, preflight, result.InvocationID, result.Agent, runMode, execCtx)
			output, marshalErr := json.Marshal(subAgentResult)
			if marshalErr != nil {
				return nil, &subAgentExecutionError{
					code:      subAgentErrorCodeExecutionFailed,
					message:   fmt.Sprintf("failed to marshal subagent execution result: %v", marshalErr),
					retryable: true,
				}
			}
			return output, nil
		},
	}

	if request.RunInBackground {
		if !supportsBackgroundLifecycleTools(parentVisible, execCtxGetAllowed(execCtx), execCtxGetDenied(execCtx)) {
			result.Error = &tools.DelegateSubAgentError{
				Code:      subAgentErrorCodeBackgroundUnavailable,
				Message:   "run_in_background requires task_output and task_stop tools",
				Retryable: false,
			}
			return result, nil
		}
		if !r.isReadOnlySubAgentToolset(preflight.EffectiveTools) {
			result.Error = &tools.DelegateSubAgentError{
				Code:      subAgentErrorCodeBackgroundWriteDenied,
				Message:   "run_in_background currently supports read-only subagents only",
				Retryable: false,
			}
			return result, nil
		}

		launch, runErr := r.runtime.RunAsync(ctx, runtimeRequest)
		if runErr != nil {
			result.Error = mapDelegateSubAgentError(runErr, subAgentErrorCodeRuntimeUnavailable)
			return result, nil
		}
		if strings.TrimSpace(string(launch.TaskID)) == "" {
			result.Error = &tools.DelegateSubAgentError{
				Code:      subAgentErrorCodeRuntimeUnavailable,
				Message:   "runtime gateway returned empty task id for background subagent",
				Retryable: true,
			}
			return result, nil
		}
		result.OK = true
		result.Status = subAgentResultStatusAccepted
		result.TaskID = string(launch.TaskID)
		result.ResultReadTool = subAgentTaskOutputTool
		result.StopTool = subAgentTaskStopTool
		result.Summary = "SubAgent task launched in background."
		return result, nil
	}

	execution, runErr := r.runtime.RunSync(ctx, runtimeRequest)
	if runErr != nil {
		if execution.TaskID != "" {
			result.TaskID = string(execution.TaskID)
		}
		result.Error = mapDelegateSubAgentError(runErr, subAgentErrorCodeRuntimeUnavailable)
		return result, nil
	}
	if execution.TaskID != "" {
		result.TaskID = string(execution.TaskID)
	}
	if execution.ExecutionError != nil {
		result.Error = mapDelegateSubAgentError(execution.ExecutionError, subAgentErrorCodeNotImplemented)
		return result, nil
	}
	if execution.Result.Status != corepkg.TaskCompleted {
		errorCode, retryable := mapSubAgentTerminalResult(execution.Result.Status, execution.Result.ErrorCode)
		result.Error = &tools.DelegateSubAgentError{
			Code:      errorCode,
			Message:   fmt.Sprintf("subagent task ended with status %s", execution.Result.Status),
			Retryable: retryable,
		}
		return result, nil
	}
	if len(strings.TrimSpace(string(execution.Result.Output))) > 0 {
		normalized, normalizeErr := normalizeDelegateSubAgentResult(
			execution.Result.Output,
			result.InvocationID,
			result.Agent,
			result.TaskID,
		)
		if normalizeErr != nil {
			result.Error = &tools.DelegateSubAgentError{
				Code:      subAgentErrorCodeInvalidResult,
				Message:   fmt.Sprintf("subagent returned invalid structured result: %v", normalizeErr),
				Retryable: true,
			}
			return result, nil
		}
		if contractErr := validateDelegateSubAgentOutputContract(normalized, preflight.RequestedOutput); contractErr != nil {
			result.Error = &tools.DelegateSubAgentError{
				Code:      subAgentErrorCodeInvalidResult,
				Message:   fmt.Sprintf("subagent result violates requested output contract: %v", contractErr),
				Retryable: true,
			}
			return result, nil
		}
		return normalized, nil
	}

	result.Error = &tools.DelegateSubAgentError{
		Code:      subAgentErrorCodeNotImplemented,
		Message:   "subagent execution returned no structured result",
		Retryable: true,
	}
	return result, nil
}

func (r *Runner) executeSubAgentTask(
	ctx context.Context,
	request tools.DelegateSubAgentRequest,
	preflight subagentspkg.PreflightResult,
	invocationID string,
	agent string,
	runMode planpkg.AgentMode,
	execCtx *tools.ExecutionContext,
) tools.DelegateSubAgentResult {
	workspace := strings.TrimSpace(r.workspace)
	if execCtx != nil {
		if scopedWorkspace := strings.TrimSpace(execCtx.Workspace); scopedWorkspace != "" {
			workspace = scopedWorkspace
		}
	}
	parentSessionID := ""
	if execCtx != nil && execCtx.Session != nil {
		parentSessionID = strings.TrimSpace(execCtx.Session.ID)
	}

	childRunner := r.newSubAgentChildRunner(workspace, preflight.Definition.MaxTurns)
	if childRunner == nil || childRunner.client == nil {
		return subAgentFailureResult(
			invocationID,
			agent,
			subAgentErrorCodeRuntimeUnavailable,
			"llm client is unavailable for subagent execution",
			true,
		)
	}
	childSession := newSubAgentSession(workspace, parentSessionID, invocationID, runMode)
	defer childRunner.clearSessionSkillBridges(childSession)

	if err := childRunner.syncExtensionTools(ctx, false); err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return subAgentFailureResult(invocationID, agent, subAgentErrorCodeExecutionFailed, err.Error(), true)
		}
	}

	userInput := buildSubAgentTaskInput(request)
	setup, err := childRunner.prepareRunPrompt(childSession, RunPromptInput{
		UserMessage: llm.NewUserTextMessage(userInput),
		DisplayText: userInput,
		SubAgent:    buildSubAgentPromptInput(request, preflight),
	}, string(runMode))
	if err != nil {
		return subAgentFailureResult(invocationID, agent, subAgentErrorCodeExecutionFailed, err.Error(), true)
	}
	applySubAgentPreflightSetup(&setup, preflight)

	answer, runErr := (&defaultEngine{runner: childRunner}).runPromptTurns(ctx, childSession, setup, nil)
	if runErr != nil {
		return subAgentFailureResult(invocationID, agent, subAgentErrorCodeExecutionFailed, runErr.Error(), true)
	}

	summary := strings.TrimSpace(answer)
	if summary == "" {
		summary = "SubAgent task completed."
	}
	return tools.DelegateSubAgentResult{
		OK:           true,
		Status:       subAgentResultStatusCompleted,
		InvocationID: strings.TrimSpace(invocationID),
		Agent:        strings.TrimSpace(agent),
		Summary:      summary,
		Findings:     []tools.DelegateSubAgentFinding{},
		References:   []tools.DelegateSubAgentReference{},
	}
}

func (r *Runner) newSubAgentChildRunner(workspace string, maxTurns int) *Runner {
	cfg := r.config
	cfg.MaxIterations = resolveSubAgentMaxIterations(cfg.MaxIterations, maxTurns)
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
		Observer:        r.observer,
		Approval:        r.approval,
		Stdin:           r.stdin,
		Stdout:          r.stdout,
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
		Findings:     []tools.DelegateSubAgentFinding{},
		References:   []tools.DelegateSubAgentReference{},
		Error: &tools.DelegateSubAgentError{
			Code:      strings.TrimSpace(code),
			Message:   strings.TrimSpace(message),
			Retryable: retryable,
		},
	}
}

func preflightResultName(agent string) string {
	name := strings.TrimSpace(agent)
	if name == "" {
		return "unknown"
	}
	return name
}

func sessionIDFromExecutionContext(execCtx *tools.ExecutionContext) corepkg.SessionID {
	if execCtx == nil || execCtx.Session == nil {
		return ""
	}
	return corepkg.SessionID(execCtx.Session.ID)
}

type subAgentExecutionError struct {
	code      string
	message   string
	retryable bool
}

func (e *subAgentExecutionError) Error() string {
	if e == nil {
		return ""
	}
	return strings.TrimSpace(e.message)
}

func mapDelegateSubAgentError(err error, fallbackCode string) *tools.DelegateSubAgentError {
	if err == nil {
		return nil
	}
	var executionErr *subAgentExecutionError
	if errors.As(err, &executionErr) && executionErr != nil {
		code := strings.TrimSpace(executionErr.code)
		if code == "" {
			code = fallbackCode
		}
		return &tools.DelegateSubAgentError{
			Code:      code,
			Message:   strings.TrimSpace(executionErr.message),
			Retryable: executionErr.retryable,
		}
	}
	var runtimeErr interface{ Code() string }
	if errors.As(err, &runtimeErr) {
		code := strings.TrimSpace(runtimeErr.Code())
		if code == "" {
			code = fallbackCode
		}
		retryable := true
		var semanticRetryable interface{ Retryable() bool }
		if errors.As(err, &semanticRetryable) {
			retryable = semanticRetryable.Retryable()
		} else {
			retryable = code != runtimepkg.ErrorCodeTaskCancelled
		}
		return &tools.DelegateSubAgentError{
			Code:      code,
			Message:   strings.TrimSpace(err.Error()),
			Retryable: retryable,
		}
	}
	return &tools.DelegateSubAgentError{
		Code:      fallbackCode,
		Message:   strings.TrimSpace(err.Error()),
		Retryable: true,
	}
}

func mapSubAgentTerminalResult(status corepkg.TaskStatus, errorCode string) (code string, retryable bool) {
	code = strings.TrimSpace(errorCode)
	if code == "" {
		switch status {
		case corepkg.TaskKilled:
			code = runtimepkg.ErrorCodeTaskCancelled
		case corepkg.TaskFailed:
			code = runtimepkg.ErrorCodeTaskExecutionFailed
		default:
			code = subAgentErrorCodeRuntimeUnavailable
		}
	}
	retryable = status != corepkg.TaskKilled && code != runtimepkg.ErrorCodeTaskCancelled
	return code, retryable
}

func normalizeDelegateSubAgentResult(
	payload []byte,
	fallbackInvocationID string,
	fallbackAgent string,
	fallbackTaskID string,
) (tools.DelegateSubAgentResult, error) {
	var result tools.DelegateSubAgentResult
	if err := json.Unmarshal(payload, &result); err != nil {
		return tools.DelegateSubAgentResult{}, err
	}
	result.InvocationID = firstNonEmpty(result.InvocationID, fallbackInvocationID)
	result.Agent = firstNonEmpty(result.Agent, fallbackAgent)
	result.TaskID = firstNonEmpty(result.TaskID, fallbackTaskID)
	result.Summary = strings.TrimSpace(result.Summary)
	if result.Findings == nil {
		result.Findings = []tools.DelegateSubAgentFinding{}
	}
	result.Findings = normalizeDelegateSubAgentFindings(result.Findings)
	if result.References == nil {
		result.References = []tools.DelegateSubAgentReference{}
	}
	result.References = normalizeDelegateSubAgentReferences(result.References)
	if result.OK && result.Error != nil {
		return tools.DelegateSubAgentResult{}, fmt.Errorf("ok result must not include error")
	}
	result.Status = strings.ToLower(strings.TrimSpace(result.Status))
	if result.Status == "" {
		if result.OK {
			result.Status = subAgentResultStatusCompleted
		} else {
			result.Status = subAgentResultStatusFailed
		}
	}
	if !isAllowedSubAgentStatus(result.Status) {
		return tools.DelegateSubAgentResult{}, fmt.Errorf("unsupported status %q", result.Status)
	}
	if result.OK && result.Status == subAgentResultStatusFailed {
		return tools.DelegateSubAgentResult{}, fmt.Errorf("ok result must not use failed status")
	}
	if result.OK && requiresTaskIDForStatus(result.Status) && strings.TrimSpace(result.TaskID) == "" {
		return tools.DelegateSubAgentResult{}, fmt.Errorf("status %q requires non-empty task_id", result.Status)
	}
	if !result.OK && result.Status != subAgentResultStatusFailed {
		return tools.DelegateSubAgentResult{}, fmt.Errorf("failed result must use status %q", subAgentResultStatusFailed)
	}
	if !result.OK {
		if result.Error == nil {
			return tools.DelegateSubAgentResult{}, fmt.Errorf("failed result must include error")
		}
		result.Error.Code = strings.ToLower(strings.TrimSpace(result.Error.Code))
		result.Error.Message = strings.TrimSpace(result.Error.Message)
		if result.Error.Code == "" || result.Error.Message == "" {
			return tools.DelegateSubAgentResult{}, fmt.Errorf("failed result must include non-empty error code/message")
		}
	}
	return result, nil
}

func firstNonEmpty(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed != "" {
		return trimmed
	}
	return strings.TrimSpace(fallback)
}

func isAllowedSubAgentStatus(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case subAgentResultStatusCompleted,
		subAgentResultStatusFailed,
		subAgentResultStatusQueued,
		subAgentResultStatusRunning,
		subAgentResultStatusAccepted:
		return true
	default:
		return false
	}
}

func validateDelegateSubAgentOutputContract(result tools.DelegateSubAgentResult, requestedOutput string) error {
	if !result.OK {
		return nil
	}
	if strings.ToLower(strings.TrimSpace(result.Status)) != subAgentResultStatusCompleted {
		return nil
	}
	switch strings.ToLower(strings.TrimSpace(requestedOutput)) {
	case "":
		return nil
	case subAgentRequestedOutputSummary:
		if strings.TrimSpace(result.Summary) == "" {
			return fmt.Errorf("requested output %q requires non-empty summary", subAgentRequestedOutputSummary)
		}
		return nil
	case subAgentRequestedOutputFindings:
		if strings.TrimSpace(result.Summary) == "" && len(result.Findings) == 0 {
			return fmt.Errorf("requested output %q requires summary or findings", subAgentRequestedOutputFindings)
		}
		return nil
	default:
		return fmt.Errorf("unsupported requested output %q", requestedOutput)
	}
}

func requiresTaskIDForStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case subAgentResultStatusQueued, subAgentResultStatusRunning, subAgentResultStatusAccepted:
		return true
	default:
		return false
	}
}

func normalizeDelegateSubAgentFindings(in []tools.DelegateSubAgentFinding) []tools.DelegateSubAgentFinding {
	if len(in) == 0 {
		return []tools.DelegateSubAgentFinding{}
	}
	out := make([]tools.DelegateSubAgentFinding, 0, len(in))
	for _, finding := range in {
		normalized := tools.DelegateSubAgentFinding{
			Title: strings.TrimSpace(finding.Title),
			Body:  strings.TrimSpace(finding.Body),
		}
		if normalized.Title == "" && normalized.Body == "" {
			continue
		}
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return []tools.DelegateSubAgentFinding{}
	}
	return out
}

func normalizeDelegateSubAgentReferences(in []tools.DelegateSubAgentReference) []tools.DelegateSubAgentReference {
	if len(in) == 0 {
		return []tools.DelegateSubAgentReference{}
	}
	out := make([]tools.DelegateSubAgentReference, 0, len(in))
	for _, reference := range in {
		normalized := tools.DelegateSubAgentReference{
			Path: strings.TrimSpace(reference.Path),
			Line: reference.Line,
			Note: strings.TrimSpace(reference.Note),
		}
		if normalized.Path == "" && normalized.Line <= 0 && normalized.Note == "" {
			continue
		}
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return []tools.DelegateSubAgentReference{}
	}
	return out
}

func effectiveToolsetHash(toolNames []string) string {
	if len(toolNames) == 0 {
		return ""
	}
	canonical := make([]string, 0, len(toolNames))
	seen := make(map[string]struct{}, len(toolNames))
	for _, name := range toolNames {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		canonical = append(canonical, trimmed)
	}
	if len(canonical) == 0 {
		return ""
	}
	sort.Strings(canonical)
	sum := sha256.Sum256([]byte(strings.Join(canonical, "\n")))
	return hex.EncodeToString(sum[:])
}

func execCtxGetAllowed(execCtx *tools.ExecutionContext) map[string]struct{} {
	if execCtx == nil {
		return nil
	}
	return execCtx.AllowedTools
}

func execCtxGetDenied(execCtx *tools.ExecutionContext) map[string]struct{} {
	if execCtx == nil {
		return nil
	}
	return execCtx.DeniedTools
}

func supportsBackgroundLifecycleTools(parentVisible []string, parentAllowed map[string]struct{}, parentDenied map[string]struct{}) bool {
	return isToolVisibleInParent(subAgentTaskOutputTool, parentVisible, parentAllowed, parentDenied) &&
		isToolVisibleInParent(subAgentTaskStopTool, parentVisible, parentAllowed, parentDenied)
}

func isToolVisibleInParent(name string, parentVisible []string, parentAllowed map[string]struct{}, parentDenied map[string]struct{}) bool {
	toolName := strings.TrimSpace(name)
	if toolName == "" {
		return false
	}
	if !slices.Contains(parentVisible, toolName) {
		return false
	}
	if len(parentAllowed) > 0 {
		if _, ok := parentAllowed[toolName]; !ok {
			return false
		}
	}
	if len(parentDenied) > 0 {
		if _, denied := parentDenied[toolName]; denied {
			return false
		}
	}
	return true
}

func (r *Runner) isReadOnlySubAgentToolset(toolNames []string) bool {
	if len(toolNames) == 0 {
		return false
	}
	type toolSpecLookup interface {
		Spec(name string) (tools.ToolSpec, bool)
	}
	lookup, ok := r.registry.(toolSpecLookup)
	if !ok {
		return false
	}
	for _, name := range toolNames {
		spec, exists := lookup.Spec(strings.TrimSpace(name))
		if !exists || !spec.ReadOnly {
			return false
		}
	}
	return true
}

func cloneToolSet(source map[string]struct{}) map[string]struct{} {
	if len(source) == 0 {
		return nil
	}
	cloned := make(map[string]struct{}, len(source))
	for key := range source {
		cloned[key] = struct{}{}
	}
	return cloned
}

func newSubAgentInvocationID() string {
	sequence := subAgentInvocationCounter.Add(1)
	return fmt.Sprintf("subagent-%d-%d", time.Now().UTC().UnixNano(), sequence)
}
