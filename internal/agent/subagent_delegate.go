package agent

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	planpkg "bytemind/internal/plan"
	subagentspkg "bytemind/internal/subagents"
	"bytemind/internal/tools"
)

const (
	subAgentErrorCodeNotImplemented = "subagent_not_implemented"
)

var subAgentInvocationCounter atomic.Uint64

func (r *Runner) delegateSubAgent(
	ctx context.Context,
	request tools.DelegateSubAgentRequest,
	execCtx *tools.ExecutionContext,
) (tools.DelegateSubAgentResult, error) {
	_ = ctx
	result := tools.DelegateSubAgentResult{
		OK:           false,
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
	_, err := gateway.Preflight(subagentspkg.PreflightRequest{
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

	result.Error = &tools.DelegateSubAgentError{
		Code:      subAgentErrorCodeNotImplemented,
		Message:   "delegate_subagent execution pipeline is not wired yet",
		Retryable: true,
	}
	return result, nil
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
