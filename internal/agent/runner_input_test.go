package agent

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
	"github.com/1024XEngineer/bytemind/internal/tools"
)

func TestRunPromptWithInputForwardsStructuredUserMessageAndAssets(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	client := &fakeClient{
		replies: []llm.Message{
			llm.NewAssistantTextMessage("done"),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "gpt-4o"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	assetID := llm.AssetID(sess.ID + ":1")
	userMessage := llm.Message{
		Role: llm.RoleUser,
		Parts: []llm.Part{
			{Type: llm.PartText, Text: &llm.TextPart{Value: "Please inspect this "}},
			{Type: llm.PartImageRef, Image: &llm.ImagePartRef{AssetID: assetID}},
		},
	}

	answer, err := runner.RunPromptWithInput(context.Background(), sess, RunPromptInput{
		UserMessage: userMessage,
		Assets: map[llm.AssetID]llm.ImageAsset{
			assetID: {
				MediaType: "image/png",
				Data:      []byte("png-binary"),
			},
		},
		DisplayText: "Please inspect this [Image #1]",
	}, "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(client.requests) == 0 {
		t.Fatal("expected request to be sent")
	}
	if len(client.requests[0].Assets) != 1 {
		t.Fatalf("expected one forwarded asset payload, got %d", len(client.requests[0].Assets))
	}
	if _, ok := client.requests[0].Assets[assetID]; !ok {
		t.Fatalf("expected request assets to include %q", assetID)
	}

	if len(sess.Messages) < 1 {
		t.Fatalf("expected session to persist user message")
	}
	first := sess.Messages[0]
	if first.Role != llm.RoleUser {
		t.Fatalf("expected first session message to be user, got %q", first.Role)
	}
	foundImage := false
	for _, part := range first.Parts {
		if part.Type == llm.PartImageRef && part.Image != nil && part.Image.AssetID == assetID {
			foundImage = true
		}
	}
	if !foundImage {
		t.Fatalf("expected session user message to keep image_ref part, got %#v", first.Parts)
	}
}

func TestRunPromptWithInputFallsBackToDisplayTextWhenUserMessageEmpty(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	client := &fakeClient{
		replies: []llm.Message{
			llm.NewAssistantTextMessage("done"),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "gpt-4o"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPromptWithInput(context.Background(), sess, RunPromptInput{
		DisplayText: "fallback user prompt",
	}, "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if len(sess.Messages) == 0 {
		t.Fatalf("expected user message persisted")
	}
	if sess.Messages[0].Role != llm.RoleUser {
		t.Fatalf("expected fallback user role, got %q", sess.Messages[0].Role)
	}
	if sess.Messages[0].Text() != "fallback user prompt" {
		t.Fatalf("expected fallback display text to be used, got %q", sess.Messages[0].Text())
	}
}

func TestRunPromptWithInputPersistsDisplayTextButSendsDelegationHintToModel(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	client := &fakeClient{
		replies: []llm.Message{
			llm.NewAssistantTextMessage("done"),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "gpt-4o"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	displayInput := "/review 分析一下最近提交的代码"
	delegationHint := "User explicitly invoked slash command preference for subagent `review`.\nTreat this as a strong delegation hint."
	answer, err := runner.RunPromptWithInput(context.Background(), sess, RunPromptInput{
		UserMessage:                     llm.NewUserTextMessage(delegationHint),
		DisplayText:                     displayInput,
		PersistDisplayTextAsUserMessage: true,
	}, "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if answer != "done" {
		t.Fatalf("unexpected answer: %q", answer)
	}

	if len(sess.Messages) == 0 {
		t.Fatalf("expected session user message to persist")
	}
	if got := sess.Messages[0].Text(); got != displayInput {
		t.Fatalf("expected session to keep original slash input, got %q", got)
	}

	if len(client.requests) == 0 {
		t.Fatal("expected request to be sent")
	}
	if got := latestUserText(client.requests[0].Messages); got != delegationHint {
		t.Fatalf("expected model request to keep delegation hint prompt, got %q", got)
	}
}

func TestRunPromptWithInputPlanModeSetsGoalFromUserMessageWhenDisplayTextBlank(t *testing.T) {
	workspace := t.TempDir()
	store, err := session.NewStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	sess := session.New(workspace)

	client := &fakeClient{
		replies: []llm.Message{
			llm.NewAssistantTextMessage("drafted plan"),
		},
	}
	runner := NewRunner(Options{
		Workspace: workspace,
		Config: config.Config{
			Provider:      config.ProviderConfig{Model: "gpt-4o"},
			MaxIterations: 4,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	answer, err := runner.RunPromptWithInput(context.Background(), sess, RunPromptInput{
		UserMessage: llm.Message{
			Role: llm.RoleUser,
			Parts: []llm.Part{
				{Type: llm.PartText, Text: &llm.TextPart{Value: "plan from structured prompt"}},
			},
		},
		DisplayText: "   ",
	}, "plan", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(answer, "drafted plan") {
		t.Fatalf("unexpected answer: %q", answer)
	}
	if sess.Plan.Goal != "plan from structured prompt" {
		t.Fatalf("expected plan goal from structured user message text, got %q", sess.Plan.Goal)
	}
	if sess.Mode != "plan" {
		t.Fatalf("expected session mode to be plan, got %q", sess.Mode)
	}
}

func latestUserText(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser {
			return messages[i].Text()
		}
	}
	return ""
}
