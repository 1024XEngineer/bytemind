package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
	if result.Status != subAgentResultStatusFailed {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusFailed, result.Status)
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
	if result.Status != subAgentResultStatusFailed {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusFailed, result.Status)
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
	if result.Status != subAgentResultStatusFailed {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusFailed, result.Status)
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
	if result.Status != subAgentResultStatusFailed {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusFailed, result.Status)
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
		Agent:   "explorer",
		Task:    "Locate prompt assembly order",
		Timeout: "90s",
		Output:  "findings",
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
	if result.Status != subAgentResultStatusFailed {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusFailed, result.Status)
	}
	if result.Error == nil || result.Error.Code != subAgentErrorCodeNotImplemented {
		t.Fatalf("expected not implemented code, got %#v", result.Error)
	}
	if result.TaskID != "runtime-subagent-task" {
		t.Fatalf("expected runtime task id, got %q", result.TaskID)
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
	if call.Metadata["isolation"] != "none" {
		t.Fatalf("expected isolation metadata none, got %q", call.Metadata["isolation"])
	}
	if call.Metadata["effective_tool_count"] != "2" {
		t.Fatalf("expected effective_tool_count 2, got %q", call.Metadata["effective_tool_count"])
	}
	if call.Metadata["effective_toolset_hash"] != "f2475c3f80104af2a4f1cf5eaaaabeb5a898b71747a09614703e99cee88b1f82" {
		t.Fatalf("expected effective_toolset_hash, got %q", call.Metadata["effective_toolset_hash"])
	}
	if call.Metadata["requested_timeout"] != "90s" {
		t.Fatalf("expected requested_timeout metadata, got %q", call.Metadata["requested_timeout"])
	}
	if call.Metadata["requested_output"] != "findings" {
		t.Fatalf("expected requested_output metadata, got %q", call.Metadata["requested_output"])
	}
	if call.Timeout != 90*time.Second {
		t.Fatalf("expected runtime timeout 90s, got %s", call.Timeout)
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
	if result.Status != subAgentResultStatusCompleted {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusCompleted, result.Status)
	}
	if result.TaskID != "runtime-subagent-task" {
		t.Fatalf("expected runtime task id, got %q", result.TaskID)
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

func TestDelegateSubAgentUsesCanonicalAgentNameWhenRequestUsesAlias(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerAliasSubAgentDefinition(t, workspace)

	gateway := &stubRuntimeGateway{
		result: runtimepkg.TaskResult{
			TaskID: "runtime-subagent-task",
			Status: corepkg.TaskCompleted,
			Output: []byte(`{
				"ok": true,
				"summary": "alias scan complete",
				"findings": [],
				"references": []
			}`),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "exp",
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
	if result.Agent != "explorer" {
		t.Fatalf("expected canonical agent name explorer, got %q", result.Agent)
	}
}

func TestDelegateSubAgentAppliesDefinitionDefaultTimeoutToRuntimeTask(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerTimeoutSubAgentDefinition(t, workspace)

	gateway := &stubRuntimeGateway{
		result: runtimepkg.TaskResult{
			TaskID: "runtime-subagent-task",
			Status: corepkg.TaskCompleted,
			Output: []byte(`{
				"ok": false,
				"error": {"code":"subagent_not_implemented","message":"stub pipeline placeholder","retryable":true},
				"findings": [],
				"references": []
			}`),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})

	_, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}

	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if len(gateway.calls) != 1 {
		t.Fatalf("expected exactly one runtime call, got %d", len(gateway.calls))
	}
	if gateway.calls[0].Timeout != 45*time.Second {
		t.Fatalf("expected runtime timeout 45s from subagent default, got %s", gateway.calls[0].Timeout)
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
	if result.Status != subAgentResultStatusFailed {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusFailed, result.Status)
	}
	if result.TaskID != "runtime-subagent-task" {
		t.Fatalf("expected runtime task id, got %q", result.TaskID)
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
	if result.Status != subAgentResultStatusFailed {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusFailed, result.Status)
	}
	if result.TaskID != "runtime-subagent-task" {
		t.Fatalf("expected runtime task id, got %q", result.TaskID)
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

func writeExplorerAliasSubAgentDefinition(t *testing.T, workspace string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "subagents", "explorer.md"), []byte(`---
name: explorer
description: repo explorer
aliases: [exp]
tools: [read_file, search_text]
mode: build
---
scan files
`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeExplorerTimeoutSubAgentDefinition(t *testing.T, workspace string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "subagents", "explorer.md"), []byte(`---
name: explorer
description: repo explorer
tools: [read_file, search_text]
mode: build
timeout: 45s
---
scan files
`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeDelegateSubAgentResultDerivesStatusFromOK(t *testing.T) {
	result, err := normalizeDelegateSubAgentResult(
		[]byte(`{"ok":true,"summary":"done","findings":[],"references":[]}`),
		"inv-1",
		"explorer",
		"task-1",
	)
	if err != nil {
		t.Fatalf("expected normalization success, got %v", err)
	}
	if result.Status != subAgentResultStatusCompleted {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusCompleted, result.Status)
	}

	result, err = normalizeDelegateSubAgentResult(
		[]byte(`{"ok":false,"error":{"code":"subagent_task_failed","message":"boom","retryable":true},"findings":[],"references":[]}`),
		"inv-2",
		"explorer",
		"task-2",
	)
	if err != nil {
		t.Fatalf("expected normalization success, got %v", err)
	}
	if result.Status != subAgentResultStatusFailed {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusFailed, result.Status)
	}
}

func TestNormalizeDelegateSubAgentResultRejectsUnsupportedStatus(t *testing.T) {
	_, err := normalizeDelegateSubAgentResult(
		[]byte(`{"ok":true,"status":"unknown","summary":"done","findings":[],"references":[]}`),
		"inv-1",
		"explorer",
		"task-1",
	)
	if err == nil || !strings.Contains(err.Error(), "unsupported status") {
		t.Fatalf("expected unsupported status error, got %v", err)
	}
}

func TestNormalizeDelegateSubAgentResultRejectsMismatchedOKStatus(t *testing.T) {
	_, err := normalizeDelegateSubAgentResult(
		[]byte(`{"ok":true,"status":"failed","summary":"done","findings":[],"references":[]}`),
		"inv-1",
		"explorer",
		"task-1",
	)
	if err == nil || !strings.Contains(err.Error(), "must not use failed status") {
		t.Fatalf("expected ok/status mismatch error, got %v", err)
	}

	_, err = normalizeDelegateSubAgentResult(
		[]byte(`{"ok":false,"status":"completed","error":{"code":"subagent_task_failed","message":"boom","retryable":true},"findings":[],"references":[]}`),
		"inv-1",
		"explorer",
		"task-1",
	)
	if err == nil || !strings.Contains(err.Error(), "must use status") {
		t.Fatalf("expected failed/status mismatch error, got %v", err)
	}
}

func TestNormalizeDelegateSubAgentResultAcceptsAsyncSuccessStatuses(t *testing.T) {
	for _, status := range []string{subAgentResultStatusQueued, subAgentResultStatusRunning, subAgentResultStatusAccepted} {
		result, err := normalizeDelegateSubAgentResult(
			[]byte(`{"ok":true,"status":"`+status+`","summary":"async","findings":[],"references":[]}`),
			"inv-1",
			"explorer",
			"task-1",
		)
		if err != nil {
			t.Fatalf("expected status %q accepted, got %v", status, err)
		}
		if result.Status != status {
			t.Fatalf("expected status %q, got %q", status, result.Status)
		}
	}
}

type semanticRuntimeErrorStub struct {
	code      string
	message   string
	retryable bool
}

func (e semanticRuntimeErrorStub) Error() string   { return e.message }
func (e semanticRuntimeErrorStub) Code() string    { return e.code }
func (e semanticRuntimeErrorStub) Retryable() bool { return e.retryable }

func TestMapDelegateSubAgentErrorUsesSemanticRetryable(t *testing.T) {
	mapped := mapDelegateSubAgentError(
		semanticRuntimeErrorStub{
			code:      "quota_exceeded",
			message:   "quota exceeded",
			retryable: false,
		},
		subAgentErrorCodeRuntimeUnavailable,
	)
	if mapped == nil {
		t.Fatal("expected mapped error")
	}
	if mapped.Code != "quota_exceeded" {
		t.Fatalf("expected code quota_exceeded, got %q", mapped.Code)
	}
	if mapped.Retryable {
		t.Fatalf("expected retryable=false, got %#v", mapped)
	}
}

func TestMapDelegateSubAgentErrorFallsBackToCancelledHeuristic(t *testing.T) {
	base := errors.New("cancelled")
	wrapped := fmtErrorWithCode{err: base, code: runtimepkg.ErrorCodeTaskCancelled}
	mapped := mapDelegateSubAgentError(wrapped, subAgentErrorCodeRuntimeUnavailable)
	if mapped == nil {
		t.Fatal("expected mapped error")
	}
	if mapped.Code != runtimepkg.ErrorCodeTaskCancelled {
		t.Fatalf("expected cancelled code, got %q", mapped.Code)
	}
	if mapped.Retryable {
		t.Fatalf("expected retryable=false for cancelled code, got %#v", mapped)
	}
}

func TestEffectiveToolsetHashStableCanonicalization(t *testing.T) {
	got := effectiveToolsetHash([]string{"search_text", " read_file ", "", "search_text"})
	const want = "f2475c3f80104af2a4f1cf5eaaaabeb5a898b71747a09614703e99cee88b1f82"
	if got != want {
		t.Fatalf("unexpected toolset hash: got %q want %q", got, want)
	}
}

func TestIsAllowedSubAgentStatus(t *testing.T) {
	for _, status := range []string{
		subAgentResultStatusCompleted,
		subAgentResultStatusFailed,
		subAgentResultStatusQueued,
		subAgentResultStatusRunning,
		subAgentResultStatusAccepted,
	} {
		if !isAllowedSubAgentStatus(status) {
			t.Fatalf("expected status %q to be allowed", status)
		}
	}
	if isAllowedSubAgentStatus("unknown") {
		t.Fatal("expected unknown status to be rejected")
	}
}

func TestDelegateSubAgentRuntimeTimeoutParsing(t *testing.T) {
	timeout, err := delegateSubAgentRuntimeTimeout("90s")
	if err != nil {
		t.Fatalf("expected parse success, got %v", err)
	}
	if timeout != 90*time.Second {
		t.Fatalf("expected 90s, got %s", timeout)
	}
	timeout, err = delegateSubAgentRuntimeTimeout("   ")
	if err != nil {
		t.Fatalf("expected empty timeout allowed, got %v", err)
	}
	if timeout != 0 {
		t.Fatalf("expected zero timeout for empty value, got %s", timeout)
	}
	if _, err := delegateSubAgentRuntimeTimeout("soon"); err == nil {
		t.Fatal("expected invalid timeout parse error")
	}
}

type fmtErrorWithCode struct {
	err  error
	code string
}

func (e fmtErrorWithCode) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e fmtErrorWithCode) Code() string { return e.code }
