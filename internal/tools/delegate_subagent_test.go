package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestDelegateSubAgentToolRequiresHandler(t *testing.T) {
	tool := DelegateSubAgentTool{}
	_, err := tool.Run(context.Background(), json.RawMessage(`{"agent":"explorer","task":"scan"}`), &ExecutionContext{})
	if err == nil {
		t.Fatal("expected missing handler error")
	}
	execErr, ok := AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestDelegateSubAgentToolValidatesRequiredFields(t *testing.T) {
	tool := DelegateSubAgentTool{}
	execCtx := &ExecutionContext{
		DelegateSubAgent: func(context.Context, DelegateSubAgentRequest, *ExecutionContext) (DelegateSubAgentResult, error) {
			return DelegateSubAgentResult{OK: true}, nil
		},
	}
	_, err := tool.Run(context.Background(), json.RawMessage(`{"agent":"","task":"scan"}`), execCtx)
	if err == nil {
		t.Fatal("expected agent required error")
	}
	execErr, ok := AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorInvalidArgs {
		t.Fatalf("unexpected error: %#v", err)
	}
	_, err = tool.Run(context.Background(), json.RawMessage(`{"agent":"explorer","task":" "}`), execCtx)
	if err == nil {
		t.Fatal("expected task required error")
	}
	execErr, ok = AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorInvalidArgs {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestDelegateSubAgentToolCallsHandlerAndReturnsJSON(t *testing.T) {
	tool := DelegateSubAgentTool{}
	called := false
	output, err := tool.Run(context.Background(), json.RawMessage(`{
		"agent":"explorer",
		"task":"Locate prompt assembly order",
		"scope":{"paths":["internal/agent"],"symbols":["systemPrompt"]},
		"timeout":"90s",
		"output":"findings"
	}`), &ExecutionContext{
		DelegateSubAgent: func(_ context.Context, req DelegateSubAgentRequest, _ *ExecutionContext) (DelegateSubAgentResult, error) {
			called = true
			if req.Agent != "explorer" || req.Timeout != "90s" {
				t.Fatalf("unexpected request payload: %+v", req)
			}
			return DelegateSubAgentResult{
				OK:           true,
				InvocationID: "subagent-1",
				Agent:        req.Agent,
				Summary:      "done",
			}, nil
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected delegate handler to be called")
	}
	if !strings.Contains(output, `"ok":true`) || !strings.Contains(output, `"invocation_id":"subagent-1"`) {
		t.Fatalf("unexpected output: %s", output)
	}
}
