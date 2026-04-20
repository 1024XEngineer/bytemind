package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	sandboxpkg "bytemind/internal/sandbox"
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
	if shouldRouteToWorker("run_shell", &ExecutionContext{SandboxEnabled: false}) {
		t.Fatal("expected sandbox disabled to skip worker route")
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
	if shouldRouteToWorker("search_text", &ExecutionContext{SandboxEnabled: true}) {
		t.Fatal("expected search_text to stay in main executor path")
	}
}

func TestExecutorRoutesSandboxToolsToWorker(t *testing.T) {
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

	out, err := executor.Execute(context.Background(), "run_shell", `{}`, &ExecutionContext{SandboxEnabled: true})
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
		name: "search_text",
		run: func(_ context.Context, _ json.RawMessage, _ *ExecutionContext) (string, error) {
			mainCalled = true
			return `{"ok":true,"main":true}`, nil
		},
	})
	executor := NewExecutor(registry)
	worker := &workerTestDouble{output: `{"ok":true,"worker":true}`}
	executor.worker = worker

	out, err := executor.Execute(context.Background(), "search_text", `{}`, &ExecutionContext{SandboxEnabled: true})
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
	resolved, ok := registry.Get("run_shell")
	if !ok {
		t.Fatal("expected run_shell tool in registry")
	}
	worker := inProcessWorker{}
	out, err := worker.Run(context.Background(), workerRunRequest{
		Resolved: resolved,
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
	resolved, ok := registry.Get("run_shell")
	if !ok {
		t.Fatal("expected run_shell tool in registry")
	}
	worker := inProcessWorker{}
	_, err := worker.Run(context.Background(), workerRunRequest{
		Resolved: resolved,
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
	resolved, ok := registry.Get("run_shell")
	if !ok {
		t.Fatal("expected run_shell tool in registry")
	}
	worker := inProcessWorker{}
	_, err := worker.Run(context.Background(), workerRunRequest{
		Resolved: resolved,
		RawArgs:  json.RawMessage(`{"command":"go test ./..."}`),
		Execution: &ExecutionContext{
			SandboxEnabled: true,
			Workspace:      t.TempDir(),
			ExecAllowlist: []sandboxpkg.ExecRule{
				{Command: "go run", ArgsPattern: []string{"./cmd/app"}},
			},
			ApprovalPolicy: "on-request",
			ApprovalMode:   "interactive",
			Approval: func(req ApprovalRequest) (bool, error) {
				approvalCalls++
				if !strings.Contains(strings.ToLower(req.Reason), "outside lease scope") {
					t.Fatalf("expected escalation reason to explain outside lease scope, got %q", req.Reason)
				}
				return true, nil
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
	resolved, ok := registry.Get("run_shell")
	if !ok {
		t.Fatal("expected run_shell tool in registry")
	}
	worker := inProcessWorker{}
	_, err := worker.Run(context.Background(), workerRunRequest{
		Resolved: resolved,
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
	resolved, ok := registry.Get("run_shell")
	if !ok {
		t.Fatal("expected run_shell tool in registry")
	}
	worker := inProcessWorker{}
	_, err := worker.Run(context.Background(), workerRunRequest{
		Resolved: resolved,
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
	resolved, ok := registry.Get("write_file")
	if !ok {
		t.Fatal("expected write_file tool in registry")
	}
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
	worker := inProcessWorker{}
	_, err = worker.Run(context.Background(), workerRunRequest{
		Resolved: resolved,
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
	if _, err := runtimeRequestForTool("read_file", json.RawMessage(`{`)); err == nil {
		t.Fatal("expected read_file invalid json error")
	}
	if _, err := runtimeRequestForTool("read_file", json.RawMessage(`{"path":"  "}`)); err == nil {
		t.Fatal("expected read_file empty path error")
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
