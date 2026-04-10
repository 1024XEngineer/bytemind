package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	planpkg "bytemind/internal/plan"
)

type ExecuteRequest struct {
	Name    string
	RawArgs string
	Mode    planpkg.AgentMode
	Context *ExecutionContext
}

type ExecuteResult struct {
	Name   string
	Spec   ToolSpec
	Output string
}

type PermissionEngine interface {
	Check(context.Context, ResolvedTool, *ExecutionContext) error
}

type ArgumentDecoder interface {
	Decode(string, ResolvedTool) (json.RawMessage, error)
}

type OutputNormalizer interface {
	Normalize(string, ResolvedTool) string
}

type Executor struct {
	registry         *Registry
	permissionEngine PermissionEngine
	argumentDecoder  ArgumentDecoder
	outputNormalizer OutputNormalizer
}

func NewExecutor(registry *Registry) *Executor {
	return &Executor{
		registry:         registry,
		permissionEngine: defaultPermissionEngine{},
		argumentDecoder:  strictJSONArgumentDecoder{},
		outputNormalizer: maxCharsOutputNormalizer{},
	}
}

func (e *Executor) Execute(ctx context.Context, name, rawArgs string, execCtx *ExecutionContext) (string, error) {
	return e.ExecuteForMode(ctx, planpkg.ModeBuild, name, rawArgs, execCtx)
}

func (e *Executor) ExecuteForMode(ctx context.Context, mode planpkg.AgentMode, name, rawArgs string, execCtx *ExecutionContext) (string, error) {
	result, err := e.ExecuteRequest(ctx, ExecuteRequest{
		Name:    name,
		RawArgs: rawArgs,
		Mode:    mode,
		Context: execCtx,
	})
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

func (e *Executor) ExecuteRequest(ctx context.Context, req ExecuteRequest) (ExecuteResult, error) {
	if e == nil || e.registry == nil {
		return ExecuteResult{}, NewToolExecError(ToolErrorInternal, "executor registry is unavailable", false, nil)
	}

	resolved, err := e.registry.ResolveForMode(req.Mode, strings.TrimSpace(req.Name))
	if err != nil {
		return ExecuteResult{}, err
	}
	if req.Context != nil {
		req.Context.Mode = planpkg.NormalizeMode(string(req.Mode))
	}

	raw, err := e.argumentDecoder.Decode(req.RawArgs, resolved)
	if err != nil {
		return ExecuteResult{}, err
	}
	if err := e.permissionEngine.Check(ctx, resolved, req.Context); err != nil {
		return ExecuteResult{}, err
	}

	execCtx := req.Context
	if execCtx == nil {
		execCtx = &ExecutionContext{}
		execCtx.Mode = planpkg.NormalizeMode(string(req.Mode))
	}

	output, runErr := resolved.Tool.Run(ctx, raw, execCtx)
	if runErr != nil {
		return ExecuteResult{}, normalizeToolError(runErr)
	}

	return ExecuteResult{
		Name:   resolved.Definition.Function.Name,
		Spec:   resolved.Spec,
		Output: e.outputNormalizer.Normalize(output, resolved),
	}, nil
}

type defaultPermissionEngine struct{}

func (defaultPermissionEngine) Check(_ context.Context, resolved ResolvedTool, execCtx *ExecutionContext) error {
	if !toolAllowedByPolicy(resolved.Definition.Function.Name, execCtx) {
		return NewToolExecError(ToolErrorPermissionDenied, fmt.Sprintf("tool %q is unavailable by active skill policy", resolved.Definition.Function.Name), false, nil)
	}
	return nil
}

type strictJSONArgumentDecoder struct{}

func (strictJSONArgumentDecoder) Decode(rawArgs string, resolved ResolvedTool) (json.RawMessage, error) {
	rawArgs = strings.TrimSpace(rawArgs)
	if rawArgs == "" {
		rawArgs = "{}"
	}

	var payload any
	if err := json.Unmarshal([]byte(rawArgs), &payload); err != nil {
		return nil, NewToolExecError(ToolErrorInvalidArgs, err.Error(), false, err)
	}

	if !resolved.Spec.StrictArgs {
		return json.RawMessage(rawArgs), nil
	}

	objectPayload, ok := payload.(map[string]any)
	if !ok {
		return nil, NewToolExecError(ToolErrorInvalidArgs, "tool arguments must be a JSON object", false, nil)
	}

	allowedFields := schemaPropertyNames(resolved.Definition.Function.Parameters)
	if len(allowedFields) == 0 {
		return json.RawMessage(rawArgs), nil
	}
	for key := range objectPayload {
		if _, ok := allowedFields[key]; ok {
			continue
		}
		return nil, NewToolExecError(ToolErrorInvalidArgs, fmt.Sprintf("unknown argument %q", key), false, nil)
	}

	return json.RawMessage(rawArgs), nil
}

type maxCharsOutputNormalizer struct{}

func (maxCharsOutputNormalizer) Normalize(output string, resolved ResolvedTool) string {
	maxChars := resolved.Spec.MaxResultChars
	if maxChars <= 0 || len(output) <= maxChars {
		return output
	}

	var payload any
	if err := json.Unmarshal([]byte(output), &payload); err != nil {
		return truncateText(output, maxChars)
	}

	normalized, ok := truncateJSONToFit(payload, maxChars)
	if !ok {
		return truncateText(output, maxChars)
	}
	return normalized
}

const truncatedSuffix = "\n...[truncated]"

func truncateJSONToFit(payload any, maxChars int) (string, bool) {
	for attempt := 0; attempt < 128; attempt++ {
		data, err := json.Marshal(payload)
		if err != nil {
			return "", false
		}
		if len(data) <= maxChars {
			return string(data), true
		}

		reduced, changed := reduceLargestString(payload, len(data)-maxChars)
		if !changed {
			return "", false
		}
		payload = reduced
	}
	return "", false
}

func reduceLargestString(value any, excess int) (any, bool) {
	switch current := value.(type) {
	case map[string]any:
		return reduceLargestMapString(current, excess)
	case []any:
		return reduceLargestSliceString(current, excess)
	default:
		return value, false
	}
}

func reduceLargestMapString(value map[string]any, excess int) (map[string]any, bool) {
	longestKey := ""
	longestLen := 0
	for key, item := range value {
		if text, ok := item.(string); ok && len(text) > longestLen {
			longestKey = key
			longestLen = len(text)
		}
	}
	if longestKey != "" {
		value[longestKey] = truncateText(value[longestKey].(string), nextStringLimit(longestLen, excess))
		return value, true
	}

	for key, item := range value {
		reduced, changed := reduceLargestString(item, excess)
		if !changed {
			continue
		}
		value[key] = reduced
		return value, true
	}
	return value, false
}

func reduceLargestSliceString(value []any, excess int) ([]any, bool) {
	longestIndex := -1
	longestLen := 0
	for index, item := range value {
		if text, ok := item.(string); ok && len(text) > longestLen {
			longestIndex = index
			longestLen = len(text)
		}
	}
	if longestIndex >= 0 {
		value[longestIndex] = truncateText(value[longestIndex].(string), nextStringLimit(longestLen, excess))
		return value, true
	}

	for index, item := range value {
		reduced, changed := reduceLargestString(item, excess)
		if !changed {
			continue
		}
		value[index] = reduced
		return value, true
	}
	return value, false
}

func nextStringLimit(length, excess int) int {
	reduction := excess + len(truncatedSuffix) + 8
	if reduction < length/2 {
		reduction = length / 2
	}
	if reduction >= length {
		return len(truncatedSuffix)
	}
	return length - reduction
}

func truncateText(value string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	if len(value) <= maxChars {
		return value
	}
	if maxChars <= len(truncatedSuffix) {
		return value[:maxChars]
	}
	return value[:maxChars-len(truncatedSuffix)] + truncatedSuffix
}

func normalizeToolError(err error) error {
	if err == nil {
		return nil
	}
	if execErr, ok := AsToolExecError(err); ok {
		return execErr
	}
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return NewToolExecError(ToolErrorTimeout, "tool execution timed out", true, err)
	case looksLikePermissionError(err):
		return NewToolExecError(ToolErrorPermissionDenied, err.Error(), false, err)
	case looksLikeInvalidArgsError(err):
		return NewToolExecError(ToolErrorInvalidArgs, err.Error(), false, err)
	default:
		return NewToolExecError(ToolErrorToolFailed, err.Error(), true, err)
	}
}

func looksLikePermissionError(err error) bool {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "approval") ||
		strings.Contains(message, "permission") ||
		strings.Contains(message, "unavailable in plan mode") ||
		strings.Contains(message, "active skill policy")
}

func looksLikeInvalidArgsError(err error) bool {
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "required") ||
		strings.Contains(message, "unknown field") ||
		strings.Contains(message, "unknown argument") ||
		strings.Contains(message, "cannot be empty") ||
		strings.Contains(message, "must be")
}

func schemaPropertyNames(parameters map[string]any) map[string]struct{} {
	properties, ok := parameters["properties"].(map[string]any)
	if !ok {
		return nil
	}
	names := make(map[string]struct{}, len(properties))
	for name := range properties {
		names[name] = struct{}{}
	}
	return names
}
