package tools

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"bytemind/internal/llm"
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

func TestExecutorAllowsUnknownArgumentsUnlessSchemaForbidsThem(t *testing.T) {
	registry := &Registry{}
	registry.Add(executorTestTool{name: "strict_tool", result: `{"ok":true}`})
	executor := NewExecutor(registry)

	got, err := executor.Execute(context.Background(), "strict_tool", `{"path":"a.txt","extra":true}`, &ExecutionContext{})
	if err != nil {
		t.Fatalf("expected extra field to be ignored, got %v", err)
	}
	if got != `{"ok":true}` {
		t.Fatalf("unexpected result: %q", got)
	}
}

func TestExecutorRejectsUnknownArgumentsWhenSchemaForbidsThem(t *testing.T) {
	registry := &Registry{}
	registry.Add(executorTestTool{
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

func TestExecutorMapsPolicyFailuresToPermissionDenied(t *testing.T) {
	registry := &Registry{}
	registry.Add(executorTestTool{name: "strict_tool", result: `{"ok":true}`})
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
	registry.Add(executorTestTool{name: "failing_tool", err: errors.New("command is required")})
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
	registry.Add(executorTestTool{name: "strict_tool", result: `{"content":"` + strings.Repeat("a", 70000) + `"}`})
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
	registry.Add(executorTestTool{name: "strict_tool", result: strings.Repeat("a", 70000)})
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
	registry.Add(executorTestTool{
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
