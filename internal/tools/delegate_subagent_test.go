package tools

import (
	"context"
	"encoding/json"
	"errors"
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

func TestDelegateSubAgentToolDefinitionAndSpec(t *testing.T) {
	tool := NewDelegateSubAgentTool([]AgentInfo{
		{Name: "explorer", Description: "Read-only repo exploration"},
		{Name: "review", Description: "Review changed code"},
	})

	def := tool.Definition()
	if def.Function.Name != "delegate_subagent" {
		t.Fatalf("expected function name delegate_subagent, got %q", def.Function.Name)
	}
	desc := def.Function.Description
	for _, want := range []string{
		"Available agents:",
		"- explorer: Read-only repo exploration",
		"- review: Review changed code",
		"Write a detailed, self-contained task description",
	} {
		if !strings.Contains(desc, want) {
			t.Fatalf("expected tool definition description to contain %q, got %q", want, desc)
		}
	}
	params, ok := def.Function.Parameters["properties"].(map[string]any)
	if !ok {
		t.Fatalf("expected properties map in definition parameters, got %#v", def.Function.Parameters["properties"])
	}
	for _, key := range []string{"agent", "task", "scope", "run_in_background", "resume_session_id"} {
		if _, exists := params[key]; !exists {
			t.Fatalf("expected definition properties to contain %q", key)
		}
	}

	spec := tool.Spec()
	if spec.Name != "delegate_subagent" || spec.ReadOnly || !spec.ConcurrencySafe || spec.Destructive {
		t.Fatalf("unexpected tool spec core flags: %#v", spec)
	}
	if spec.SafetyClass != SafetyClassSensitive || !spec.StrictArgs {
		t.Fatalf("unexpected tool spec safety flags: %#v", spec)
	}
	if spec.DefaultTimeoutS != 120 || spec.MaxTimeoutS != 900 || spec.MaxResultChars != 64*1024 {
		t.Fatalf("unexpected tool spec limits: %#v", spec)
	}
}

func TestDelegateSubAgentToolRunInvalidJSONAndHandlerError(t *testing.T) {
	tool := DelegateSubAgentTool{}
	execCtx := &ExecutionContext{
		DelegateSubAgent: func(context.Context, DelegateSubAgentRequest, *ExecutionContext) (DelegateSubAgentResult, error) {
			return DelegateSubAgentResult{OK: true}, nil
		},
	}
	_, err := tool.Run(context.Background(), json.RawMessage(`{"agent"`), execCtx)
	if err == nil {
		t.Fatal("expected invalid JSON error")
	}
	if execErr, ok := AsToolExecError(err); !ok || execErr.Code != ToolErrorInvalidArgs {
		t.Fatalf("expected invalid args tool error, got %#v", err)
	}

	wantErr := errors.New("delegate failed")
	_, err = tool.Run(context.Background(), json.RawMessage(`{"agent":"explorer","task":"scan"}`), &ExecutionContext{
		DelegateSubAgent: func(context.Context, DelegateSubAgentRequest, *ExecutionContext) (DelegateSubAgentResult, error) {
			return DelegateSubAgentResult{}, wantErr
		},
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("expected handler error to be returned directly, got %v", err)
	}
}

func TestDelegateSubAgentResultModifiedFiles(t *testing.T) {
	result := DelegateSubAgentResult{
		OK:            true,
		InvocationID:  "inv-1",
		Agent:         "general",
		Summary:       "changed foo.go and bar.go",
		ModifiedFiles: []string{"foo.go", "bar.go"},
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}
	if !strings.Contains(string(data), "modified_files") {
		t.Fatal("expected modified_files in JSON output")
	}
	if !strings.Contains(string(data), "foo.go") {
		t.Fatal("expected foo.go in JSON output")
	}

	// Verify omitempty: empty ModifiedFiles should not appear in JSON
	resultEmpty := DelegateSubAgentResult{
		OK:           true,
		InvocationID: "inv-2",
		Agent:        "explorer",
		Summary:      "done",
	}
	dataEmpty, err := json.Marshal(resultEmpty)
	if err != nil {
		t.Fatalf("failed to marshal result: %v", err)
	}
	if strings.Contains(string(dataEmpty), "modified_files") {
		t.Fatal("expected modified_files to be omitted when empty")
	}
}
