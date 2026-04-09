package agent

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
	"time"
	"unicode"

	"bytemind/internal/config"
	"bytemind/internal/llm"
	"bytemind/internal/session"
	"bytemind/internal/tokenusage"
	"bytemind/internal/tools"
)

type fakeClient struct {
	replies  []llm.Message
	requests []llm.ChatRequest
	index    int
}

func (f *fakeClient) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
	f.requests = append(f.requests, req)
	if len(f.replies) == 0 {
		return llm.Message{}, nil
	}
	if f.index >= len(f.replies) {
		return f.replies[len(f.replies)-1], nil
	}
	message := f.replies[f.index]
	f.index++
	return message, nil
}

func (f *fakeClient) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	message, err := f.CreateMessage(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	if onDelta != nil && message.Content != "" {
		onDelta(message.Content)
	}
	return message, nil
}

func TestRunPromptReturnsBudgetSummaryInsteadOfError(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 1,
			Stream:        false,
		},
		Client: &fakeClient{replies: []llm.Message{{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		}}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "inspect workspace", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer, "Paused before a final answer.") {
		t.Fatalf("expected pause summary, got %q", answer)
	}
	if !strings.Contains(answer, "execution budget of 1 turns") {
		t.Fatalf("expected budget detail, got %q", answer)
	}
}

func TestRunPromptStopsOnRepeatedToolPlan(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	repeatedReply := llm.Message{
		Role: "assistant",
		ToolCalls: []llm.ToolCall{{
			ID:   "call-repeat",
			Type: "function",
			Function: llm.ToolFunctionCall{
				Name:      "list_files",
				Arguments: `{}`,
			},
		}},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 5,
			Stream:        false,
		},
		Client:   &fakeClient{replies: []llm.Message{repeatedReply, repeatedReply, repeatedReply, repeatedReply}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "looping task", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer, "repeated the same tool sequence") {
		t.Fatalf("expected repeat-detection summary, got %q", answer)
	}
}

func TestRunPromptCompletesMinimalToolLoop(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &fakeClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "list_files",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "Workspace inspected.",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "inspect workspace", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Workspace inspected." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(sess.Messages) != 4 {
		t.Fatalf("expected 4 session messages, got %#v", sess.Messages)
	}
	if sess.Messages[0].Role != "user" {
		t.Fatalf("expected first message to be user, got %#v", sess.Messages[0])
	}
	if len(sess.Messages[1].ToolCalls) != 1 || sess.Messages[1].ToolCalls[0].Function.Name != "list_files" {
		t.Fatalf("expected second message to record tool call, got %#v", sess.Messages[1])
	}
	if sess.Messages[2].Role != "user" || !strings.Contains(sess.Messages[2].Content, `"items"`) {
		t.Fatalf("expected third message to be tool result, got %#v", sess.Messages[2])
	}
	if len(sess.Messages[2].Parts) != 1 || sess.Messages[2].Parts[0].ToolResult == nil {
		t.Fatalf("expected third message to carry tool_result part, got %#v", sess.Messages[2])
	}
	if sess.Messages[3].Role != "assistant" || sess.Messages[3].Content != "Workspace inspected." {
		t.Fatalf("expected final assistant message, got %#v", sess.Messages[3])
	}
}

func TestRunPromptAutoCompactsLongHistory(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	for i := 0; i < 8; i++ {
		sess.Messages = append(sess.Messages,
			llm.NewUserTextMessage(strings.Repeat("history user segment ", 30)),
			llm.NewAssistantTextMessage(strings.Repeat("history assistant segment ", 30)),
		)
	}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "Goal: keep working\nDecisions: use go tests\nPending: continue implementation"},
			{Role: llm.RoleAssistant, Content: "done"},
		},
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    220,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "continue implementation", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) != 2 {
		t.Fatalf("expected one compaction request + one turn request, got %d", len(client.requests))
	}
	if len(client.requests[0].Tools) != 0 {
		t.Fatalf("expected compaction request to disable tools, got %#v", client.requests[0].Tools)
	}
	if len(client.requests[0].Messages) < 2 || client.requests[0].Messages[0].Role != llm.RoleSystem {
		t.Fatalf("expected compaction request with system prompt, got %#v", client.requests[0].Messages)
	}
	if !strings.Contains(strings.ToLower(client.requests[0].Messages[0].Text()), "compaction") {
		t.Fatalf("expected compaction system prompt, got %q", client.requests[0].Messages[0].Text())
	}
	if len(sess.Messages) != 3 {
		t.Fatalf("expected compacted session to keep summary + latest user + final assistant, got %#v", sess.Messages)
	}
	if sess.Messages[0].Role != llm.RoleAssistant || !strings.Contains(sess.Messages[0].Text(), "Goal: keep working") {
		t.Fatalf("expected first message to be compaction summary, got %#v", sess.Messages[0])
	}
	if sess.Messages[1].Role != llm.RoleUser || strings.TrimSpace(sess.Messages[1].Text()) != "continue implementation" {
		t.Fatalf("expected latest user message to be preserved, got %#v", sess.Messages[1])
	}
}

func TestCompactSessionManualRewritesHistory(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.Messages = append(sess.Messages,
		llm.NewUserTextMessage("first ask"),
		llm.NewAssistantTextMessage("first answer"),
		llm.NewUserTextMessage("second ask"),
		llm.NewAssistantTextMessage("second answer"),
	)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &fakeClient{
		replies: []llm.Message{
			{Role: llm.RoleAssistant, Content: "Goal: first ask\nCompleted: first answer\nPending: second ask"},
		},
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    5000,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	summary, changed, err := runner.CompactSession(context.Background(), sess)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatalf("expected compaction to change session state")
	}
	if strings.TrimSpace(summary) == "" {
		t.Fatalf("expected non-empty compaction summary")
	}
	if len(sess.Messages) != 1 {
		t.Fatalf("expected compacted session to keep one summary message, got %#v", sess.Messages)
	}
	if sess.Messages[0].Role != llm.RoleAssistant {
		t.Fatalf("expected compacted summary role assistant, got %#v", sess.Messages[0])
	}
	if !strings.Contains(sess.Messages[0].Text(), "Goal: first ask") {
		t.Fatalf("expected summary content to be persisted, got %#v", sess.Messages[0])
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected one compaction request, got %d", len(client.requests))
	}
}

func TestRunPromptEncodesToolExecutionErrorsAndContinues(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	client := &fakeClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "missing_tool",
					Arguments: `{}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "Recovered after tool failure.",
		},
	}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "trigger failing tool", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Recovered after tool failure." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(sess.Messages) != 4 {
		t.Fatalf("expected 4 session messages, got %#v", sess.Messages)
	}
	if sess.Messages[2].Role != "user" {
		t.Fatalf("expected third message to be tool result, got %#v", sess.Messages[2])
	}
	if len(sess.Messages[2].Parts) != 1 || sess.Messages[2].Parts[0].ToolResult == nil {
		t.Fatalf("expected third message to carry tool_result part, got %#v", sess.Messages[2])
	}
	if !strings.Contains(sess.Messages[2].Content, `"ok":false`) || !strings.Contains(sess.Messages[2].Content, `unknown tool`) {
		t.Fatalf("expected encoded tool error payload, got %q", sess.Messages[2].Content)
	}
}

func TestRunPromptFallsBackWhenAssistantReplyIsEmpty(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client: &fakeClient{replies: []llm.Message{{
			Role: "assistant",
		}}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "hello", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer, "Model returned an empty response") {
		t.Fatalf("expected fallback message, got %q", answer)
	}
	if len(sess.Messages) != 2 {
		t.Fatalf("expected user and assistant messages, got %#v", sess.Messages)
	}
	if strings.TrimSpace(sess.Messages[1].Content) == "" {
		t.Fatalf("expected persisted assistant fallback message, got %#v", sess.Messages[1])
	}
}

func TestRunPromptEmitsUsageUpdatedEventWhenUsageAvailable(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	events := make([]Event, 0, 4)

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client: &fakeClient{replies: []llm.Message{{
			Role:    llm.RoleAssistant,
			Content: "done",
			Usage: &llm.Usage{
				InputTokens:   100,
				OutputTokens:  20,
				ContextTokens: 5,
				TotalTokens:   125,
			},
		}}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Observer: ObserverFunc(func(event Event) {
			events = append(events, event)
		}),
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
	})

	_, err = runner.RunPrompt(context.Background(), sess, "hello", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, event := range events {
		if event.Type == EventUsageUpdated {
			found = true
			if event.Usage.TotalTokens != 125 || event.Usage.InputTokens != 100 || event.Usage.OutputTokens != 20 || event.Usage.ContextTokens != 5 {
				t.Fatalf("unexpected usage payload: %+v", event.Usage)
			}
		}
	}
	if !found {
		t.Fatalf("expected EventUsageUpdated to be emitted, got %+v", events)
	}
}

func TestRunPromptEmitsEstimatedUsageWhenProviderUsageMissing(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	events := make([]Event, 0, 4)

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client: &fakeClient{replies: []llm.Message{{
			Role:    llm.RoleAssistant,
			Content: "plain answer without usage payload",
		}}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Observer: ObserverFunc(func(event Event) {
			events = append(events, event)
		}),
		Stdin:  strings.NewReader(""),
		Stdout: io.Discard,
	})

	_, err = runner.RunPrompt(context.Background(), sess, "hello", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, event := range events {
		if event.Type == EventUsageUpdated {
			found = true
			if event.Usage.TotalTokens <= 0 {
				t.Fatalf("expected estimated usage tokens > 0, got %+v", event.Usage)
			}
		}
	}
	if !found {
		t.Fatalf("expected estimated EventUsageUpdated to be emitted, got %+v", events)
	}
}

func TestGetTokenRealtimeSnapshotReturnsSessionAndGlobalStats(t *testing.T) {
	manager, err := tokenusage.NewTokenUsageManager(&tokenusage.Config{
		StorageType:    "memory",
		EnableRealtime: true,
		BackupInterval: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()

	runner := NewRunner(Options{
		Workspace:    t.TempDir(),
		Config:       config.Config{},
		Client:       &fakeClient{},
		TokenManager: manager,
	})

	if err := manager.RecordTokenUsage(context.Background(), &tokenusage.TokenRecordRequest{
		SessionID:    "sess-1",
		ModelName:    "gpt-5.4",
		InputTokens:  20,
		OutputTokens: 8,
		Latency:      150 * time.Millisecond,
		Success:      true,
	}); err != nil {
		t.Fatal(err)
	}

	snapshot, err := runner.GetTokenRealtimeSnapshot("sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SessionTotalTokens != 28 || snapshot.SessionInputTokens != 20 || snapshot.SessionOutputTokens != 8 {
		t.Fatalf("unexpected session snapshot: %+v", snapshot)
	}
	if snapshot.GlobalTotalTokens != 28 {
		t.Fatalf("expected global total 28, got %d", snapshot.GlobalTotalTokens)
	}
	if snapshot.ActiveSessions < 1 {
		t.Fatalf("expected active sessions >= 1, got %d", snapshot.ActiveSessions)
	}
}

func TestCompactWhitespacePreservesUTF8WhenTruncating(t *testing.T) {
	text := "Continue previous context and list the key MVP test points."
	got := compactWhitespace(text, 18)
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("expected valid utf-8 preview, got %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated preview to end with ellipsis, got %q", got)
	}
}

func TestRunPromptAppliesActiveSkillToolAllowlist(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "skill.json"), []byte(`{
  "name":"review",
  "description":"Review changes",
  "tools":{"policy":"allowlist","items":["read_file","search_text","list_files"]}
}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "SKILL.md"), []byte("# review\nCheck correctness."), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.ActiveSkill = &session.ActiveSkill{Name: "review"}

	client := &fakeClient{replies: []llm.Message{{
		Role:    "assistant",
		Content: "reviewed",
	}}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "review this", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "reviewed" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) == 0 {
		t.Fatal("expected at least one request")
	}
	if len(client.requests[0].Messages) == 0 || client.requests[0].Messages[0].Role != "system" {
		t.Fatalf("expected first request message to be system prompt, got %#v", client.requests[0].Messages)
	}
	if !strings.Contains(client.requests[0].Messages[0].Content, "[Available Skills]") ||
		!strings.Contains(client.requests[0].Messages[0].Content, "- review: Review changes enabled=true") {
		t.Fatalf("expected system prompt to include available skills list, got %q", client.requests[0].Messages[0].Content)
	}
	names := make([]string, 0, len(client.requests[0].Tools))
	for _, def := range client.requests[0].Tools {
		names = append(names, def.Function.Name)
	}
	sort.Strings(names)
	want := []string{"list_files", "read_file", "search_text"}
	if !slices.Equal(names, want) {
		t.Fatalf("unexpected tool list after allowlist filter: got=%v want=%v", names, want)
	}
}

func TestRunPromptBlocksToolCallOutsideActiveSkillPolicy(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "skill.json"), []byte(`{
  "name":"review",
  "description":"Review changes",
  "tools":{"policy":"allowlist","items":["read_file","search_text"]}
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	sess.ActiveSkill = &session.ActiveSkill{Name: "review"}

	client := &fakeClient{replies: []llm.Message{
		{
			Role: "assistant",
			ToolCalls: []llm.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      "write_file",
					Arguments: `{"path":"x.txt","content":"x"}`,
				},
			}},
		},
		{
			Role:    "assistant",
			Content: "recovered",
		},
	}}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "review this", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "recovered" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(sess.Messages) < 3 {
		t.Fatalf("expected tool result message, got %#v", sess.Messages)
	}
	toolMsg := sess.Messages[2]
	if toolMsg.Role != "user" || !strings.Contains(toolMsg.Content, "active skill policy") {
		t.Fatalf("expected policy rejection in tool message, got %#v", toolMsg)
	}
	if len(toolMsg.Parts) != 1 || toolMsg.Parts[0].ToolResult == nil {
		t.Fatalf("expected tool_result part in tool message, got %#v", toolMsg)
	}
}

func TestRunPromptDropsToolDefinitionsForNoToolModels(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	client := &fakeClient{replies: []llm.Message{{
		Role:    "assistant",
		Content: "done",
	}}}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "gpt-5.4-no-tool"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPrompt(context.Background(), sess, "hello", "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) == 0 {
		t.Fatal("expected request captured")
	}
	if len(client.requests[0].Tools) != 0 {
		t.Fatalf("expected no tool definitions for no-tool model, got %#v", client.requests[0].Tools)
	}
}

func TestActivateAndClearSkillPersistsSessionState(t *testing.T) {
	workspace := t.TempDir()
	if err := os.MkdirAll(filepath.Join(workspace, "internal", "skills", "review"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(workspace, "internal", "skills", "review", "skill.json"), []byte(`{
  "name":"review",
  "description":"Review changes",
  "tools":{"policy":"allowlist","items":["read_file","search_text"]},
  "args":[{"name":"base_ref","required":true,"type":"string"}]
}`), 0o644); err != nil {
		t.Fatal(err)
	}

	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   &fakeClient{},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	if _, err := runner.ActivateSkill(sess, "review", nil); err == nil {
		t.Fatal("expected missing required args to fail")
	}
	skill, err := runner.ActivateSkill(sess, "review", map[string]string{"base_ref": "main"})
	if err != nil {
		t.Fatal(err)
	}
	if skill.Name != "review" {
		t.Fatalf("unexpected activated skill: %#v", skill)
	}
	if sess.ActiveSkill == nil || sess.ActiveSkill.Name != "review" {
		t.Fatalf("expected session active skill to be set, got %#v", sess.ActiveSkill)
	}

	loaded, err := store.Load(sess.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.ActiveSkill == nil || loaded.ActiveSkill.Args["base_ref"] != "main" {
		t.Fatalf("expected persisted active skill args, got %#v", loaded.ActiveSkill)
	}

	if err := runner.ClearActiveSkill(sess); err != nil {
		t.Fatal(err)
	}
	if sess.ActiveSkill != nil {
		t.Fatalf("expected active skill to be cleared, got %#v", sess.ActiveSkill)
	}
}

func TestAuthorSkillTranslatesChineseBriefToEnglish(t *testing.T) {
	workspace := t.TempDir()
	client := &fakeClient{
		replies: []llm.Message{
			llm.NewAssistantTextMessage("Review backend changes and highlight regression risks."),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
			Stream:   false,
		},
		Client:   client,
		Registry: tools.DefaultRegistry(),
	})

	result, err := runner.AuthorSkill("review-plus", hanReviewBriefWithRisk())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "Review backend changes and highlight regression risks.") {
		t.Fatalf("expected translated english description in manifest, got %q", text)
	}
	if containsHanForTest(text) {
		t.Fatalf("expected no Han text in authored manifest, got %q", text)
	}
	if len(client.requests) == 0 {
		t.Fatal("expected translation request to be sent to llm client")
	}
	if len(client.requests[0].Messages) < 2 || !strings.Contains(client.requests[0].Messages[0].Text(), "Translate the user's skill description") {
		t.Fatalf("expected translation system instruction in first request, got %#v", client.requests[0].Messages)
	}
}

func TestAuthorSkillFallsBackToEnglishWhenTranslationFails(t *testing.T) {
	workspace := t.TempDir()
	client := &fakeClient{
		replies: []llm.Message{
			llm.NewAssistantTextMessage(hanReviewBrief()),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider: config.ProviderConfig{Model: "test-model"},
			Stream:   false,
		},
		Client:   client,
		Registry: tools.DefaultRegistry(),
	})

	result, err := runner.AuthorSkill("review-plus", hanReviewBrief())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, skillAuthorEnglishFallback) {
		t.Fatalf("expected english fallback description in manifest, got %q", text)
	}
	if containsHanForTest(text) {
		t.Fatalf("expected no Han text in fallback manifest, got %q", text)
	}
}

func containsHanForTest(text string) bool {
	for _, r := range text {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func hanReviewBrief() string {
	return string([]rune{0x7528, 0x4e8e, 0x4ee3, 0x7801, 0x8bc4, 0x5ba1})
}

func hanReviewBriefWithRisk() string {
	return string([]rune{
		0x7528, 0x4e8e, 0x4ee3, 0x7801, 0x8bc4, 0x5ba1,
		0xff0c,
		0x91cd, 0x70b9, 0x5173, 0x6ce8, 0x56de, 0x5f52, 0x98ce, 0x9669,
	})
}
