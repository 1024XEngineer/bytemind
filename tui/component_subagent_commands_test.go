package tui

import (
	"context"
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
	subAgents    []subagentspkg.Agent
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

func (s *subAgentCommandRunnerStub) SubAgentManager() *subagentspkg.Manager {
	return nil
}

func (s *subAgentCommandRunnerStub) ListModels(context.Context) ([]provider.ModelInfo, []provider.Warning, error) {
	return s.models, s.warnings, s.modelsErr
}

func (s *subAgentCommandRunnerStub) ListSubAgents() ([]subagentspkg.Agent, []subagentspkg.Diagnostic) {
	return s.subAgents, nil
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

func TestRunAgentsCommandAppendsCommandExchange(t *testing.T) {
	m := &model{
		runner: &subAgentCommandRunnerStub{
			subAgents: []subagentspkg.Agent{
				{Name: "explorer", Scope: "builtin", Description: "Explore code"},
				{Name: "review", Scope: "workspace", Description: ""},
			},
		},
	}

	if err := m.runAgentsCommand("/agents"); err != nil {
		t.Fatalf("expected runAgentsCommand to succeed, got %v", err)
	}
	if !containsChatEntry(m.chatItems, "assistant", "Available subagents:") {
		t.Fatalf("expected /agents exchange output in chat, got %#v", m.chatItems)
	}
	if !containsChatEntry(m.chatItems, "assistant", "- review [workspace]: No description provided.") {
		t.Fatalf("expected missing descriptions to use fallback text, got %#v", m.chatItems)
	}
	if m.statusNote != "Discovered 2 subagent(s)." {
		t.Fatalf("expected status note to reflect discovered count, got %q", m.statusNote)
	}
}

func TestRenderSubAgentsViewFallbacks(t *testing.T) {
	if got := renderSubAgentsView(nil); got != "No subagents available." {
		t.Fatalf("expected empty agent set fallback, got %q", got)
	}

	view := renderSubAgentsView([]subagentspkg.Agent{
		{Name: "review", Scope: "builtin", Description: "   "},
	})
	if !strings.Contains(view, "- review [builtin]: No description provided.") {
		t.Fatalf("expected renderSubAgentsView description fallback, got %q", view)
	}
}

func TestRenderSubAgentResultCardCompletedAndErrorStates(t *testing.T) {
	completed := stripANSI(renderSubAgentResultCard(tools.DelegateSubAgentResult{
		OK:           true,
		Agent:        "",
		TaskID:       "task-1",
		InvocationID: "inv-1",
		Summary:      "",
	}, 32))
	for _, want := range []string{
		"SubAgent",
		"subagent",
		"COMPLETED",
		"task: task-1",
		"invocation: inv-1",
		"SubAgent task completed.",
	} {
		if !strings.Contains(completed, want) {
			t.Fatalf("expected completed card to contain %q, got %q", want, completed)
		}
	}

	failed := stripANSI(renderSubAgentResultCard(tools.DelegateSubAgentResult{
		OK:     false,
		Agent:  "reviewer",
		Status: "",
		Error: &tools.DelegateSubAgentError{
			Code:    "timeout",
			Message: "worker exceeded time budget",
		},
	}, 80))
	for _, want := range []string{"SubAgent", "reviewer", "FAILED", "Error:", "[timeout]", "worker exceeded time budget"} {
		if !strings.Contains(failed, want) {
			t.Fatalf("expected failed card to contain %q, got %q", want, failed)
		}
	}
}

func TestRenderSubAgentDispatchResultUsesCardRenderer(t *testing.T) {
	out := stripANSI(renderSubAgentDispatchResult(tools.DelegateSubAgentResult{
		OK:      true,
		Agent:   "explorer",
		Status:  "running",
		Summary: "scanning files",
	}))
	for _, want := range []string{"SubAgent", "explorer", "RUNNING", "scanning files"} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected dispatch output to contain %q, got %q", want, out)
		}
	}
}

func TestFormatElapsedBranches(t *testing.T) {
	if got := formatElapsed(450 * time.Millisecond); got != "0s" {
		t.Fatalf("expected sub-second duration to clamp to 0s, got %q", got)
	}
	if got := formatElapsed(9 * time.Second); got != "9s" {
		t.Fatalf("expected seconds formatting, got %q", got)
	}
	if got := formatElapsed(125 * time.Second); got != "2m5s" {
		t.Fatalf("expected minute-second formatting, got %q", got)
	}
}

func TestRenderSubAgentProgressCardIncludesTaskAndElapsed(t *testing.T) {
	progress := stripANSI(renderSubAgentProgressCard(
		"explorer",
		"inspect module boundaries and collect critical test paths for regression coverage",
		"◐",
		"12s",
		40,
	))
	for _, want := range []string{"explorer", "Elapsed: 12s"} {
		if !strings.Contains(progress, want) {
			t.Fatalf("expected progress card to contain %q, got %q", want, progress)
		}
	}
	if !strings.Contains(progress, "...") {
		t.Fatalf("expected long task text to be truncated, got %q", progress)
	}
}

func TestSubAgentStatusBadgeTypeAndBorderAccentMappings(t *testing.T) {
	tests := []struct {
		status    string
		wantBadge string
		wantColor string
	}{
		{status: "completed", wantBadge: "success", wantColor: string(semanticColors.Success)},
		{status: "failed", wantBadge: "error", wantColor: string(semanticColors.Danger)},
		{status: "accepted", wantBadge: "accent", wantColor: string(semanticColors.Accent)},
		{status: "queued", wantBadge: "accent", wantColor: string(semanticColors.Accent)},
		{status: "running", wantBadge: "accent", wantColor: string(semanticColors.Accent)},
		{status: "unknown", wantBadge: "neutral", wantColor: string(semanticColors.Border)},
	}

	for _, tc := range tests {
		if got := subAgentStatusBadgeType(tc.status); got != tc.wantBadge {
			t.Fatalf("status=%q expected badge %q, got %q", tc.status, tc.wantBadge, got)
		}
		if got := string(subAgentBorderAccent(tc.status)); got != tc.wantColor {
			t.Fatalf("status=%q expected border color %q, got %q", tc.status, tc.wantColor, got)
		}
	}
}

func TestWrapTextBreaksOnSpaceAndHardCuts(t *testing.T) {
	spaceWrapped := wrapText("alpha beta gamma", 6)
	if !strings.Contains(spaceWrapped, "\n") || !strings.Contains(spaceWrapped, "alpha") {
		t.Fatalf("expected wrapText to split by width with spaces, got %q", spaceWrapped)
	}

	hardCut := wrapText("abcdefghij", 4)
	if hardCut != "abcd\nefgh\nij" {
		t.Fatalf("expected wrapText hard-cut fallback, got %q", hardCut)
	}

	if got := wrapText("short", 0); got != "short" {
		t.Fatalf("expected non-positive width to return original text, got %q", got)
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

func TestResolveSubAgentToolCallsFromMetaNil(t *testing.T) {
	msg := llm.Message{}
	got := resolveSubAgentToolCallsFromMeta(msg)
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestResolveSubAgentToolCallsFromMetaFromSlice(t *testing.T) {
	input := []SubAgentToolCall{
		{ToolName: "read_file", ToolCallID: "c1", CompactBody: "main.go", Status: "done"},
	}
	msg := llm.Message{
		Meta: llm.MessageMeta{"subagent_tool_calls": input},
	}
	got := resolveSubAgentToolCallsFromMeta(msg)
	if len(got) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(got))
	}
	if got[0].ToolName != "read_file" || got[0].CompactBody != "main.go" {
		t.Errorf("expected read_file/main.go, got %s/%s", got[0].ToolName, got[0].CompactBody)
	}
}

func TestResolveSubAgentToolCallsFromMetaFromAnySlice(t *testing.T) {
	// Simulate JSON deserialization: []any with map[string]any elements.
	input := []any{
		map[string]any{
			"ToolName":    "grep",
			"ToolCallID":  "c2",
			"CompactBody": `"pattern"`,
			"Status":      "done",
		},
	}
	msg := llm.Message{
		Meta: llm.MessageMeta{"subagent_tool_calls": input},
	}
	got := resolveSubAgentToolCallsFromMeta(msg)
	if len(got) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(got))
	}
	if got[0].ToolName != "grep" {
		t.Errorf("expected tool name 'grep', got %q", got[0].ToolName)
	}
}

func TestResolveSubAgentToolCallsFromMetaInvalidAnySlice(t *testing.T) {
	msg := llm.Message{
		Meta: llm.MessageMeta{
			"subagent_tool_calls": []any{
				map[string]any{
					"ToolName": 123,
					"Status":   "done",
				},
			},
		},
	}
	if got := resolveSubAgentToolCallsFromMeta(msg); got != nil {
		t.Fatalf("expected invalid []any payload to fail decoding, got %#v", got)
	}
}

func TestRebuildSessionTimelineWithSubAgentTools(t *testing.T) {
	sess := session.New("/workspace")
	sess.Messages = []llm.Message{
		{Role: llm.RoleAssistant, Parts: []llm.Part{
			{Type: llm.PartToolUse, ToolUse: &llm.ToolUsePart{
				ID:        "call-1",
				Name:      "delegate_subagent",
				Arguments: `{"agent":"explorer","task":"scan files"}`,
			}},
		}},
		{Role: llm.RoleUser, Parts: []llm.Part{
			{Type: llm.PartToolResult, ToolResult: &llm.ToolResultPart{
				ToolUseID: "call-1",
				Content:   `{"ok":true,"summary":"done","invocation_id":"inv-1"}`,
			}},
		}},
	}
	// Inject subagent_tool_calls into Meta.
	if sess.Messages[1].Meta == nil {
		sess.Messages[1].Meta = llm.MessageMeta{}
	}
	sess.Messages[1].Meta["subagent_tool_calls"] = []SubAgentToolCall{
		{ToolName: "read_file", ToolCallID: "c10", CompactBody: "a.go", Status: "done"},
		{ToolName: "grep", ToolCallID: "c11", CompactBody: `"pattern"`, Status: "done"},
	}

	items := rebuildSessionTimeline(sess)
	// Find the tool entry for delegate_subagent.
	found := false
	for _, item := range items {
		if item.Kind == "tool" && item.AgentID == "explorer" {
			found = true
			if len(item.SubAgentTools) != 2 {
				t.Errorf("expected 2 SubAgentTools, got %d", len(item.SubAgentTools))
			}
			if len(item.SubAgentTools) > 0 && item.SubAgentTools[0].ToolName != "read_file" {
				t.Errorf("expected first tool 'read_file', got %q", item.SubAgentTools[0].ToolName)
			}
			break
		}
	}
	if !found {
		t.Fatal("expected to find delegate_subagent tool entry")
	}
}

func TestRebuildSessionTimelineWithoutSubAgentToolsMeta(t *testing.T) {
	sess := session.New("/workspace")
	sess.Messages = []llm.Message{
		{Role: llm.RoleAssistant, Parts: []llm.Part{
			{Type: llm.PartToolUse, ToolUse: &llm.ToolUsePart{
				ID:        "call-1",
				Name:      "delegate_subagent",
				Arguments: `{"agent":"explorer","task":"scan"}`,
			}},
		}},
		{Role: llm.RoleUser, Parts: []llm.Part{
			{Type: llm.PartToolResult, ToolResult: &llm.ToolResultPart{
				ToolUseID: "call-1",
				Content:   `{"ok":true,"summary":"done","invocation_id":"inv-1"}`,
			}},
		}},
	}

	items := rebuildSessionTimeline(sess)
	for _, item := range items {
		if item.Kind == "tool" && item.AgentID == "explorer" {
			if len(item.SubAgentTools) != 0 {
				t.Errorf("expected 0 SubAgentTools without meta, got %d", len(item.SubAgentTools))
			}
			return
		}
	}
}

func TestRebuildSessionTimelineUsesFullDelegateSubAgentResultFromMeta(t *testing.T) {
	sess := session.New("/workspace")
	sess.Messages = []llm.Message{
		{
			Role: llm.RoleAssistant,
			Parts: []llm.Part{
				{
					Type: llm.PartToolUse,
					ToolUse: &llm.ToolUsePart{
						ID:        "call-1",
						Name:      "delegate_subagent",
						Arguments: `{"agent":"explorer","task":"scan files"}`,
					},
				},
			},
		},
		{
			Role: llm.RoleUser,
			Parts: []llm.Part{
				{
					Type: llm.PartToolResult,
					ToolResult: &llm.ToolResultPart{
						ToolUseID: "call-1",
						Content:   `{"ok":true,"agent":"explorer","summary":"trimmed summary"}`,
					},
				},
			},
			Meta: llm.MessageMeta{
				"delegate_subagent_result": `{"ok":true,"agent":"explorer","task":"scan files","content":"full natural language result from subagent"}`,
			},
		},
	}

	items := rebuildSessionTimeline(sess)
	for _, item := range items {
		if item.Kind != "tool" || item.AgentID != "explorer" {
			continue
		}
		if item.TaskPrompt != "scan files" {
			t.Fatalf("expected task prompt to be restored from arguments, got %q", item.TaskPrompt)
		}
		if !strings.Contains(item.Body, "full natural language result from subagent") {
			t.Fatalf("expected full delegate result payload to be used, got %q", item.Body)
		}
		return
	}
	t.Fatal("expected to find restored delegate_subagent tool entry")
}

func TestRebuildSessionTimelineRestoresDelegateSubAgentToolRoleMessage(t *testing.T) {
	sess := session.New("/workspace")
	sess.Messages = []llm.Message{
		{
			Role: llm.RoleAssistant,
			ToolCalls: []llm.ToolCall{
				{
					ID: "call-2",
					Function: llm.ToolFunctionCall{
						Name:      "delegate_subagent",
						Arguments: `{"agent":"reviewer","task":"verify docs"}`,
					},
				},
			},
		},
		{
			Role:       llm.Role("tool"),
			ToolCallID: "call-2",
			Content:    `{"ok":true,"agent":"reviewer","summary":"trimmed"}`,
			Meta: llm.MessageMeta{
				"delegate_subagent_result": `{"ok":true,"agent":"reviewer","task":"verify docs","content":"tool-role full payload"}`,
				"subagent_tool_calls": []any{
					map[string]any{
						"ToolName":    "read_file",
						"ToolCallID":  "tc-1",
						"CompactBody": "docs.md",
						"Status":      "done",
					},
				},
			},
		},
	}

	items := rebuildSessionTimeline(sess)
	for _, item := range items {
		if item.Kind != "tool" || item.AgentID != "reviewer" {
			continue
		}
		if item.TaskPrompt != "verify docs" {
			t.Fatalf("expected task prompt to be restored for tool-role message, got %q", item.TaskPrompt)
		}
		if item.TotalToolCalls != 1 || len(item.SubAgentTools) != 1 {
			t.Fatalf("expected one restored subagent tool call, got total=%d items=%d", item.TotalToolCalls, len(item.SubAgentTools))
		}
		if item.SubAgentTools[0].ToolName != "read_file" {
			t.Fatalf("expected restored subagent tool to be read_file, got %#v", item.SubAgentTools[0])
		}
		if !strings.Contains(item.Body, "tool-role full payload") {
			t.Fatalf("expected full tool-role delegate payload to be used, got %q", item.Body)
		}
		return
	}
	t.Fatal("expected to find tool-role delegate_subagent entry")
}
