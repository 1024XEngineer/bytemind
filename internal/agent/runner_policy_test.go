package agent

import (
	"context"
	"encoding/json"
	"io"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
	corepkg "github.com/1024XEngineer/bytemind/internal/core"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
	storagepkg "github.com/1024XEngineer/bytemind/internal/storage"
	"github.com/1024XEngineer/bytemind/internal/tools"
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
	deniedPayload := findToolResultPayloadByReasonCode(t, sess, policyReasonExplicitDeny)
	if deniedPayload.SystemSandbox.Mode != "best_effort" {
		t.Fatalf("expected denied tool_result sandbox mode best_effort, got %#v", deniedPayload.SystemSandbox)
	}
	if deniedPayload.SystemSandbox.Backend == "" {
		t.Fatalf("expected denied tool_result sandbox backend to be set, got %#v", deniedPayload.SystemSandbox)
	}
	if deniedPayload.SystemSandbox.CapabilityLevel == "" {
		t.Fatalf("expected denied tool_result sandbox capability_level to be set, got %#v", deniedPayload.SystemSandbox)
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
			assertSandboxNetworkIsolationMetadata(t, event.Metadata)
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
			assertSandboxNetworkIsolationMetadata(t, event.Metadata)
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
			decision, approvalErr := execCtx.Approval(tools.ApprovalRequest{
				ToolName: "ask_tool",
				Command:  "ask_tool",
				Reason:   "high-risk tool requires approval",
			})
			if approvalErr != nil {
				t.Fatalf("unexpected approval error: %v", approvalErr)
			}
			approvalRequested = true
			if !decision.Approved() {
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
		Approval: func(req tools.ApprovalRequest) (tools.ApprovalDecision, error) {
			if req.Command != "ask_tool" {
				t.Fatalf("unexpected approval command: %q", req.Command)
			}
			return tools.ApprovalDecision{Disposition: tools.ApprovalApproveOnce}, nil
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
			assertSandboxNetworkIsolationMetadata(t, event.Metadata)
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
			assertSandboxNetworkIsolationMetadata(t, event.Metadata)
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
			assertSandboxNetworkIsolationMetadata(t, event.Metadata)
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

func TestRunPromptPolicyGatewaySandboxGuardDeniesWebFetchBeforeExecution(t *testing.T) {
	testCases := []struct {
		name            string
		backend         string
		capabilityLevel string
	}{
		{name: "windows_required", backend: "windows_job_object", capabilityLevel: "guarded"},
		{name: "linux_required", backend: "linux_unshare", capabilityLevel: "full"},
		{name: "darwin_required", backend: "darwin_sandbox_exec", capabilityLevel: "full"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			caps := tools.SystemSandboxBackendCapabilitiesForName(tc.backend)
			original := resolveAgentSystemSandboxRuntimeStatus
			resolveAgentSystemSandboxRuntimeStatus = func(enabled bool, mode string) (tools.SystemSandboxRuntimeStatus, error) {
				if !enabled {
					return tools.SystemSandboxRuntimeStatus{}, nil
				}
				return tools.SystemSandboxRuntimeStatus{
					Mode:                   "required",
					BackendEnabled:         true,
					BackendName:            tc.backend,
					RequiredCapable:        true,
					CapabilityLevel:        tc.capabilityLevel,
					ShellNetworkIsolation:  caps.ShellNetworkIsolation,
					WorkerNetworkIsolation: caps.WorkerNetworkIsolation,
					Message:                `system sandbox backend "` + tc.backend + `" is active`,
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

			client := &recordingClient{replies: []llm.Message{
				{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{{
						ID:   "call-web-fetch-" + tc.name,
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      "web_fetch",
							Arguments: `{"url":"https://example.com"}`,
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
					ApprovalPolicy:    "always",
					SandboxEnabled:    true,
					SystemSandboxMode: "required",
				},
				Client:        client,
				Store:         store,
				Registry:      tools.DefaultRegistry(),
				PolicyGateway: NewDefaultPolicyGateway(),
				AuditStore:    auditStore,
				Stdin:         strings.NewReader(""),
				Stdout:        io.Discard,
			})

			answer, err := runner.RunPrompt(context.Background(), sess, "fetch web source", "build", io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			if answer != "Policy handled." {
				t.Fatalf("unexpected answer: %q", answer)
			}

			deniedPayload := findToolResultPayloadByReasonCode(t, sess, policyReasonSandboxGuard)
			if deniedPayload.SystemSandbox.Mode != "required" {
				t.Fatalf("expected denied tool_result sandbox mode required, got %#v", deniedPayload.SystemSandbox)
			}
			if deniedPayload.SystemSandbox.Backend != tc.backend {
				t.Fatalf("expected denied tool_result sandbox backend %s, got %#v", tc.backend, deniedPayload.SystemSandbox)
			}
			if deniedPayload.SystemSandbox.ShellNetworkIsolation != caps.ShellNetworkIsolation {
				t.Fatalf("expected denied tool_result shell_network_isolation=%t, got %#v", caps.ShellNetworkIsolation, deniedPayload.SystemSandbox)
			}
			if deniedPayload.SystemSandbox.WorkerNetworkIsolation != caps.WorkerNetworkIsolation {
				t.Fatalf("expected denied tool_result worker_network_isolation=%t, got %#v", caps.WorkerNetworkIsolation, deniedPayload.SystemSandbox)
			}

			foundPermissionDecision := false
			foundExecuteStart := false
			foundDeniedResult := false
			for _, event := range auditStore.snapshot() {
				if event.Action == "permission_decision" && event.Metadata["tool_name"] == "web_fetch" {
					if event.ReasonCode != policyReasonSandboxGuard || event.Decision != corepkg.DecisionDeny {
						t.Fatalf("expected sandbox_guard deny decision, got %+v", event)
					}
					assertSandboxNetworkIsolationMetadataEqual(t, event.Metadata, caps.ShellNetworkIsolation, caps.WorkerNetworkIsolation)
					foundPermissionDecision = true
				}
				if event.Action == "tool_execute_start" && event.Metadata["tool_name"] == "web_fetch" {
					foundExecuteStart = true
				}
				if event.Action == "tool_execute_result" && event.Metadata["tool_name"] == "web_fetch" && event.Result == "denied" {
					assertSandboxNetworkIsolationMetadataEqual(t, event.Metadata, caps.ShellNetworkIsolation, caps.WorkerNetworkIsolation)
					foundDeniedResult = true
				}
			}
			if !foundPermissionDecision {
				t.Fatal("expected permission_decision audit event for web_fetch")
			}
			if foundExecuteStart {
				t.Fatal("did not expect tool_execute_start audit event for policy-denied web_fetch")
			}
			if !foundDeniedResult {
				t.Fatal("expected denied tool_execute_result audit event for web_fetch")
			}
		})
	}
}

func TestRunPromptPolicyGatewaySandboxGuardDeniesNetworkTargetRunShellBeforeExecution(t *testing.T) {
	testCases := []struct {
		name            string
		backend         string
		capabilityLevel string
	}{
		{name: "windows_required", backend: "windows_job_object", capabilityLevel: "guarded"},
		{name: "darwin_required", backend: "darwin_sandbox_exec", capabilityLevel: "full"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			caps := tools.SystemSandboxBackendCapabilitiesForName(tc.backend)
			original := resolveAgentSystemSandboxRuntimeStatus
			resolveAgentSystemSandboxRuntimeStatus = func(enabled bool, mode string) (tools.SystemSandboxRuntimeStatus, error) {
				if !enabled {
					return tools.SystemSandboxRuntimeStatus{}, nil
				}
				return tools.SystemSandboxRuntimeStatus{
					Mode:                   "required",
					BackendEnabled:         true,
					BackendName:            tc.backend,
					RequiredCapable:        true,
					CapabilityLevel:        tc.capabilityLevel,
					ShellNetworkIsolation:  caps.ShellNetworkIsolation,
					WorkerNetworkIsolation: caps.WorkerNetworkIsolation,
					Message:                `system sandbox backend "` + tc.backend + `" is active`,
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

			client := &recordingClient{replies: []llm.Message{
				{
					Role: "assistant",
					ToolCalls: []llm.ToolCall{{
						ID:   "call-run-shell-" + tc.name,
						Type: "function",
						Function: llm.ToolFunctionCall{
							Name:      "run_shell",
							Arguments: `{"command":"curl https://example.com/data"}`,
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
					ApprovalPolicy:    "always",
					SandboxEnabled:    true,
					SystemSandboxMode: "required",
				},
				Client:        client,
				Store:         store,
				Registry:      tools.DefaultRegistry(),
				PolicyGateway: NewDefaultPolicyGateway(),
				AuditStore:    auditStore,
				Stdin:         strings.NewReader(""),
				Stdout:        io.Discard,
			})

			answer, err := runner.RunPrompt(context.Background(), sess, "run curl command", "build", io.Discard)
			if err != nil {
				t.Fatal(err)
			}
			if answer != "Policy handled." {
				t.Fatalf("unexpected answer: %q", answer)
			}

			deniedPayload := findToolResultPayloadByReasonCode(t, sess, policyReasonSandboxGuard)
			if deniedPayload.SystemSandbox.Mode != "required" {
				t.Fatalf("expected denied tool_result sandbox mode required, got %#v", deniedPayload.SystemSandbox)
			}
			if deniedPayload.SystemSandbox.Backend != tc.backend {
				t.Fatalf("expected denied tool_result sandbox backend %s, got %#v", tc.backend, deniedPayload.SystemSandbox)
			}
			if deniedPayload.SystemSandbox.ShellNetworkIsolation != caps.ShellNetworkIsolation {
				t.Fatalf("expected denied tool_result shell_network_isolation=%t, got %#v", caps.ShellNetworkIsolation, deniedPayload.SystemSandbox)
			}
			if deniedPayload.SystemSandbox.WorkerNetworkIsolation != caps.WorkerNetworkIsolation {
				t.Fatalf("expected denied tool_result worker_network_isolation=%t, got %#v", caps.WorkerNetworkIsolation, deniedPayload.SystemSandbox)
			}

			foundPermissionDecision := false
			foundExecuteStart := false
			foundDeniedResult := false
			for _, event := range auditStore.snapshot() {
				if event.Action == "permission_decision" && event.Metadata["tool_name"] == "run_shell" {
					if event.ReasonCode != policyReasonSandboxGuard || event.Decision != corepkg.DecisionDeny {
						t.Fatalf("expected sandbox_guard deny decision, got %+v", event)
					}
					assertSandboxNetworkIsolationMetadataEqual(t, event.Metadata, caps.ShellNetworkIsolation, caps.WorkerNetworkIsolation)
					foundPermissionDecision = true
				}
				if event.Action == "tool_execute_start" && event.Metadata["tool_name"] == "run_shell" {
					foundExecuteStart = true
				}
				if event.Action == "tool_execute_result" && event.Metadata["tool_name"] == "run_shell" && event.Result == "denied" {
					assertSandboxNetworkIsolationMetadataEqual(t, event.Metadata, caps.ShellNetworkIsolation, caps.WorkerNetworkIsolation)
					foundDeniedResult = true
				}
			}
			if !foundPermissionDecision {
				t.Fatal("expected permission_decision audit event for run_shell")
			}
			if foundExecuteStart {
				t.Fatal("did not expect tool_execute_start audit event for policy-denied run_shell")
			}
			if !foundDeniedResult {
				t.Fatal("expected denied tool_execute_result audit event for run_shell")
			}
		})
	}
}

func assertSandboxNetworkIsolationMetadata(t *testing.T, metadata map[string]string) {
	t.Helper()
	if got := strings.TrimSpace(metadata["sandbox_shell_network_isolation"]); got != "true" && got != "false" {
		t.Fatalf("expected sandbox_shell_network_isolation boolean text, got %q", got)
	}
	if got := strings.TrimSpace(metadata["sandbox_worker_network_isolation"]); got != "true" && got != "false" {
		t.Fatalf("expected sandbox_worker_network_isolation boolean text, got %q", got)
	}
}

func assertSandboxNetworkIsolationMetadataEqual(t *testing.T, metadata map[string]string, shell, worker bool) {
	t.Helper()
	assertSandboxNetworkIsolationMetadata(t, metadata)
	if got := strings.TrimSpace(metadata["sandbox_shell_network_isolation"]); got != strconv.FormatBool(shell) {
		t.Fatalf("expected sandbox_shell_network_isolation=%t, got %q", shell, got)
	}
	if got := strings.TrimSpace(metadata["sandbox_worker_network_isolation"]); got != strconv.FormatBool(worker) {
		t.Fatalf("expected sandbox_worker_network_isolation=%t, got %q", worker, got)
	}
}

type policyToolResultPayload struct {
	ReasonCode    string `json:"reason_code"`
	SystemSandbox struct {
		Mode                   string `json:"mode"`
		Backend                string `json:"backend"`
		RequiredCapable        bool   `json:"required_capable"`
		CapabilityLevel        string `json:"capability_level"`
		ShellNetworkIsolation  bool   `json:"shell_network_isolation"`
		WorkerNetworkIsolation bool   `json:"worker_network_isolation"`
		Fallback               bool   `json:"fallback"`
		Status                 string `json:"status"`
		FallbackReason         string `json:"fallback_reason"`
	} `json:"system_sandbox"`
}

func findToolResultPayloadByReasonCode(t *testing.T, sess *session.Session, reasonCode string) policyToolResultPayload {
	t.Helper()
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
	wantReason := strings.TrimSpace(reasonCode)
	for _, message := range sess.Messages {
		if message.Role != llm.RoleUser {
			continue
		}
		content := strings.TrimSpace(message.Content)
		if content == "" || !strings.HasPrefix(content, "{") {
			continue
		}
		var payload policyToolResultPayload
		if err := json.Unmarshal([]byte(content), &payload); err != nil {
			continue
		}
		gotReason := strings.TrimSpace(payload.ReasonCode)
		if wantReason == "" && gotReason != "" {
			continue
		}
		if wantReason != "" && gotReason != wantReason {
			continue
		}
		return payload
	}
	t.Fatalf("expected tool_result payload with reason_code=%q, got messages=%#v", wantReason, sess.Messages)
	return policyToolResultPayload{}
}
