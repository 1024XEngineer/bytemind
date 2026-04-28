package tools

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildDarwinSandboxProfileIncludesCoreRulesAndWritableRoots(t *testing.T) {
	workspace := t.TempDir()
	writable := filepath.Join(workspace, "out")
	execCtx := &ExecutionContext{
		Workspace:     workspace,
		WritableRoots: []string{writable},
	}
	profile, err := buildDarwinSandboxProfile(execCtx, true)
	if err != nil {
		t.Fatalf("build profile: %v", err)
	}
	if !strings.Contains(profile, "(deny default)") {
		t.Fatalf("expected deny default rule, got %q", profile)
	}
	if !strings.Contains(profile, "(allow process*)") {
		t.Fatalf("expected process allow rule, got %q", profile)
	}
	if !strings.Contains(profile, "(allow file-read*)") {
		t.Fatalf("expected file-read allow rule, got %q", profile)
	}
	if !strings.Contains(profile, "(allow network*)") {
		t.Fatalf("expected network allow rule when requested, got %q", profile)
	}

	roots, err := darwinSandboxWritableRoots(execCtx)
	if err != nil {
		t.Fatalf("resolve writable roots: %v", err)
	}
	for _, root := range roots {
		rule := `(allow file-write* (subpath "` + escapeDarwinSandboxLiteral(root) + `"))`
		if !strings.Contains(profile, rule) {
			t.Fatalf("expected writable root rule %q, got %q", rule, profile)
		}
	}
}

func TestBuildDarwinSandboxProfileOmitsNetworkWhenDisabled(t *testing.T) {
	workspace := t.TempDir()
	profile, err := buildDarwinSandboxProfile(&ExecutionContext{Workspace: workspace}, false)
	if err != nil {
		t.Fatalf("build profile: %v", err)
	}
	if strings.Contains(profile, "(allow network*)") {
		t.Fatalf("expected no network allow rule, got %q", profile)
	}
}

func TestBuildDarwinSandboxProfileRequiresWorkspace(t *testing.T) {
	if _, err := buildDarwinSandboxProfile(nil, true); err == nil {
		t.Fatal("expected missing workspace error")
	}
	if _, err := buildDarwinSandboxProfile(&ExecutionContext{}, true); err == nil {
		t.Fatal("expected missing workspace error")
	}
}

func TestBuildDarwinSandboxArgsHelpers(t *testing.T) {
	profile := "(version 1)"
	command := "echo ok"
	shellArgs := buildDarwinSandboxShellArgs(profile, command)
	if len(shellArgs) != 5 {
		t.Fatalf("unexpected shell args: %#v", shellArgs)
	}
	if shellArgs[0] != "-p" || shellArgs[1] != profile || shellArgs[2] != "sh" || shellArgs[3] != "-lc" || shellArgs[4] != command {
		t.Fatalf("unexpected shell args: %#v", shellArgs)
	}

	workerArgs := buildDarwinSandboxWorkerArgs(profile, "/tmp/bytemind")
	if len(workerArgs) != 5 {
		t.Fatalf("unexpected worker args: %#v", workerArgs)
	}
	if workerArgs[0] != "-p" || workerArgs[1] != profile || workerArgs[2] != "/tmp/bytemind" || workerArgs[3] != sandboxWorkerSubcommand || workerArgs[4] != sandboxWorkerStdioFlag {
		t.Fatalf("unexpected worker args: %#v", workerArgs)
	}
}
