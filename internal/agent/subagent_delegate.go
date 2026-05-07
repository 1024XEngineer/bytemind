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
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	runtimepkg "github.com/1024XEngineer/bytemind/internal/runtime"
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

	subAgentRequestedOutputSummary = "summary"

	subAgentTaskOutputTool = "task_output"
	subAgentTaskStopTool   = "task_stop"
)

var subAgentInvocationCounter atomic.Uint64

func (r *Runner) delegateSubAgent(
	ctx context.Context,
	request tools.DelegateSubAgentRequest,
	execCtx *tools.ExecutionContext,
	streamObserver Observer,
) (tools.DelegateSubAgentResult, error) {
	result := tools.DelegateSubAgentResult{
		OK:           false,
		Status:       subAgentResultStatusFailed,
		InvocationID: newSubAgentInvocationID(),
		Agent:        request.Agent,
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

	parentSession := execCtx.Session
	runtimeRequest := RuntimeTaskRequest{
		SessionID:  sessionIDFromExecutionContext(execCtx),
		Name:       "delegate_subagent/" + preflightResultName(result.Agent),
		Kind:       "subagent",
		Background: request.RunInBackground,
		Timeout:    preflight.RequestedTimeoutDuration,
		Metadata:   metadata,
		Execute: func(taskCtx context.Context) ([]byte, error) {
			subAgentResult, execErr := r.subAgentExecutor.Execute(taskCtx, SubAgentExecutionInput{
				Request:      request,
				Preflight:    preflight,
				InvocationID: result.InvocationID,
				Agent:        result.Agent,
				RunMode:      runMode,
				ExecCtx:      execCtx,
				Observer:     streamObserver,
				Store:        r.store,
			})
			if execErr != nil {
				return nil, execErr
			}
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
		OnTaskStateChanged: func(task runtimepkg.Task) {
			if !runtimepkg.IsTerminalTaskStatus(task.Status) || r.subAgentNotifier == nil {
				return
			}
			notification := SubAgentCompletionNotification{
				ParentSession: parentSession,
				TaskID:        string(task.ID),
				Agent:         result.Agent,
				InvocationID:  result.InvocationID,
			}
			if task.Status == corepkg.TaskCompleted {
				notification.Status = subAgentResultStatusCompleted
				var parsed tools.DelegateSubAgentResult
				if jsonErr := json.Unmarshal(task.Output, &parsed); jsonErr == nil {
					notification.Summary = parsed.Summary
				}
			} else {
				notification.Status = subAgentResultStatusFailed
				notification.ErrorCode = task.ErrorCode
				notification.ErrorMessage = fmt.Sprintf("subagent task ended with status %s", task.Status)
			}
			r.subAgentNotifier.NotifyCompletion(notification)
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
	if execution.TaskID != "" {
		result.TaskID = string(execution.TaskID)
	}
	if runErr != nil {
		// RunSync can return a parent wait timeout/cancel while still carrying a
		// settled terminal result. Prefer the settled completed output when present.
		waitTimedOutOrCanceled := errors.Is(runErr, context.Canceled) || errors.Is(runErr, context.DeadlineExceeded)
		settledCompleted := execution.ExecutionError == nil && execution.Result.Status == corepkg.TaskCompleted
		if !(waitTimedOutOrCanceled && settledCompleted) {
			result.Error = mapDelegateSubAgentError(runErr, subAgentErrorCodeRuntimeUnavailable)
			return result, nil
		}
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
	rawOutput := strings.TrimSpace(string(execution.Result.Output))
	if len(rawOutput) > 0 {
		normalized, normalizeErr := normalizeDelegateSubAgentResult(
			execution.Result.Output,
			result.InvocationID,
			result.Agent,
			result.TaskID,
		)
		if normalizeErr != nil {
			// Structured parse failed — treat raw output as plain text summary.
			result.OK = true
			result.Status = subAgentResultStatusCompleted
			result.Summary = truncateSubAgentSummary(rawOutput)
			result.Content = result.Summary
			return result, nil
		}
		// Structured parse succeeded — fill in summary from raw text if empty.
		if normalized.Summary == "" {
			normalized.Summary = truncateSubAgentSummary(rawOutput)
		}
		return normalized, nil
	}

	// Empty output — use a fallback summary.
	result.OK = true
	result.Status = subAgentResultStatusCompleted
	result.Summary = "SubAgent task completed."
	result.Content = result.Summary
	return result, nil
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

	// Reconcile OK/Error: if both are set, prefer the error.
	if result.OK && result.Error != nil {
		result.OK = false
		result.Status = subAgentResultStatusFailed
	}

	// Normalize status.
	result.Status = strings.ToLower(strings.TrimSpace(result.Status))
	if result.Status == "" {
		if result.OK {
			result.Status = subAgentResultStatusCompleted
		} else {
			result.Status = subAgentResultStatusFailed
		}
	}

	// Ensure failed results have an error.
	if !result.OK && result.Error == nil {
		result.Error = &tools.DelegateSubAgentError{
			Code:    subAgentErrorCodeExecutionFailed,
			Message: "subagent execution failed",
		}
	}
	if result.Error != nil {
		result.Error.Code = strings.ToLower(strings.TrimSpace(result.Error.Code))
		result.Error.Message = strings.TrimSpace(result.Error.Message)
		if result.Error.Code == "" {
			result.Error.Code = subAgentErrorCodeExecutionFailed
		}
		if result.Error.Message == "" {
			result.Error.Message = "subagent execution failed"
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

const maxSubAgentSummaryLen = 500

func truncateSubAgentSummary(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) <= maxSubAgentSummaryLen {
		return trimmed
	}
	return trimmed[:maxSubAgentSummaryLen-3] + "..."
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
