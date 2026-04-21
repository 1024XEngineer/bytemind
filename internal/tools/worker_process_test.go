package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	sandboxpkg "bytemind/internal/sandbox"
)

type fakeWorkerInvoker struct {
	called  bool
	lastReq workerRPCRequest
	resp    workerRPCResponse
	stderr  string
	err     error
}

func (f *fakeWorkerInvoker) Invoke(_ context.Context, req workerRPCRequest) (workerRPCResponse, string, error) {
	f.called = true
	f.lastReq = req
	return f.resp, f.stderr, f.err
}

func TestShouldUseSubprocessWorker(t *testing.T) {
	if shouldUseSubprocessWorker(nil) {
		t.Fatal("nil execution context should not use subprocess worker")
	}
	if shouldUseSubprocessWorker(&ExecutionContext{SandboxEnabled: false, ApprovalMode: "away"}) {
		t.Fatal("sandbox disabled should not use subprocess worker")
	}
	if !shouldUseSubprocessWorker(&ExecutionContext{SandboxEnabled: true, ApprovalMode: "away"}) {
		t.Fatal("away mode with sandbox enabled should use subprocess worker")
	}
	if !shouldUseSubprocessWorker(&ExecutionContext{SandboxEnabled: true, ApprovalMode: "interactive", ApprovalPolicy: "never"}) {
		t.Fatal("approval_policy=never with sandbox enabled should use subprocess worker")
	}
	if !shouldUseSubprocessWorker(&ExecutionContext{SandboxEnabled: true, ApprovalMode: "interactive", ApprovalPolicy: "on-request"}) {
		t.Fatal("sandbox enabled should use subprocess worker in interactive mode")
	}
}

func TestSubprocessWorkerFallsBackToInProcessWhenNotEligible(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name:   "run_shell",
		result: `{"ok":true,"path":"fallback"}`,
	})
	invoker := &fakeWorkerInvoker{
		resp: workerRPCResponse{Output: `{"ok":true,"path":"subprocess"}`},
	}
	worker := subprocessWorker{
		fallback: inProcessWorker{registry: registry},
		invoker:  invoker,
	}

	out, err := worker.Run(context.Background(), workerRunRequest{
		ToolName:  "run_shell",
		RawArgs:   json.RawMessage(`{}`),
		Execution: &ExecutionContext{SandboxEnabled: false},
	})
	if err != nil {
		t.Fatalf("worker run: %v", err)
	}
	if invoker.called {
		t.Fatal("expected in-process fallback path, not subprocess invoker")
	}
	if out != `{"ok":true,"path":"fallback"}` {
		t.Fatalf("unexpected fallback output: %q", out)
	}
}

func TestSubprocessWorkerFailsClosedWhenSandboxEnabledAndInvokerMissing(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "run_shell",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			t.Fatal("in-process fallback must not run when sandbox worker invoker is unavailable")
			return "", nil
		},
	})
	worker := subprocessWorker{
		fallback: inProcessWorker{registry: registry},
		invoker:  nil,
	}

	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"git status"}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			ApprovalMode:   "away",
			ApprovalPolicy: "on-request",
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "git status"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected fail-closed error when subprocess invoker is unavailable")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorInternal {
		t.Fatalf("expected internal error code, got %s", execErr.Code)
	}
	if !strings.Contains(execErr.Message, "subprocess is unavailable") {
		t.Fatalf("unexpected error message: %q", execErr.Message)
	}
}

func TestSubprocessWorkerUsesInvokerWhenEligible(t *testing.T) {
	invoker := &fakeWorkerInvoker{
		resp: workerRPCResponse{Output: `{"ok":true,"path":"subprocess"}`},
	}
	worker := subprocessWorker{
		fallback: inProcessWorker{},
		invoker:  invoker,
	}

	out, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"git status"}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			ApprovalMode:   "away",
			ApprovalPolicy: "on-request",
			Workspace:      "C:\\workspace",
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "git status"},
			},
		},
	})
	if err != nil {
		t.Fatalf("worker run: %v", err)
	}
	if !invoker.called {
		t.Fatal("expected subprocess invoker call")
	}
	if invoker.lastReq.Execution.Workspace != "C:\\workspace" {
		t.Fatalf("expected encoded workspace, got %#v", invoker.lastReq.Execution)
	}
	if out != `{"ok":true,"path":"subprocess"}` {
		t.Fatalf("unexpected subprocess output: %q", out)
	}
}

func TestSubprocessWorkerPropagatesWorkerError(t *testing.T) {
	invoker := &fakeWorkerInvoker{
		resp: workerRPCResponse{
			Error: &workerRPCError{
				Code:      string(ToolErrorPermissionDenied),
				Message:   "blocked by sandbox lease",
				Retryable: false,
			},
		},
	}
	worker := subprocessWorker{
		fallback: inProcessWorker{},
		invoker:  invoker,
	}

	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"git status"}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			ApprovalMode:   "away",
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "git status"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected worker error")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("unexpected error code: %s", execErr.Code)
	}
}

func TestSubprocessWorkerPreApprovesInteractiveEscalation(t *testing.T) {
	invoker := &fakeWorkerInvoker{
		resp: workerRPCResponse{Output: `{"ok":true}`},
	}
	approvalCalls := 0
	worker := subprocessWorker{
		fallback: inProcessWorker{},
		invoker:  invoker,
	}

	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"git status"}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			ApprovalMode:   "interactive",
			ApprovalPolicy: "on-request",
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "go run", ArgsPattern: []string{"./cmd/app"}},
			},
			Approval: func(req ApprovalRequest) (bool, error) {
				approvalCalls++
				if !strings.Contains(strings.ToLower(req.Reason), "outside lease scope") {
					t.Fatalf("unexpected escalation reason: %q", req.Reason)
				}
				return true, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("expected interactive escalation to be pre-approved, got %v", err)
	}
	if approvalCalls != 1 {
		t.Fatalf("expected one approval call, got %d", approvalCalls)
	}
	if !invoker.called {
		t.Fatal("expected subprocess invoker to run after approval")
	}
	if !invoker.lastReq.Execution.SandboxEscalationApproved {
		t.Fatalf("expected sandbox escalation approval marker in worker request, got %#v", invoker.lastReq.Execution)
	}
	if !invoker.lastReq.Execution.SkipShellApproval {
		t.Fatalf("expected shell approval skip marker in worker request, got %#v", invoker.lastReq.Execution)
	}
}

func TestSubprocessWorkerPreApprovesInteractiveShellRiskOnce(t *testing.T) {
	invoker := &fakeWorkerInvoker{
		resp: workerRPCResponse{Output: `{"ok":true}`},
	}
	approvalCalls := 0
	worker := subprocessWorker{
		fallback: inProcessWorker{},
		invoker:  invoker,
	}

	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"go test ./..."}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			ApprovalMode:   "interactive",
			ApprovalPolicy: "on-request",
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "go test", ArgsPattern: []string{"./..."}},
			},
			Approval: func(req ApprovalRequest) (bool, error) {
				approvalCalls++
				if !strings.Contains(strings.ToLower(req.Command), "go test") {
					t.Fatalf("unexpected approval command: %q", req.Command)
				}
				return true, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("expected shell risk pre-approval to allow run, got %v", err)
	}
	if approvalCalls != 1 {
		t.Fatalf("expected exactly one approval prompt in parent process, got %d", approvalCalls)
	}
	if !invoker.called {
		t.Fatal("expected subprocess invoker call after parent approval")
	}
	if !invoker.lastReq.Execution.SkipShellApproval {
		t.Fatalf("expected skip_shell_approval marker in worker request, got %#v", invoker.lastReq.Execution)
	}
}

func TestSubprocessWorkerAwayModeDeniesShellRiskBeforeInvoke(t *testing.T) {
	invoker := &fakeWorkerInvoker{
		resp: workerRPCResponse{Output: `{"ok":true}`},
	}
	worker := subprocessWorker{
		fallback: inProcessWorker{},
		invoker:  invoker,
	}

	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"go test ./..."}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			ApprovalMode:   "away",
			AwayPolicy:     "auto_deny_continue",
			ApprovalPolicy: "on-request",
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "go test", ArgsPattern: []string{"./..."}},
			},
		},
	})
	if err == nil {
		t.Fatal("expected away mode to deny approval-required shell command before subprocess invoke")
	}
	if invoker.called {
		t.Fatal("expected subprocess invoker to be skipped on away-mode denial")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T", err)
	}
	if execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("unexpected error code: %s", execErr.Code)
	}
	if !strings.Contains(strings.ToLower(execErr.Message), "away mode") {
		t.Fatalf("unexpected away denial message: %q", execErr.Message)
	}
}

func TestRunWorkerProcessWithRegistryReturnsStructuredErrorPayload(t *testing.T) {
	request := workerRPCRequest{
		Version:   sandboxWorkerProtocolV1,
		ToolName:  "unknown_tool",
		RawArgs:   json.RawMessage(`{}`),
		Execution: workerRPCExecutionContext{},
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	var out bytes.Buffer
	err = runWorkerProcessWithRegistry(context.Background(), bytes.NewReader(payload), &out, DefaultRegistry())
	if err != nil {
		t.Fatalf("run worker process: %v", err)
	}

	var response workerRPCResponse
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.Error == nil {
		t.Fatalf("expected structured worker error, got %q", out.String())
	}
	if response.Error.Code != string(ToolErrorInvalidArgs) {
		t.Fatalf("expected invalid_args error code, got %#v", response.Error)
	}
}

func TestRunWorkerProcessWithRegistryRejectsUnsupportedProtocolVersion(t *testing.T) {
	request := workerRPCRequest{
		Version:   "legacy",
		ToolName:  "run_shell",
		RawArgs:   json.RawMessage(`{"command":"echo ok"}`),
		Execution: workerRPCExecutionContext{},
	}
	payload, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}

	err = runWorkerProcessWithRegistry(context.Background(), bytes.NewReader(payload), &bytes.Buffer{}, DefaultRegistry())
	if err == nil {
		t.Fatal("expected unsupported protocol version error")
	}
	if !strings.Contains(err.Error(), "unsupported worker protocol version") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunWorkerProcessWithRegistryValidatesStreams(t *testing.T) {
	err := runWorkerProcessWithRegistry(context.Background(), nil, &bytes.Buffer{}, DefaultRegistry())
	if err == nil || !strings.Contains(err.Error(), "stdin") {
		t.Fatalf("expected stdin validation error, got %v", err)
	}

	err = runWorkerProcessWithRegistry(context.Background(), bytes.NewReader(nil), nil, DefaultRegistry())
	if err == nil || !strings.Contains(err.Error(), "stdout") {
		t.Fatalf("expected stdout validation error, got %v", err)
	}
}

func TestBuildWorkerProcessEnvStripsSensitiveKeys(t *testing.T) {
	env := buildWorkerProcessEnv([]string{
		"PATH=/bin",
		"BYTEMIND_API_KEY=secret",
		"BYTEMIND_PROVIDER_API_KEY=provider-secret",
		"BYTEMIND_SANDBOX_WORKER=0",
		"HOME=/home/test",
	})
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "BYTEMIND_API_KEY=") {
		t.Fatalf("expected BYTEMIND_API_KEY to be removed, got %q", joined)
	}
	if strings.Contains(joined, "BYTEMIND_PROVIDER_API_KEY=") {
		t.Fatalf("expected BYTEMIND_PROVIDER_API_KEY to be removed, got %q", joined)
	}
	if !strings.Contains(joined, "BYTEMIND_SANDBOX_WORKER=1") {
		t.Fatalf("expected sandbox worker marker to be injected, got %q", joined)
	}
}

func TestNewDefaultExecutorWorkerSkipsSubprocessInsideWorkerEnv(t *testing.T) {
	t.Setenv(sandboxWorkerEnvKey, sandboxWorkerEnvValue)
	worker := newDefaultExecutorWorker(DefaultRegistry(), maxCharsOutputNormalizer{})
	if _, ok := worker.(inProcessWorker); !ok {
		t.Fatalf("expected in-process worker when %s is set, got %T", sandboxWorkerEnvKey, worker)
	}
}
