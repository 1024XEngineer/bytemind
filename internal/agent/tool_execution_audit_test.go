package agent

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"

	"bytemind/internal/config"
	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	runtimepkg "bytemind/internal/runtime"
	"bytemind/internal/session"
	storagepkg "bytemind/internal/storage"
	"bytemind/internal/tools"
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

func TestRunPromptRecordsTaskStateChangedAuditWithSessionTaskTrace(t *testing.T) {
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
				ID:   "trace-tool-1",
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
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
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
		if event.TraceID != corepkg.TraceID("trace-tool-1") {
			t.Fatalf("expected trace id %q, got %q", "trace-tool-1", event.TraceID)
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
	mu    sync.Mutex
	calls []RuntimeTaskRequest
}

func (g *stubRuntimeGateway) RunSync(_ context.Context, request RuntimeTaskRequest) (RuntimeTaskExecution, error) {
	g.mu.Lock()
	g.calls = append(g.calls, request)
	g.mu.Unlock()
	return RuntimeTaskExecution{
		TaskID: "runtime-task-1",
		Result: runtimepkg.TaskResult{
			TaskID: "runtime-task-1",
			Status: corepkg.TaskCompleted,
			Output: []byte(`{"ok":true,"via":"runtime_gateway"}`),
		},
	}, nil
}

func TestRunPromptExecutesToolThroughRuntimeGatewayBoundary(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	client := &fakeClient{replies: []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{{
				ID:   "tool-via-runtime",
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
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
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
	if call.Name != "list_files" || call.Kind != "tool" {
		t.Fatalf("expected runtime gateway tool request, got %+v", call)
	}

	if len(sess.Messages) < 3 {
		t.Fatalf("expected tool result message to be persisted, got %#v", sess.Messages)
	}
	toolResult := sess.Messages[2].Content
	if !strings.Contains(toolResult, `"via":"runtime_gateway"`) {
		t.Fatalf("expected tool result from runtime gateway, got %q", toolResult)
	}
}
