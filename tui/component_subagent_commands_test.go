package tui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/agent"
	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/skills"
	subagentspkg "github.com/1024XEngineer/bytemind/internal/subagents"
	"github.com/1024XEngineer/bytemind/internal/tools"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
)

type subAgentCommandRunnerStub struct {
	builtinAgent subagentspkg.Agent
	builtinOK    bool
}

func (s *subAgentCommandRunnerStub) RunPromptWithInput(context.Context, *session.Session, RunPromptInput, string, io.Writer) (string, error) {
	return "", nil
}

func (s *subAgentCommandRunnerStub) SetObserver(Observer) {}

func (s *subAgentCommandRunnerStub) SetApprovalHandler(ApprovalHandler) {}

func (s *subAgentCommandRunnerStub) UpdateProvider(config.ProviderConfig, llm.Client) {}

func (s *subAgentCommandRunnerStub) ListSkills() ([]skills.Skill, []skills.Diagnostic) {
	return nil, nil
}

func (s *subAgentCommandRunnerStub) GetActiveSkill(*session.Session) (skills.Skill, bool) {
	return skills.Skill{}, false
}

func (s *subAgentCommandRunnerStub) ActivateSkill(*session.Session, string, map[string]string) (skills.Skill, error) {
	return skills.Skill{}, nil
}

func (s *subAgentCommandRunnerStub) ClearActiveSkill(*session.Session) error {
	return nil
}

func (s *subAgentCommandRunnerStub) ClearSkill(string) (skills.ClearResult, error) {
	return skills.ClearResult{}, nil
}

func (s *subAgentCommandRunnerStub) SubAgentManager() *subagentspkg.Manager {
	return nil
}

func (s *subAgentCommandRunnerStub) ListSubAgents() ([]subagentspkg.Agent, []subagentspkg.Diagnostic) {
	return nil, nil
}

func (s *subAgentCommandRunnerStub) FindSubAgent(string) (subagentspkg.Agent, bool) {
	return subagentspkg.Agent{}, false
}

func (s *subAgentCommandRunnerStub) FindBuiltinSubAgent(string) (subagentspkg.Agent, bool) {
	if s.builtinOK {
		return s.builtinAgent, true
	}
	return subagentspkg.Agent{}, false
}

func TestCommandPaletteListsSubAgentCommands(t *testing.T) {
	required := map[string]bool{
		"/agents": false,
	}
	for _, item := range commandItems {
		if _, ok := required[item.Name]; ok && item.Kind == "command" {
			required[item.Name] = true
		}
	}
	for name, found := range required {
		if !found {
			t.Fatalf("expected command palette to include %s", name)
		}
	}
}

func TestHandleSlashAgentsRequiresSubAgentRunner(t *testing.T) {
	m := &model{}
	err := m.handleSlashCommand("/agents")
	if err == nil {
		t.Fatal("expected /agents to fail when runner is unavailable")
	}
	if !strings.Contains(err.Error(), "runner is unavailable") {
		t.Fatalf("expected runner unavailable error, got %v", err)
	}
}

func TestHandleSlashAgentsListsSubAgents(t *testing.T) {
	workspace := t.TempDir()
	writeSubAgentDef(t, filepath.Join(workspace, "internal", "subagents", "review.md"), `---
name: review
description: builtin reviewer
---
review files
`)

	m := newSubAgentCommandModel(t, workspace, nil)

	if err := m.handleSlashCommand("/agents"); err != nil {
		t.Fatalf("expected /agents to succeed, got %v", err)
	}
	if len(m.chatItems) < 2 {
		t.Fatalf("expected /agents command exchange in chat, got %#v", m.chatItems)
	}
	body := m.chatItems[len(m.chatItems)-1].Body
	if !strings.Contains(body, "Available subagents:") {
		t.Fatalf("expected /agents output heading, got %q", body)
	}
	if !strings.Contains(body, "- review [builtin]: builtin reviewer") {
		t.Fatalf("expected /agents output to include builtin review definition, got %q", body)
	}
}

func newSubAgentCommandModel(t *testing.T, workspace string, client llm.Client) *model {
	t.Helper()

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := agent.NewRunner(agent.Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
	})
	input := textarea.New()
	input.Focus()
	return &model{
		runner:    wrapTestRunner(runner),
		store:     store,
		sess:      sess,
		async:     make(chan tea.Msg, 8),
		input:     input,
		workspace: workspace,
		screen:    screenChat,
	}
}

func writeSubAgentDef(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func containsChatEntry(items []chatEntry, kind, needle string) bool {
	kind = strings.TrimSpace(kind)
	needle = strings.TrimSpace(needle)
	if needle == "" {
		return false
	}
	for _, item := range items {
		if kind != "" && item.Kind != kind {
			continue
		}
		if strings.Contains(item.Body, needle) {
			return true
		}
	}
	return false
}
