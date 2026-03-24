package model

import (
	"context"
	"errors"
	"testing"

	"forgecli/internal/config"
)

type fakeChatClient struct {
	response ChatResponse
	err      error
}

func (c fakeChatClient) Complete(_ context.Context, _ ChatRequest) (ChatResponse, error) {
	if c.err != nil {
		return ChatResponse{}, c.err
	}
	return c.response, nil
}

func TestParseProposalResponseSupportsMarkdownFence(t *testing.T) {
	proposal, err := parseProposalResponse("```json\n{\"summary\":\"done\",\"noop\":false,\"new_content\":\"hello\\n\"}\n```", "old\n")
	if err != nil {
		t.Fatal(err)
	}
	if proposal.NewContent != "hello\n" {
		t.Fatalf("unexpected content: %q", proposal.NewContent)
	}
	if proposal.Summary != "done" {
		t.Fatalf("unexpected summary: %q", proposal.Summary)
	}
}

func TestLLMProviderProposeChange(t *testing.T) {
	provider, err := NewLLMProvider(config.ModelConfig{Model: "demo-model"}, fakeChatClient{
		response: ChatResponse{Message: Message{Content: `{"summary":"updated","noop":false,"new_content":"package main\n"}`}},
	})
	if err != nil {
		t.Fatal(err)
	}

	proposal, err := provider.ProposeChange("update file", "main.go", []byte("package main\n// old\n"))
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Summary != "updated" {
		t.Fatalf("unexpected summary: %q", proposal.Summary)
	}
	if proposal.NewContent != "package main\n" {
		t.Fatalf("unexpected content: %q", proposal.NewContent)
	}
}

func TestLLMProviderReturnsErrorWhenModelFails(t *testing.T) {
	provider, err := NewLLMProvider(config.ModelConfig{Model: "demo-model"}, fakeChatClient{err: errors.New("boom")})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := provider.ProposeChange("update file", "main.go", []byte("package main\n")); err == nil {
		t.Fatal("expected model error")
	}
}
