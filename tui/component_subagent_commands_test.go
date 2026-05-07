package tui

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/bytemind/internal/agent"
	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/provider"
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
	lastRequest  tools.DelegateSubAgentRequest
	models       []provider.ModelInfo
	warnings     []provider.Warning
	modelsErr    error
	runtimeCfg   config.ProviderRuntimeConfig
	providerCfg  config.ProviderConfig
	client       llm.Client
}

func (s *subAgentCommandRunnerStub) RunPromptWithInput(context.Context, *session.Session, RunPromptInput, string, io.Writer) (string, error) {
	return "", nil
}

func (s *subAgentCommandRunnerStub) SetObserver(Observer) {}

func (s *subAgentCommandRunnerStub) SetApprovalHandler(ApprovalHandler) {}

func (s *subAgentCommandRunnerStub) UpdateProvider(config.ProviderConfig, llm.Client) {}

func (s *subAgentCommandRunnerStub) UpdateProviderRuntime(runtimeCfg config.ProviderRuntimeConfig, providerCfg config.ProviderConfig, client llm.Client) {
	s.runtimeCfg = runtimeCfg
	s.providerCfg = providerCfg
	s.client = client
}

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

func (s *subAgentCommandRunnerStub) ListModels(context.Context) ([]provider.ModelInfo, []provider.Warning, error) {
	return s.models, s.warnings, s.modelsErr
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

func (s *subAgentCommandRunnerStub) DispatchSubAgent(_ context.Context, _ *session.Session, _ string, request tools.DelegateSubAgentRequest, _ Observer) (tools.DelegateSubAgentResult, error) {
	s.lastRequest = request
	return tools.DelegateSubAgentResult{
		OK:      true,
		Status:  "completed",
		Summary: "stub result",
	}, nil
}

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

func TestHandleSlashBuiltinSubAgentUnavailable(t *testing.T) {
	m := &model{
		runner: &subAgentCommandRunnerStub{},
		screen: screenChat,
	}

	if err := m.handleSlashCommand("/review inspect code changes"); err != nil {
		t.Fatalf("expected /review to render unavailable hint, got %v", err)
	}
	if len(m.chatItems) < 2 {
		t.Fatalf("expected /review command exchange in chat, got %#v", m.chatItems)
	}
	body := m.chatItems[len(m.chatItems)-1].Body
	if !strings.Contains(body, "builtin subagent is unavailable") {
		t.Fatalf("expected unavailable response, got %q", body)
	}
	if !strings.Contains(m.statusNote, "unavailable") {
		t.Fatalf("expected unavailable status note, got %q", m.statusNote)
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

	if !m.busy {
		t.Fatalf("expected /review to mark model busy while the subagent runs")
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

	if !m.busy {
		t.Fatalf("expected compact /explorer command to mark model busy")
	}
	if len(m.chatItems) == 0 {
		t.Fatalf("expected compact /explorer command to append user slash input to chat")
	}
	if !containsChatEntryWithPrefix(m.chatItems, "user", "/explorer ") {
		t.Fatalf("expected compact /explorer command to normalize with whitespace, got %#v", m.chatItems)
	}
}

func TestSubmitBuiltinSubAgentPreferenceSynthesizesDisplayInput(t *testing.T) {
	workspace := t.TempDir()
	client := &compactCommandTestClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "delegated"},
		},
	}
	m := newSubAgentCommandModel(t, workspace, client)

	if err := m.submitBuiltinSubAgentPreference("", "explorer", "locate runtime prompt"); err != nil {
		t.Fatalf("expected synthesized display input submission to succeed, got %v", err)
	}
	if !m.busy {
		t.Fatal("expected synthesized submission to mark model busy")
	}
	if !containsChatEntry(m.chatItems, "user", "/explorer locate runtime prompt") {
		t.Fatalf("expected synthesized slash input in chat history, got %#v", m.chatItems)
	}
}

func TestSubmitBuiltinSubAgentPreferencePersistsDisplayTextInChatHistory(t *testing.T) {
	runner := &subAgentCommandRunnerStub{
		builtinAgent: subagentspkg.Agent{Name: "review"},
		builtinOK:    true,
	}
	input := textarea.New()
	input.Focus()
	m := &model{
		runner: runner,
		sess:   session.New(t.TempDir()),
		async:  make(chan tea.Msg, 8),
		input:  input,
		screen: screenChat,
	}

	if err := m.submitBuiltinSubAgentPreference("/review inspect changed files", "review", "inspect changed files"); err != nil {
		t.Fatalf("expected slash preference submission to succeed, got %v", err)
	}
	if !m.busy {
		t.Fatal("expected slash preference to mark model busy")
	}
	if !containsChatEntry(m.chatItems, "user", "/review inspect changed files") {
		t.Fatalf("expected slash command input in chat history, got %#v", m.chatItems)
	}

	select {
	case msg := <-m.async:
		result, ok := msg.(subAgentResultMsg)
		if !ok {
			t.Fatalf("expected subAgentResultMsg, got %#v", msg)
		}
		if result.Err != nil {
			t.Fatalf("expected no dispatch error, got %v", result.Err)
		}
		if !strings.Contains(result.Response, "stub result") {
			t.Fatalf("expected dispatch result to contain stub summary, got %q", result.Response)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async dispatch result")
	}
}

func TestSubmitBuiltinSubAgentPreferenceUpdatesRunIndicator(t *testing.T) {
	runner := &subAgentCommandRunnerStub{
		builtinAgent: subagentspkg.Agent{Name: "review"},
		builtinOK:    true,
	}
	input := textarea.New()
	input.Focus()
	m := model{
		runner:            runner,
		sess:              session.New(t.TempDir()),
		async:             make(chan tea.Msg, 8),
		input:             input,
		screen:            screenChat,
		runIndicatorState: runIndicatorReady,
	}

	if err := m.submitBuiltinSubAgentPreference("/review inspect changed files", "review", "inspect changed files"); err != nil {
		t.Fatalf("expected slash preference submission to succeed, got %v", err)
	}
	if m.runIndicatorState != runIndicatorRunning {
		t.Fatalf("expected slash preference to set running indicator, got %q", m.runIndicatorState)
	}
	if m.runStartedAt.IsZero() {
		t.Fatal("expected slash preference to record run start time")
	}

	select {
	case msg := <-m.async:
		result, ok := msg.(subAgentResultMsg)
		if !ok {
			t.Fatalf("expected subAgentResultMsg, got %#v", msg)
		}
		next, _ := m.Update(result)
		updated := next.(model)
		if updated.runIndicatorState != runIndicatorComplete {
			t.Fatalf("expected completed indicator after subagent result, got %q", updated.runIndicatorState)
		}
		if updated.phase != "idle" {
			t.Fatalf("expected phase to become idle after subagent result, got %q", updated.phase)
		}
		if updated.runStartedAt != (time.Time{}) {
			t.Fatalf("expected run start time to reset after completion, got %v", updated.runStartedAt)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async dispatch result")
	}
}

func TestSubmitBuiltinSubAgentPreferenceConfiguresSpinnerAndTimeout(t *testing.T) {
	runner := &subAgentCommandRunnerStub{
		builtinAgent: subagentspkg.Agent{Name: "review"},
		builtinOK:    true,
	}
	input := textarea.New()
	input.Focus()
	m := model{
		runner:            runner,
		sess:              session.New(t.TempDir()),
		async:             make(chan tea.Msg, 8),
		input:             input,
		screen:            screenChat,
		runIndicatorState: runIndicatorReady,
	}

	if err := m.submitBuiltinSubAgentPreference("/review inspect changed files", "review", "inspect changed files"); err != nil {
		t.Fatalf("expected slash preference submission to succeed, got %v", err)
	}
	if m.pendingCommandCmd == nil {
		t.Fatal("expected slash preference to schedule spinner command")
	}

	select {
	case msg := <-m.async:
		if _, ok := msg.(subAgentResultMsg); !ok {
			t.Fatalf("expected subAgentResultMsg, got %#v", msg)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for async dispatch result")
	}

	if runner.lastRequest.Timeout != "" {
		t.Fatalf("expected empty subagent timeout (bounded by MaxTurns only), got %q", runner.lastRequest.Timeout)
	}
}

func TestSubAgentResultErrorSetsFailedIndicator(t *testing.T) {
	m := model{
		async:             make(chan tea.Msg, 1),
		busy:              true,
		subAgentPending:   true,
		subAgentName:      "review",
		runStartedAt:      time.Now().Add(-80 * time.Millisecond),
		runIndicatorState: runIndicatorRunning,
		screen:            screenChat,
	}

	next, _ := m.Update(subAgentResultMsg{
		Input: "/review inspect changed files",
		Err:   errors.New("boom"),
	})
	updated := next.(model)

	if updated.runIndicatorState != runIndicatorFailed {
		t.Fatalf("expected failed indicator after subagent error, got %q", updated.runIndicatorState)
	}
	if !containsChatEntry(updated.chatItems, "assistant", "Subagent failed: boom") {
		t.Fatalf("expected assistant error entry in chat, got %#v", updated.chatItems)
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

func TestCommandPaletteEnterOnModelOpensPicker(t *testing.T) {
	input := textarea.New()
	input.SetValue("/model picker")
	m := model{
		screen:      screenChat,
		commandOpen: true,
		input:       input,
		runner: &subAgentCommandRunnerStub{
			models: []provider.ModelInfo{
				{ProviderID: "openai", ModelID: "gpt-5.4"},
				{ProviderID: "deepseek", ModelID: "deepseek-chat"},
			},
		},
		cfg: config.Config{
			ProviderRuntime: config.ProviderRuntimeConfig{
				DefaultProvider: "openai",
				DefaultModel:    "gpt-5.4",
				Providers: map[string]config.ProviderConfig{
					"openai":   {Type: "openai-compatible", Model: "gpt-5.4"},
					"deepseek": {Type: "openai-compatible", Model: "deepseek-chat"},
				},
			},
		},
	}
	m.syncCommandPalette()

	got, _ := m.handleCommandPaletteKey(tea.KeyMsg{Type: tea.KeyEnter})
	updated := got.(model)

	if updated.commandOpen {
		t.Fatalf("expected command palette to close after opening model picker")
	}
	if !updated.modelsOpen {
		t.Fatal("expected model picker to open")
	}
	if len(updated.chatItems) != 0 {
		t.Fatalf("expected opening model picker not to append chat items, got %#v", updated.chatItems)
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
