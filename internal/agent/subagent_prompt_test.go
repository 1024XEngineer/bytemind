package agent

import (
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/tools"
)

func TestPromptSubAgentsNilManager(t *testing.T) {
	runner := &Runner{}
	if got := runner.promptSubAgents(); got != nil {
		t.Fatalf("expected nil for nil manager, got %v", got)
	}
}

func TestPromptSubAgentsIncludesBuiltins(t *testing.T) {
	workspace := t.TempDir()
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
	})
	got := runner.promptSubAgents()
	if len(got) < 2 {
		t.Fatalf("expected at least 2 builtin agents, got %d", len(got))
	}
	names := make([]string, len(got))
	for i, a := range got {
		names[i] = a.Name
	}
	if !contains(names, "explorer") || !contains(names, "review") {
		t.Fatalf("expected builtin explorer and review, got %v", names)
	}
}

func TestPromptSubAgentsAllHaveNonEmptyNameAndDescription(t *testing.T) {
	workspace := t.TempDir()
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
	})
	got := runner.promptSubAgents()
	for _, a := range got {
		if strings.TrimSpace(a.Name) == "" {
			t.Fatal("expected all agents to have non-empty name")
		}
		if strings.TrimSpace(a.Description) == "" {
			t.Fatalf("expected agent %q to have non-empty description", a.Name)
		}
	}
}

func TestPromptSubAgentsSortedCaseInsensitive(t *testing.T) {
	workspace := t.TempDir()
	runner := NewRunner(Options{
		Workspace: workspace,
		Registry:  tools.DefaultRegistry(),
	})
	got := runner.promptSubAgents()
	if len(got) < 2 {
		t.Fatalf("expected at least 2 agents, got %d", len(got))
	}
	// Verify sorted order (case-insensitive)
	for i := 1; i < len(got); i++ {
		prev := strings.ToLower(got[i-1].Name)
		curr := strings.ToLower(got[i].Name)
		if prev > curr {
			t.Fatalf("agents not sorted: %q > %q at index %d", prev, curr, i)
		}
	}
}

func contains(ss []string, target string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
