package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/1024XEngineer/bytemind/internal/config"
	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	runtimepkg "github.com/1024XEngineer/bytemind/internal/runtime"
	"github.com/1024XEngineer/bytemind/internal/session"
	subagentspkg "github.com/1024XEngineer/bytemind/internal/subagents"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

func TestDelegateSubAgentReturnsRuntimeUnavailableWhenClientMissing(t *testing.T) {
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
	}, nil, "")
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if !result.OK {
		t.Fatalf("expected OK:true for error-as-content, got %#v", result)
	}
	if !strings.Contains(result.Summary, "llm client is unavailable") {
		t.Fatalf("expected error message in summary, got %q", result.Summary)
	}
	if strings.TrimSpace(result.InvocationID) == "" {
		t.Fatalf("expected invocation id, got %#v", result)
	}
}

func TestDelegateSubAgentExecutesWithTemporaryChildSession(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	client := &fakeClient{replies: []llm.Message{
		{
			Role:    llm.RoleAssistant,
			Content: "scoped scan complete",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
		},
		Client:   client,
		Registry: tools.DefaultRegistry(),
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	}, nil, "")
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if !result.OK {
		t.Fatalf("expected success result, got %#v", result)
	}
	if result.Status != subAgentResultStatusCompleted {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusCompleted, result.Status)
	}
	if strings.TrimSpace(result.Summary) == "" {
		t.Fatalf("expected non-empty summary, got %#v", result)
	}
	if len(client.requests) == 0 {
		t.Fatal("expected child session to issue at least one llm request")
	}
	if len(client.requests[0].Messages) == 0 {
		t.Fatal("expected child request to include system prompt")
	}
	systemPrompt := client.requests[0].Messages[0].Content
	if !strings.Contains(systemPrompt, "[SubAgent Runtime]") {
		t.Fatalf("expected child system prompt to include subagent runtime block, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "name: explorer") {
		t.Fatalf("expected child system prompt to include subagent name, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "task: Locate prompt assembly order") {
		t.Fatalf("expected child system prompt to include delegated task, got %q", systemPrompt)
	}
	if !strings.Contains(systemPrompt, "[SubAgent Definition]") {
		t.Fatalf("expected child system prompt to include subagent definition block, got %q", systemPrompt)
	}
	if strings.Contains(systemPrompt, "invocation_id:") || strings.Contains(systemPrompt, "parent_session_id:") {
		t.Fatalf("did not expect invocation/session ids in child system prompt, got %q", systemPrompt)
	}
}

func TestDelegateSubAgentChildSessionUsesNarrowedTools(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	client := &fakeClient{replies: []llm.Message{
		{
			Role:    llm.RoleAssistant,
			Content: "done",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
		},
		Client:   client,
		Registry: tools.DefaultRegistry(),
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	}, nil, "")
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if !result.OK {
		t.Fatalf("expected success result, got %#v", result)
	}
	if len(client.requests) == 0 {
		t.Fatal("expected llm request from child session")
	}

	toolNames := make([]string, 0, len(client.requests[0].Tools))
	for _, tool := range client.requests[0].Tools {
		name := strings.TrimSpace(tool.Function.Name)
		if name != "" {
			toolNames = append(toolNames, name)
		}
	}
	slices.Sort(toolNames)
	if strings.Contains(strings.Join(toolNames, ","), "delegate_subagent") {
		t.Fatalf("expected delegate_subagent to be removed from child toolset, got %v", toolNames)
	}
	if !slices.Contains(toolNames, "read_file") || !slices.Contains(toolNames, "search_text") {
		t.Fatalf("expected narrowed read tools in child toolset, got %v", toolNames)
	}
	if slices.Contains(toolNames, "write_file") {
		t.Fatalf("expected write_file excluded from child toolset, got %v", toolNames)
	}
}

func TestDelegateSubAgentChildSessionDoesNotPersistTemporarySession(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	parent := session.New(workspace)
	parent.Messages = append(parent.Messages, llm.NewUserTextMessage("parent task"))
	if err := store.Save(parent); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{replies: []llm.Message{
		{
			Role:    llm.RoleAssistant,
			Content: "child analysis complete",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
	})

	beforeCount := len(parent.Messages)
	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode:    planpkg.ModeBuild,
		Session: parent,
	}, nil, "")
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if !result.OK {
		t.Fatalf("expected success result, got %#v", result)
	}
	if got := len(parent.Messages); got != beforeCount {
		t.Fatalf("expected parent in-memory messages unchanged (%d), got %d", beforeCount, got)
	}

	summaries, _, err := store.List(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected two persisted sessions (parent + child), got %d", len(summaries))
	}
	foundParent := false
	for _, s := range summaries {
		if s.ID == parent.ID {
			foundParent = true
		}
	}
	if !foundParent {
		t.Fatalf("expected parent session %q in persisted list", parent.ID)
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
	}, nil, "")
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
}

func TestDelegateSubAgentLaunchesBackgroundTaskWhenLifecycleToolsAvailable(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)

	gateway := &stubRuntimeGateway{
		result: runtimepkg.TaskResult{
			TaskID: "runtime-subagent-task",
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Runtime:   gateway,
		Registry:  tools.DefaultRegistry(),
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent:           "explorer",
		Task:            "Locate prompt assembly order",
		RunInBackground: true,
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	}, nil, "")
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if !result.OK {
		t.Fatalf("expected success result, got %#v", result)
	}
	if result.Status != subAgentResultStatusAccepted {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusAccepted, result.Status)
	}
	if result.TaskID != "runtime-subagent-task" {
		t.Fatalf("expected runtime task id, got %q", result.TaskID)
	}
	if result.ResultReadTool != subAgentTaskOutputTool {
		t.Fatalf("expected result_read_tool %q, got %q", subAgentTaskOutputTool, result.ResultReadTool)
	}
	if result.StopTool != subAgentTaskStopTool {
		t.Fatalf("expected stop_tool %q, got %q", subAgentTaskStopTool, result.StopTool)
	}
	if result.Error != nil {
		t.Fatalf("expected nil error, got %#v", result.Error)
	}

	gateway.mu.Lock()
	defer gateway.mu.Unlock()
	if len(gateway.asyncCalls) != 1 {
		t.Fatalf("expected exactly one async runtime call, got %d", len(gateway.asyncCalls))
	}
	if !gateway.asyncCalls[0].Background {
		t.Fatalf("expected runtime task background flag true, got %#v", gateway.asyncCalls[0])
	}
}

func TestDelegateSubAgentRejectsBackgroundWhenLifecycleToolsUnavailable(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)

	registry := &tools.Registry{}
	registry.Add(tools.ListFilesTool{})
	registry.Add(tools.ReadFileTool{})
	registry.Add(tools.SearchTextTool{})
	registry.Add(tools.DelegateSubAgentTool{})

	runner := NewRunner(Options{
		Workspace: workspace,
		Runtime:   &stubRuntimeGateway{},
		Registry:  registry,
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent:           "explorer",
		Task:            "Locate prompt assembly order",
		RunInBackground: true,
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	}, nil, "")
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failure result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != subAgentErrorCodeBackgroundUnavailable {
		t.Fatalf("expected background unavailable code, got %#v", result.Error)
	}
}

func TestDelegateSubAgentRejectsBackgroundWriteToolSubAgent(t *testing.T) {
	workspace := t.TempDir()
	writeWriterSubAgentDefinition(t, workspace)

	runner := NewRunner(Options{
		Workspace: workspace,
		Runtime:   &stubRuntimeGateway{},
		Registry:  tools.DefaultRegistry(),
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent:           "writer",
		Task:            "Modify a file",
		RunInBackground: true,
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	}, nil, "")
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failure result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != subAgentErrorCodeBackgroundWriteDenied {
		t.Fatalf("expected background write denied code, got %#v", result.Error)
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
	}, nil, "")
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

func TestDelegateSubAgentMapsKilledRuntimeResultWithoutErrorCode(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)

	gateway := &stubRuntimeGateway{
		result: runtimepkg.TaskResult{
			TaskID: "runtime-subagent-task",
			Status: corepkg.TaskKilled,
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
	}, nil, "")
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failed result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != runtimepkg.ErrorCodeTaskCancelled {
		t.Fatalf("expected cancelled code, got %#v", result.Error)
	}
	if result.Error.Retryable {
		t.Fatalf("expected retryable=false for cancelled status, got %#v", result.Error)
	}
}

func TestDelegateSubAgentMapsFailedRuntimeResultWithoutErrorCode(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)

	gateway := &stubRuntimeGateway{
		result: runtimepkg.TaskResult{
			TaskID: "runtime-subagent-task",
			Status: corepkg.TaskFailed,
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
	}, nil, "")
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failed result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != runtimepkg.ErrorCodeTaskExecutionFailed {
		t.Fatalf("expected task execution failed code, got %#v", result.Error)
	}
	if !result.Error.Retryable {
		t.Fatalf("expected retryable=true for failed status, got %#v", result.Error)
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
		Output:  "summary",
	}, &tools.ExecutionContext{
		Mode:    planpkg.ModeBuild,
		Session: sess,
	}, nil, "")
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
	if call.Metadata["effective_tools"] != "read_file,search_text" {
		t.Fatalf("expected effective_tools list, got %q", call.Metadata["effective_tools"])
	}
	if call.Metadata["requested_timeout"] != "1m30s" {
		t.Fatalf("expected requested_timeout metadata, got %q", call.Metadata["requested_timeout"])
	}
	if call.Metadata["requested_timeout_ms"] != "90000" {
		t.Fatalf("expected requested_timeout_ms metadata, got %q", call.Metadata["requested_timeout_ms"])
	}
	if call.Metadata["requested_output"] != "summary" {
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
				"summary": "scoped scan complete"
			}`),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent:  "explorer",
		Task:   "Locate prompt assembly order",
		Output: "summary",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	}, nil, "")
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
	if strings.TrimSpace(result.Summary) == "" {
		t.Fatalf("expected non-empty summary, got %#v", result)
	}
}

func TestDelegateSubAgentRejectsSummaryOutputWithoutSummary(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)

	gateway := &stubRuntimeGateway{
		result: runtimepkg.TaskResult{
			TaskID: "runtime-subagent-task",
			Status: corepkg.TaskCompleted,
			Output: []byte(`{
				"ok": true,
				"summary": " "
			}`),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent:  "explorer",
		Task:   "Locate prompt assembly order",
		Output: "summary",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	}, nil, "")
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if !result.OK {
		t.Fatalf("expected success with raw fallback, got %#v", result)
	}
	if result.Summary == "" {
		t.Fatal("expected non-empty summary from raw output fallback")
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
				"summary": "alias scan complete"
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
	}, nil, "")
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
				"error": {"code":"subagent_not_implemented","message":"stub pipeline placeholder","retryable":true}
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
	}, nil, "")
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

func TestDelegateSubAgentRejectsInvalidRequestedTimeoutFromPreflight(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)

	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent:   "explorer",
		Task:    "Locate prompt assembly order",
		Timeout: "soon",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	}, nil, "")
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failed result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != subagentspkg.ErrorCodeSubAgentInvalidRequest {
		t.Fatalf("expected invalid request code, got %#v", result.Error)
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
	}, nil, "")
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
	if result.Error == nil || result.Error.Code != subAgentErrorCodeExecutionFailed {
		t.Fatalf("expected execution_failed code for ok:false without error, got %#v", result.Error)
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
	}, nil, "")
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

func writeWriterSubAgentDefinition(t *testing.T, workspace string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "subagents", "writer.md"), []byte(`---
name: writer
description: repo writer
tools: [read_file, write_file]
mode: build
---
edit files
`), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeDelegateSubAgentResultDerivesStatusFromOK(t *testing.T) {
	result, err := normalizeDelegateSubAgentResult(
		[]byte(`{"ok":true,"summary":"done"}`),
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
		[]byte(`{"ok":false,"error":{"code":"subagent_task_failed","message":"boom","retryable":true}}`),
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
	if result.Error == nil || result.Error.Code != "subagent_task_failed" {
		t.Fatalf("expected normalized error code subagent_task_failed, got %#v", result.Error)
	}
}

func TestNormalizeDelegateSubAgentResultAcceptsAnyStatus(t *testing.T) {
	result, err := normalizeDelegateSubAgentResult(
		[]byte(`{"ok":true,"status":"unknown","summary":"done"}`),
		"inv-1",
		"explorer",
		"task-1",
	)
	if err != nil {
		t.Fatalf("expected normalization success, got %v", err)
	}
	if result.Status != "unknown" {
		t.Fatalf("expected status %q, got %q", "unknown", result.Status)
	}
}

func TestNormalizeDelegateSubAgentResultReconcilesOKStatusMismatch(t *testing.T) {
	// OK=true with status=failed → reconcile to OK=false, status=failed
	result, err := normalizeDelegateSubAgentResult(
		[]byte(`{"ok":true,"status":"failed","error":{"code":"x","message":"y"}}`),
		"inv-1",
		"explorer",
		"task-1",
	)
	if err != nil {
		t.Fatalf("expected normalization success, got %v", err)
	}
	if result.OK {
		t.Fatal("expected OK=false after reconciliation")
	}
	if result.Status != subAgentResultStatusFailed {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusFailed, result.Status)
	}

	// OK=false with status=completed → accepted as-is
	result, err = normalizeDelegateSubAgentResult(
		[]byte(`{"ok":false,"status":"completed","error":{"code":"subagent_task_failed","message":"boom"}}`),
		"inv-1",
		"explorer",
		"task-1",
	)
	if err != nil {
		t.Fatalf("expected normalization success, got %v", err)
	}
	if result.OK {
		t.Fatal("expected OK=false")
	}
}

func TestNormalizeDelegateSubAgentResultNormalizesErrorCodeCase(t *testing.T) {
	result, err := normalizeDelegateSubAgentResult(
		[]byte(`{"ok":false,"status":"FAILED","error":{"code":"  SUBAGENT_TASK_FAILED  ","message":" boom ","retryable":true}}`),
		"inv-1",
		"explorer",
		"task-1",
	)
	if err != nil {
		t.Fatalf("expected normalization success, got %v", err)
	}
	if result.Status != subAgentResultStatusFailed {
		t.Fatalf("expected normalized status failed, got %q", result.Status)
	}
	if result.Error == nil || result.Error.Code != "subagent_task_failed" {
		t.Fatalf("expected normalized lower-case error code, got %#v", result.Error)
	}
	if result.Error.Message != "boom" {
		t.Fatalf("expected trimmed error message boom, got %#v", result.Error)
	}
}

func TestNormalizeDelegateSubAgentResultAcceptsAsyncSuccessStatuses(t *testing.T) {
	for _, status := range []string{subAgentResultStatusQueued, subAgentResultStatusRunning, subAgentResultStatusAccepted} {
		result, err := normalizeDelegateSubAgentResult(
			[]byte(`{"ok":true,"status":"`+status+`","task_id":"task-1","summary":"async"}`),
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

func TestNormalizeDelegateSubAgentResultTrimsSummary(t *testing.T) {
	result, err := normalizeDelegateSubAgentResult(
		[]byte(`{
			"ok": true,
			"status": "completed",
			"summary": "  done  "
		}`),
		"inv-1",
		"explorer",
		"task-1",
	)
	if err != nil {
		t.Fatalf("expected normalization success, got %v", err)
	}
	if result.Summary != "done" {
		t.Fatalf("expected trimmed summary, got %q", result.Summary)
	}
}

func TestNormalizeDelegateSubAgentResultAcceptsAsyncStatusWithoutTaskID(t *testing.T) {
	for _, status := range []string{subAgentResultStatusQueued, subAgentResultStatusRunning, subAgentResultStatusAccepted} {
		result, err := normalizeDelegateSubAgentResult(
			[]byte(`{"ok":true,"status":"`+status+`","summary":"async"}`),
			"inv-1",
			"explorer",
			"",
		)
		if err != nil {
			t.Fatalf("expected status %q accepted without task_id, got %v", status, err)
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

func TestRequiresTaskIDForStatus(t *testing.T) {
	for _, status := range []string{subAgentResultStatusQueued, subAgentResultStatusRunning, subAgentResultStatusAccepted} {
		if !requiresTaskIDForStatus(status) {
			t.Fatalf("expected status %q to require task_id", status)
		}
	}
	for _, status := range []string{subAgentResultStatusCompleted, subAgentResultStatusFailed, ""} {
		if requiresTaskIDForStatus(status) {
			t.Fatalf("expected status %q not to require task_id", status)
		}
	}
}

func TestValidateDelegateSubAgentOutputContract(t *testing.T) {
	okSummary := tools.DelegateSubAgentResult{
		OK:      true,
		Status:  subAgentResultStatusCompleted,
		Summary: "done",
	}
	if err := validateDelegateSubAgentOutputContract(okSummary, subAgentRequestedOutputSummary); err != nil {
		t.Fatalf("expected summary contract pass, got %v", err)
	}

	empty := tools.DelegateSubAgentResult{
		OK:     true,
		Status: subAgentResultStatusCompleted,
	}
	if err := validateDelegateSubAgentOutputContract(empty, subAgentRequestedOutputSummary); err == nil {
		t.Fatal("expected summary contract error")
	}

	queued := tools.DelegateSubAgentResult{
		OK:     true,
		Status: subAgentResultStatusQueued,
	}
	if err := validateDelegateSubAgentOutputContract(queued, subAgentRequestedOutputSummary); err != nil {
		t.Fatalf("expected queued status to skip summary contract, got %v", err)
	}

	if err := validateDelegateSubAgentOutputContract(okSummary, "json"); err == nil {
		t.Fatal("expected unsupported requested output error")
	}
}

func TestMapSubAgentTerminalResultDefaults(t *testing.T) {
	code, retryable := mapSubAgentTerminalResult(corepkg.TaskKilled, "")
	if code != runtimepkg.ErrorCodeTaskCancelled || retryable {
		t.Fatalf("unexpected killed mapping: code=%q retryable=%v", code, retryable)
	}
	code, retryable = mapSubAgentTerminalResult(corepkg.TaskFailed, "")
	if code != runtimepkg.ErrorCodeTaskExecutionFailed || !retryable {
		t.Fatalf("unexpected failed mapping: code=%q retryable=%v", code, retryable)
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

func TestPreflightResultName(t *testing.T) {
	if got := preflightResultName("explorer"); got != "explorer" {
		t.Fatalf("expected explorer, got %q", got)
	}
	if got := preflightResultName("  "); got != "unknown" {
		t.Fatalf("expected unknown for blank, got %q", got)
	}
	if got := preflightResultName(""); got != "unknown" {
		t.Fatalf("expected unknown for empty, got %q", got)
	}
}

func TestSessionIDFromExecutionContext(t *testing.T) {
	if got := sessionIDFromExecutionContext(nil); got != "" {
		t.Fatalf("expected empty for nil execCtx, got %q", got)
	}
	if got := sessionIDFromExecutionContext(&tools.ExecutionContext{}); got != "" {
		t.Fatalf("expected empty for nil session, got %q", got)
	}
	sess := session.New("/ws")
	got := sessionIDFromExecutionContext(&tools.ExecutionContext{Session: sess})
	if got == "" {
		t.Fatal("expected non-empty session id")
	}
}

func TestMapDelegateSubAgentErrorNil(t *testing.T) {
	if got := mapDelegateSubAgentError(nil, "fallback"); got != nil {
		t.Fatalf("expected nil for nil error, got %v", got)
	}
}

func TestMapDelegateSubAgentErrorPlainError(t *testing.T) {
	mapped := mapDelegateSubAgentError(errors.New("something broke"), "fallback_code")
	if mapped == nil {
		t.Fatal("expected mapped error")
	}
	if mapped.Code != "fallback_code" {
		t.Fatalf("expected fallback code, got %q", mapped.Code)
	}
	if !mapped.Retryable {
		t.Fatal("expected retryable true for plain error")
	}
}

func TestMapDelegateSubAgentErrorExecutionError(t *testing.T) {
	execErr := &subAgentExecutionError{code: "custom_code", message: "exec failed", retryable: false}
	mapped := mapDelegateSubAgentError(execErr, "fallback")
	if mapped == nil {
		t.Fatal("expected mapped error")
	}
	if mapped.Code != "custom_code" {
		t.Fatalf("expected custom code, got %q", mapped.Code)
	}
	if mapped.Retryable {
		t.Fatal("expected retryable false")
	}
}

func TestMapDelegateSubAgentErrorExecutionErrorEmptyCode(t *testing.T) {
	execErr := &subAgentExecutionError{code: "", message: "exec failed", retryable: true}
	mapped := mapDelegateSubAgentError(execErr, "fallback")
	if mapped.Code != "fallback" {
		t.Fatalf("expected fallback code for empty execution error code, got %q", mapped.Code)
	}
}

func TestCloneToolSet(t *testing.T) {
	if got := cloneToolSet(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
	if got := cloneToolSet(map[string]struct{}{}); got != nil {
		t.Fatalf("expected nil for empty input, got %v", got)
	}
	src := map[string]struct{}{"a": {}, "b": {}}
	cloned := cloneToolSet(src)
	if len(cloned) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(cloned))
	}
	// Mutating source should not affect clone
	src["c"] = struct{}{}
	if len(cloned) != 2 {
		t.Fatal("clone was affected by source mutation")
	}
}

func TestIsToolVisibleInParent(t *testing.T) {
	visible := []string{"read_file", "write_file", "task_output", "task_stop"}

	if isToolVisibleInParent("", visible, nil, nil) {
		t.Fatal("expected empty name to be invisible")
	}
	if isToolVisibleInParent("nonexistent", visible, nil, nil) {
		t.Fatal("expected nonexistent tool to be invisible")
	}
	if !isToolVisibleInParent("read_file", visible, nil, nil) {
		t.Fatal("expected read_file to be visible")
	}

	// With allowed list
	allowed := map[string]struct{}{"read_file": {}}
	if !isToolVisibleInParent("read_file", visible, allowed, nil) {
		t.Fatal("expected read_file in allowed list")
	}
	if isToolVisibleInParent("write_file", visible, allowed, nil) {
		t.Fatal("expected write_file not in allowed list")
	}

	// With denied list
	denied := map[string]struct{}{"read_file": {}}
	if isToolVisibleInParent("read_file", visible, nil, denied) {
		t.Fatal("expected read_file to be denied")
	}
}

func TestIsReadOnlySubAgentToolset(t *testing.T) {
	runner := NewRunner(Options{
		Workspace: t.TempDir(),
		Registry:  tools.DefaultRegistry(),
	})

	if runner.isReadOnlySubAgentToolset(nil) {
		t.Fatal("expected false for nil toolset")
	}
	if runner.isReadOnlySubAgentToolset([]string{}) {
		t.Fatal("expected false for empty toolset")
	}
	// read_file is read-only
	if !runner.isReadOnlySubAgentToolset([]string{"read_file", "search_text"}) {
		t.Fatal("expected read-only tools to return true")
	}
	// write_file is not read-only
	if runner.isReadOnlySubAgentToolset([]string{"read_file", "write_file"}) {
		t.Fatal("expected mixed toolset to return false")
	}
}

func TestNewSubAgentInvocationID(t *testing.T) {
	id1 := newSubAgentInvocationID()
	id2 := newSubAgentInvocationID()
	if id1 == "" || id2 == "" {
		t.Fatal("expected non-empty invocation ids")
	}
	if id1 == id2 {
		t.Fatal("expected unique invocation ids")
	}
	if !strings.HasPrefix(id1, "subagent-") {
		t.Fatalf("expected subagent- prefix, got %q", id1)
	}
}

func TestMapSubAgentTerminalResultWithErrorCode(t *testing.T) {
	code, retryable := mapSubAgentTerminalResult(corepkg.TaskKilled, "custom_code")
	if code != "custom_code" {
		t.Fatalf("expected custom code, got %q", code)
	}
	if retryable {
		t.Fatal("expected retryable false for killed with custom code")
	}

	code, retryable = mapSubAgentTerminalResult(corepkg.TaskFailed, "custom_code")
	if code != "custom_code" {
		t.Fatalf("expected custom code, got %q", code)
	}
	if !retryable {
		t.Fatal("expected retryable true for failed")
	}
}

// runtimeErrorWithEmptyCode implements Code() string returning empty.
type runtimeErrorWithEmptyCode struct{ msg string }

func (e runtimeErrorWithEmptyCode) Error() string { return e.msg }
func (e runtimeErrorWithEmptyCode) Code() string   { return "" }

func TestMapDelegateSubAgentErrorRuntimeErrorEmptyCode(t *testing.T) {
	mapped := mapDelegateSubAgentError(
		runtimeErrorWithEmptyCode{msg: "runtime exploded"},
		"fallback_code",
	)
	if mapped == nil {
		t.Fatal("expected mapped error")
	}
	if mapped.Code != "fallback_code" {
		t.Fatalf("expected fallback code for empty Code(), got %q", mapped.Code)
	}
	if !mapped.Retryable {
		t.Fatal("expected retryable true for runtime error with empty code")
	}
}

func TestMapSubAgentTerminalResultDefaultStatus(t *testing.T) {
	code, retryable := mapSubAgentTerminalResult(corepkg.TaskStatus("unknown_status"), "")
	if code != subAgentErrorCodeRuntimeUnavailable {
		t.Fatalf("expected runtime unavailable code, got %q", code)
	}
	if !retryable {
		t.Fatal("expected retryable true for unknown status")
	}
}

func TestNormalizeDelegateSubAgentResultOKWithErrorReconciles(t *testing.T) {
	result, err := normalizeDelegateSubAgentResult(
		[]byte(`{"ok":true,"error":{"code":"x","message":"y"}}`),
		"inv-1", "explorer", "task-1",
	)
	if err != nil {
		t.Fatalf("expected normalization success, got %v", err)
	}
	if result.OK {
		t.Fatal("expected OK=false after reconciliation")
	}
	if result.Error == nil || result.Error.Code != "x" {
		t.Fatalf("expected error code x, got %#v", result.Error)
	}
}

func TestNormalizeDelegateSubAgentResultFailedEmptyErrorCodeFillsDefaults(t *testing.T) {
	result, err := normalizeDelegateSubAgentResult(
		[]byte(`{"ok":false,"error":{"code":"","message":"boom"}}`),
		"inv-1", "explorer", "task-1",
	)
	if err != nil {
		t.Fatalf("expected normalization success, got %v", err)
	}
	if result.Error == nil || result.Error.Code != subAgentErrorCodeExecutionFailed {
		t.Fatalf("expected default error code, got %#v", result.Error)
	}

	result, err = normalizeDelegateSubAgentResult(
		[]byte(`{"ok":false,"error":{"code":"x","message":"  "}}`),
		"inv-1", "explorer", "task-1",
	)
	if err != nil {
		t.Fatalf("expected normalization success, got %v", err)
	}
	if result.Error == nil || result.Error.Message != "subagent execution failed" {
		t.Fatalf("expected default error message, got %#v", result.Error)
	}
}

func TestNormalizeDelegateSubAgentResultFailedNonFailedStatusAccepted(t *testing.T) {
	result, err := normalizeDelegateSubAgentResult(
		[]byte(`{"ok":false,"status":"completed","error":{"code":"x","message":"y"}}`),
		"inv-1", "explorer", "task-1",
	)
	if err != nil {
		t.Fatalf("expected normalization success, got %v", err)
	}
	if result.OK {
		t.Fatal("expected OK=false")
	}
	if result.Status != "completed" {
		t.Fatalf("expected status %q, got %q", "completed", result.Status)
	}
}

func TestEffectiveToolsetHashAllEmpty(t *testing.T) {
	got := effectiveToolsetHash([]string{"", "  ", ""})
	if got != "" {
		t.Fatalf("expected empty hash for all-empty entries, got %q", got)
	}
}

func TestEffectiveToolsetHashDedupToEmpty(t *testing.T) {
	got := effectiveToolsetHash([]string{"  ", "  "})
	if got != "" {
		t.Fatalf("expected empty hash for whitespace-only entries, got %q", got)
	}
}

func TestExecCtxGetAllowed(t *testing.T) {
	if got := execCtxGetAllowed(nil); got != nil {
		t.Fatalf("expected nil for nil execCtx, got %v", got)
	}
	if got := execCtxGetAllowed(&tools.ExecutionContext{}); got != nil {
		t.Fatalf("expected nil for nil AllowedTools, got %v", got)
	}
	allowed := map[string]struct{}{"read_file": {}}
	got := execCtxGetAllowed(&tools.ExecutionContext{AllowedTools: allowed})
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if _, ok := got["read_file"]; !ok {
		t.Fatal("expected read_file in allowed")
	}
}

func TestExecCtxGetDenied(t *testing.T) {
	if got := execCtxGetDenied(nil); got != nil {
		t.Fatalf("expected nil for nil execCtx, got %v", got)
	}
	if got := execCtxGetDenied(&tools.ExecutionContext{}); got != nil {
		t.Fatalf("expected nil for nil DeniedTools, got %v", got)
	}
	denied := map[string]struct{}{"write_file": {}}
	got := execCtxGetDenied(&tools.ExecutionContext{DeniedTools: denied})
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if _, ok := got["write_file"]; !ok {
		t.Fatal("expected write_file in denied")
	}
}

// flexibleRuntimeGateway allows per-test control of RunSync/RunAsync behavior.
type flexibleRuntimeGateway struct {
	mu          sync.Mutex
	syncResult  RuntimeTaskExecution
	syncErr     error
	asyncResult RuntimeTaskLaunch
	asyncErr    error
	asyncCalls  []RuntimeTaskRequest
	syncCalls   []RuntimeTaskRequest
}

func (g *flexibleRuntimeGateway) RunSync(_ context.Context, req RuntimeTaskRequest) (RuntimeTaskExecution, error) {
	g.mu.Lock()
	g.syncCalls = append(g.syncCalls, req)
	g.mu.Unlock()
	return g.syncResult, g.syncErr
}

func (g *flexibleRuntimeGateway) RunAsync(_ context.Context, req RuntimeTaskRequest) (RuntimeTaskLaunch, error) {
	g.mu.Lock()
	g.asyncCalls = append(g.asyncCalls, req)
	g.mu.Unlock()
	return g.asyncResult, g.asyncErr
}

func TestDelegateSubAgentRunSyncError(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	gateway := &flexibleRuntimeGateway{
		syncResult: RuntimeTaskExecution{TaskID: "task-1"},
		syncErr:    errors.New("runtime sync failure"),
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})
	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "test task",
	}, &tools.ExecutionContext{Mode: planpkg.ModeBuild}, nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.OK {
		t.Fatal("expected failure result")
	}
	if result.TaskID != "task-1" {
		t.Fatalf("expected task id from execution, got %q", result.TaskID)
	}
	if result.Error == nil {
		t.Fatal("expected error")
	}
}

func TestDelegateSubAgentRunSyncDeadlineExceededWithSettledCompletedResult(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	gateway := &flexibleRuntimeGateway{
		syncResult: RuntimeTaskExecution{
			TaskID: "task-1",
			Result: runtimepkg.TaskResult{
				TaskID: "task-1",
				Status: corepkg.TaskCompleted,
				Output: []byte(`{"ok":true,"status":"completed","summary":"scan complete","content":"scan complete"}`),
			},
		},
		syncErr: context.DeadlineExceeded,
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})
	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "test task",
	}, &tools.ExecutionContext{Mode: planpkg.ModeBuild}, nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.OK {
		t.Fatalf("expected success result, got %#v", result)
	}
	if result.Error != nil {
		t.Fatalf("expected nil error, got %#v", result.Error)
	}
	if result.Status != subAgentResultStatusCompleted {
		t.Fatalf("expected status %q, got %q", subAgentResultStatusCompleted, result.Status)
	}
	if result.Summary != "scan complete" {
		t.Fatalf("expected summary to be preserved, got %q", result.Summary)
	}
	if result.TaskID != "task-1" {
		t.Fatalf("expected task id from execution, got %q", result.TaskID)
	}
}

func TestDelegateSubAgentRunSyncEmptyOutput(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	gateway := &flexibleRuntimeGateway{
		syncResult: RuntimeTaskExecution{
			TaskID: "task-1",
			Result: runtimepkg.TaskResult{
				TaskID: "task-1",
				Status: corepkg.TaskCompleted,
				Output: []byte{},
			},
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})
	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "test task",
	}, &tools.ExecutionContext{Mode: planpkg.ModeBuild}, nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.OK {
		t.Fatal("expected success with fallback summary for empty output")
	}
	if result.Summary != "SubAgent task completed." {
		t.Fatalf("expected fallback summary, got %q", result.Summary)
	}
}

func TestDelegateSubAgentRunSyncInvalidJSONOutput(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	gateway := &flexibleRuntimeGateway{
		syncResult: RuntimeTaskExecution{
			TaskID: "task-1",
			Result: runtimepkg.TaskResult{
				TaskID: "task-1",
				Status: corepkg.TaskCompleted,
				Output: []byte(`{invalid json`),
			},
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})
	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "test task",
	}, &tools.ExecutionContext{Mode: planpkg.ModeBuild}, nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.OK {
		t.Fatal("expected success with raw text fallback for invalid JSON")
	}
	if result.Summary != "{invalid json" {
		t.Fatalf("expected raw text as summary, got %q", result.Summary)
	}
}

func TestDelegateSubAgentRunAsyncError(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	gateway := &flexibleRuntimeGateway{
		asyncErr: errors.New("async runtime failure"),
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})
	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent:           "explorer",
		Task:            "test task",
		RunInBackground: true,
	}, &tools.ExecutionContext{Mode: planpkg.ModeBuild}, nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.OK {
		t.Fatal("expected failure for async error")
	}
	if result.Error == nil {
		t.Fatal("expected error")
	}
}

func TestDelegateSubAgentRunAsyncEmptyTaskID(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	gateway := &flexibleRuntimeGateway{
		asyncResult: RuntimeTaskLaunch{TaskID: ""},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})
	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent:           "explorer",
		Task:            "test task",
		RunInBackground: true,
	}, &tools.ExecutionContext{Mode: planpkg.ModeBuild}, nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.OK {
		t.Fatal("expected failure for empty task id")
	}
	if result.Error == nil || result.Error.Code != subAgentErrorCodeRuntimeUnavailable {
		t.Fatalf("expected runtime_unavailable code, got %v", result.Error)
	}
}

func TestDelegateSubAgentRunSyncExecutionError(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	gateway := &flexibleRuntimeGateway{
		syncResult: RuntimeTaskExecution{
			TaskID:         "task-1",
			ExecutionError: errors.New("execution exploded"),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})
	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "test task",
	}, &tools.ExecutionContext{Mode: planpkg.ModeBuild}, nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.OK {
		t.Fatal("expected failure for execution error")
	}
	if result.Error == nil {
		t.Fatal("expected error")
	}
}

// invokingRuntimeGateway calls Execute synchronously and invokes OnTaskStateChanged.
type invokingRuntimeGateway struct {
	mu         sync.Mutex
	calls      []RuntimeTaskRequest
	result     runtimepkg.TaskResult
	execErr    error
	taskStatus corepkg.TaskStatus
}

func (g *invokingRuntimeGateway) RunSync(_ context.Context, req RuntimeTaskRequest) (RuntimeTaskExecution, error) {
	g.mu.Lock()
	g.calls = append(g.calls, req)
	g.mu.Unlock()

	var output []byte
	var execErr error
	if req.Execute != nil {
		output, execErr = req.Execute(context.Background())
	}

	status := g.taskStatus
	if status == "" {
		status = corepkg.TaskCompleted
	}
	result := g.result
	result.TaskID = "runtime-task-1"
	result.Status = status
	if len(result.Output) == 0 && output != nil {
		result.Output = output
	}

	if req.OnTaskStateChanged != nil {
		req.OnTaskStateChanged(runtimepkg.Task{
			ID:        "runtime-task-1",
			Status:    status,
			Output:    result.Output,
			ErrorCode: result.ErrorCode,
		})
	}

	if execErr != nil {
		return RuntimeTaskExecution{TaskID: "runtime-task-1", Result: result, ExecutionError: execErr}, nil
	}
	return RuntimeTaskExecution{TaskID: "runtime-task-1", Result: result}, nil
}

func (g *invokingRuntimeGateway) RunAsync(_ context.Context, _ RuntimeTaskRequest) (RuntimeTaskLaunch, error) {
	return RuntimeTaskLaunch{TaskID: "runtime-task-1"}, nil
}

func TestDelegateSubAgentExecuteCallbackError(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	gateway := &invokingRuntimeGateway{}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config:    config.Config{Provider: config.ProviderConfig{Model: "test-model"}, MaxIterations: 2},
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})
	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "test",
	}, &tools.ExecutionContext{Mode: planpkg.ModeBuild}, nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.OK {
		t.Fatal("expected OK:true for error-as-content (execute callback error)")
	}
	if !strings.Contains(result.Summary, "SubAgent error") && !strings.Contains(result.Summary, "error") {
		t.Fatalf("expected error information in summary, got %q", result.Summary)
	}
}

func TestDelegateSubAgentOnTaskStateChangedFailedStatus(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	client := &fakeClient{replies: []llm.Message{
		{Role: llm.RoleAssistant, Content: "done"},
	}}
	notifier := &defaultSubAgentNotifier{}
	gateway := &invokingRuntimeGateway{
		taskStatus: corepkg.TaskFailed,
		result:     runtimepkg.TaskResult{ErrorCode: "worker_crash"},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config:    config.Config{Provider: config.ProviderConfig{Model: "test-model"}, MaxIterations: 2},
		Client:    client,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})
	runner.subAgentNotifier = notifier
	sess := session.New(workspace)
	result, _ := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "test",
	}, &tools.ExecutionContext{Mode: planpkg.ModeBuild, Session: sess}, nil, "")
	if result.OK {
		t.Fatal("expected failure for failed task")
	}
	// Verify notification was enqueued
	pending := notifier.DrainPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(pending))
	}
	if pending[0].Status != subAgentResultStatusFailed {
		t.Fatalf("expected failed status in notification, got %q", pending[0].Status)
	}
	if pending[0].ErrorCode != "worker_crash" {
		t.Fatalf("expected worker_crash error code, got %q", pending[0].ErrorCode)
	}
}

func TestDelegateSubAgentOnTaskStateChangedCompletedWithSummary(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	client := &fakeClient{replies: []llm.Message{
		{Role: llm.RoleAssistant, Content: "done"},
	}}
	notifier := &defaultSubAgentNotifier{}
	gateway := &invokingRuntimeGateway{
		taskStatus: corepkg.TaskCompleted,
		result: runtimepkg.TaskResult{
			Output: []byte(`{"ok":true,"status":"completed","summary":"scan complete"}`),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config:    config.Config{Provider: config.ProviderConfig{Model: "test-model"}, MaxIterations: 2},
		Client:    client,
		Registry:  tools.DefaultRegistry(),
		Runtime:   gateway,
	})
	runner.subAgentNotifier = notifier
	sess := session.New(workspace)
	result, _ := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent:  "explorer",
		Task:   "test",
		Output: "summary",
	}, &tools.ExecutionContext{Mode: planpkg.ModeBuild, Session: sess}, nil, "")
	if !result.OK {
		t.Fatalf("expected success, got %#v", result)
	}
	pending := notifier.DrainPending()
	if len(pending) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(pending))
	}
	if pending[0].Status != subAgentResultStatusCompleted {
		t.Fatalf("expected completed status, got %q", pending[0].Status)
	}
	if pending[0].Summary != "scan complete" {
		t.Fatalf("expected summary in notification, got %q", pending[0].Summary)
	}
}

func TestEffectiveToolsetHashNilInput(t *testing.T) {
	if got := effectiveToolsetHash(nil); got != "" {
		t.Fatalf("expected empty for nil, got %q", got)
	}
	if got := effectiveToolsetHash([]string{}); got != "" {
		t.Fatalf("expected empty for empty slice, got %q", got)
	}
}

// minimalRegistry does not implement toolSpecLookup (no Spec method).
type minimalRegistry struct{}

func (r *minimalRegistry) DefinitionsForMode(_ planpkg.AgentMode) []llm.ToolDefinition {
	return nil
}
func (r *minimalRegistry) DefinitionsForModeWithFilters(_ planpkg.AgentMode, _, _ []string) []llm.ToolDefinition {
	return nil
}

func TestIsReadOnlySubAgentToolsetNonSpecRegistry(t *testing.T) {
	runner := &Runner{registry: &minimalRegistry{}}
	if runner.isReadOnlySubAgentToolset([]string{"read_file"}) {
		t.Fatal("expected false for non-Spec registry")
	}
}

func TestDelegateSubAgentNilRunner(t *testing.T) {
	var r *Runner
	result, err := r.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "test",
	}, nil, nil, "")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.OK {
		t.Fatal("expected failure for nil runner")
	}
	if result.Error == nil || result.Error.Code != subagentspkg.ErrorCodeSubAgentUnavailable {
		t.Fatalf("expected subagent_unavailable, got %v", result.Error)
	}
}
