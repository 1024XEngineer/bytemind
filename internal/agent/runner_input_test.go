package agent

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/1024XEngineer/bytemind/internal/config"
	"github.com/1024XEngineer/bytemind/internal/llm"
	planpkg "github.com/1024XEngineer/bytemind/internal/plan"
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

func TestRunPromptWithInputUsesSelectedModelForImageCapabilities(t *testing.T) {
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
			ProviderRuntime: config.ProviderRuntimeConfig{
				CurrentProvider: "qwen",
				DefaultProvider: "qwen",
				DefaultModel:    "qwen3.6-plus",
				Providers: map[string]config.ProviderConfig{
					"qwen": {Model: "qwen3.6-plus"},
				},
			},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	runner.config.Provider.Model = "text-only-model"
	assetID := llm.AssetID(sess.ID + ":1")
	answer, err := runner.RunPromptWithInput(context.Background(), sess, RunPromptInput{
		UserMessage: llm.Message{
			Role: llm.RoleUser,
			Parts: []llm.Part{
				{Type: llm.PartImageRef, Image: &llm.ImagePartRef{AssetID: assetID}},
			},
		},
		Assets: map[llm.AssetID]llm.ImageAsset{
			assetID: {
				MediaType: "image/png",
				Data:      []byte("png-binary"),
			},
		},
		DisplayText: "[Image#1]",
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
	if client.requests[0].Model != "qwen3.6-plus" {
		t.Fatalf("expected selected runtime model, got %q", client.requests[0].Model)
	}

	var latestUser llm.Message
	for i := len(client.requests[0].Messages) - 1; i >= 0; i-- {
		if client.requests[0].Messages[i].Role == llm.RoleUser {
			latestUser = client.requests[0].Messages[i]
			break
		}
	}
	for _, part := range latestUser.Parts {
		if part.Type == llm.PartImageRef && part.Image != nil && part.Image.AssetID == assetID {
			return
		}
	}
	t.Fatalf("expected request user message to keep image_ref for selected vision model, got %#v", latestUser.Parts)
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
			{
				Role: llm.RoleAssistant,
				ToolCalls: []llm.ToolCall{{
					ID:   "call-1",
					Type: "function",
					Function: llm.ToolFunctionCall{
						Name: "update_plan",
						Arguments: `{
							"summary":"Drafted the initial plan from the structured user prompt.",
							"phase":"draft",
							"decision_gaps":[],
							"plan":[
								{"step":"Inspect the relevant repo area","status":"pending"},
								{"step":"Draft the implementation approach","status":"pending"},
								{"step":"Define verification","status":"pending"}
							]
						}`,
					},
				}},
			},
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
	if strings.Contains(answer, planpkg.StructuredPlanReminder) {
		t.Fatalf("expected structured plan repair flow instead of reminder-only answer, got %q", answer)
	}
	if !strings.Contains(answer, "<proposed_plan>") {
		t.Fatalf("expected structured plan block in answer, got %q", answer)
	}
	if sess.Plan.Goal != "plan from structured prompt" {
		t.Fatalf("expected plan goal from structured user message text, got %q", sess.Plan.Goal)
	}
	if sess.Mode != "plan" {
		t.Fatalf("expected session mode to be plan, got %q", sess.Mode)
	}
}

func TestRunPromptWithInputPersistsUserMessageMetaForTimelineRestore(t *testing.T) {
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
			Provider:      config.ProviderConfig{Model: "mimo-v2.5"},
			MaxIterations: 2,
			Stream:        false,
		},
		Client:   client,
		Store:    store,
		Registry: tools.DefaultRegistry(),
		Stdin:    strings.NewReader(""),
		Stdout:   io.Discard,
	})

	_, err = runner.RunPromptWithInput(context.Background(), sess, RunPromptInput{
		UserMessage: llm.NewUserTextMessage("check message metadata"),
		DisplayText: "check message metadata",
	}, "build", io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if len(sess.Messages) == 0 {
		t.Fatalf("expected persisted session message")
	}
	first := sess.Messages[0]
	if strings.TrimSpace(first.CreatedAt) == "" {
		t.Fatalf("expected CreatedAt on persisted user message")
	}
	if _, err := time.Parse(time.RFC3339Nano, first.CreatedAt); err != nil {
		t.Fatalf("expected RFC3339Nano CreatedAt, got %q: %v", first.CreatedAt, err)
	}
	if first.Meta == nil {
		t.Fatalf("expected metadata map on persisted user message")
	}
	if got, _ := first.Meta[userMessageModelMetaKey].(string); got != "mimo-v2.5" {
		t.Fatalf("expected %q=%q, got %q", userMessageModelMetaKey, "mimo-v2.5", got)
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
