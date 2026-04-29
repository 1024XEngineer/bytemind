package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	sandboxpkg "github.com/1024XEngineer/bytemind/internal/sandbox"
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

func TestSubprocessWorkerRequiredModeLaunchFailureReturnsPermissionDenied(t *testing.T) {
	worker := subprocessWorker{
		fallback: inProcessWorker{},
		invoker: osExecWorkerInvoker{
			executablePath: "/tmp/bytemind",
			goos:           "freebsd",
			lookPath: func(string) (string, error) {
				t.Fatal("lookPath should not be called on unsupported OS")
				return "", nil
			},
		},
	}

	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"git status"}`),
		Execution: &ExecutionContext{
			Workspace:         t.TempDir(),
			SandboxEnabled:    true,
			SystemSandboxMode: "required",
			ApprovalMode:      "interactive",
			ApprovalPolicy:    "never",
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "git status"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected required-mode launch failure")
	}
	execErr, ok := AsToolExecError(err)
	if !ok {
		t.Fatalf("expected ToolExecError, got %T (%v)", err, err)
	}
	if execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("expected permission_denied code, got %s (%v)", execErr.Code, err)
	}
	if !strings.Contains(strings.ToLower(execErr.Message), "required") {
		t.Fatalf("expected required-mode context in error message, got %q", execErr.Message)
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
			Approval: func(req ApprovalRequest) (ApprovalDecision, error) {
				approvalCalls++
				if !strings.Contains(strings.ToLower(req.Reason), "outside lease scope") {
					t.Fatalf("unexpected escalation reason: %q", req.Reason)
				}
				return ApprovalDecision{Disposition: ApprovalApproveOnce}, nil
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
			Approval: func(req ApprovalRequest) (ApprovalDecision, error) {
				approvalCalls++
				if !strings.Contains(strings.ToLower(req.Command), "go test") {
					t.Fatalf("unexpected approval command: %q", req.Command)
				}
				return ApprovalDecision{Disposition: ApprovalApproveOnce}, nil
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
	}, "off", "linux")
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

func TestBuildWorkerProcessEnvRequiredLinuxKeepsAllowlistedKeysOnly(t *testing.T) {
	env := buildWorkerProcessEnv([]string{
		"PATH=/usr/bin",
		"HOME=/home/test",
		"WINDIR=C:\\Windows",
		"OPENAI_API_KEY=secret",
	}, "required", "linux")
	joined := strings.Join(env, "\n")
	if strings.Contains(joined, "OPENAI_API_KEY=") {
		t.Fatalf("expected sensitive key to be removed in required mode, got %q", joined)
	}
	if strings.Contains(joined, "WINDIR=") {
		t.Fatalf("expected non-linux allowlist key to be removed, got %q", joined)
	}
	if !strings.Contains(joined, "PATH=/usr/bin") {
		t.Fatalf("expected PATH to remain, got %q", joined)
	}
	if !strings.Contains(joined, "BYTEMIND_SANDBOX_WORKER=1") {
		t.Fatalf("expected sandbox worker marker, got %q", joined)
	}
}

func TestResolveLaunchFailsClosedWhenRequiredBackendUnavailable(t *testing.T) {
	invoker := osExecWorkerInvoker{
		executablePath: "/tmp/bytemind",
		goos:           "freebsd",
		lookPath: func(string) (string, error) {
			t.Fatal("lookPath should not be called on unsupported OS")
			return "", nil
		},
	}
	_, err := invoker.resolveLaunch(workerRPCRequest{
		Execution: workerRPCExecutionContext{
			SystemSandboxMode: "required",
		},
	})
	if err == nil {
		t.Fatal("expected required mode to fail closed when backend is unavailable")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "required") {
		t.Fatalf("expected required-mode error, got %v", err)
	}
}

func TestResolveLaunchWindowsBestEffortKeepsDirectExecutableAndTracksBackend(t *testing.T) {
	invoker := osExecWorkerInvoker{
		executablePath: `C:\bytemind.exe`,
		goos:           "windows",
		lookPath: func(string) (string, error) {
			t.Fatal("lookPath should not be called for windows job-object backend")
			return "", nil
		},
	}
	launch, err := invoker.resolveLaunch(workerRPCRequest{
		Execution: workerRPCExecutionContext{
			Workspace:         `C:\workspace`,
			SystemSandboxMode: "best_effort",
		},
	})
	if err != nil {
		t.Fatalf("resolve launch: %v", err)
	}
	if launch.Path != `C:\bytemind.exe` {
		t.Fatalf("expected direct executable path for windows backend, got %q", launch.Path)
	}
	if len(launch.Args) != 3 || launch.Args[0] != `C:\bytemind.exe` || launch.Args[1] != sandboxWorkerSubcommand || launch.Args[2] != sandboxWorkerStdioFlag {
		t.Fatalf("unexpected windows launch args: %#v", launch.Args)
	}
	if launch.SystemSandboxBackendName != "windows_job_object" {
		t.Fatalf("expected windows_job_object backend marker, got %#v", launch)
	}
	if launch.SystemSandboxMode != "best_effort" {
		t.Fatalf("expected best_effort mode marker, got %#v", launch)
	}
}

func TestResolveLaunchWindowsRequiredUsesJobObjectBackend(t *testing.T) {
	invoker := osExecWorkerInvoker{
		executablePath: `C:\bytemind.exe`,
		goos:           "windows",
		lookPath: func(string) (string, error) {
			t.Fatal("lookPath should not be called for windows job-object backend")
			return "", nil
		},
	}
	launch, err := invoker.resolveLaunch(workerRPCRequest{
		Execution: workerRPCExecutionContext{
			Workspace:         `C:\workspace`,
			SystemSandboxMode: "required",
		},
	})
	if err != nil {
		t.Fatalf("resolve launch: %v", err)
	}
	if launch.Path != `C:\bytemind.exe` {
		t.Fatalf("expected direct executable path for windows backend, got %q", launch.Path)
	}
	if len(launch.Args) != 3 || launch.Args[0] != `C:\bytemind.exe` || launch.Args[1] != sandboxWorkerSubcommand || launch.Args[2] != sandboxWorkerStdioFlag {
		t.Fatalf("unexpected windows launch args: %#v", launch.Args)
	}
	if launch.SystemSandboxBackendName != "windows_job_object" {
		t.Fatalf("expected windows_job_object backend marker, got %#v", launch)
	}
	if launch.SystemSandboxMode != "required" {
		t.Fatalf("expected required mode marker, got %#v", launch)
	}
}

func TestResolveLaunchBestEffortFallsBackWithoutBackend(t *testing.T) {
	invoker := osExecWorkerInvoker{
		executablePath: "/tmp/bytemind",
		goos:           "linux",
		lookPath: func(string) (string, error) {
			return "", errors.New("missing backend")
		},
	}
	launch, err := invoker.resolveLaunch(workerRPCRequest{
		Execution: workerRPCExecutionContext{
			Workspace:         "/tmp/workspace",
			SystemSandboxMode: "best_effort",
		},
	})
	if err != nil {
		t.Fatalf("resolve launch: %v", err)
	}
	if launch.Path != "/tmp/bytemind" {
		t.Fatalf("expected fallback executable path, got %q", launch.Path)
	}
	if len(launch.Args) != 3 || launch.Args[1] != sandboxWorkerSubcommand || launch.Args[2] != sandboxWorkerStdioFlag {
		t.Fatalf("unexpected fallback args: %#v", launch.Args)
	}
	if launch.Dir != "/tmp/workspace" {
		t.Fatalf("expected workspace dir propagation, got %q", launch.Dir)
	}
}

func TestResolveLaunchUsesLinuxBackendWhenAvailable(t *testing.T) {
	invoker := osExecWorkerInvoker{
		executablePath: "/tmp/bytemind",
		goos:           "linux",
		lookPath: func(name string) (string, error) {
			if name != "unshare" {
				t.Fatalf("unexpected binary lookup: %q", name)
			}
			return "/usr/bin/unshare", nil
		},
	}
	launch, err := invoker.resolveLaunch(workerRPCRequest{
		Execution: workerRPCExecutionContext{
			SystemSandboxMode: "best_effort",
		},
	})
	if err != nil {
		t.Fatalf("resolve launch: %v", err)
	}
	if launch.Path != "/usr/bin/unshare" {
		t.Fatalf("expected unshare backend, got %q", launch.Path)
	}
	expected := append([]string{"/usr/bin/unshare"}, append(linuxSystemSandboxWorkerArgs(), "/tmp/bytemind", sandboxWorkerSubcommand, sandboxWorkerStdioFlag)...)
	if strings.Join(launch.Args, "\n") != strings.Join(expected, "\n") {
		t.Fatalf("unexpected backend args:\nwant: %#v\ngot:  %#v", expected, launch.Args)
	}
	for _, arg := range launch.Args {
		if arg == "--net" {
			t.Fatalf("worker launch should not force --net isolation: %#v", launch.Args)
		}
	}
}

func TestResolveLaunchRequiredLinuxWrapsWorkerWithFilesystemIsolation(t *testing.T) {
	invoker := osExecWorkerInvoker{
		executablePath: "/tmp/bytemind",
		goos:           "linux",
		lookPath: func(name string) (string, error) {
			if name != "unshare" {
				t.Fatalf("unexpected binary lookup: %q", name)
			}
			return "/usr/bin/unshare", nil
		},
	}
	launch, err := invoker.resolveLaunch(workerRPCRequest{
		Execution: workerRPCExecutionContext{
			Workspace:         "/tmp/workspace",
			WritableRoots:     []string{"/tmp/workspace/out"},
			SystemSandboxMode: "required",
		},
	})
	if err != nil {
		t.Fatalf("resolve launch: %v", err)
	}
	if launch.Path != "/usr/bin/unshare" {
		t.Fatalf("expected unshare backend, got %q", launch.Path)
	}
	if len(launch.Args) < 5 {
		t.Fatalf("unexpected launch args: %#v", launch.Args)
	}
	last := launch.Args[len(launch.Args)-1]
	if !strings.Contains(last, "mount -o remount,ro /") {
		t.Fatalf("expected filesystem isolation wrapper in command, got %#v", launch.Args)
	}
	if !strings.Contains(last, "exec '/tmp/bytemind' 'worker' '--sandbox-stdio'") {
		t.Fatalf("expected worker exec bootstrap in wrapped command, got %q", last)
	}
}

func TestResolveLaunchDarwinUsesSandboxExecProfile(t *testing.T) {
	invoker := osExecWorkerInvoker{
		executablePath: "/tmp/bytemind",
		goos:           "darwin",
		lookPath: func(name string) (string, error) {
			if name != "sandbox-exec" {
				t.Fatalf("unexpected binary lookup: %q", name)
			}
			return "/usr/bin/sandbox-exec", nil
		},
	}
	launch, err := invoker.resolveLaunch(workerRPCRequest{
		Execution: workerRPCExecutionContext{
			Workspace:         "/tmp/workspace",
			WritableRoots:     []string{"/tmp/workspace/out"},
			SystemSandboxMode: "best_effort",
		},
	})
	if err != nil {
		t.Fatalf("resolve launch: %v", err)
	}
	if launch.Path != "/usr/bin/sandbox-exec" {
		t.Fatalf("expected sandbox-exec backend, got %q", launch.Path)
	}
	if len(launch.Args) != 6 {
		t.Fatalf("unexpected launch args: %#v", launch.Args)
	}
	if launch.Args[1] != "-p" {
		t.Fatalf("expected -p profile arg, got %#v", launch.Args)
	}
	if !strings.Contains(launch.Args[2], "(deny default)") {
		t.Fatalf("expected sandbox profile payload, got %q", launch.Args[2])
	}
	if !strings.Contains(launch.Args[2], "(allow network*)") {
		t.Fatalf("expected best_effort darwin worker profile to allow network, got %q", launch.Args[2])
	}
	if launch.Args[3] != "/tmp/bytemind" || launch.Args[4] != sandboxWorkerSubcommand || launch.Args[5] != sandboxWorkerStdioFlag {
		t.Fatalf("unexpected worker command args: %#v", launch.Args)
	}
}

func TestResolveLaunchDarwinRequiredDisablesNetworkInProfile(t *testing.T) {
	invoker := osExecWorkerInvoker{
		executablePath: "/tmp/bytemind",
		goos:           "darwin",
		lookPath: func(name string) (string, error) {
			if name != "sandbox-exec" {
				t.Fatalf("unexpected binary lookup: %q", name)
			}
			return "/usr/bin/sandbox-exec", nil
		},
	}
	launch, err := invoker.resolveLaunch(workerRPCRequest{
		Execution: workerRPCExecutionContext{
			Workspace:         "/tmp/workspace",
			WritableRoots:     []string{"/tmp/workspace/out"},
			SystemSandboxMode: "required",
		},
	})
	if err != nil {
		t.Fatalf("resolve launch: %v", err)
	}
	if launch.Path != "/usr/bin/sandbox-exec" {
		t.Fatalf("expected sandbox-exec backend, got %q", launch.Path)
	}
	if len(launch.Args) != 6 {
		t.Fatalf("unexpected launch args: %#v", launch.Args)
	}
	if strings.Contains(launch.Args[2], "(allow network*)") {
		t.Fatalf("expected required darwin worker profile to deny network, got %q", launch.Args[2])
	}
}

func TestNewDefaultExecutorWorkerSkipsSubprocessInsideWorkerEnv(t *testing.T) {
	t.Setenv(sandboxWorkerEnvKey, sandboxWorkerEnvValue)
	worker := newDefaultExecutorWorker(DefaultRegistry(), maxCharsOutputNormalizer{})
	if _, ok := worker.(inProcessWorker); !ok {
		t.Fatalf("expected in-process worker when %s is set, got %T", sandboxWorkerEnvKey, worker)
	}
}

func TestWorkerExecutionContextRoundTripsSystemSandboxMode(t *testing.T) {
	payload := encodeWorkerExecutionContext(&ExecutionContext{
		Workspace:         "C:\\workspace",
		SandboxEnabled:    true,
		SystemSandboxMode: "required",
	})
	decoded := decodeWorkerExecutionContext(payload)
	if decoded == nil {
		t.Fatal("expected decoded execution context")
	}
	if decoded.SystemSandboxMode != "required" {
		t.Fatalf("expected system sandbox mode to roundtrip, got %#v", decoded)
	}
}
