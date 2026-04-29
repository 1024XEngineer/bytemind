package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type executorTestTool struct {
	name   string
	result string
	err    error
	run    func(context.Context, json.RawMessage, *ExecutionContext) (string, error)
}

func (t executorTestTool) Definition() llm.ToolDefinition {
	return llm.ToolDefinition{
		Type: "function",
		Function: llm.FunctionDefinition{
			Name: t.name,
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"path":            map[string]any{"type": "string"},
					"timeout_seconds": map[string]any{"type": "integer"},
				},
			},
		},
	}
}

func (t executorTestTool) Run(ctx context.Context, raw json.RawMessage, execCtx *ExecutionContext) (string, error) {
	if t.run != nil {
		return t.run(ctx, raw, execCtx)
	}
	return t.result, t.err
}

func registerBuiltinExecutorTool(t *testing.T, registry *Registry, tool executorTestTool) {
	t.Helper()
	if err := registry.Register(tool, RegisterOptions{Source: RegistrationSourceBuiltin}); err != nil {
		t.Fatal(err)
	}
}

func TestExecutorRejectsUnknownArgumentsByDefault(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{name: "strict_tool", result: `{"ok":true}`})
	executor := NewExecutor(registry)

	_, err := executor.Execute(context.Background(), "strict_tool", `{"path":"a.txt","extra":true}`, &ExecutionContext{})
	if err == nil {
		t.Fatal("expected argument validation error")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorInvalidArgs {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
}

func TestExecutorRejectsUnknownArgumentsWhenSchemaHasNoProperties(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{name: "strict_tool", result: `{"ok":true}`})
	registry.tools["strict_tool"] = ResolvedTool{
		Definition: llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name: "strict_tool",
				Parameters: map[string]any{
					"type": "object",
				},
			},
		},
		Spec: registry.tools["strict_tool"].Spec,
		Tool: registry.tools["strict_tool"].Tool,
	}
	executor := NewExecutor(registry)

	_, err := executor.Execute(context.Background(), "strict_tool", `{"extra":true}`, &ExecutionContext{})
	if err == nil {
		t.Fatal("expected argument validation error")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorInvalidArgs {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
}

func TestExecutorDefaultsEmptyArgsToJSONObject(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "strict_tool",
		run: func(_ context.Context, raw json.RawMessage, _ *ExecutionContext) (string, error) {
			if string(raw) != "{}" {
				t.Fatalf("expected empty args to default to {}, got %q", string(raw))
			}
			return `{"ok":true}`, nil
		},
	})
	executor := NewExecutor(registry)

	got, err := executor.Execute(context.Background(), "strict_tool", "", &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"ok":true}` {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestExecutorRejectsUnknownArgumentsWhenSchemaForbidsThem(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name:   "strict_tool",
		result: `{"ok":true}`,
		run:    nil,
	})
	registry.tools["strict_tool"] = ResolvedTool{
		Definition: llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name: "strict_tool",
				Parameters: map[string]any{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]any{
						"path": map[string]any{"type": "string"},
					},
				},
			},
		},
		Spec: registry.tools["strict_tool"].Spec,
		Tool: registry.tools["strict_tool"].Tool,
	}
	executor := NewExecutor(registry)

	_, err := executor.Execute(context.Background(), "strict_tool", `{"path":"a.txt","extra":true}`, &ExecutionContext{})
	if err == nil {
		t.Fatal("expected argument validation error")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorInvalidArgs {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
}

func TestExecutorAllowsUnknownArgumentsWhenSchemaAllowsAdditionalProperties(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name:   "strict_tool",
		result: `{"ok":true}`,
	})
	registry.tools["strict_tool"] = ResolvedTool{
		Definition: llm.ToolDefinition{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name: "strict_tool",
				Parameters: map[string]any{
					"type":                 "object",
					"additionalProperties": true,
					"properties": map[string]any{
						"path": map[string]any{"type": "string"},
					},
				},
			},
		},
		Spec: registry.tools["strict_tool"].Spec,
		Tool: registry.tools["strict_tool"].Tool,
	}
	executor := NewExecutor(registry)

	got, err := executor.Execute(context.Background(), "strict_tool", `{"path":"a.txt","extra":true}`, &ExecutionContext{})
	if err != nil {
		t.Fatalf("expected additionalProperties=true to allow unknown fields, got %v", err)
	}
	if got != `{"ok":true}` {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestExecutorMapsPolicyFailuresToPermissionDenied(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{name: "strict_tool", result: `{"ok":true}`})
	executor := NewExecutor(registry)

	_, err := executor.Execute(context.Background(), "strict_tool", `{"path":"a.txt"}`, &ExecutionContext{
		AllowedTools: map[string]struct{}{"read_file": {}},
	})
	if err == nil {
		t.Fatal("expected permission error")
	}
	execErr, ok := AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestExecutorNormalizesToolFailure(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{name: "failing_tool", err: errors.New("command is required")})
	executor := NewExecutor(registry)

	_, err := executor.Execute(context.Background(), "failing_tool", `{"path":"a.txt"}`, &ExecutionContext{})
	if err == nil {
		t.Fatal("expected tool failure")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorInvalidArgs {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
}

func TestExecutorPreservesValidJSONOutputWhenOverLimit(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{name: "strict_tool", result: `{"content":"` + strings.Repeat("a", 70000) + `"}`})
	executor := NewExecutor(registry)

	got, err := executor.Execute(context.Background(), "strict_tool", `{"path":"a.txt"}`, &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("expected valid JSON output, got %q", got[:64])
	}
}

func TestExecutorTruncatesNonJSONOutputSafely(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{name: "strict_tool", result: strings.Repeat("a", 70000)})
	executor := NewExecutor(registry)

	got, err := executor.Execute(context.Background(), "strict_tool", `{"path":"a.txt"}`, &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "\n...[truncated]") {
		t.Fatalf("expected truncated output suffix, got %q", got[len(got)-16:])
	}
}

func TestExecutorHonorsRequestedTimeoutSeconds(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "strict_tool",
		run: func(ctx context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				t.Fatal("expected execution deadline")
			}
			remaining := time.Until(deadline)
			if remaining < 55*time.Second || remaining > 61*time.Second {
				t.Fatalf("unexpected timeout window: %s", remaining)
			}
			return `{"ok":true}`, nil
		},
	})
	executor := NewExecutor(registry)

	if _, err := executor.Execute(context.Background(), "strict_tool", `{"path":"a.txt","timeout_seconds":60}`, &ExecutionContext{}); err != nil {
		t.Fatal(err)
	}
}

func TestExecutorRequiresApprovalForDestructiveToolsByDefault(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{name: "write_file", result: `{"ok":true}`})
	executor := NewExecutor(registry)

	_, err := executor.Execute(context.Background(), "write_file", `{"path":"a.txt"}`, &ExecutionContext{
		ApprovalPolicy: "on-request",
	})
	if err == nil {
		t.Fatal("expected approval error")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
	if !strings.Contains(execErr.Error(), "approval channel is unavailable") {
		t.Fatalf("expected explicit approval channel unavailable reason, got %v", execErr)
	}
}

func TestExecutorRequiresApprovalForDestructiveToolsWhenContextMissing(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{name: "write_file", result: `{"ok":true}`})
	executor := NewExecutor(registry)

	_, err := executor.Execute(context.Background(), "write_file", `{"path":"a.txt"}`, nil)
	if err == nil {
		t.Fatal("expected approval error")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
}

func TestExecutorAllowsDestructiveToolWhenApprovalGranted(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{name: "write_file", result: `{"ok":true}`})
	executor := NewExecutor(registry)

	got, err := executor.Execute(context.Background(), "write_file", `{"path":"a.txt"}`, &ExecutionContext{
		ApprovalPolicy: "on-request",
		Approval: func(req ApprovalRequest) (ApprovalDecision, error) {
			if req.Command != "write_file" {
				t.Fatalf("unexpected approval command: %q", req.Command)
			}
			if !strings.Contains(req.Reason, "destructive tool") {
				t.Fatalf("unexpected approval reason: %q", req.Reason)
			}
			return ApprovalDecision{Disposition: ApprovalApproveOnce}, nil
		},
	})
	if err != nil {
		t.Fatalf("expected approval success, got %v", err)
	}
	if got != `{"ok":true}` {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestExecutorSkipsDestructiveApprovalWhenPolicyNever(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{name: "write_file", result: `{"ok":true}`})
	executor := NewExecutor(registry)

	got, err := executor.Execute(context.Background(), "write_file", `{"path":"a.txt"}`, &ExecutionContext{
		ApprovalPolicy: "never",
	})
	if err != nil {
		t.Fatalf("expected no approval under never policy, got %v", err)
	}
	if got != `{"ok":true}` {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestExecutorAwayModeAutoDenySkipsDestructiveApprovalPrompt(t *testing.T) {
	registry := &Registry{}
	registry.Add(executorTestTool{name: "write_file", result: `{"ok":true}`})
	executor := NewExecutor(registry)

	asked := false
	_, err := executor.Execute(context.Background(), "write_file", `{"path":"a.txt"}`, &ExecutionContext{
		ApprovalPolicy: "on-request",
		ApprovalMode:   "away",
		AwayPolicy:     "auto_deny_continue",
		Approval: func(req ApprovalRequest) (ApprovalDecision, error) {
			asked = true
			return ApprovalDecision{Disposition: ApprovalApproveOnce}, nil
		},
	})
	if err == nil {
		t.Fatal("expected away mode to deny destructive tool without prompting")
	}
	if asked {
		t.Fatal("expected away mode to skip approval prompt handler")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
	if !strings.Contains(execErr.Error(), "away mode") {
		t.Fatalf("expected away mode reason, got %v", execErr)
	}
}

func TestExecutorAwayModeFailFastStillReturnsPermissionDeniedForDestructiveTool(t *testing.T) {
	registry := &Registry{}
	registry.Add(executorTestTool{name: "write_file", result: `{"ok":true}`})
	executor := NewExecutor(registry)

	_, err := executor.Execute(context.Background(), "write_file", `{"path":"a.txt"}`, &ExecutionContext{
		ApprovalPolicy: "on-request",
		ApprovalMode:   "away",
		AwayPolicy:     "fail_fast",
	})
	if err == nil {
		t.Fatal("expected away fail_fast to deny destructive tool")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("unexpected code: %s", execErr.Code)
	}
	if !strings.Contains(execErr.Error(), "away_policy=fail_fast") {
		t.Fatalf("expected fail_fast policy in error, got %v", execErr)
	}
}
