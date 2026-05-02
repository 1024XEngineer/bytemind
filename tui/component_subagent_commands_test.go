package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bytemind/internal/agent"
	"bytemind/internal/config"
	"bytemind/internal/llm"
	"bytemind/internal/session"
	"bytemind/internal/tools"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
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

func TestHandleSlashBuiltinSubAgentRequiresTask(t *testing.T) {
	workspace := t.TempDir()
	writeSubAgentDef(t, filepath.Join(workspace, "internal", "subagents", "review.md"), `---
name: review
description: builtin reviewer
---
builtin body
`)

	m := newSubAgentCommandModel(t, workspace, nil)

	if err := m.handleSlashCommand("/review"); err != nil {
		t.Fatalf("expected /review to succeed, got %v", err)
	}
	body := m.chatItems[len(m.chatItems)-1].Body
	if !strings.Contains(body, "usage: /review <task>") {
		t.Fatalf("expected /review to render usage hint when task is missing, got %q", body)
	}
	if !strings.Contains(body, "Tip: use /agents review") {
		t.Fatalf("expected /review to include agents tip, got %q", body)
	}
}

func TestHandleSlashBuiltinSubAgentDelegatesTask(t *testing.T) {
	workspace := t.TempDir()
	writeSubAgentDef(t, filepath.Join(workspace, "internal", "subagents", "review.md"), `---
name: review
description: builtin reviewer
tools: [list_files, read_file, search_text]
---
review files
`)
	writeSubAgentDef(t, filepath.Join(workspace, "internal", "subagents", "explorer.md"), `---
name: explorer
description: builtin explorer
tools: [list_files, read_file, search_text]
---
explore files
`)

	client := &compactCommandTestClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "review summary"},
		},
	}
	m := newSubAgentCommandModel(t, workspace, client)

	if err := m.handleSlashCommand("/review inspect prompt assembly ordering"); err != nil {
		t.Fatalf("expected /review <task> to succeed, got %v", err)
	}

	if m.pendingCommandCmd == nil {
		t.Fatalf("expected /review to queue a main-agent run command")
	}
	if !m.busy {
		t.Fatalf("expected /review to mark model busy while the command run is queued")
	}
	if len(m.chatItems) == 0 {
		t.Fatalf("expected /review to append user slash input to chat")
	}
	if !containsChatEntry(m.chatItems, "user", "/review inspect prompt assembly ordering") {
		t.Fatalf("expected /review command text to appear in user chat entry, got %#v", m.chatItems)
	}

	if err := m.handleSlashCommand("/exploer locate task lifecycle codepath"); err != nil {
		t.Fatalf("expected /exploer <task> alias to succeed, got %v", err)
	}
	if len(m.chatItems) == 0 {
		t.Fatalf("expected /exploer to append user slash input to chat")
	}
	if !containsChatEntry(m.chatItems, "user", "/exploer locate task lifecycle codepath") &&
		!containsChatEntry(m.chatItems, "user", "/explorer locate task lifecycle codepath") {
		t.Fatalf("expected /exploer alias command text to be preserved in user chat entry, got %#v", m.chatItems)
	}
}

func TestHandleSlashBuiltinSubAgentDelegatesTaskWithoutWhitespace(t *testing.T) {
	workspace := t.TempDir()
	writeSubAgentDef(t, filepath.Join(workspace, "internal", "subagents", "explorer.md"), `---
name: explorer
description: builtin explorer
tools: [list_files, read_file, search_text]
---
explore files
`)

	client := &compactCommandTestClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "explorer compact summary"},
		},
	}
	m := newSubAgentCommandModel(t, workspace, client)

	if err := m.handleSlashCommand("/explorer分析一下agent模块功能和作用"); err != nil {
		t.Fatalf("expected compact /explorer command to succeed, got %v", err)
	}

	if m.pendingCommandCmd == nil {
		t.Fatalf("expected compact /explorer command to queue a main-agent run")
	}
	if len(m.chatItems) == 0 {
		t.Fatalf("expected compact /explorer command to append user slash input to chat")
	}
	if !containsChatEntryWithPrefix(m.chatItems, "user", "/explorer ") {
		t.Fatalf("expected compact /explorer command to normalize with whitespace, got %#v", m.chatItems)
	}
}

func TestCommandPaletteSelectExplorerPrefillsCommand(t *testing.T) {
	input := textarea.New()
	input.SetValue("/expl")
	m := model{
		screen:      screenChat,
		commandOpen: true,
		input:       input,
	}
	m.syncCommandPalette()

	got, cmd := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd != nil {
		t.Fatalf("expected palette selection to prefill /explorer template instead of executing immediately")
	}
	updated := got.(model)
	if updated.commandOpen {
		t.Fatalf("expected command palette to close after selecting /explorer")
	}
	if updated.input.Value() != "/explorer" {
		t.Fatalf("expected /explorer usage to be inserted, got %q", updated.input.Value())
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

func containsChatEntryWithPrefix(items []chatEntry, kind, prefix string) bool {
	kind = strings.TrimSpace(kind)
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return false
	}
	for _, item := range items {
		if kind != "" && item.Kind != kind {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(item.Body), prefix) {
			return true
		}
	}
	return false
}
