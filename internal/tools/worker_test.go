package tools

import (
	"context"
	"encoding/json"
	"path/filepath"
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
