package subagents

import (
	"fmt"
	"sort"
	"strings"

	planpkg "bytemind/internal/plan"
)

const (
	ErrorCodeSubAgentUnavailable      = "subagent_unavailable"
	ErrorCodeSubAgentInvalidRequest   = "subagent_invalid_request"
	ErrorCodeSubAgentTaskNotEligible  = "subagent_task_not_eligible"
	ErrorCodeSubAgentAgentNotFound    = "subagent_agent_not_found"
	ErrorCodeSubAgentModeNotAllowed   = "subagent_mode_not_allowed"
	ErrorCodeSubAgentToolDenied       = "subagent_tool_denied"
	DelegateSubAgentToolName          = "delegate_subagent"
	defaultRequestedIsolationFallback = "none"
)

type GatewayError struct {
	Code      string
	Message   string
	Retryable bool
}

func (e *GatewayError) Error() string {
	if e == nil {
		return ""
	}
	return strings.TrimSpace(e.Message)
}

func newGatewayError(code, message string, retryable bool) error {
	return &GatewayError{
		Code:      strings.TrimSpace(code),
		Message:   strings.TrimSpace(message),
		Retryable: retryable,
	}
}

type PreflightRequest struct {
	Agent              string
	Task               string
	Mode               planpkg.AgentMode
	ParentVisible      []string
	ParentAllowed      map[string]struct{}
	ParentDenied       map[string]struct{}
	RequestedTimeout   string
	RequestedOutput    string
	RequestedIsolation string
}

type PreflightResult struct {
	Definition       Agent
	AllowedTools     map[string]struct{}
	DeniedTools      map[string]struct{}
	EffectiveTools   []string
	RequestedTimeout string
	RequestedOutput  string
	Isolation        string
}

type Gateway struct {
	manager *Manager
}

func NewGateway(manager *Manager) *Gateway {
	return &Gateway{manager: manager}
}

func (g *Gateway) Preflight(request PreflightRequest) (PreflightResult, error) {
	if g == nil || g.manager == nil {
		return PreflightResult{}, newGatewayError(ErrorCodeSubAgentUnavailable, "subagent manager is unavailable", true)
	}

	agentName := strings.TrimSpace(request.Agent)
	if agentName == "" {
		return PreflightResult{}, newGatewayError(ErrorCodeSubAgentInvalidRequest, "agent is required", false)
	}
	task := strings.TrimSpace(request.Task)
	if task == "" {
		return PreflightResult{}, newGatewayError(ErrorCodeSubAgentTaskNotEligible, "task is required", false)
	}

	definition, ok := g.manager.Find(agentName)
	if !ok {
		return PreflightResult{}, newGatewayError(ErrorCodeSubAgentAgentNotFound, fmt.Sprintf("subagent not found: %s", agentName), false)
	}
	if !modeAllowedForDefinition(definition, request.Mode) {
		return PreflightResult{}, newGatewayError(ErrorCodeSubAgentModeNotAllowed, fmt.Sprintf("subagent %q is not allowed in %s mode", definition.Name, planpkg.NormalizeMode(string(request.Mode))), false)
	}

	visibleSet := normalizeNameSet(request.ParentVisible)
	if len(visibleSet) == 0 {
		return PreflightResult{}, newGatewayError(ErrorCodeSubAgentToolDenied, "no parent-visible tools available for delegation", false)
	}

	workingSet := cloneSet(visibleSet)
	if len(definition.Tools) > 0 {
		workingSet = intersectSet(workingSet, normalizeNameSet(definition.Tools))
	}
	if len(request.ParentAllowed) > 0 {
		workingSet = intersectSet(workingSet, normalizeMapSet(request.ParentAllowed))
	}

	deniedSet := normalizeMapSet(request.ParentDenied)
	for name := range normalizeNameSet(definition.DisallowedTools) {
		deniedSet[name] = struct{}{}
	}
	deniedSet[DelegateSubAgentToolName] = struct{}{}
	delete(workingSet, DelegateSubAgentToolName)

	for name := range deniedSet {
		delete(workingSet, name)
	}

	effectiveTools := setToSortedSlice(workingSet)
	if len(effectiveTools) == 0 {
		return PreflightResult{}, newGatewayError(ErrorCodeSubAgentToolDenied, fmt.Sprintf("subagent %q has no effective tools after policy narrowing", definition.Name), false)
	}

	allowedTools := make(map[string]struct{}, len(effectiveTools))
	for _, name := range effectiveTools {
		allowedTools[name] = struct{}{}
	}

	isolation := strings.TrimSpace(request.RequestedIsolation)
	if isolation == "" {
		isolation = strings.TrimSpace(definition.Isolation)
	}
	if isolation == "" {
		isolation = defaultRequestedIsolationFallback
	}

	return PreflightResult{
		Definition:       definition,
		AllowedTools:     allowedTools,
		DeniedTools:      deniedSet,
		EffectiveTools:   effectiveTools,
		RequestedTimeout: strings.TrimSpace(request.RequestedTimeout),
		RequestedOutput:  strings.TrimSpace(request.RequestedOutput),
		Isolation:        isolation,
	}, nil
}

func modeAllowedForDefinition(definition Agent, mode planpkg.AgentMode) bool {
	required := planpkg.NormalizeMode(strings.TrimSpace(definition.Mode))
	if required == "" {
		return true
	}
	return required == planpkg.NormalizeMode(string(mode))
}

func normalizeNameSet(items []string) map[string]struct{} {
	set := make(map[string]struct{}, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item)
		if name == "" {
			continue
		}
		set[name] = struct{}{}
	}
	return set
}

func normalizeMapSet(items map[string]struct{}) map[string]struct{} {
	if len(items) == 0 {
		return map[string]struct{}{}
	}
	set := make(map[string]struct{}, len(items))
	for name := range items {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		set[trimmed] = struct{}{}
	}
	return set
}

func cloneSet(in map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(in))
	for key := range in {
		out[key] = struct{}{}
	}
	return out
}

func intersectSet(left map[string]struct{}, right map[string]struct{}) map[string]struct{} {
	if len(left) == 0 || len(right) == 0 {
		return map[string]struct{}{}
	}
	out := make(map[string]struct{})
	for key := range left {
		if _, ok := right[key]; ok {
			out[key] = struct{}{}
		}
	}
	return out
}

func setToSortedSlice(set map[string]struct{}) []string {
	if len(set) == 0 {
		return nil
	}
	items := make([]string, 0, len(set))
	for item := range set {
		items = append(items, item)
	}
	sort.Strings(items)
	return items
}
