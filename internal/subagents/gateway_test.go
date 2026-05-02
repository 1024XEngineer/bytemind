package subagents

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
)

func TestGatewayPreflightBuildsEffectiveToolSet(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: reviewer
tools: [read_file, search_text, delegate_subagent]
disallowed_tools: [run_shell]
mode: build
---
review files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	result, err := gateway.Preflight(PreflightRequest{
		Agent:         "review",
		Task:          "Find defects in runtime task flow",
		Mode:          planpkg.ModeBuild,
		ParentVisible: []string{"read_file", "search_text", "run_shell", "write_file", "delegate_subagent"},
		ParentDenied:  map[string]struct{}{"write_file": {}},
	})
	if err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	if !slices.Equal(result.EffectiveTools, []string{"read_file", "search_text"}) {
		t.Fatalf("unexpected effective tools: %#v", result.EffectiveTools)
	}
	if _, ok := result.AllowedTools["read_file"]; !ok {
		t.Fatalf("expected read_file in allowlist: %#v", result.AllowedTools)
	}
	if _, ok := result.DeniedTools["delegate_subagent"]; !ok {
		t.Fatalf("expected delegate_subagent in denylist: %#v", result.DeniedTools)
	}
	if _, ok := result.DeniedTools["run_shell"]; !ok {
		t.Fatalf("expected run_shell in denylist: %#v", result.DeniedTools)
	}
	if _, ok := result.DeniedTools["write_file"]; !ok {
		t.Fatalf("expected inherited parent denylist: %#v", result.DeniedTools)
	}
}

func TestGatewayPreflightAppliesParentAllowlistIntersection(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "explorer.md"), `---
name: explorer
description: explorer
tools: [read_file, search_text]
mode: build
---
scan files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	result, err := gateway.Preflight(PreflightRequest{
		Agent:         "explorer",
		Task:          "Locate prompt assembly entry",
		Mode:          planpkg.ModeBuild,
		ParentVisible: []string{"read_file", "search_text", "list_files"},
		ParentAllowed: map[string]struct{}{"read_file": {}},
	})
	if err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	if !slices.Equal(result.EffectiveTools, []string{"read_file"}) {
		t.Fatalf("unexpected effective tools: %#v", result.EffectiveTools)
	}
}

func TestGatewayPreflightRejectsModeMismatch(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: reviewer
mode: build
---
review files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	_, err := gateway.Preflight(PreflightRequest{
		Agent:         "review",
		Task:          "check",
		Mode:          planpkg.ModePlan,
		ParentVisible: []string{"read_file"},
	})
	if err == nil {
		t.Fatal("expected mode mismatch error")
	}
	gatewayErr, ok := err.(*GatewayError)
	if !ok || gatewayErr.Code != ErrorCodeSubAgentModeNotAllowed {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestGatewayPreflightRejectsUnknownAgent(t *testing.T) {
	workspace := t.TempDir()
	manager := NewManagerWithDirs(
		workspace,
		filepath.Join(workspace, "internal", "subagents"),
		filepath.Join(workspace, "user"),
		filepath.Join(workspace, "project"),
	)
	gateway := NewGateway(manager)
	_, err := gateway.Preflight(PreflightRequest{
		Agent:         "unknown",
		Task:          "check",
		Mode:          planpkg.ModeBuild,
		ParentVisible: []string{"read_file"},
	})
	if err == nil {
		t.Fatal("expected unknown agent error")
	}
	gatewayErr, ok := err.(*GatewayError)
	if !ok || gatewayErr.Code != ErrorCodeSubAgentAgentNotFound {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestGatewayPreflightRejectsWhenNoEffectiveToolsRemain(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: reviewer
tools: [read_file]
---
review files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	_, err := gateway.Preflight(PreflightRequest{
		Agent:         "review",
		Task:          "check",
		Mode:          planpkg.ModeBuild,
		ParentVisible: []string{"write_file"},
	})
	if err == nil {
		t.Fatal("expected tool denied error")
	}
	gatewayErr, ok := err.(*GatewayError)
	if !ok || gatewayErr.Code != ErrorCodeSubAgentToolDenied {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestGatewayPreflightRejectsEmptyTask(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: reviewer
---
review files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	_, err := gateway.Preflight(PreflightRequest{
		Agent:         "review",
		Task:          "   ",
		Mode:          planpkg.ModeBuild,
		ParentVisible: []string{"read_file"},
	})
	if err == nil {
		t.Fatal("expected task not eligible error")
	}
	gatewayErr, ok := err.(*GatewayError)
	if !ok || gatewayErr.Code != ErrorCodeSubAgentTaskNotEligible {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestGatewayPreflightInheritsDefinitionDefaults(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: reviewer
tools: [read_file]
timeout: 45s
output: findings
isolation: worktree
---
review files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	result, err := gateway.Preflight(PreflightRequest{
		Agent:         "review",
		Task:          "check",
		Mode:          planpkg.ModeBuild,
		ParentVisible: []string{"read_file"},
	})
	if err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	if result.RequestedTimeout != "45s" {
		t.Fatalf("expected default timeout 45s, got %q", result.RequestedTimeout)
	}
	if result.RequestedTimeoutDuration != 45*time.Second {
		t.Fatalf("expected default timeout duration 45s, got %s", result.RequestedTimeoutDuration)
	}
	if result.RequestedOutput != "findings" {
		t.Fatalf("expected default output findings, got %q", result.RequestedOutput)
	}
	if result.Isolation != isolationWorktree {
		t.Fatalf("expected default isolation %q, got %q", isolationWorktree, result.Isolation)
	}
}

func TestGatewayPreflightNormalizesRequestedOutput(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: reviewer
tools: [read_file]
---
review files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	result, err := gateway.Preflight(PreflightRequest{
		Agent:           "review",
		Task:            "check",
		Mode:            planpkg.ModeBuild,
		ParentVisible:   []string{"read_file"},
		RequestedOutput: "  SUMMARY ",
	})
	if err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	if result.RequestedOutput != outputSummary {
		t.Fatalf("expected normalized output %q, got %q", outputSummary, result.RequestedOutput)
	}
}

func TestGatewayPreflightRejectsInvalidRequestedOutput(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: reviewer
tools: [read_file]
---
review files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	_, err := gateway.Preflight(PreflightRequest{
		Agent:           "review",
		Task:            "check",
		Mode:            planpkg.ModeBuild,
		ParentVisible:   []string{"read_file"},
		RequestedOutput: "json",
	})
	if err == nil {
		t.Fatal("expected invalid output error")
	}
	gatewayErr, ok := err.(*GatewayError)
	if !ok || gatewayErr.Code != ErrorCodeSubAgentInvalidRequest {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestGatewayPreflightRejectsInvalidRequestedTimeout(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: reviewer
tools: [read_file]
---
review files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	_, err := gateway.Preflight(PreflightRequest{
		Agent:            "review",
		Task:             "check",
		Mode:             planpkg.ModeBuild,
		ParentVisible:    []string{"read_file"},
		RequestedTimeout: "soon",
	})
	if err == nil {
		t.Fatal("expected invalid timeout error")
	}
	gatewayErr, ok := err.(*GatewayError)
	if !ok || gatewayErr.Code != ErrorCodeSubAgentInvalidRequest {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestGatewayPreflightRejectsNegativeRequestedTimeout(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: reviewer
tools: [read_file]
---
review files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	_, err := gateway.Preflight(PreflightRequest{
		Agent:            "review",
		Task:             "check",
		Mode:             planpkg.ModeBuild,
		ParentVisible:    []string{"read_file"},
		RequestedTimeout: "-5s",
	})
	if err == nil {
		t.Fatal("expected invalid timeout error")
	}
	gatewayErr, ok := err.(*GatewayError)
	if !ok || gatewayErr.Code != ErrorCodeSubAgentInvalidRequest {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestGatewayPreflightRejectsTooLargeRequestedTimeout(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: reviewer
tools: [read_file]
---
review files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	_, err := gateway.Preflight(PreflightRequest{
		Agent:            "review",
		Task:             "check",
		Mode:             planpkg.ModeBuild,
		ParentVisible:    []string{"read_file"},
		RequestedTimeout: "16m",
	})
	if err == nil {
		t.Fatal("expected invalid timeout error")
	}
	gatewayErr, ok := err.(*GatewayError)
	if !ok || gatewayErr.Code != ErrorCodeSubAgentInvalidRequest {
		t.Fatalf("unexpected error: %#v", err)
	}
}

func TestGatewayPreflightNormalizesRequestedTimeoutString(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: reviewer
tools: [read_file]
---
review files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	result, err := gateway.Preflight(PreflightRequest{
		Agent:            "review",
		Task:             "check",
		Mode:             planpkg.ModeBuild,
		ParentVisible:    []string{"read_file"},
		RequestedTimeout: "90s",
	})
	if err != nil {
		t.Fatalf("unexpected preflight error: %v", err)
	}
	if result.RequestedTimeout != "1m30s" {
		t.Fatalf("expected normalized timeout 1m30s, got %q", result.RequestedTimeout)
	}
	if result.RequestedTimeoutDuration != 90*time.Second {
		t.Fatalf("expected timeout duration 90s, got %s", result.RequestedTimeoutDuration)
	}
}

func TestGatewayPreflightRejectsInvalidRequestedIsolation(t *testing.T) {
	workspace := t.TempDir()
	builtinDir := filepath.Join(workspace, "internal", "subagents")
	writeSubAgentFile(t, filepath.Join(builtinDir, "review.md"), `---
name: review
description: reviewer
tools: [read_file]
---
review files
`)

	manager := NewManagerWithDirs(workspace, builtinDir, filepath.Join(workspace, "user"), filepath.Join(workspace, "project"))
	gateway := NewGateway(manager)
	_, err := gateway.Preflight(PreflightRequest{
		Agent:              "review",
		Task:               "check",
		Mode:               planpkg.ModeBuild,
		ParentVisible:      []string{"read_file"},
		RequestedIsolation: "sandbox",
	})
	if err == nil {
		t.Fatal("expected invalid isolation error")
	}
	gatewayErr, ok := err.(*GatewayError)
	if !ok || gatewayErr.Code != ErrorCodeSubAgentInvalidRequest {
		t.Fatalf("unexpected error: %#v", err)
	}
}
