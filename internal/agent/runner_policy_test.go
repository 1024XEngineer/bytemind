package agent

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"testing"

	"bytemind/internal/config"
	corepkg "bytemind/internal/core"
	"bytemind/internal/llm"
	"bytemind/internal/session"
	storagepkg "bytemind/internal/storage"
	"bytemind/internal/tools"
)

type recordingAuditStore struct {
	mu     sync.Mutex
	events []storagepkg.AuditEvent
}

func (s *recordingAuditStore) Append(_ context.Context, event storagepkg.AuditEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = append(s.events, event)
	return nil
}

func (s *recordingAuditStore) snapshot() []storagepkg.AuditEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]storagepkg.AuditEvent, len(s.events))
	copy(out, s.events)
	return out
}

type policyGatewayFunc func(context.Context, ToolDecisionInput) (ToolDecision, error)

func (f policyGatewayFunc) DecideTool(ctx context.Context, in ToolDecisionInput) (ToolDecision, error) {
	return f(ctx, in)
}

func TestRunPromptPolicyGatewayDeniesToolBeforeExecutor(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	executed := false
	blockedTool := &fakeTool{
		name: "blocked_tool",
		run: func(raw json.RawMessage, execCtx *tools.ExecutionContext) (string, error) {
			executed = true
			return `{"ok":true}`, nil
		},
	}
	registry := tools.DefaultRegistry()
	if err := registry.Register(blockedTool, tools.RegisterOptions{Source: tools.RegistrationSourceBuiltin}); err != nil {
		t.Fatal(err)
	}

	client := &recordingClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-blocked",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "blocked_tool",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "Policy handled.",
		},
	}}

	auditStore := &recordingAuditStore{}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:          config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations:     3,
			Stream:            false,
			TokenQuota:        generousTokenQuota,
			ApprovalPolicy:    "on-request",
			SandboxEnabled:    true,
			SystemSandboxMode: "best_effort",
		},
		Client:   client,
		Store:    store,
		Registry: registry,
		PolicyGateway: policyGatewayFunc(func(_ context.Context, in ToolDecisionInput) (ToolDecision, error) {
			if in.ToolName != "blocked_tool" {
				t.Fatalf("unexpected tool name: %q", in.ToolName)
			}
			return ToolDecision{
				Decision:   corepkg.DecisionDeny,
				ReasonCode: policyReasonExplicitDeny,
				Reason:     "blocked by test policy",
				RiskLevel:  corepkg.RiskHigh,
			}, nil
		}),
		AuditStore: auditStore,
		Stdin:      strings.NewReader(""),
		Stdout:     io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "run blocked tool", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Policy handled." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if executed {
		t.Fatal("expected tool execution to be blocked by policy")
	}

	foundPermissionDecision := false
	foundExecuteStart := false
	foundDeniedResult := false
	for _, event := range auditStore.snapshot() {
		if event.Action == "permission_decision" && event.Decision == corepkg.DecisionDeny && event.ReasonCode == policyReasonExplicitDeny {
			foundPermissionDecision = true
			if got := event.Metadata["sandbox_enabled"]; got != "true" {
				t.Fatalf("expected deny permission_decision sandbox_enabled=true, got %q", got)
			}
			if got := event.Metadata["sandbox_mode"]; got != "best_effort" {
				t.Fatalf("expected deny permission_decision sandbox_mode=best_effort, got %q", got)
			}
			if got := event.Metadata["sandbox_required_capable"]; got != "true" && got != "false" {
				t.Fatalf("expected deny permission_decision sandbox_required_capable boolean text, got %q", got)
			}
		}
		if event.Action == "tool_execute_start" && event.Metadata["tool_name"] == "blocked_tool" {
			foundExecuteStart = true
		}
		if event.Action == "tool_execute_result" && event.Metadata["tool_name"] == "blocked_tool" && event.Result == "denied" {
			foundDeniedResult = true
			if got := event.Metadata["sandbox_enabled"]; got != "true" {
				t.Fatalf("expected denied tool_execute_result sandbox_enabled=true, got %q", got)
			}
			if got := event.Metadata["sandbox_mode"]; got != "best_effort" {
				t.Fatalf("expected denied tool_execute_result sandbox_mode=best_effort, got %q", got)
			}
			if got := event.Metadata["sandbox_required_capable"]; got != "true" && got != "false" {
				t.Fatalf("expected denied tool_execute_result sandbox_required_capable boolean text, got %q", got)
			}
		}
	}
	if !foundPermissionDecision {
		t.Fatal("expected permission_decision audit event for denied tool")
	}
	if foundExecuteStart {
		t.Fatal("did not expect tool_execute_start audit event for denied tool")
	}
	if !foundDeniedResult {
		t.Fatal("expected denied tool_execute_result audit event for blocked tool")
	}
}

func TestRunPromptPolicyGatewayAskRequestsApprovalAndExecutesTool(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	executed := false
	approvalRequested := false
	askTool := &fakeTool{
		name: "ask_tool",
		run: func(raw json.RawMessage, execCtx *tools.ExecutionContext) (string, error) {
			if execCtx == nil || execCtx.Approval == nil {
				t.Fatal("expected approval handler in execution context")
			}
			approved, approvalErr := execCtx.Approval(tools.ApprovalRequest{
				Command: "ask_tool",
				Reason:  "high-risk tool requires approval",
			})
			if approvalErr != nil {
				t.Fatalf("unexpected approval error: %v", approvalErr)
			}
			approvalRequested = true
			if !approved {
				t.Fatal("expected approval handler to approve execution")
			}
			executed = true
			return `{"ok":true,"status":"executed"}`, nil
		},
	}
	registry := tools.DefaultRegistry()
	if err := registry.Register(askTool, tools.RegisterOptions{Source: tools.RegistrationSourceBuiltin}); err != nil {
		t.Fatal(err)
	}

	client := &recordingClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-ask",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "ask_tool",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "Ask path handled.",
		},
	}}

	auditStore := &recordingAuditStore{}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:          config.ProviderConfig{Type: "openai-compatible", Model: "test-model"},
			MaxIterations:     3,
			Stream:            false,
			TokenQuota:        generousTokenQuota,
			ApprovalPolicy:    "on-request",
			SandboxEnabled:    true,
			SystemSandboxMode: "best_effort",
		},
		Client:   client,
		Store:    store,
		Registry: registry,
		PolicyGateway: policyGatewayFunc(func(_ context.Context, in ToolDecisionInput) (ToolDecision, error) {
			if in.ToolName != "ask_tool" {
				t.Fatalf("unexpected tool name: %q", in.ToolName)
			}
			return ToolDecision{
				Decision:   corepkg.DecisionAsk,
				ReasonCode: policyReasonRiskRule,
				Reason:     "requires explicit approval",
				RiskLevel:  corepkg.RiskHigh,
			}, nil
		}),
		AuditStore: auditStore,
		Approval: func(req tools.ApprovalRequest) (bool, error) {
			if req.Command != "ask_tool" {
				t.Fatalf("unexpected approval command: %q", req.Command)
			}
			return true, nil
		},
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "run ask tool", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Ask path handled." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if !approvalRequested {
		t.Fatal("expected approval to be requested for ask decision path")
	}
	if !executed {
		t.Fatal("expected tool execution after approval for ask decision path")
	}

	foundPermissionDecisionAsk := false
	foundExecuteStart := false
	foundExecuteResult := false
	for _, event := range auditStore.snapshot() {
		if event.Action == "permission_decision" && event.Decision == corepkg.DecisionAsk && event.ReasonCode == policyReasonRiskRule {
			foundPermissionDecisionAsk = true
			if got := event.Metadata["sandbox_enabled"]; got != "true" {
				t.Fatalf("expected ask permission_decision sandbox_enabled=true, got %q", got)
			}
			if got := event.Metadata["sandbox_mode"]; got != "best_effort" {
				t.Fatalf("expected ask permission_decision sandbox_mode=best_effort, got %q", got)
			}
			if got := event.Metadata["sandbox_required_capable"]; got != "true" && got != "false" {
				t.Fatalf("expected ask permission_decision sandbox_required_capable boolean text, got %q", got)
			}
		}
		if event.Action == "tool_execute_start" && event.Metadata["tool_name"] == "ask_tool" {
			foundExecuteStart = true
			if got := event.Metadata["sandbox_enabled"]; got != "true" {
				t.Fatalf("expected ask tool_execute_start sandbox_enabled=true, got %q", got)
			}
			if got := event.Metadata["sandbox_mode"]; got != "best_effort" {
				t.Fatalf("expected ask tool_execute_start sandbox_mode=best_effort, got %q", got)
			}
			if got := event.Metadata["sandbox_required_capable"]; got != "true" && got != "false" {
				t.Fatalf("expected ask tool_execute_start sandbox_required_capable boolean text, got %q", got)
			}
		}
		if event.Action == "tool_execute_result" && event.Metadata["tool_name"] == "ask_tool" && event.Result == "ok" {
			foundExecuteResult = true
			if got := event.Metadata["sandbox_enabled"]; got != "true" {
				t.Fatalf("expected ask tool_execute_result sandbox_enabled=true, got %q", got)
			}
			if got := event.Metadata["sandbox_mode"]; got != "best_effort" {
				t.Fatalf("expected ask tool_execute_result sandbox_mode=best_effort, got %q", got)
			}
			if got := event.Metadata["sandbox_required_capable"]; got != "true" && got != "false" {
				t.Fatalf("expected ask tool_execute_result sandbox_required_capable boolean text, got %q", got)
			}
		}
	}
	if !foundPermissionDecisionAsk {
		t.Fatal("expected permission_decision audit event for ask tool")
	}
	if !foundExecuteStart {
		t.Fatal("expected tool_execute_start audit event for ask tool")
	}
	if !foundExecuteResult {
		t.Fatal("expected successful tool_execute_result audit event for ask tool")
	}
}
