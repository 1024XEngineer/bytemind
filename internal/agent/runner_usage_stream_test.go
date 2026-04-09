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

type observingStreamClient struct {
	reply llm.Message
}

func (c *observingStreamClient) CreateMessage(context.Context, llm.ChatRequest) (llm.Message, error) {
	return c.reply, nil
}

func (c *observingStreamClient) StreamMessage(_ context.Context, _ llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	if onDelta != nil && strings.TrimSpace(c.reply.Content) != "" {
		onDelta(c.reply.Content)
	}
	return c.reply, nil
}

func TestRunPromptEmitsUsageEventForStreamingReplyUsage(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	var events []Event
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "test-model"},
			MaxIterations: 1,
			Stream:        true,
		},
		Client: &observingStreamClient{reply: llm.Message{
			Role:    llm.RoleAssistant,
			Content: "streamed reply",
			Usage: &llm.Usage{
				InputTokens:   100,
				OutputTokens:  25,
				ContextTokens: 10,
				TotalTokens:   135,
			},
		}},
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Observer: ObserverFunc(func(event Event) { events = append(events, event) }),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	if _, err := runner.RunPrompt(context.Background(), sess, "hello", "build", io.Discard); err != nil {
		t.Fatal(err)
	}

	usageEvents := 0
	for _, event := range events {
		if event.Type != EventUsageUpdated {
			continue
		}
		usageEvents++
		if event.Usage.TotalTokens != 135 || event.Usage.InputTokens != 100 || event.Usage.OutputTokens != 25 || event.Usage.ContextTokens != 10 {
			t.Fatalf("unexpected usage payload: %+v", event.Usage)
		}
	}
	if usageEvents != 1 {
		t.Fatalf("expected exactly 1 usage event, got %d", usageEvents)
	}
}
