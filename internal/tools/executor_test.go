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
					"path": map[string]any{"type": "string"},
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

func TestExecutorRejectsUnknownArgumentsForStrictSpecs(t *testing.T) {
	registry := &Registry{}
	registry.Add(executorTestTool{name: "strict_tool", result: `{"ok":true}`})
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

func TestExecutorTruncatesLongOutput(t *testing.T) {
	registry := &Registry{}
	registry.Add(executorTestTool{
		name:   "strict_tool",
		result: `{"ok":true,"stdout":"` + strings.Repeat("a", 70000) + `"}`,
	})
	executor := NewExecutor(registry)

	got, err := executor.Execute(context.Background(), "strict_tool", `{"path":"a.txt"}`, &ExecutionContext{})
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid([]byte(got)) {
		t.Fatalf("expected valid JSON output, got %q", got)
	}

	var payload struct {
		OK     bool   `json:"ok"`
		Stdout string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(got), &payload); err != nil {
		t.Fatal(err)
	}
	if !payload.OK {
		t.Fatalf("expected OK payload, got %s", got)
	}
	if !strings.HasSuffix(payload.Stdout, "\n...[truncated]") {
		t.Fatalf("expected truncated stdout suffix, got %q", payload.Stdout[len(payload.Stdout)-16:])
	}
}

func TestExecutorDoesNotOverrideToolManagedTimeouts(t *testing.T) {
	registry := &Registry{}
	registry.Add(executorTestTool{
		name: "strict_tool",
		run: func(ctx context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				return `{"ok":true}`, nil
			}
			return "", errors.New("unexpected deadline: " + deadline.Format(time.RFC3339))
		},
	})
	executor := NewExecutor(registry)

	if _, err := executor.Execute(context.Background(), "strict_tool", `{"path":"a.txt"}`, &ExecutionContext{}); err != nil {
		t.Fatal(err)
	}
}
