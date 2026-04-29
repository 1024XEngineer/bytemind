package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	corepkg "bytemind/internal/core"
	planpkg "bytemind/internal/plan"
	runtimepkg "bytemind/internal/runtime"
	"bytemind/internal/session"
	"bytemind/internal/tools"
)

func TestDelegateSubAgentReturnsStructuredNotImplementedAfterPreflight(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "subagents", "explorer.md"), []byte(`---
name: explorer
description: repo explorer
tools: [read_file, search_text]
mode: build
---
scan files
`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failure placeholder result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != subAgentErrorCodeNotImplemented {
		t.Fatalf("expected not implemented code, got %#v", result.Error)
	}
	if strings.TrimSpace(result.InvocationID) == "" {
		t.Fatalf("expected invocation id, got %#v", result)
	}
	if result.Findings == nil || result.References == nil {
		t.Fatalf("expected findings/references arrays, got %#v", result)
	}
}

func TestDelegateSubAgentReturnsStructuredPreflightFailure(t *testing.T) {
	workspace := t.TempDir()
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "unknown",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failure result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != "subagent_agent_not_found" {
		t.Fatalf("expected preflight agent_not_found code, got %#v", result.Error)
	}
	if result.Findings == nil || result.References == nil {
		t.Fatalf("expected findings/references arrays, got %#v", result)
	}
}

func TestDelegateSubAgentRejectsBackgroundMode(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)

	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent:           "explorer",
		Task:            "Locate prompt assembly order",
		RunInBackground: true,
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failure result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != subAgentErrorCodeBackgroundUnsupported {
		t.Fatalf("expected background unsupported code, got %#v", result.Error)
	}
	if result.Findings == nil || result.References == nil {
		t.Fatalf("expected findings/references arrays, got %#v", result)
	}
}

func TestDelegateSubAgentReturnsRuntimeUnavailableWhenGatewayMissing(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)

	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
	})
	runner.runtime = nil

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failure result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != subAgentErrorCodeRuntimeUnavailable {
		t.Fatalf("expected runtime unavailable code, got %#v", result.Error)
	}
}

func TestDelegateSubAgentWrapsExecutionInRuntimeTask(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)

	gateway := &stubRuntimeGateway{
		result: runtimepkg.TaskResult{
			TaskID: "runtime-subagent-task",
			Status: corepkg.TaskCompleted,
			Output: []byte(`{
				"ok": false,
				"error": {"code":"subagent_not_implemented","message":"stub pipeline placeholder","retryable":true}
			}`),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})
	sess := session.New(workspace)

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode:    planpkg.ModeBuild,
		Session: sess,
	})
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failure placeholder result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != subAgentErrorCodeNotImplemented {
		t.Fatalf("expected not implemented code, got %#v", result.Error)
	}
	if !strings.Contains(result.Error.Message, "stub pipeline placeholder") {
		t.Fatalf("expected fallback message, got %#v", result.Error)
	}

	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if len(gateway.calls) != 1 {
		t.Fatalf("expected exactly one runtime call, got %d", len(gateway.calls))
	}
	call := gateway.calls[0]
	if call.Kind != "subagent" {
		t.Fatalf("expected subagent task kind, got %q", call.Kind)
	}
	if call.Name != "delegate_subagent/explorer" {
		t.Fatalf("expected runtime task name, got %q", call.Name)
	}
	if call.SessionID != corepkg.SessionID(sess.ID) {
		t.Fatalf("expected session id %q, got %q", corepkg.SessionID(sess.ID), call.SessionID)
	}
	if call.Metadata["invocation_id"] != result.InvocationID {
		t.Fatalf("expected invocation metadata %q, got %q", result.InvocationID, call.Metadata["invocation_id"])
	}
	if call.Metadata["agent"] != "explorer" {
		t.Fatalf("expected agent metadata, got %q", call.Metadata["agent"])
	}
	if call.Metadata["mode"] != string(planpkg.ModeBuild) {
		t.Fatalf("expected mode metadata, got %q", call.Metadata["mode"])
	}
}

func TestDelegateSubAgentAcceptsStructuredRuntimeOutput(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)

	gateway := &stubRuntimeGateway{
		result: runtimepkg.TaskResult{
			TaskID: "runtime-subagent-task",
			Status: corepkg.TaskCompleted,
			Output: []byte(`{
				"ok": true,
				"summary": "scoped scan complete",
				"findings": [{"title":"Prompt order","body":"default -> mode -> runtime context"}],
				"references": [{"path":"internal/agent/prompt.go","line":42,"note":"assembly entry"}]
			}`),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if !result.OK {
		t.Fatalf("expected success result, got %#v", result)
	}
	if result.Error != nil {
		t.Fatalf("expected nil error for success result, got %#v", result.Error)
	}
	if result.Agent != "explorer" {
		t.Fatalf("expected agent explorer, got %q", result.Agent)
	}
	if strings.TrimSpace(result.InvocationID) == "" {
		t.Fatalf("expected invocation id, got %#v", result)
	}
	if len(result.Findings) != 1 || len(result.References) != 1 {
		t.Fatalf("expected findings/references from runtime result, got %#v", result)
	}
}

func TestDelegateSubAgentRejectsInvalidStructuredRuntimeOutput(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)

	gateway := &stubRuntimeGateway{
		result: runtimepkg.TaskResult{
			TaskID: "runtime-subagent-task",
			Status: corepkg.TaskCompleted,
			Output: []byte(`{"ok":false}`),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failed result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != subAgentErrorCodeInvalidResult {
		t.Fatalf("expected invalid result code, got %#v", result.Error)
	}
	if result.Findings == nil || result.References == nil {
		t.Fatalf("expected findings/references arrays, got %#v", result)
	}
}

func TestDelegateSubAgentNormalizesMissingArraysInStructuredRuntimeOutput(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)

	gateway := &stubRuntimeGateway{
		result: runtimepkg.TaskResult{
			TaskID: "runtime-subagent-task",
			Status: corepkg.TaskCompleted,
			Output: []byte(`{
				"ok": false,
				"error": {"code":"subagent_task_failed","message":"worker timeout","retryable":true}
			}`),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failed result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != "subagent_task_failed" {
		t.Fatalf("expected propagated error, got %#v", result.Error)
	}
	if result.Findings == nil || result.References == nil {
		t.Fatalf("expected findings/references arrays, got %#v", result)
	}
}

func writeExplorerSubAgentDefinition(t *testing.T, workspace string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "subagents", "explorer.md"), []byte(`---
name: explorer
description: repo explorer
tools: [read_file, search_text]
mode: build
---
scan files
`), 0o644); err != nil {
		t.Fatal(err)
	}
}
