package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	planpkg "bytemind/internal/plan"
	"bytemind/internal/tools"
)

func TestDelegateSubAgentReturnsStructuredNotImplementedAfterPreflight(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "subagents"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "subagents", "explorer.md"), []byte(`---
name: explorer
description: repo explorer
tools: [read_file, search_text]
mode: build
---
scan files
`), 0o644); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failure placeholder result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != subAgentErrorCodeNotImplemented {
		t.Fatalf("expected not implemented code, got %#v", result.Error)
	}
	if strings.TrimSpace(result.InvocationID) == "" {
		t.Fatalf("expected invocation id, got %#v", result)
	}
	if result.Findings == nil || result.References == nil {
		t.Fatalf("expected findings/references arrays, got %#v", result)
	}
}

func TestDelegateSubAgentReturnsStructuredPreflightFailure(t *testing.T) {
	workspace := t.TempDir()
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
	})

	result, err := runner.delegateSubAgent(context.Background(), tools.DelegateSubAgentRequest{
		Agent: "unknown",
		Task:  "Locate prompt assembly order",
	}, &tools.ExecutionContext{
		Mode: planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected structured tool result without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected failure result, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != "subagent_agent_not_found" {
		t.Fatalf("expected preflight agent_not_found code, got %#v", result.Error)
	}
	if result.Findings == nil || result.References == nil {
		t.Fatalf("expected findings/references arrays, got %#v", result)
	}
}
