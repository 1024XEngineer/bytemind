package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sandboxpkg "github.com/1024XEngineer/bytemind/internal/sandbox"
)

type workerTestDouble struct {
	called bool
	output string
	err    error
}

func (w *workerTestDouble) Run(_ context.Context, _ workerRunRequest) (string, error) {
	w.called = true
	return w.output, w.err
}

func TestShouldRouteToWorker(t *testing.T) {
	if !shouldRouteToWorker("run_shell", &ExecutionContext{SandboxEnabled: false}) {
		t.Fatal("expected run_shell to route to worker regardless of sandbox toggle")
	}
	if !shouldRouteToWorker("run_shell", &ExecutionContext{SandboxEnabled: true}) {
		t.Fatal("expected run_shell to route to worker in sandbox mode")
	}
	if !shouldRouteToWorker("read_file", &ExecutionContext{SandboxEnabled: true}) {
		t.Fatal("expected read_file to route to worker in sandbox mode")
	}
	if !shouldRouteToWorker("write_file", &ExecutionContext{SandboxEnabled: true}) {
		t.Fatal("expected write_file to route to worker in sandbox mode")
	}
	if !shouldRouteToWorker("list_files", &ExecutionContext{SandboxEnabled: true}) {
		t.Fatal("expected list_files to route to worker in sandbox mode")
	}
	if !shouldRouteToWorker("search_text", &ExecutionContext{SandboxEnabled: true}) {
		t.Fatal("expected search_text to route to worker in sandbox mode")
	}
	if !shouldRouteToWorker("replace_in_file", &ExecutionContext{SandboxEnabled: true}) {
		t.Fatal("expected replace_in_file to route to worker in sandbox mode")
	}
	if !shouldRouteToWorker("apply_patch", &ExecutionContext{SandboxEnabled: true}) {
		t.Fatal("expected apply_patch to route to worker in sandbox mode")
	}
	if !shouldRouteToWorker("web_fetch", &ExecutionContext{SandboxEnabled: true}) {
		t.Fatal("expected web_fetch to route to worker in sandbox mode")
	}
	if !shouldRouteToWorker("web_search", &ExecutionContext{SandboxEnabled: true}) {
		t.Fatal("expected web_search to route to worker in sandbox mode")
	}
	if shouldRouteToWorker("update_plan", &ExecutionContext{SandboxEnabled: true}) {
		t.Fatal("expected update_plan to stay in main executor path")
	}
}

func TestExecutorRoutesCoreToolsToWorker(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "run_shell",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			t.Fatal("tool should have been executed by worker route")
			return "", nil
		},
	})
	executor := NewExecutor(registry)
	worker := &workerTestDouble{output: `{"ok":true,"worker":true}`}
	executor.worker = worker

	out, err := executor.Execute(context.Background(), "run_shell", `{}`, &ExecutionContext{SandboxEnabled: false})
	if err != nil {
		t.Fatalf("executor execute: %v", err)
	}
	if !worker.called {
		t.Fatal("expected worker to be called for sandbox tool")
	}
	if out != `{"ok":true,"worker":true}` {
		t.Fatalf("unexpected worker output: %q", out)
	}
}

func TestExecutorKeepsNonSandboxToolsOnMainPath(t *testing.T) {
	registry := &Registry{}
	mainCalled := false
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "update_plan",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			mainCalled = true
			return `{"ok":true,"main":true}`, nil
		},
	})
	executor := NewExecutor(registry)
	worker := &workerTestDouble{output: `{"ok":true,"worker":true}`}
	executor.worker = worker

	out, err := executor.Execute(context.Background(), "update_plan", `{}`, &ExecutionContext{SandboxEnabled: true})
	if err != nil {
		t.Fatalf("executor execute: %v", err)
	}
	if worker.called {
		t.Fatal("did not expect worker to be called for non-sandbox tool")
	}
	if !mainCalled {
		t.Fatal("expected main execution path to run tool")
	}
	if out != `{"ok":true,"main":true}` {
		t.Fatalf("unexpected main output: %q", out)
	}
}

func TestInProcessWorkerAllowsRunShellWhenCommandInLeaseAllowlist(t *testing.T) {
	registry := &Registry{}
	called := false
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "run_shell",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			called = true
			return `{"ok":true}`, nil
		},
	})
	worker := inProcessWorker{registry: registry}
	out, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"go test ./..."}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			Workspace:      t.TempDir(),
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "go test", ArgsPattern: []string{"./..."}},
			},
		},
	})
	if err != nil {
		t.Fatalf("worker run: %v", err)
	}
	if !called {
		t.Fatal("expected underlying tool run when command is allowed")
	}
	if out != `{"ok":true}` {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestInProcessWorkerDeniesRunShellWhenCommandNotInAllowlist(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "run_shell",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			t.Fatal("tool should not run when command is blocked")
			return "", nil
		},
	})
	worker := inProcessWorker{registry: registry}
	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"go test ./..."}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			Workspace:      t.TempDir(),
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "go run", ArgsPattern: []string{"./cmd/app"}},
			},
		},
	})
	if err == nil {
		t.Fatal("expected command denial error")
	}
	execErr, ok := AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("expected permission denied tool error, got %#v", err)
	}
}

func TestInProcessWorkerEscalatesRunShellWhenCommandNotInAllowlist(t *testing.T) {
	registry := &Registry{}
	called := false
	approvalCalls := 0
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "run_shell",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			called = true
			return `{"ok":true}`, nil
		},
	})
	worker := inProcessWorker{registry: registry}
	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"go test ./..."}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			Workspace:      t.TempDir(),
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "go run", ArgsPattern: []string{"./cmd/app"}},
			},
			ApprovalPolicy: "on-request",
			ApprovalMode:   "interactive",
			Approval: func(req ApprovalRequest) (ApprovalDecision, error) {
				approvalCalls++
				if !strings.Contains(strings.ToLower(req.Reason), "outside lease scope") {
					t.Fatalf("expected escalation reason to explain outside lease scope, got %q", req.Reason)
				}
				return ApprovalDecision{Disposition: ApprovalApproveOnce}, nil
			},
		},
	})
	if err != nil {
		t.Fatalf("expected escalation approval path to allow run, got %v", err)
	}
	if approvalCalls != 1 {
		t.Fatalf("expected one approval call, got %d", approvalCalls)
	}
	if !called {
		t.Fatal("expected underlying tool to run after approval")
	}
}

func TestInProcessWorkerAllowsPreApprovedEscalationWithoutPrompt(t *testing.T) {
	registry := &Registry{}
	called := false
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "run_shell",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			called = true
			return `{"ok":true}`, nil
		},
	})
	worker := inProcessWorker{registry: registry}
	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"go test ./..."}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			Workspace:      t.TempDir(),
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "go run", ArgsPattern: []string{"./cmd/app"}},
			},
			ApprovalPolicy:            "on-request",
			ApprovalMode:              "interactive",
			SandboxEscalationApproved: true,
			SkipShellApproval:         true,
		},
	})
	if err != nil {
		t.Fatalf("expected pre-approved escalation to allow run, got %v", err)
	}
	if !called {
		t.Fatal("expected underlying tool to run after pre-approved escalation")
	}
}

func TestInProcessWorkerEscalatesRunShellWithStdinFallback(t *testing.T) {
	registry := &Registry{}
	called := false
	var out bytes.Buffer
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "run_shell",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			called = true
			return `{"ok":true}`, nil
		},
	})
	worker := inProcessWorker{registry: registry}
	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"go test ./..."}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			Workspace:      t.TempDir(),
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "go run", ArgsPattern: []string{"./cmd/app"}},
			},
			ApprovalPolicy: "on-request",
			ApprovalMode:   "interactive",
			Stdin:          strings.NewReader("yes\n"),
			Stdout:         &out,
		},
	})
	if err != nil {
		t.Fatalf("expected stdin fallback approval to allow run, got %v", err)
	}
	if !called {
		t.Fatal("expected underlying tool to run after stdin approval")
	}
	if !strings.Contains(out.String(), `Approve tool (`) {
		t.Fatalf("expected stdin fallback prompt output, got %q", out.String())
	}
}

func TestInProcessWorkerEscalatesRunShellWithStdinFallbackDenied(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "run_shell",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			t.Fatal("tool should not run after stdin denial")
			return "", nil
		},
	})
	worker := inProcessWorker{registry: registry}
	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"go test ./..."}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			Workspace:      t.TempDir(),
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "go run", ArgsPattern: []string{"./cmd/app"}},
			},
			ApprovalPolicy: "on-request",
			ApprovalMode:   "interactive",
			Stdin:          strings.NewReader("n\n"),
			Stdout:         &bytes.Buffer{},
		},
	})
	if err == nil {
		t.Fatal("expected stdin fallback denial error")
	}
	execErr, ok := AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("expected permission denied tool error, got %#v", err)
	}
}

func TestInProcessWorkerDeniesWriteFileOutsideLeaseScope(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "write_file",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			t.Fatal("tool should not run when path is outside lease scope")
			return "", nil
		},
	})
	workspace := t.TempDir()
	allowed := t.TempDir()
	outside := t.TempDir()
	payload, err := json.Marshal(map[string]any{
		"path":    filepath.Join(outside, "out.txt"),
		"content": "hello",
	})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	worker := inProcessWorker{registry: registry}
	_, err = worker.Run(context.Background(), workerRunRequest{
		ToolName: "write_file",
		RawArgs:  json.RawMessage(payload),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			Workspace:      workspace,
			FSWrite:        []string{allowed},
		},
	})
	if err == nil {
		t.Fatal("expected fs_out_of_scope denial")
	}
	execErr, ok := AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("expected permission denied tool error, got %#v", err)
	}
}

func TestInProcessWorkerDeniesRunShellNetworkOutsideLeaseAllowlist(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "run_shell",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			t.Fatal("tool should not run when network target is outside lease allowlist")
			return "", nil
		},
	})
	worker := inProcessWorker{registry: registry}
	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"curl https://example.org/data"}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			Workspace:      t.TempDir(),
			ApprovalPolicy: "never",
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "curl", ArgsPattern: []string{"https://example.org/data"}},
			},
			NetworkAllowlist: []sandboxpkg.NetworkRule{
				{Host: "api.openai.com", Port: 443, Scheme: "https"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected network allowlist denial")
	}
	execErr, ok := AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("expected permission denied tool error, got %#v", err)
	}
	if !strings.Contains(execErr.Message, "network_not_allowed") {
		t.Fatalf("expected network_not_allowed reason, got %q", execErr.Message)
	}
}

func TestInProcessWorkerDeniesRunShellNestedNetworkOutsideLeaseAllowlist(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "run_shell",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			t.Fatal("tool should not run when nested shell network target is outside lease allowlist")
			return "", nil
		},
	})
	worker := inProcessWorker{registry: registry}
	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"sh -lc \"curl https://example.org/data\""}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			Workspace:      t.TempDir(),
			ApprovalPolicy: "never",
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "sh", ArgsPattern: []string{"-lc", "curl https://example.org/data"}},
			},
			NetworkAllowlist: []sandboxpkg.NetworkRule{
				{Host: "api.openai.com", Port: 443, Scheme: "https"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected nested network allowlist denial")
	}
	execErr, ok := AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("expected permission denied tool error, got %#v", err)
	}
	if !strings.Contains(execErr.Message, "network_not_allowed") {
		t.Fatalf("expected network_not_allowed reason, got %q", execErr.Message)
	}
}

func TestInProcessWorkerRejectsMissingRegistry(t *testing.T) {
	worker := inProcessWorker{}
	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"go test ./..."}`),
	})
	if err == nil {
		t.Fatal("expected missing registry error")
	}
	execErr, ok := AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorInternal {
		t.Fatalf("expected internal tool error, got %#v", err)
	}
}

func TestResolvePolicyLeaseRejectsProvidedLeaseWithoutKeyring(t *testing.T) {
	lease := &sandboxpkg.Lease{
		Version: sandboxpkg.LeaseVersionV1,
	}
	_, _, err := resolvePolicyLease(&ExecutionContext{
		Lease:        lease,
		LeaseKeyring: map[string][]byte{},
	})
	if err == nil {
		t.Fatal("expected missing keyring error for provided lease")
	}
	if !strings.Contains(err.Error(), "sandbox lease keyring is unavailable") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeWorkerRootsDropsEmptyWritableEntries(t *testing.T) {
	roots := normalizeWorkerRoots(nil, "workspace", []string{"   ", "out", "\t"})
	if len(roots) != 2 {
		t.Fatalf("expected workspace and one writable root, got %#v", roots)
	}
	if roots[0] != "workspace" || roots[1] != "out" {
		t.Fatalf("unexpected normalized roots: %#v", roots)
	}
}

func TestRuntimeRequestForReadFileValidationBranches(t *testing.T) {
	if _, err := runtimeRequestForTool("read_file", json.RawMessage(`{`), nil); err == nil {
		t.Fatal("expected read_file invalid json error")
	}
	if _, err := runtimeRequestForTool("read_file", json.RawMessage(`{"path":"  "}`), nil); err == nil {
		t.Fatal("expected read_file empty path error")
	}
}

func TestRuntimeRequestForListFilesDefaultsToWorkspace(t *testing.T) {
	workspace := t.TempDir()
	req, err := runtimeRequestForTool("list_files", json.RawMessage(`{}`), &ExecutionContext{
		Workspace: workspace,
	})
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if req.FileAccess != sandboxpkg.FileAccessRead {
		t.Fatalf("expected read access, got %q", req.FileAccess)
	}
	if !filepath.IsAbs(req.FilePath) {
		t.Fatalf("expected absolute file path, got %q", req.FilePath)
	}
}

func TestRuntimeRequestForSearchTextDefaultsToWorkspace(t *testing.T) {
	workspace := t.TempDir()
	req, err := runtimeRequestForTool("search_text", json.RawMessage(`{"query":"todo"}`), &ExecutionContext{
		Workspace: workspace,
	})
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if req.FileAccess != sandboxpkg.FileAccessRead {
		t.Fatalf("expected read access, got %q", req.FileAccess)
	}
	if !filepath.IsAbs(req.FilePath) {
		t.Fatalf("expected absolute file path, got %q", req.FilePath)
	}
}

func TestRuntimeRequestForRunShellKeepsQuotedArgumentsInOrder(t *testing.T) {
	req, err := runtimeRequestForTool("run_shell", json.RawMessage(`{"command":"deploy --to \"prod env\" --region cn"}`), nil)
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if req.Command != "deploy" {
		t.Fatalf("expected command deploy, got %q", req.Command)
	}
	if len(req.Args) != 4 {
		t.Fatalf("expected 4 args, got %#v", req.Args)
	}
	expected := []string{"--to", "prod env", "--region", "cn"}
	for i := range expected {
		if req.Args[i] != expected[i] {
			t.Fatalf("expected arg[%d]=%q, got %q", i, expected[i], req.Args[i])
		}
	}
}

func TestRuntimeRequestForRunShellRejectsEmptyCommand(t *testing.T) {
	if _, err := runtimeRequestForTool("run_shell", json.RawMessage(`{"command":"   "}`), nil); err == nil {
		t.Fatal("expected run_shell empty command error")
	}
}

func TestRuntimeRequestForRunShellExtractsCurlNetworkTarget(t *testing.T) {
	req, err := runtimeRequestForTool("run_shell", json.RawMessage(`{"command":"curl https://api.openai.com/v1/models"}`), nil)
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if req.Network.Host != "api.openai.com" || req.Network.Scheme != "https" || req.Network.Port != 443 {
		t.Fatalf("expected https api.openai.com:443 network target, got %#v", req.Network)
	}
}

func TestRunShellArgumentsContainNetworkTarget(t *testing.T) {
	if !RunShellArgumentsContainNetworkTarget(`{"command":"curl https://api.openai.com/v1/models"}`) {
		t.Fatal("expected curl command arguments to be detected as network-targeted")
	}
	if RunShellArgumentsContainNetworkTarget(`{"command":"go test ./..."}`) {
		t.Fatal("expected local command arguments to have no network target")
	}
	if RunShellArgumentsContainNetworkTarget(`{`) {
		t.Fatal("expected malformed arguments payload to return false")
	}
}

func TestRuntimeRequestForWebFetchExtractsNetworkTarget(t *testing.T) {
	req, err := runtimeRequestForTool("web_fetch", json.RawMessage(`{"url":"api.openai.com/v1/models"}`), nil)
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if req.Network.Host != "api.openai.com" || req.Network.Scheme != "https" || req.Network.Port != 443 {
		t.Fatalf("expected https api.openai.com:443 network target, got %#v", req.Network)
	}
}

func TestRuntimeRequestForWebSearchUsesDefaultSearchEndpointTarget(t *testing.T) {
	req, err := runtimeRequestForTool("web_search", json.RawMessage(`{"query":"golang"}`), nil)
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if req.Network.Host == "" || req.Network.Scheme != "https" || req.Network.Port != 443 {
		t.Fatalf("expected https network target for web_search, got %#v", req.Network)
	}
}

func TestRuntimeRequestForRunShellExtractsPowerShellUriTarget(t *testing.T) {
	req, err := runtimeRequestForTool("run_shell", json.RawMessage(`{"command":"iwr -UseBasicParsing -Uri http://example.org:80/path"}`), nil)
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if req.Network.Host != "example.org" || req.Network.Scheme != "http" || req.Network.Port != 80 {
		t.Fatalf("expected http example.org:80 network target, got %#v", req.Network)
	}
}

func TestRuntimeRequestForRunShellExtractsNestedShellTarget(t *testing.T) {
	req, err := runtimeRequestForTool("run_shell", json.RawMessage(`{"command":"sh -lc \"curl https://api.openai.com/v1/models\""}`), nil)
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if req.Network.Host != "api.openai.com" || req.Network.Scheme != "https" || req.Network.Port != 443 {
		t.Fatalf("expected nested https api.openai.com:443 target, got %#v", req.Network)
	}
}

func TestRuntimeRequestForRunShellExtractsNestedPowerShellTarget(t *testing.T) {
	req, err := runtimeRequestForTool("run_shell", json.RawMessage(`{"command":"powershell -NoProfile -Command \"iwr -Uri https://example.org/api\""}`), nil)
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if req.Network.Host != "example.org" || req.Network.Scheme != "https" || req.Network.Port != 443 {
		t.Fatalf("expected nested powershell https target, got %#v", req.Network)
	}
}

func TestRuntimeRequestForRunShellIgnoresNonNetworkCommand(t *testing.T) {
	req, err := runtimeRequestForTool("run_shell", json.RawMessage(`{"command":"go test ./..."}`), nil)
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if req.Network.Host != "" || req.Network.Scheme != "" || req.Network.Port != 0 {
		t.Fatalf("expected empty network target for local command, got %#v", req.Network)
	}
}

func TestRuntimeRequestForWriteFileResolvesRelativePathAgainstWorkspace(t *testing.T) {
	workspace := t.TempDir()
	req, err := runtimeRequestForTool("write_file", json.RawMessage(`{"path":"notes/out.txt","content":"ok"}`), &ExecutionContext{
		Workspace: workspace,
	})
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if !filepath.IsAbs(req.FilePath) {
		t.Fatalf("expected absolute file path, got %q", req.FilePath)
	}
	if !strings.HasPrefix(strings.ToLower(req.FilePath), strings.ToLower(workspace)) {
		t.Fatalf("expected resolved path under workspace %q, got %q", workspace, req.FilePath)
	}
}

func TestRuntimeRequestForReplaceInFileRequiresPath(t *testing.T) {
	if _, err := runtimeRequestForTool("replace_in_file", json.RawMessage(`{"path":"  ","old":"a","new":"b"}`), nil); err == nil {
		t.Fatal("expected replace_in_file empty path error")
	}
}

func TestRuntimeRequestForApplyPatchRejectsEmptyPatch(t *testing.T) {
	if _, err := runtimeRequestForTool("apply_patch", json.RawMessage(`{"patch":"  "}`), nil); err == nil {
		t.Fatal("expected apply_patch empty patch error")
	}
}

func TestRuntimeRequestForApplyPatchReturnsOffendingPathForLeaseBoundary(t *testing.T) {
	workspace := t.TempDir()
	allowed := filepath.Join(workspace, "allowed")
	if err := os.MkdirAll(allowed, 0o755); err != nil {
		t.Fatalf("mkdir allowed: %v", err)
	}
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: blocked/out.txt",
		"+hello",
		"*** End Patch",
	}, "\n")
	payload, err := json.Marshal(map[string]any{"patch": patch})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := runtimeRequestForTool("apply_patch", json.RawMessage(payload), &ExecutionContext{
		Workspace:      workspace,
		SandboxEnabled: true,
		FSWrite:        []string{allowed},
	})
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	if req.FileAccess != sandboxpkg.FileAccessWrite {
		t.Fatalf("expected write access, got %q", req.FileAccess)
	}
	expected := filepath.Join(workspace, "blocked", "out.txt")
	if filepath.Clean(req.FilePath) != filepath.Clean(expected) {
		t.Fatalf("expected offending path %q, got %q", expected, req.FilePath)
	}
}

func TestRuntimeRequestForApplyPatchUsesFirstResolvedPathWhenAllowed(t *testing.T) {
	workspace := t.TempDir()
	patch := strings.Join([]string{
		"*** Begin Patch",
		"*** Add File: allowed/one.txt",
		"+hello",
		"*** Update File: allowed/two.txt",
		"@@",
		"*** End Patch",
	}, "\n")
	payload, err := json.Marshal(map[string]any{"patch": patch})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req, err := runtimeRequestForTool("apply_patch", json.RawMessage(payload), &ExecutionContext{
		Workspace:      workspace,
		SandboxEnabled: true,
		FSWrite:        []string{workspace},
	})
	if err != nil {
		t.Fatalf("runtime request: %v", err)
	}
	expected := filepath.Join(workspace, "allowed", "one.txt")
	if filepath.Clean(req.FilePath) != filepath.Clean(expected) {
		t.Fatalf("expected first path %q, got %q", expected, req.FilePath)
	}
}

func TestInProcessWorkerDeniesWebFetchNetworkOutsideLeaseAllowlist(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "web_fetch",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			t.Fatal("tool should not run when web_fetch target is outside lease allowlist")
			return "", nil
		},
	})
	worker := inProcessWorker{registry: registry}
	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "web_fetch",
		RawArgs:  json.RawMessage(`{"url":"https://example.org/docs"}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			Workspace:      t.TempDir(),
			ApprovalPolicy: "never",
			NetworkAllowlist: []sandboxpkg.NetworkRule{
				{Host: "api.openai.com", Port: 443, Scheme: "https"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected web_fetch network allowlist denial")
	}
	execErr, ok := AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("expected permission denied tool error, got %#v", err)
	}
	if !strings.Contains(execErr.Message, "network_not_allowed") {
		t.Fatalf("expected network_not_allowed reason, got %q", execErr.Message)
	}
}

func TestInProcessWorkerRequiredModeDeniesWebFetchWhenBackendLacksNetworkIsolation(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "web_fetch",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			t.Fatal("tool should not run when required mode backend lacks network isolation")
			return "", nil
		},
	})
	originalResolver := workerSandboxBackendResolver
	workerSandboxBackendResolver = func(mode, goos string, lookPath func(string) (string, error)) (systemSandboxRuntimeBackend, error) {
		return systemSandboxRuntimeBackend{
			Enabled: true,
			Name:    "windows_job_object",
			Shell: systemSandboxLaunchSpec{
				Policy: systemSandboxPolicy{NetworkIsolation: false},
			},
			Worker: systemSandboxLaunchSpec{
				Policy: systemSandboxPolicy{NetworkIsolation: false},
			},
		}, nil
	}
	defer func() { workerSandboxBackendResolver = originalResolver }()

	worker := inProcessWorker{registry: registry}
	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "web_fetch",
		RawArgs:  json.RawMessage(`{"url":"https://api.openai.com/v1/models"}`),
		Execution: &ExecutionContext{
			SandboxEnabled:    true,
			SystemSandboxMode: systemSandboxModeRequired,
			Workspace:         t.TempDir(),
			NetworkAllowlist: []sandboxpkg.NetworkRule{
				{Host: "api.openai.com", Port: 443, Scheme: "https"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected required mode network isolation denial")
	}
	execErr, ok := AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("expected permission denied tool error, got %#v", err)
	}
	if !strings.Contains(strings.ToLower(execErr.Message), "lacks network isolation") {
		t.Fatalf("expected network isolation denial message, got %q", execErr.Message)
	}
}

func TestInProcessWorkerRequiredModeAllowsWebFetchWhenBackendHasWorkerNetworkIsolation(t *testing.T) {
	registry := &Registry{}
	called := false
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "web_fetch",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			called = true
			return `{"ok":true}`, nil
		},
	})
	originalResolver := workerSandboxBackendResolver
	workerSandboxBackendResolver = func(mode, goos string, lookPath func(string) (string, error)) (systemSandboxRuntimeBackend, error) {
		return systemSandboxRuntimeBackend{
			Enabled: true,
			Name:    "linux_unshare",
			Shell: systemSandboxLaunchSpec{
				Policy: systemSandboxPolicy{NetworkIsolation: true},
			},
			Worker: systemSandboxLaunchSpec{
				Policy: systemSandboxPolicy{NetworkIsolation: true},
			},
		}, nil
	}
	defer func() { workerSandboxBackendResolver = originalResolver }()

	worker := inProcessWorker{registry: registry}
	out, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "web_fetch",
		RawArgs:  json.RawMessage(`{"url":"https://api.openai.com/v1/models"}`),
		Execution: &ExecutionContext{
			SandboxEnabled:    true,
			SystemSandboxMode: systemSandboxModeRequired,
			Workspace:         t.TempDir(),
			NetworkAllowlist: []sandboxpkg.NetworkRule{
				{Host: "api.openai.com", Port: 443, Scheme: "https"},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected required mode to allow with worker network isolation, got %v", err)
	}
	if !called {
		t.Fatal("expected underlying tool to run")
	}
	if out != `{"ok":true}` {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestInProcessWorkerRequiredModeDeniesRunShellNetworkWhenShellIsolationMissing(t *testing.T) {
	registry := &Registry{}
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "run_shell",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			t.Fatal("tool should not run when required mode shell lacks network isolation")
			return "", nil
		},
	})
	originalResolver := workerSandboxBackendResolver
	workerSandboxBackendResolver = func(mode, goos string, lookPath func(string) (string, error)) (systemSandboxRuntimeBackend, error) {
		return systemSandboxRuntimeBackend{
			Enabled: true,
			Name:    "windows_job_object",
			Shell: systemSandboxLaunchSpec{
				Policy: systemSandboxPolicy{NetworkIsolation: false},
			},
			Worker: systemSandboxLaunchSpec{
				Policy: systemSandboxPolicy{NetworkIsolation: false},
			},
		}, nil
	}
	defer func() { workerSandboxBackendResolver = originalResolver }()

	worker := inProcessWorker{registry: registry}
	_, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"curl https://api.openai.com/v1/models"}`),
		Execution: &ExecutionContext{
			SandboxEnabled:    true,
			SystemSandboxMode: systemSandboxModeRequired,
			Workspace:         t.TempDir(),
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "curl", ArgsPattern: []string{"https://api.openai.com/v1/models"}},
			},
			NetworkAllowlist: []sandboxpkg.NetworkRule{
				{Host: "api.openai.com", Port: 443, Scheme: "https"},
			},
		},
	})
	if err == nil {
		t.Fatal("expected required mode shell network isolation denial")
	}
	execErr, ok := AsToolExecError(err)
	if !ok || execErr.Code != ToolErrorPermissionDenied {
		t.Fatalf("expected permission denied tool error, got %#v", err)
	}
	if !strings.Contains(strings.ToLower(execErr.Message), "lacks network isolation") {
		t.Fatalf("expected network isolation denial message, got %q", execErr.Message)
	}
}

func TestInProcessWorkerRequiredModeAllowsRunShellNetworkWhenShellIsolationPresent(t *testing.T) {
	registry := &Registry{}
	called := false
	registerBuiltinExecutorTool(t, registry, executorTestTool{
		name: "run_shell",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			called = true
			return `{"ok":true}`, nil
		},
	})
	originalResolver := workerSandboxBackendResolver
	workerSandboxBackendResolver = func(mode, goos string, lookPath func(string) (string, error)) (systemSandboxRuntimeBackend, error) {
		return systemSandboxRuntimeBackend{
			Enabled: true,
			Name:    "linux_unshare",
			Shell: systemSandboxLaunchSpec{
				Policy: systemSandboxPolicy{NetworkIsolation: true},
			},
			Worker: systemSandboxLaunchSpec{
				Policy: systemSandboxPolicy{NetworkIsolation: false},
			},
		}, nil
	}
	defer func() { workerSandboxBackendResolver = originalResolver }()

	worker := inProcessWorker{registry: registry}
	out, err := worker.Run(context.Background(), workerRunRequest{
		ToolName: "run_shell",
		RawArgs:  json.RawMessage(`{"command":"curl https://api.openai.com/v1/models"}`),
		Execution: &ExecutionContext{
			SandboxEnabled:    true,
			SystemSandboxMode: systemSandboxModeRequired,
			Workspace:         t.TempDir(),
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "curl", ArgsPattern: []string{"https://api.openai.com/v1/models"}},
			},
			NetworkAllowlist: []sandboxpkg.NetworkRule{
				{Host: "api.openai.com", Port: 443, Scheme: "https"},
			},
		},
	})
	if err != nil {
		t.Fatalf("expected run_shell to run when shell network isolation is present, got %v", err)
	}
	if !called {
		t.Fatal("expected underlying tool to run")
	}
	if out != `{"ok":true}` {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestRuntimeRequestForWriteFileRejectsPathOutsideWorkspaceRoots(t *testing.T) {
	workspace := t.TempDir()
	outside := t.TempDir()
	_, err := runtimeRequestForTool("write_file", json.RawMessage(`{"path":"`+filepath.ToSlash(filepath.Join(outside, "x.txt"))+`","content":"ok"}`), &ExecutionContext{
		Workspace: workspace,
	})
	if err == nil {
		t.Fatal("expected out-of-scope write path to be rejected")
	}
}

func TestCloneKeyringHandlesEmptyAndBlankKids(t *testing.T) {
	if got := cloneKeyring(nil); got != nil {
		t.Fatalf("expected nil clone for nil keyring, got %#v", got)
	}

	source := map[string][]byte{
		"   ":   []byte("skip"),
		"kid-1": []byte("secret"),
	}
	cloned := cloneKeyring(source)
	if len(cloned) != 1 {
		t.Fatalf("expected one cloned key, got %#v", cloned)
	}
	if !bytes.Equal(cloned["kid-1"], []byte("secret")) {
		t.Fatalf("unexpected cloned key value: %#v", cloned["kid-1"])
	}
	cloned["kid-1"][0] = 'S'
	if bytes.Equal(source["kid-1"], cloned["kid-1"]) {
		t.Fatalf("expected cloned key bytes to be independent from source")
	}
}
