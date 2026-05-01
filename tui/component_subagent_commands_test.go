package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/session"
	"bytemind/internal/tools"
)

func TestCommandPaletteListsSubAgentCommands(t *testing.T) {
	required := map[string]bool{
		"/agents":   false,
		"/review":   false,
		"/explorer": false,
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

func TestHandleSlashAgentsListsAndDescribesSubAgents(t *testing.T) {
	workspace := t.TempDir()
	writeSubAgentDef(t, filepath.Join(workspace, "internal", "subagents", "review.md"), `---
name: review
description: builtin reviewer
---
review files
`)

	m := newSubAgentCommandModel(t, workspace)

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

	if err := m.handleSlashCommand("/agents review"); err != nil {
		t.Fatalf("expected /agents review to succeed, got %v", err)
	}
	detailBody := m.chatItems[len(m.chatItems)-1].Body
	for _, want := range []string{"subagent review", "scope builtin", "description builtin reviewer"} {
		if !strings.Contains(detailBody, want) {
			t.Fatalf("expected /agents review output to contain %q, got %q", want, detailBody)
		}
	}

	if err := m.handleSlashCommand("/agents missing"); err != nil {
		t.Fatalf("expected /agents missing to succeed with not-found message, got %v", err)
	}
	missingBody := m.chatItems[len(m.chatItems)-1].Body
	if !strings.Contains(missingBody, "subagent not found: missing") {
		t.Fatalf("expected /agents missing output to contain not-found message, got %q", missingBody)
	}
}

func TestHandleSlashBuiltinSubAgentIgnoresProjectOverride(t *testing.T) {
	workspace := t.TempDir()
	writeSubAgentDef(t, filepath.Join(workspace, "internal", "subagents", "review.md"), `---
name: review
description: builtin reviewer
---
builtin body
`)
	writeSubAgentDef(t, filepath.Join(workspace, ".bytemind", "subagents", "review.md"), `---
name: review
description: project reviewer override
---
project body
`)

	m := newSubAgentCommandModel(t, workspace)

	if err := m.handleSlashCommand("/review"); err != nil {
		t.Fatalf("expected /review to succeed, got %v", err)
	}
	body := m.chatItems[len(m.chatItems)-1].Body
	if !strings.Contains(body, "scope builtin") {
		t.Fatalf("expected /review to render builtin scope, got %q", body)
	}
	if !strings.Contains(body, "description builtin reviewer") {
		t.Fatalf("expected /review to render builtin definition details, got %q", body)
	}
	if strings.Contains(body, "project reviewer override") {
		t.Fatalf("expected /review to ignore project override, got %q", body)
	}
}

func newSubAgentCommandModel(t *testing.T, workspace string) *model {
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
		Store:    store,
		Registry: tools.DefaultRegistry(),
	})
	return &model{
		runner:    wrapTestRunner(runner),
		store:     store,
		sess:      sess,
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
