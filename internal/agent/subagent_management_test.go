package agent

import (
	"context"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

func TestRunnerSubAgentManagementNilManagerGuards(t *testing.T) {
	runner := &Runner{}

	agents, diags := runner.ListSubAgents()
	if agents != nil || diags != nil {
		t.Fatalf("expected nil list/diagnostics when manager is unavailable, got %#v %#v", agents, diags)
	}
	if _, ok := runner.FindSubAgent("explorer"); ok {
		t.Fatal("expected FindSubAgent false when manager is unavailable")
	}
	if _, ok := runner.FindBuiltinSubAgent("/explorer"); ok {
		t.Fatal("expected FindBuiltinSubAgent false when manager is unavailable")
	}

	result, err := runner.DispatchSubAgent(context.Background(), nil, "build", tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "locate prompt assembly",
	}, nil)
	if err != nil {
		t.Fatalf("expected structured dispatch failure without Go error, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected dispatch failure when manager is unavailable, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != "subagent_unavailable" {
		t.Fatalf("expected subagent_unavailable error code, got %#v", result.Error)
	}
}

func TestRunnerSubAgentManagementListFindAndDispatch(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
	})
	runner.runtime = nil

	agents, _ := runner.ListSubAgents()
	if len(agents) == 0 {
		t.Fatalf("expected at least one subagent, got %#v", agents)
	}
	if _, ok := runner.FindSubAgent("explorer"); !ok {
		t.Fatal("expected FindSubAgent to resolve explorer")
	}
	if _, ok := runner.FindBuiltinSubAgent("/explorer"); !ok {
		t.Fatal("expected FindBuiltinSubAgent to resolve /explorer")
	}

	// Session workspace should override runner workspace when dispatch builds execution context.
	scopedSession := session.New(workspace)
	scopedSession.Workspace = strings.TrimSpace(workspace)

	result, err := runner.DispatchSubAgent(nil, scopedSession, "build", tools.DelegateSubAgentRequest{
		Agent: "explorer",
		Task:  "locate prompt assembly",
	}, nil)
	if err != nil {
		t.Fatalf("expected structured dispatch result, got %v", err)
	}
	if result.OK {
		t.Fatalf("expected dispatch failure when runtime is unavailable, got %#v", result)
	}
	if result.Error == nil || result.Error.Code != subAgentErrorCodeRuntimeUnavailable {
		t.Fatalf("expected runtime unavailable error code, got %#v", result.Error)
	}
	if result.Agent != "explorer" {
		t.Fatalf("expected canonical agent name explorer, got %q", result.Agent)
	}
}
