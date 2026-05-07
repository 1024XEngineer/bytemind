package app

import (
	"context"
	"io"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/provider"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/skills"
	subagentspkg "github.com/1024XEngineer/bytemind/internal/subagents"
	"github.com/1024XEngineer/bytemind/tui"
)

type agentSourceRunnerStub struct {
	mgr *subagentspkg.Manager
}

func (s agentSourceRunnerStub) RunPromptWithInput(context.Context, *session.Session, tui.RunPromptInput, string, io.Writer) (string, error) {
	return "", nil
}
func (s agentSourceRunnerStub) SetObserver(tui.Observer)                         {}
func (s agentSourceRunnerStub) SetApprovalHandler(tui.ApprovalHandler)           {}
func (s agentSourceRunnerStub) UpdateProvider(config.ProviderConfig, llm.Client) {}
func (s agentSourceRunnerStub) ListSkills() ([]skills.Skill, []skills.Diagnostic) {
	return nil, nil
}
func (s agentSourceRunnerStub) GetActiveSkill(*session.Session) (skills.Skill, bool) {
	return skills.Skill{}, false
}
func (s agentSourceRunnerStub) ActivateSkill(*session.Session, string, map[string]string) (skills.Skill, error) {
	return skills.Skill{}, nil
}
func (s agentSourceRunnerStub) ClearActiveSkill(*session.Session) error { return nil }
func (s agentSourceRunnerStub) ClearSkill(string) (skills.ClearResult, error) {
	return skills.ClearResult{}, nil
}
func (s agentSourceRunnerStub) SubAgentManager() *subagentspkg.Manager { return s.mgr }
func (s agentSourceRunnerStub) ListModels(context.Context) ([]provider.ModelInfo, []provider.Warning, error) {
	return nil, nil, nil
}

func TestTUIRunnerAgentSourceListAgentsHandlesNilRunnerAndManager(t *testing.T) {
	source := &tuiRunnerAgentSource{}
	if got := source.ListAgents(); got != nil {
		t.Fatalf("expected nil agents when runner is nil, got %#v", got)
	}

	source.runner = agentSourceRunnerStub{}
	if got := source.ListAgents(); got != nil {
		t.Fatalf("expected nil agents when subagent manager is nil, got %#v", got)
	}
}

func TestTUIRunnerAgentSourceListAgentsMapsManagerEntries(t *testing.T) {
	source := &tuiRunnerAgentSource{
		runner: agentSourceRunnerStub{mgr: subagentspkg.NewManager(t.TempDir())},
	}

	entries := source.ListAgents()
	if len(entries) == 0 {
		t.Fatalf("expected builtin agents from manager, got %#v", entries)
	}

	var explorerFound bool
	for _, entry := range entries {
		if entry.Name == "explorer" {
			explorerFound = true
			if entry.Scope != string(subagentspkg.ScopeBuiltin) {
				t.Fatalf("expected explorer scope %q, got %#v", subagentspkg.ScopeBuiltin, entry)
			}
			if entry.Description == "" {
				t.Fatalf("expected explorer description to be mapped, got %#v", entry)
			}
			break
		}
	}
	if !explorerFound {
		t.Fatalf("expected explorer entry in mapped agents, got %#v", entries)
	}
}
