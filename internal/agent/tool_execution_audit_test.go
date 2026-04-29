package agent

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
	runtimepkg "github.com/1024XEngineer/bytemind/internal/runtime"
	"github.com/1024XEngineer/bytemind/internal/session"
	storagepkg "github.com/1024XEngineer/bytemind/internal/storage"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

type toolExecutionAuditStore struct {
	mu     sync.Mutex
	events []storagepkg.AuditEvent
}

func (s *toolExecutionAuditStore) Append(_ context.Context, event storagepkg.AuditEvent) error {
	s.mu.Lock()
	s.events = append(s.events, event)
	s.mu.Unlock()
	return nil
}

func (s *toolExecutionAuditStore) snapshot() []storagepkg.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	copied := make([]storagepkg.AuditEvent, len(s.events))
	copy(copied, s.events)
	return copied
}

func TestRunPromptRecordsTaskStateChangedAuditForRuntimeBackedTool(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	auditStore := &toolExecutionAuditStore{}
	probe := &managerProbeTool{}
	registry := tools.DefaultRegistry()
	if err := registry.Register(probe, tools.RegisterOptions{Source: tools.RegistrationSourceBuiltin}); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{replies: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "trace-runtime-tool-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "manager_probe",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "done",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:          config.ProviderConfig{Model: "test-model"},
			MaxIterations:     4,
			Stream:            false,
			SandboxEnabled:    true,
			SystemSandboxMode: "best_effort",
		},
		Client:     client,
		Store:      store,
		Registry:   registry,
		AuditStore: auditStore,
		Stdin:      strings.NewReader(""),
		Stdout:     io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "probe managers", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}

	events := auditStore.snapshot()
	stateEvents := make([]storagepkg.AuditEvent, 0, 4)
	for _, event := range events {
		if event.Action == "task_state_changed" {
			stateEvents = append(stateEvents, event)
		}
	}
	if len(stateEvents) == 0 {
		t.Fatalf("expected task_state_changed audit events, got %+v", events)
	}

	sessionID := corepkg.SessionID(sess.ID)
	seenTerminal := false
	for _, event := range stateEvents {
		if event.SessionID != sessionID {
			t.Fatalf("expected session id %q, got %q", sessionID, event.SessionID)
		}
		if event.TaskID == "" {
			t.Fatalf("expected non-empty task id, got %+v", event)
		}
		if event.TraceID != corepkg.TraceID("trace-runtime-tool-1") {
			t.Fatalf("expected trace id %q, got %q", "trace-runtime-tool-1", event.TraceID)
		}
		if got := event.Metadata["sandbox_enabled"]; got != "true" {
			t.Fatalf("expected task_state_changed sandbox_enabled=true, got %q", got)
		}
		if got := event.Metadata["sandbox_mode"]; got != "best_effort" {
			t.Fatalf("expected task_state_changed sandbox_mode=best_effort, got %q", got)
		}
		if got := event.Metadata["sandbox_required_capable"]; got != "true" && got != "false" {
			t.Fatalf("expected task_state_changed sandbox_required_capable to be boolean text, got %q", got)
		}
		if got := event.Metadata["sandbox_capability_level"]; got == "" {
			t.Fatalf("expected task_state_changed sandbox_capability_level, got %q", got)
		}
		if got := event.Metadata["sandbox_fallback"]; got != "true" && got != "false" {
			t.Fatalf("expected task_state_changed sandbox_fallback to be boolean text, got %q", got)
		}
		if event.Result == string(corepkg.TaskCompleted) || event.Result == string(corepkg.TaskFailed) || event.Result == string(corepkg.TaskKilled) {
			seenTerminal = true
		}
	}
	if !seenTerminal {
		t.Fatalf("expected at least one terminal task_state_changed event, got %+v", stateEvents)
	}
}

type stubRuntimeGateway struct {
	mu     sync.Mutex
	calls  []RuntimeTaskRequest
	result runtimepkg.TaskResult
}

func (g *stubRuntimeGateway) RunSync(_ context.Context, request RuntimeTaskRequest) (RuntimeTaskExecution, error) {
	g.mu.Lock()
	g.calls = append(g.calls, request)
	g.mu.Unlock()
	result := g.result
	if result.TaskID == "" {
		result.TaskID = "runtime-task-1"
	}
	if result.Status == "" {
		result.Status = corepkg.TaskCompleted
	}
	if len(result.Output) == 0 {
		result.Output = []byte(`{"ok":true,"via":"runtime_gateway"}`)
	}
	return RuntimeTaskExecution{
		TaskID: result.TaskID,
		Result: result,
	}, nil
}

func TestShouldExecuteToolDirectlyRoutesSynchronousBuiltins(t *testing.T) {
	for _, name := range []string{
		"list_files",
		"read_file",
		"search_text",
		"web_search",
		"web_fetch",
		"write_file",
		"replace_in_file",
		"apply_patch",
		"update_plan",
	} {
		if !shouldExecuteToolDirectly(name) {
			t.Fatalf("expected %s to execute directly", name)
		}
	}
	for _, name := range []string{"run_shell", "manager_probe"} {
		if shouldExecuteToolDirectly(name) {
			t.Fatalf("expected %s to remain runtime-backed", name)
		}
	}
}

func TestRunPromptExecutesReadOnlyToolDirectly(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	auditStore := &toolExecutionAuditStore{}

	client := &fakeClient{replies: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "tool-direct-read",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "done",
		},
	}}
	gateway := &stubRuntimeGateway{}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:          config.ProviderConfig{Model: "test-model"},
			MaxIterations:     4,
			Stream:            false,
			SandboxEnabled:    true,
			SystemSandboxMode: "best_effort",
		},
		Client:     client,
		Store:      store,
		Registry:   tools.DefaultRegistry(),
		Runtime:    gateway,
		AuditStore: auditStore,
		Stdin:      strings.NewReader(""),
		Stdout:     io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "run tool", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}

	gateway.mu.Lock()
	callCount := len(gateway.calls)
	gateway.mu.Unlock()
	if callCount != 0 {
		t.Fatalf("expected read-only tool to bypass runtime gateway, got %d calls", callCount)
	}

	if len(sess.Messages) < 3 {
		t.Fatalf("expected tool result message to be persisted, got %#v", sess.Messages)
	}
	toolResult := sess.Messages[2].Content
	if strings.Contains(toolResult, `"via":"runtime_gateway"`) {
		t.Fatalf("expected direct tool result, got %q", toolResult)
	}

	events := auditStore.snapshot()
	for _, event := range events {
		if event.Action == "task_state_changed" {
			t.Fatalf("did not expect task_state_changed for direct tool, got %+v", event)
		}
	}
	var resultEvent *storagepkg.AuditEvent
	for i := range events {
		if events[i].Action == "tool_execute_result" && events[i].Metadata["tool_name"] == "list_files" {
			resultEvent = &events[i]
		}
	}
	if resultEvent == nil {
		t.Fatalf("expected tool_execute_result audit event, got %+v", events)
	}
	if resultEvent.TaskID != "" {
		t.Fatalf("expected direct tool audit to omit task id, got %+v", resultEvent)
	}
	if resultEvent.TraceID != corepkg.TraceID("tool-direct-read") {
		t.Fatalf("expected trace id %q, got %q", "tool-direct-read", resultEvent.TraceID)
	}
}

func TestRunPromptExecutesWriteFileDirectly(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	auditStore := &toolExecutionAuditStore{}

	client := &fakeClient{replies: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "tool-direct-write",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "write_file",
					Arguments: `{"path":"x.txt","content":"hello"}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "done",
		},
	}}
	gateway := &stubRuntimeGateway{}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:       config.ProviderConfig{Model: "test-model"},
			MaxIterations:  4,
			Stream:         false,
			ApprovalPolicy: "on-request",
		},
		Client:     client,
		Store:      store,
		Registry:   tools.DefaultRegistry(),
		Runtime:    gateway,
		AuditStore: auditStore,
		Stdin:      strings.NewReader("y\n"),
		Stdout:     io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "write file", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}

	gateway.mu.Lock()
	callCount := len(gateway.calls)
	gateway.mu.Unlock()
	if callCount != 0 {
		t.Fatalf("expected write_file to bypass runtime gateway, got %d calls", callCount)
	}
	data, err := os.ReadFile(filepath.Join(workspace, "x.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected file content: %q", string(data))
	}

	events := auditStore.snapshot()
	for _, event := range events {
		if event.Action == "task_state_changed" {
			t.Fatalf("did not expect task_state_changed for direct write tool, got %+v", event)
		}
	}
	var resultEvent *storagepkg.AuditEvent
	for i := range events {
		if events[i].Action == "tool_execute_result" && events[i].Metadata["tool_name"] == "write_file" {
			resultEvent = &events[i]
		}
	}
	if resultEvent == nil {
		t.Fatalf("expected tool_execute_result audit event, got %+v", events)
	}
	if resultEvent.TaskID != "" {
		t.Fatalf("expected direct write audit to omit task id, got %+v", resultEvent)
	}
	if resultEvent.TraceID != corepkg.TraceID("tool-direct-write") {
		t.Fatalf("expected trace id %q, got %q", "tool-direct-write", resultEvent.TraceID)
	}
}

func TestRunPromptExecutesUpdatePlanDirectly(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	auditStore := &toolExecutionAuditStore{}
	events := make([]Event, 0, 6)

	client := &fakeClient{replies: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "tool-direct-plan",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "update_plan",
					Arguments: `{"explanation":"starting","plan":[{"step":"Inspect provider","status":"completed"},{"step":"Add tests","status":"in_progress"}]}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "done",
		},
	}}
	gateway := &stubRuntimeGateway{}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:     client,
		Store:      store,
		Registry:   tools.DefaultRegistry(),
		Runtime:    gateway,
		AuditStore: auditStore,
		Observer: ObserverFunc(func(event Event) {
			events = append(events, event)
		}),
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "update plan", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}

	gateway.mu.Lock()
	callCount := len(gateway.calls)
	gateway.mu.Unlock()
	if callCount != 0 {
		t.Fatalf("expected update_plan to bypass runtime gateway, got %d calls", callCount)
	}
	if len(sess.Plan.Steps) != 2 || sess.Plan.Steps[1].Title != "Add tests" {
		t.Fatalf("unexpected session plan: %#v", sess.Plan)
	}
	seenPlanUpdated := false
	for _, event := range events {
		if event.Type == EventPlanUpdated {
			seenPlanUpdated = true
			break
		}
	}
	if !seenPlanUpdated {
		t.Fatalf("expected EventPlanUpdated, got %+v", events)
	}

	auditEvents := auditStore.snapshot()
	for _, event := range auditEvents {
		if event.Action == "task_state_changed" {
			t.Fatalf("did not expect task_state_changed for direct update_plan, got %+v", event)
		}
	}
	var resultEvent *storagepkg.AuditEvent
	for i := range auditEvents {
		if auditEvents[i].Action == "tool_execute_result" && auditEvents[i].Metadata["tool_name"] == "update_plan" {
			resultEvent = &auditEvents[i]
		}
	}
	if resultEvent == nil {
		t.Fatalf("expected tool_execute_result audit event, got %+v", auditEvents)
	}
	if resultEvent.TaskID != "" {
		t.Fatalf("expected direct update_plan audit to omit task id, got %+v", resultEvent)
	}
	if resultEvent.TraceID != corepkg.TraceID("tool-direct-plan") {
		t.Fatalf("expected trace id %q, got %q", "tool-direct-plan", resultEvent.TraceID)
	}
}

func TestRunPromptKeepsNonDirectToolOnRuntimeGateway(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	probe := &managerProbeTool{}
	registry := tools.DefaultRegistry()
	if err := registry.Register(probe, tools.RegisterOptions{Source: tools.RegistrationSourceBuiltin}); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{replies: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "tool-via-runtime",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "manager_probe",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "done",
		},
	}}
	gateway := &stubRuntimeGateway{}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:          config.ProviderConfig{Model: "test-model"},
			MaxIterations:     4,
			Stream:            false,
			SandboxEnabled:    true,
			SystemSandboxMode: "best_effort",
		},
		Client:   client,
		Store:    store,
		Registry: registry,
		Runtime:  gateway,
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "run tool", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}

	gateway.mu.Lock()
	callCount := len(gateway.calls)
	var call RuntimeTaskRequest
	if callCount > 0 {
		call = gateway.calls[0]
	}
	gateway.mu.Unlock()
	if callCount != 1 {
		t.Fatalf("expected exactly 1 runtime gateway call, got %d", callCount)
	}
	if call.Name != "manager_probe" || call.Kind != "tool" {
		t.Fatalf("expected runtime gateway tool request, got %+v", call)
	}
	if got := call.Metadata["sandbox_enabled"]; got != "true" {
		t.Fatalf("expected runtime metadata sandbox_enabled=true, got %q", got)
	}
	if got := call.Metadata["sandbox_mode"]; got != "best_effort" {
		t.Fatalf("expected runtime metadata sandbox_mode=best_effort, got %q", got)
	}
	if got := call.Metadata["sandbox_required_capable"]; got != "true" && got != "false" {
		t.Fatalf("expected runtime metadata sandbox_required_capable boolean text, got %q", got)
	}
	if got := call.Metadata["sandbox_capability_level"]; got == "" {
		t.Fatalf("expected runtime metadata sandbox_capability_level, got %q", got)
	}

	if len(sess.Messages) < 3 {
		t.Fatalf("expected tool result message to be persisted, got %#v", sess.Messages)
	}
	toolResult := sess.Messages[2].Content
	if !strings.Contains(toolResult, `"via":"runtime_gateway"`) {
		t.Fatalf("expected tool result from runtime gateway, got %q", toolResult)
	}
}

func TestRunPromptRecordsSystemSandboxMetadataInToolExecuteAudit(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	auditStore := &toolExecutionAuditStore{}

	client := &fakeClient{replies: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "tool-sandbox-audit",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "run_shell",
					Arguments: `{"command":"echo ok"}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "done",
		},
	}}
	gateway := &stubRuntimeGateway{
		result: runtimepkg.TaskResult{
			TaskID: "runtime-task-1",
			Status: corepkg.TaskCompleted,
			Output: []byte(`{"ok":true,"exit_code":0,"stdout":"ok","stderr":"","system_sandbox":{"mode":"best_effort","backend":"linux_unshare","active":false,"required_capable":false,"fallback":true,"status":"fallback","fallback_reason":"linux backend unavailable"}}`),
		},
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:          config.ProviderConfig{Model: "test-model"},
			MaxIterations:     4,
			Stream:            false,
			SandboxEnabled:    true,
			SystemSandboxMode: "best_effort",
		},
		Client:     client,
		Store:      store,
		Registry:   tools.DefaultRegistry(),
		Runtime:    gateway,
		AuditStore: auditStore,
		Stdin:      strings.NewReader(""),
		Stdout:     io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "run shell", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}

	events := auditStore.snapshot()
	var startEvent *storagepkg.AuditEvent
	var resultEvent *storagepkg.AuditEvent
	for i := range events {
		if events[i].Action == "tool_execute_start" && events[i].Metadata["tool_name"] == "run_shell" {
			startEvent = &events[i]
		}
		if events[i].Action == "tool_execute_result" && events[i].Metadata["tool_name"] == "run_shell" {
			resultEvent = &events[i]
		}
	}
	if startEvent == nil {
		t.Fatalf("expected tool_execute_start audit event, got %+v", events)
	}
	if got := startEvent.Metadata["sandbox_enabled"]; got != "true" {
		t.Fatalf("expected start event sandbox_enabled=true, got %q", got)
	}
	if got := startEvent.Metadata["sandbox_mode"]; got != "best_effort" {
		t.Fatalf("expected start event sandbox_mode=best_effort, got %q", got)
	}
	if got := startEvent.Metadata["sandbox_required_capable"]; got != "true" && got != "false" {
		t.Fatalf("expected start event sandbox_required_capable boolean text, got %q", got)
	}
	if got := startEvent.Metadata["sandbox_capability_level"]; got == "" {
		t.Fatalf("expected start event sandbox_capability_level, got %q", got)
	}
	if resultEvent == nil {
		t.Fatalf("expected tool_execute_result audit event, got %+v", events)
	}
	if got := resultEvent.Metadata["sandbox_mode"]; got != "best_effort" {
		t.Fatalf("expected sandbox_mode=best_effort, got %q", got)
	}
	if got := resultEvent.Metadata["sandbox_backend"]; got != "linux_unshare" {
		t.Fatalf("expected sandbox_backend=linux_unshare, got %q", got)
	}
	if got := resultEvent.Metadata["sandbox_status"]; got != "fallback" {
		t.Fatalf("expected sandbox_status=fallback, got %q", got)
	}
	if got := resultEvent.Metadata["sandbox_required_capable"]; got != "false" {
		t.Fatalf("expected sandbox_required_capable=false, got %q", got)
	}
	if got := resultEvent.Metadata["sandbox_capability_level"]; got == "" {
		t.Fatalf("expected sandbox_capability_level in result metadata, got %q", got)
	}
	if got := resultEvent.Metadata["sandbox_fallback"]; got != "true" {
		t.Fatalf("expected sandbox_fallback=true, got %q", got)
	}
	if got := resultEvent.Metadata["sandbox_fallback_reason"]; got != "linux backend unavailable" {
		t.Fatalf("expected sandbox_fallback_reason to be recorded, got %q", got)
	}
	if got, want := resultEvent.Metadata["sandbox_lease_id"], "session-"+sess.ID; got != want {
		t.Fatalf("expected sandbox_lease_id=%q, got %q", want, got)
	}
	if got, want := resultEvent.Metadata["sandbox_run_id"], "trace-tool-sandbox-audit"; got != want {
		t.Fatalf("expected sandbox_run_id=%q, got %q", want, got)
	}
}

func TestRunPromptRecordsSystemSandboxStartupAudit(t *testing.T) {
	original := resolveAgentSystemSandboxRuntimeStatus
	resolveAgentSystemSandboxRuntimeStatus = func(enabled bool, mode string) (tools.SystemSandboxRuntimeStatus, error) {
		if !enabled {
			return tools.SystemSandboxRuntimeStatus{}, nil
		}
		return tools.SystemSandboxRuntimeStatus{
			Mode:           mode,
			BackendEnabled: false,
			BackendName:    "none",
			Fallback:       true,
			Message:        "system sandbox best_effort fallback: test backend unavailable",
		}, nil
	}
	t.Cleanup(func() {
		resolveAgentSystemSandboxRuntimeStatus = original
	})

	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	auditStore := &toolExecutionAuditStore{}

	client := &fakeClient{replies: []llm.Message{
		{
			Role:    llm.RoleAssistant,
			Content: "done",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:          config.ProviderConfig{Model: "test-model"},
			MaxIterations:     2,
			Stream:            false,
			SandboxEnabled:    true,
			SystemSandboxMode: "best_effort",
		},
		Client:     client,
		Store:      store,
		Registry:   tools.DefaultRegistry(),
		AuditStore: auditStore,
		Stdin:      strings.NewReader(""),
		Stdout:     io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "hello", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}

	events := auditStore.snapshot()
	var startupEvent *storagepkg.AuditEvent
	for i := range events {
		if events[i].Action == "system_sandbox_startup" {
			startupEvent = &events[i]
			break
		}
	}
	if startupEvent == nil {
		t.Fatalf("expected system_sandbox_startup audit event, got %+v", events)
	}
	if got, want := startupEvent.Result, "fallback"; got != want {
		t.Fatalf("expected startup audit result=%q, got %q", want, got)
	}
	for k, want := range map[string]string{
		"sandbox_enabled":                  "true",
		"sandbox_mode":                     "best_effort",
		"sandbox_backend":                  "none",
		"sandbox_required_capable":         "false",
		"sandbox_capability_level":         "none",
		"sandbox_shell_network_isolation":  "false",
		"sandbox_worker_network_isolation": "false",
		"sandbox_status":                   "fallback",
		"sandbox_fallback":                 "true",
	} {
		if got := startupEvent.Metadata[k]; got != want {
			t.Fatalf("expected startup audit metadata[%q]=%q, got %q", k, want, got)
		}
	}
	if got := startupEvent.Metadata["sandbox_message"]; got != "system sandbox best_effort fallback: test backend unavailable" {
		t.Fatalf("expected startup audit sandbox_message to be recorded, got %q", got)
	}
}

func TestRunPromptPropagatesSandboxFallbackContextToPermissionAndStartAudit(t *testing.T) {
	original := resolveAgentSystemSandboxRuntimeStatus
	resolveAgentSystemSandboxRuntimeStatus = func(enabled bool, mode string) (tools.SystemSandboxRuntimeStatus, error) {
		if !enabled {
			return tools.SystemSandboxRuntimeStatus{}, nil
		}
		return tools.SystemSandboxRuntimeStatus{
			Mode:            mode,
			BackendEnabled:  false,
			BackendName:     "none",
			RequiredCapable: false,
			Fallback:        true,
			Message:         "system sandbox best_effort fallback: test backend unavailable",
		}, nil
	}
	t.Cleanup(func() {
		resolveAgentSystemSandboxRuntimeStatus = original
	})

	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	auditStore := &toolExecutionAuditStore{}

	client := &fakeClient{replies: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "tool-fallback-audit",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    llm.RoleAssistant,
			Content: "done",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:          config.ProviderConfig{Model: "test-model"},
			MaxIterations:     4,
			Stream:            false,
			SandboxEnabled:    true,
			SystemSandboxMode: "best_effort",
		},
		Client:     client,
		Store:      store,
		Registry:   tools.DefaultRegistry(),
		AuditStore: auditStore,
		Stdin:      strings.NewReader(""),
		Stdout:     io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "list files", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}

	events := auditStore.snapshot()
	var permissionEvent *storagepkg.AuditEvent
	var startEvent *storagepkg.AuditEvent
	for i := range events {
		event := &events[i]
		if event.Action == "permission_decision" && event.Metadata["tool_name"] == "list_files" {
			permissionEvent = event
		}
		if event.Action == "tool_execute_start" && event.Metadata["tool_name"] == "list_files" {
			startEvent = event
		}
	}
	if permissionEvent == nil {
		t.Fatalf("expected permission_decision event, got %+v", events)
	}
	if startEvent == nil {
		t.Fatalf("expected tool_execute_start event, got %+v", events)
	}

	assertFallback := func(label string, metadata map[string]string) {
		t.Helper()
		if got := metadata["sandbox_enabled"]; got != "true" {
			t.Fatalf("%s: expected sandbox_enabled=true, got %q", label, got)
		}
		if got := metadata["sandbox_mode"]; got != "best_effort" {
			t.Fatalf("%s: expected sandbox_mode=best_effort, got %q", label, got)
		}
		if got := metadata["sandbox_backend"]; got != "none" {
			t.Fatalf("%s: expected sandbox_backend=none, got %q", label, got)
		}
		if got := metadata["sandbox_required_capable"]; got != "false" {
			t.Fatalf("%s: expected sandbox_required_capable=false, got %q", label, got)
		}
		if got := metadata["sandbox_capability_level"]; got != "none" {
			t.Fatalf("%s: expected sandbox_capability_level=none, got %q", label, got)
		}
		if got := metadata["sandbox_shell_network_isolation"]; got != "false" {
			t.Fatalf("%s: expected sandbox_shell_network_isolation=false, got %q", label, got)
		}
		if got := metadata["sandbox_worker_network_isolation"]; got != "false" {
			t.Fatalf("%s: expected sandbox_worker_network_isolation=false, got %q", label, got)
		}
		if got := metadata["sandbox_fallback"]; got != "true" {
			t.Fatalf("%s: expected sandbox_fallback=true, got %q", label, got)
		}
		if got := metadata["sandbox_status"]; got != "fallback" {
			t.Fatalf("%s: expected sandbox_status=fallback, got %q", label, got)
		}
		if got := metadata["sandbox_fallback_reason"]; got != "system sandbox best_effort fallback: test backend unavailable" {
			t.Fatalf("%s: expected sandbox_fallback_reason to be recorded, got %q", label, got)
		}
	}

	assertFallback("permission_decision", permissionEvent.Metadata)
	assertFallback("tool_execute_start", startEvent.Metadata)
}
