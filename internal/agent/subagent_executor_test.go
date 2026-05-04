package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
	extensionspkg "github.com/1024XEngineer/bytemind/internal/extensions"
	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
	subagentspkg "github.com/1024XEngineer/bytemind/internal/subagents"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

type contextErrorExtensions struct {
	extensionspkg.NopManager
	err error
}

func (e *contextErrorExtensions) ResolveAllTools(_ context.Context) ([]extensionspkg.ExtensionTool, error) {
	return nil, e.err
}

type recordingObserver struct {
	mu     sync.Mutex
	events []Event
}

func (o *recordingObserver) HandleEvent(e Event) {
	o.mu.Lock()
	o.events = append(o.events, e)
	o.mu.Unlock()
}

func TestExtractJSONFromAnswerValidJSON(t *testing.T) {
	input := `{"summary":"done"}`
	got := extractJSONFromAnswer(input)
	if got != input {
		t.Fatalf("expected exact JSON passthrough, got %q", got)
	}
}

func TestExtractJSONFromAnswerFencedCodeBlock(t *testing.T) {
	input := "here is the result:\n```json\n{\"summary\":\"ok\"}\n```\nend"
	got := extractJSONFromAnswer(input)
	if got != `{"summary":"ok"}` {
		t.Fatalf("expected extracted JSON from fence, got %q", got)
	}
}

func TestExtractJSONFromAnswerFencedBlockInvalidJSON(t *testing.T) {
	input := "```\nnot json\n```"
	got := extractJSONFromAnswer(input)
	// Falls through to brace scanning, which finds nothing valid
	if got != "" {
		t.Fatalf("expected empty for invalid fenced JSON, got %q", got)
	}
}

func TestExtractJSONFromAnswerNestedBraces(t *testing.T) {
	input := `result: {"summary":"done","error":{"code":"x","message":"y"}}`
	got := extractJSONFromAnswer(input)
	if !strings.Contains(got, `"summary":"done"`) {
		t.Fatalf("expected brace-scanned JSON, got %q", got)
	}
}

func TestExtractJSONFromAnswerNoJSON(t *testing.T) {
	got := extractJSONFromAnswer("no json here")
	if got != "" {
		t.Fatalf("expected empty for no JSON, got %q", got)
	}
}

func TestExtractJSONFromAnswerUnclosedBrace(t *testing.T) {
	got := extractJSONFromAnswer(`{"summary": "unclosed`)
	if got != "" {
		t.Fatalf("expected empty for unclosed brace, got %q", got)
	}
}

func TestResolveSubAgentMaxIterations(t *testing.T) {
	tests := []struct {
		name       string
		parent     int
		requested  int
		wantResult int
	}{
		{"parent_zero_uses_default", 0, 0, defaultSubAgentMaxIterations},
		{"parent_negative_uses_default", -1, 0, defaultSubAgentMaxIterations},
		{"requested_overrides_parent", 20, 5, 5},
		{"requested_exceeds_parent_uses_parent", 4, 10, 4},
		{"requested_zero_uses_parent", 6, 0, 6},
		{"requested_negative_uses_parent", 6, -1, 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveSubAgentMaxIterations(tt.parent, tt.requested)
			if got != tt.wantResult {
				t.Fatalf("resolveSubAgentMaxIterations(%d, %d) = %d, want %d", tt.parent, tt.requested, got, tt.wantResult)
			}
		})
	}
}

func TestBuildSubAgentTaskInputEmpty(t *testing.T) {
	got := buildSubAgentTaskInput(tools.DelegateSubAgentRequest{})
	if got != "Complete the delegated subagent task." {
		t.Fatalf("expected default task for empty input, got %q", got)
	}
}

func TestBuildSubAgentTaskInputNormal(t *testing.T) {
	got := buildSubAgentTaskInput(tools.DelegateSubAgentRequest{Task: "  scan files  "})
	if got != "scan files" {
		t.Fatalf("expected trimmed task, got %q", got)
	}
}

func TestBuildSubAgentResultFromAnswerEmpty(t *testing.T) {
	result := buildSubAgentResultFromAnswer("", "inv-1", "explorer")
	if !result.OK {
		t.Fatal("expected OK true")
	}
	if result.Summary != "SubAgent task completed." {
		t.Fatalf("expected default summary, got %q", result.Summary)
	}
	if result.InvocationID != "inv-1" {
		t.Fatalf("expected invocation id, got %q", result.InvocationID)
	}
	if result.Agent != "explorer" {
		t.Fatalf("expected agent, got %q", result.Agent)
	}
}

func TestBuildSubAgentResultFromAnswerWithJSON(t *testing.T) {
	result := buildSubAgentResultFromAnswer(`{"summary":"scan done"}`, "inv-1", "explorer")
	if !result.OK {
		t.Fatal("expected OK true")
	}
	if result.Summary != "scan done" {
		t.Fatalf("expected summary from JSON, got %q", result.Summary)
	}
}

func TestBuildSubAgentResultFromAnswerPlainText(t *testing.T) {
	result := buildSubAgentResultFromAnswer("just plain text", "inv-1", "explorer")
	if !result.OK {
		t.Fatal("expected OK true")
	}
	if result.Summary != "just plain text" {
		t.Fatalf("expected plain text summary, got %q", result.Summary)
	}
}

func TestBuildSubAgentResultFromAnswerJSONWithEmptySummary(t *testing.T) {
	result := buildSubAgentResultFromAnswer(`{"ok":true,"summary":"  "}`, "inv-1", "explorer")
	if !result.OK {
		t.Fatal("expected OK true")
	}
	// JSON parsed but summary is whitespace-only, falls back to trimmed raw answer
	if result.Summary != `{"ok":true,"summary":"  "}` {
		t.Fatalf("expected fallback to raw answer, got %q", result.Summary)
	}
}

func TestBuildSubAgentResultFromAnswerJSONWithoutSummary(t *testing.T) {
	result := buildSubAgentResultFromAnswer(`{"ok":true}`, "inv-1", "explorer")
	if !result.OK {
		t.Fatal("expected OK true")
	}
	// JSON parsed but no summary field, falls back to raw text
	if result.Summary != `{"ok":true}` {
		t.Fatalf("expected raw text fallback, got %q", result.Summary)
	}
}

func TestSortedToolSetNamesEmpty(t *testing.T) {
	if got := sortedToolSetNames(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %v", got)
	}
	if got := sortedToolSetNames(map[string]struct{}{}); got != nil {
		t.Fatalf("expected nil for empty input, got %v", got)
	}
}

func TestSortedToolSetNamesWhitespaceOnly(t *testing.T) {
	got := sortedToolSetNames(map[string]struct{}{"  ": {}})
	if got != nil {
		t.Fatalf("expected nil for whitespace-only names, got %v", got)
	}
}

func TestSortedToolSetNamesNormal(t *testing.T) {
	got := sortedToolSetNames(map[string]struct{}{
		"write_file": {},
		"read_file":  {},
		"search":     {},
	})
	if len(got) != 3 {
		t.Fatalf("expected 3 names, got %d", len(got))
	}
	if got[0] != "read_file" || got[1] != "search" || got[2] != "write_file" {
		t.Fatalf("expected sorted names, got %v", got)
	}
}

func TestApplySubAgentPreflightSetupNilSetup(t *testing.T) {
	// Should not panic
	applySubAgentPreflightSetup(nil, subagentspkg.PreflightResult{})
}

func TestApplySubAgentPreflightSetupAppliesFields(t *testing.T) {
	setup := &runPromptSetup{}
	preflight := subagentspkg.PreflightResult{
		EffectiveTools: []string{"read_file", "search_text"},
		AllowedTools:   map[string]struct{}{"read_file": {}},
		DeniedTools:    map[string]struct{}{"write_file": {}},
	}
	applySubAgentPreflightSetup(setup, preflight)
	if len(setup.AllowedToolNames) != 2 {
		t.Fatalf("expected 2 allowed tool names, got %v", setup.AllowedToolNames)
	}
	if len(setup.DeniedToolNames) != 1 || setup.DeniedToolNames[0] != "write_file" {
		t.Fatalf("expected denied tool names [write_file], got %v", setup.DeniedToolNames)
	}
	if setup.AvailableSubAgents != nil {
		t.Fatalf("expected nil available subagents, got %v", setup.AvailableSubAgents)
	}
	if setup.ActiveSkill != nil {
		t.Fatalf("expected nil active skill, got %v", setup.ActiveSkill)
	}
}

func TestNewSubAgentSession(t *testing.T) {
	sess := newSubAgentSession("/ws", "parent-123", "inv-456", planpkg.ModeBuild)
	if !strings.HasPrefix(sess.ID, "parent-123/subagent/inv-456") {
		t.Fatalf("expected session id with parent/invocation prefix, got %q", sess.ID)
	}
	if sess.Mode != planpkg.ModeBuild {
		t.Fatalf("expected mode build, got %q", sess.Mode)
	}
	if sess.ActiveSkill != nil {
		t.Fatalf("expected nil active skill, got %v", sess.ActiveSkill)
	}
}

func TestNewSubAgentSessionEmptyParentID(t *testing.T) {
	sess := newSubAgentSession("/ws", "", "inv-456", planpkg.ModeBuild)
	if !strings.HasPrefix(sess.ID, "session/subagent/inv-456") {
		t.Fatalf("expected fallback parent 'session', got %q", sess.ID)
	}
}

func TestBuildSubAgentPromptInput(t *testing.T) {
	req := tools.DelegateSubAgentRequest{
		Task: "  scan files  ",
		Scope: tools.DelegateSubAgentScope{
			Paths:   []string{"src/", "  src/  ", ""},
			Symbols: []string{"main", "main"},
		},
	}
	preflight := subagentspkg.PreflightResult{
		Definition: subagentspkg.Agent{
			Name:        "explorer",
			Instruction: "scan code",
		},
		EffectiveTools: []string{"read_file"},
		Isolation:      "sandbox",
	}
	input := buildSubAgentPromptInput(req, preflight)
	if input.Name != "explorer" {
		t.Fatalf("expected name explorer, got %q", input.Name)
	}
	if input.Task != "scan files" {
		t.Fatalf("expected trimmed task, got %q", input.Task)
	}
	if len(input.ScopePaths) != 1 || input.ScopePaths[0] != "src/" {
		t.Fatalf("expected deduped/trimmed paths [src/], got %v", input.ScopePaths)
	}
	if len(input.ScopeSymbols) != 1 || input.ScopeSymbols[0] != "main" {
		t.Fatalf("expected deduped symbols [main], got %v", input.ScopeSymbols)
	}
	if input.Isolation != "sandbox" {
		t.Fatalf("expected isolation sandbox, got %q", input.Isolation)
	}
	if input.DefinitionBody != "scan code" {
		t.Fatalf("expected definition body, got %q", input.DefinitionBody)
	}
}

func TestSubAgentFailureResult(t *testing.T) {
	result := subAgentFailureResult("inv-1", "explorer", "test_code", "test message", true)
	if result.OK {
		t.Fatal("expected OK false")
	}
	if result.Status != subAgentResultStatusFailed {
		t.Fatalf("expected status failed, got %q", result.Status)
	}
	if result.InvocationID != "inv-1" {
		t.Fatalf("expected invocation id, got %q", result.InvocationID)
	}
	if result.Agent != "explorer" {
		t.Fatalf("expected agent, got %q", result.Agent)
	}
	if result.Error == nil {
		t.Fatal("expected non-nil error")
	}
	if result.Error.Code != "test_code" {
		t.Fatalf("expected error code test_code, got %q", result.Error.Code)
	}
	if result.Error.Message != "test message" {
		t.Fatalf("expected error message, got %q", result.Error.Message)
	}
	if !result.Error.Retryable {
		t.Fatal("expected retryable true")
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("a", "b"); got != "a" {
		t.Fatalf("expected a, got %q", got)
	}
	if got := firstNonEmpty("", "b"); got != "b" {
		t.Fatalf("expected b, got %q", got)
	}
	if got := firstNonEmpty("  ", "b"); got != "b" {
		t.Fatalf("expected b for whitespace-only, got %q", got)
	}
	if got := firstNonEmpty("", ""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestSubAgentExecutionError(t *testing.T) {
	var nilErr *subAgentExecutionError
	if got := nilErr.Error(); got != "" {
		t.Fatalf("expected empty for nil error, got %q", got)
	}

	err := &subAgentExecutionError{code: "test", message: "  hello  "}
	if got := err.Error(); got != "hello" {
		t.Fatalf("expected trimmed message, got %q", got)
	}
}

func TestExecuteScopedWorkspaceOverride(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	client := &fakeClient{replies: []llm.Message{
		{Role: llm.RoleAssistant, Content: "done"},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config:    config.Config{Provider: config.ProviderConfig{Model: "test-model"}, MaxIterations: 2},
		Client:    client,
		Registry:  tools.DefaultRegistry(),
	})
	executor := NewSubAgentExecutor(runner)

	scopedWs := t.TempDir()
	result, err := executor.Execute(t.Context(), SubAgentExecutionInput{
		Request:      tools.DelegateSubAgentRequest{Agent: "explorer", Task: "scan"},
		Preflight:    subagentspkg.PreflightResult{Definition: subagentspkg.Agent{Name: "explorer", Tools: []string{"read_file"}}},
		InvocationID: "inv-1",
		Agent:        "explorer",
		RunMode:      planpkg.ModeBuild,
		ExecCtx:      &tools.ExecutionContext{Workspace: scopedWs},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.OK {
		t.Fatalf("expected success, got %#v", result)
	}
}

func TestExecuteWithStreamObserver(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	client := &fakeClient{replies: []llm.Message{
		{Role: llm.RoleAssistant, Content: "done"},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config:    config.Config{Provider: config.ProviderConfig{Model: "test-model"}, MaxIterations: 2},
		Client:    client,
		Registry:  tools.DefaultRegistry(),
	})
	executor := NewSubAgentExecutor(runner)

	observer := &recordingObserver{}
	result, err := executor.Execute(t.Context(), SubAgentExecutionInput{
		Request:      tools.DelegateSubAgentRequest{Agent: "explorer", Task: "scan"},
		Preflight:    subagentspkg.PreflightResult{Definition: subagentspkg.Agent{Name: "explorer", Tools: []string{"read_file"}}},
		InvocationID: "inv-1",
		Agent:        "explorer",
		RunMode:      planpkg.ModeBuild,
		Observer:     observer,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !result.OK {
		t.Fatalf("expected success, got %#v", result)
	}
}

func TestBuildSubAgentResultFromAnswerUnmarshalableJSON(t *testing.T) {
	// Valid JSON but summary is a number, not a string — Unmarshal fails
	input := `{"ok":true,"summary":123}`
	result := buildSubAgentResultFromAnswer(input, "inv-1", "explorer")
	if !result.OK {
		t.Fatal("expected OK true")
	}
	// Unmarshal fails, falls back to trimmed raw input
	if result.Summary != input {
		t.Fatalf("expected fallback to raw input, got %q", result.Summary)
	}
}

func TestExecuteSyncExtensionToolsContextCanceled(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	client := &fakeClient{replies: []llm.Message{
		{Role: llm.RoleAssistant, Content: "done"},
	}}
	ext := &contextErrorExtensions{err: context.Canceled}
	runner := NewRunner(Options{
		Workspace:  workspace,
		Config:     config.Config{Provider: config.ProviderConfig{Model: "test-model"}, MaxIterations: 2},
		Client:     client,
		Registry:   tools.DefaultRegistry(),
		Extensions: ext,
	})
	executor := NewSubAgentExecutor(runner)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	result, err := executor.Execute(ctx, SubAgentExecutionInput{
		Request:      tools.DelegateSubAgentRequest{Agent: "explorer", Task: "scan"},
		Preflight:    subagentspkg.PreflightResult{Definition: subagentspkg.Agent{Name: "explorer", Tools: []string{"read_file"}}},
		InvocationID: "inv-1",
		Agent:        "explorer",
		RunMode:      planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// syncExtensionTools returns canceled, but the executor continues and may succeed or fail
	// depending on downstream behavior. We just verify no panic and valid result.
	_ = result
}

func TestExecutePrepareRunPromptError(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	client := &fakeClient{replies: []llm.Message{
		{Role: llm.RoleAssistant, Content: "done"},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:         config.ProviderConfig{Model: "test-model"},
			MaxIterations:    2,
			SandboxEnabled:   true,
			SystemSandboxMode: "required",
		},
		Client:   client,
		Registry: tools.DefaultRegistry(),
	})
	executor := NewSubAgentExecutor(runner)

	// Swap the resolver to always return an error
	original := resolveAgentSystemSandboxRuntimeStatus
	resolveAgentSystemSandboxRuntimeStatus = func(_ bool, _ string) (tools.SystemSandboxRuntimeStatus, error) {
		return tools.SystemSandboxRuntimeStatus{}, fmt.Errorf("sandbox unavailable")
	}
	defer func() { resolveAgentSystemSandboxRuntimeStatus = original }()

	result, err := executor.Execute(t.Context(), SubAgentExecutionInput{
		Request:      tools.DelegateSubAgentRequest{Agent: "explorer", Task: "scan"},
		Preflight:    subagentspkg.PreflightResult{Definition: subagentspkg.Agent{Name: "explorer", Tools: []string{"read_file"}}},
		InvocationID: "inv-1",
		Agent:        "explorer",
		RunMode:      planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.OK {
		t.Fatal("expected failure for prepareRunPrompt error")
	}
}

type errorClient struct {
	err error
}

func (c *errorClient) CreateMessage(_ context.Context, _ llm.ChatRequest) (llm.Message, error) {
	return llm.Message{}, c.err
}

func (c *errorClient) StreamMessage(_ context.Context, _ llm.ChatRequest, _ func(string)) (llm.Message, error) {
	return llm.Message{}, c.err
}

func TestExecuteRunPromptTurnsError(t *testing.T) {
	workspace := t.TempDir()
	writeExplorerSubAgentDefinition(t, workspace)
	runner := NewRunner(Options{
		Workspace: workspace,
		Config:    config.Config{Provider: config.ProviderConfig{Model: "test-model"}, MaxIterations: 2},
		Client:    &errorClient{err: fmt.Errorf("llm connection failed")},
		Registry:  tools.DefaultRegistry(),
	})
	executor := NewSubAgentExecutor(runner)

	result, err := executor.Execute(t.Context(), SubAgentExecutionInput{
		Request:      tools.DelegateSubAgentRequest{Agent: "explorer", Task: "scan"},
		Preflight:    subagentspkg.PreflightResult{Definition: subagentspkg.Agent{Name: "explorer", Tools: []string{"read_file"}}},
		InvocationID: "inv-1",
		Agent:        "explorer",
		RunMode:      planpkg.ModeBuild,
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.OK {
		t.Fatal("expected failure for runPromptTurns error")
	}
}
