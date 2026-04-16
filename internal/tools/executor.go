package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

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

type WriteApprovalEngine interface {
	Check(context.Context, ResolvedTool, *ExecutionContext) error
}

type Executor struct {
	registry         *Registry
	permissionEngine PermissionEngine
	writeApproval    WriteApprovalEngine
	argumentDecoder  ArgumentDecoder
	outputNormalizer OutputNormalizer
}

func NewExecutor(registry *Registry) *Executor {
	return &Executor{
		registry:         registry,
		permissionEngine: defaultPermissionEngine{},
		writeApproval:    defaultWriteApprovalEngine{},
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
	if e.writeApproval != nil {
		if err := e.writeApproval.Check(ctx, resolved, req.Context); err != nil {
			return ExecuteResult{}, err
		}
	}

	execCtx := req.Context
	if execCtx == nil {
		execCtx = &ExecutionContext{}
		execCtx.Mode = planpkg.NormalizeMode(string(req.Mode))
	}

	runCtx, cancel := context.WithTimeout(ctx, executionTimeout(raw, resolved.Spec))
	defer cancel()

	output, runErr := resolved.Tool.Run(runCtx, raw, execCtx)
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

type defaultWriteApprovalEngine struct{}

func (defaultWriteApprovalEngine) Check(_ context.Context, resolved ResolvedTool, execCtx *ExecutionContext) error {
	if !resolved.Spec.Destructive {
		return nil
	}
	if execCtx == nil {
		return NewToolExecError(ToolErrorPermissionDenied, fmt.Sprintf("tool %q requires approval context", resolved.Definition.Function.Name), false, nil)
	}

	switch strings.TrimSpace(execCtx.ApprovalPolicy) {
	case "never":
		return nil
	case "always", "on-request", "":
		return promptForWriteApproval(resolved.Definition.Function.Name, execCtx)
	default:
		return promptForWriteApproval(resolved.Definition.Function.Name, execCtx)
	}
}

func promptForWriteApproval(toolName string, execCtx *ExecutionContext) error {
	reason := "writes files in the workspace"
	if execCtx.Approval != nil {
		approved, err := execCtx.Approval(ApprovalRequest{
			Command: toolName,
			Reason:  reason,
		})
		if err != nil {
			return NewToolExecError(ToolErrorPermissionDenied, err.Error(), false, err)
		}
		if !approved {
			return NewToolExecError(ToolErrorPermissionDenied, fmt.Sprintf("tool %q was not run because approval was denied", toolName), false, nil)
		}
		return nil
	}

	if execCtx.Stdin == nil {
		return NewToolExecError(ToolErrorPermissionDenied, fmt.Sprintf("tool %q requires approval but no stdin is available", toolName), false, nil)
	}
	if execCtx.Stdout != nil {
		fmt.Fprintf(execCtx.Stdout, "Approve tool %q (%s)? [y/N]: ", toolName, reason)
	}
	reader := bufio.NewReader(execCtx.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return NewToolExecError(ToolErrorPermissionDenied, err.Error(), false, err)
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	if answer != "y" && answer != "yes" {
		return NewToolExecError(ToolErrorPermissionDenied, fmt.Sprintf("tool %q was not run because approval was denied", toolName), false, nil)
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

	if schemaAllowsUnknownFields(resolved.Definition.Function.Parameters) {
		return json.RawMessage(rawArgs), nil
	}

	allowedFields := schemaPropertyNames(resolved.Definition.Function.Parameters)
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
	if maxChars <= 0 || utf8.RuneCountInString(output) <= maxChars {
		return output
	}
	if json.Valid([]byte(output)) {
		return output
	}
	const suffix = "\n...[truncated]"
	suffixChars := utf8.RuneCountInString(suffix)
	if maxChars <= suffixChars {
		return truncateRunesUnsafe(output, maxChars)
	}
	return truncateRunesUnsafe(output, maxChars-suffixChars) + suffix
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

func schemaAllowsUnknownFields(parameters map[string]any) bool {
	value, ok := parameters["additionalProperties"]
	if !ok {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case map[string]any:
		return true
	default:
		return false
	}
}

func executionTimeout(raw json.RawMessage, spec ToolSpec) time.Duration {
	timeoutSeconds := spec.DefaultTimeoutS
	if requested, ok := requestedTimeoutSeconds(raw); ok {
		timeoutSeconds = requested
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = spec.DefaultTimeoutS
	}
	if spec.MaxTimeoutS > 0 && timeoutSeconds > spec.MaxTimeoutS {
		timeoutSeconds = spec.MaxTimeoutS
	}
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	return time.Duration(timeoutSeconds) * time.Second
}

func requestedTimeoutSeconds(raw json.RawMessage) (int, bool) {
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return 0, false
	}
	value, ok := payload["timeout_seconds"]
	if !ok {
		return 0, false
	}
	timeout, ok := value.(float64)
	if !ok {
		return 0, false
	}
	return int(timeout), true
}

func truncateRunesUnsafe(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	count := 0
	for index := range value {
		if count == limit {
			return value[:index]
		}
		count++
	}
	return value
}
