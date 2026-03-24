package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"forgecli/internal/config"
)

type LLMProvider struct {
	client ChatClient
	cfg    config.ModelConfig
}

type llmProposalResponse struct {
	Summary    string `json:"summary"`
	Noop       bool   `json:"noop"`
	NewContent string `json:"new_content"`
	Content    string `json:"content"`
}

func NewLLMProvider(cfg config.ModelConfig, client ChatClient) (*LLMProvider, error) {
	if client == nil {
		return nil, errors.New("chat client is required")
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, errors.New("model name is required")
	}

	return &LLMProvider{
		client: client,
		cfg:    cfg,
	}, nil
}

func (p *LLMProvider) ProposeChange(task, targetPath string, content []byte) (Proposal, error) {
	resp, err := p.client.Complete(context.Background(), ChatRequest{
		Model: p.cfg.Model,
		Messages: []Message{
			{
				Role: "system",
				Content: strings.Join([]string{
					"You are a careful single-file code editor.",
					"You receive one task, one target file path, and the current full file content.",
					"Return JSON only with keys: summary, noop, new_content.",
					"If the task is already satisfied, set noop to true and return the original content in new_content.",
					"Do not wrap the JSON in Markdown fences.",
				}, "\n"),
			},
			{
				Role:    "user",
				Content: fmt.Sprintf("Task:\n%s\n\nTarget file:\n%s\n\nCurrent file content:\n<<<FILE\n%s\nFILE", task, targetPath, string(content)),
			},
		},
		Temperature: p.cfg.Temperature,
	})
	if err != nil {
		return Proposal{}, err
	}

	return parseProposalResponse(resp.Message.Content, string(content))
}

func parseProposalResponse(raw, current string) (Proposal, error) {
	payload := strings.TrimSpace(raw)
	if payload == "" {
		return Proposal{}, errors.New("model returned an empty proposal")
	}

	if strings.HasPrefix(payload, "```") {
		payload = strings.TrimPrefix(payload, "```json")
		payload = strings.TrimPrefix(payload, "```")
		payload = strings.TrimSuffix(payload, "```")
		payload = strings.TrimSpace(payload)
	}

	var parsed llmProposalResponse
	if err := json.Unmarshal([]byte(payload), &parsed); err != nil {
		return Proposal{}, fmt.Errorf("parse model proposal JSON: %w", err)
	}

	newContent := parsed.NewContent
	if strings.TrimSpace(newContent) == "" && parsed.Content != "" {
		newContent = parsed.Content
	}
	if parsed.Noop && newContent == "" {
		newContent = current
	}
	if !parsed.Noop && newContent == "" {
		return Proposal{}, errors.New("model proposal did not include new_content")
	}

	proposal := Proposal{
		NewContent: newContent,
		Summary:    strings.TrimSpace(parsed.Summary),
		Noop:       parsed.Noop,
	}
	if proposal.Summary == "" {
		if proposal.Noop || proposal.NewContent == current {
			proposal.Summary = "Task already appears to be satisfied."
			proposal.Noop = true
			proposal.NewContent = current
		} else {
			proposal.Summary = "Updated the target file for the requested task."
		}
	}

	return proposal, nil
}
