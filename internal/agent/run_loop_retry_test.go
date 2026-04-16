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

type scriptedTurnStep struct {
	reply llm.Message
	err   error
}

type scriptedTurnClient struct {
	steps    []scriptedTurnStep
	requests []llm.ChatRequest
	index    int
}

func (c *scriptedTurnClient) CreateMessage(_ context.Context, req llm.ChatRequest) (llm.Message, error) {
	c.requests = append(c.requests, req)
	if len(c.steps) == 0 {
		return llm.Message{}, nil
	}
	if c.index >= len(c.steps) {
		last := c.steps[len(c.steps)-1]
		if last.err != nil {
			return llm.Message{}, last.err
		}
		return last.reply, nil
	}
	step := c.steps[c.index]
	c.index++
	if step.err != nil {
		return llm.Message{}, step.err
	}
	return step.reply, nil
}

func (c *scriptedTurnClient) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	message, err := c.CreateMessage(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	if onDelta != nil && strings.TrimSpace(message.Content) != "" {
		onDelta(message.Content)
	}
	return message, nil
}

func TestRunPromptReactiveCompactRetrySucceedsAfterContextTooLong(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	for i := 0; i < 4; i++ {
		sess.Messages = append(sess.Messages,
			llm.NewUserTextMessage("history question"),
			llm.NewAssistantTextMessage("history answer"),
		)
	}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &scriptedTurnClient{
		steps: []scriptedTurnStep{
			{
				err: &llm.ProviderError{
					Code:    llm.ErrorCodeContextTooLong,
					Message: "maximum context length exceeded",
				},
			},
			{
				reply: llm.NewAssistantTextMessage("Goal: continue implementation\nPending: finalize response"),
			},
			{
				reply: llm.NewAssistantTextMessage("final answer"),
			},
		},
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    200000,
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
	if answer != "final answer" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) != 3 {
		t.Fatalf("expected failed turn + compaction + retry turn requests, got %d", len(client.requests))
	}
	if len(client.requests[1].Tools) != 0 {
		t.Fatalf("expected compaction request to disable tools, got %#v", client.requests[1].Tools)
	}
	if len(client.requests[1].Messages) != 2 {
		t.Fatalf("expected compaction request to have system+user messages, got %#v", client.requests[1].Messages)
	}
	if client.requests[1].Messages[0].Role != llm.RoleSystem || client.requests[1].Messages[1].Role != llm.RoleUser {
		t.Fatalf("expected compaction request roles to be system+user, got %#v", client.requests[1].Messages)
	}
	if client.requests[1].Temperature != 0 {
		t.Fatalf("expected compaction request to enforce deterministic temperature=0, got %v", client.requests[1].Temperature)
	}
	if len(client.requests[2].Messages) >= len(client.requests[0].Messages) {
		t.Fatalf("expected retry request to be rebuilt from compacted context (fewer messages), first=%d retry=%d", len(client.requests[0].Messages), len(client.requests[2].Messages))
	}
	if len(sess.Messages) != 3 {
		t.Fatalf("expected compacted summary + latest user + final assistant, got %#v", sess.Messages)
	}
	if !strings.Contains(sess.Messages[0].Text(), "Goal: continue implementation") {
		t.Fatalf("expected compacted summary message, got %#v", sess.Messages[0])
	}
	if strings.TrimSpace(sess.Messages[1].Text()) != "continue implementation" {
		t.Fatalf("expected latest user message preserved, got %#v", sess.Messages[1])
	}
	retryMessages := client.requests[2].Messages
	if len(retryMessages) < 2 {
		t.Fatalf("expected retry request to include compacted session messages, got %#v", retryMessages)
	}
	gotSummary := strings.TrimSpace(retryMessages[len(retryMessages)-2].Text())
	if gotSummary != strings.TrimSpace(sess.Messages[0].Text()) {
		t.Fatalf("expected retry request to use compacted summary message, got %q want %q", gotSummary, strings.TrimSpace(sess.Messages[0].Text()))
	}
	gotLatestUser := strings.TrimSpace(retryMessages[len(retryMessages)-1].Text())
	if gotLatestUser != strings.TrimSpace(sess.Messages[1].Text()) {
		t.Fatalf("expected retry request to keep latest user message, got %q want %q", gotLatestUser, strings.TrimSpace(sess.Messages[1].Text()))
	}
}

func TestRunPromptReactiveCompactRetryOnlyOnce(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	for i := 0; i < 4; i++ {
		sess.Messages = append(sess.Messages,
			llm.NewUserTextMessage("history question"),
			llm.NewAssistantTextMessage("history answer"),
		)
	}
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &scriptedTurnClient{
		steps: []scriptedTurnStep{
			{
				err: &llm.ProviderError{
					Code:    llm.ErrorCodeContextTooLong,
					Message: "maximum context length exceeded",
				},
			},
			{
				reply: llm.NewAssistantTextMessage("Goal: continue implementation\nPending: finalize response"),
			},
			{
				err: &llm.ProviderError{
					Code:    llm.ErrorCodeContextTooLong,
					Message: "still exceeds context window",
				},
			},
		},
	}

	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 2,
			Stream:        false,
			TokenQuota:    200000,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	_, err = runner.RunPrompt(context.Background(), sess, "continue implementation", "build", io.Discard)
	if err == nil {
		t.Fatal("expected context-too-long error after single retry")
	}
	if !isPromptTooLongError(err) {
		t.Fatalf("expected context-too-long error, got %v", err)
	}
	if len(client.requests) != 3 {
		t.Fatalf("expected failed turn + compaction + retry turn requests, got %d", len(client.requests))
	}
	if len(sess.Messages) != 2 {
		t.Fatalf("expected compacted session to keep summary + latest user after failed retry, got %#v", sess.Messages)
	}
}

func TestRunPromptReactiveCompactRetryReturnsOriginalErrorWhenCompactionNotPossible(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)
	if err := store.Save(sess); err != nil {
		t.Fatal(err)
	}

	client := &scriptedTurnClient{
		steps: []scriptedTurnStep{
			{
				err: &llm.ProviderError{
					Code:    llm.ErrorCodeContextTooLong,
					Message: "context window exceeded",
				},
			},
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

	_, err = runner.RunPrompt(context.Background(), sess, "single request", "build", io.Discard)
	if err == nil {
		t.Fatal("expected context-too-long error")
	}
	if !isPromptTooLongError(err) {
		t.Fatalf("expected original context-too-long error, got %v", err)
	}
	if len(client.requests) != 1 {
		t.Fatalf("expected only one failed model request without compaction call, got %d", len(client.requests))
	}
	if len(sess.Messages) != 1 || strings.TrimSpace(sess.Messages[0].Text()) != "single request" {
		t.Fatalf("expected session to keep only user message, got %#v", sess.Messages)
	}
}
