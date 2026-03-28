package agent

import (
	"context"
	"io"
	"strings"
	"testing"

	"bytemind/internal/config"
	"bytemind/internal/llm"
	"bytemind/internal/session"
	"bytemind/internal/tools"
)

type fakeClient struct {
	replies []llm.Message
	index   int
}

func (f *fakeClient) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
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

	answer, err := runner.RunPrompt(context.Background(), sess, "inspect workspace", io.Discard)
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

	answer, err := runner.RunPrompt(context.Background(), sess, "looping task", io.Discard)
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

	answer, err := runner.RunPrompt(context.Background(), sess, "inspect workspace", io.Discard)
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
	if sess.Messages[2].Role != "tool" || !strings.Contains(sess.Messages[2].Content, `"items"`) {
		t.Fatalf("expected third message to be tool result, got %#v", sess.Messages[2])
	}
	if sess.Messages[3].Role != "assistant" || sess.Messages[3].Content != "Workspace inspected." {
		t.Fatalf("expected final assistant message, got %#v", sess.Messages[3])
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

	answer, err := runner.RunPrompt(context.Background(), sess, "trigger failing tool", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "Recovered after tool failure." {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(sess.Messages) != 4 {
		t.Fatalf("expected 4 session messages, got %#v", sess.Messages)
	}
	if sess.Messages[2].Role != "tool" {
		t.Fatalf("expected third message to be tool result, got %#v", sess.Messages[2])
	}
	if !strings.Contains(sess.Messages[2].Content, `"ok":false`) || !strings.Contains(sess.Messages[2].Content, `unknown tool`) {
		t.Fatalf("expected encoded tool error payload, got %q", sess.Messages[2].Content)
	}
}

func TestCompactWhitespacePreservesUTF8WhenTruncating(t *testing.T) {
	text := "继续刚才的上下文，给我列一下当前主 MVP 最关键的测试点"
	got := compactWhitespace(text, 18)
	if strings.ContainsRune(got, '\uFFFD') {
		t.Fatalf("expected valid utf-8 preview, got %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("expected truncated preview to end with ellipsis, got %q", got)
	}
}
