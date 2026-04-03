package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"bytemind/internal/llm"
)

type Anthropic struct {
	baseURL          string
	apiKey           string
	model            string
	anthropicVersion string
	httpClient       *http.Client
}

func NewAnthropic(cfg Config) *Anthropic {
	version := cfg.AnthropicVersion
	if version == "" {
		version = "2023-06-01"
	}
	return &Anthropic{
		baseURL:          strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:           cfg.APIKey,
		model:            cfg.Model,
		anthropicVersion: version,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

func (c *Anthropic) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
	system, messages := anthropicMessages(req.Messages)
	payload := map[string]any{
		"model":       choose(req.Model, c.model),
		"max_tokens":  4096,
		"messages":    messages,
		"temperature": req.Temperature,
	}
	if system != "" {
		payload["system"] = system
	}
	if len(req.Tools) > 0 {
		payload["tools"] = anthropicTools(req.Tools)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return llm.Message{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return llm.Message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", c.anthropicVersion)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return llm.Message{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.Message{}, err
	}
	if resp.StatusCode >= 300 {
		return llm.Message{}, fmt.Errorf("provider error %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var completion struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text"`
			ID    string          `json:"id"`
			Name  string          `json:"name"`
			Input json.RawMessage `json:"input"`
		} `json:"content"`
	}
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return llm.Message{}, err
	}

	message := llm.Message{Role: "assistant"}
	for _, block := range completion.Content {
		switch block.Type {
		case "text":
			message.Content += block.Text
		case "tool_use":
			arguments := "{}"
			if len(block.Input) > 0 {
				arguments = string(block.Input)
			}
			message.ToolCalls = append(message.ToolCalls, llm.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: llm.ToolFunctionCall{
					Name:      block.Name,
					Arguments: arguments,
				},
			})
		}
	}
	return message, nil
}

func (c *Anthropic) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	message, err := c.CreateMessage(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	if onDelta != nil && message.Content != "" {
		onDelta(message.Content)
	}
	return message, nil
}

func anthropicTools(tools []llm.ToolDefinition) []map[string]any {
	result := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		result = append(result, map[string]any{
			"name":         tool.Function.Name,
			"description":  tool.Function.Description,
			"input_schema": tool.Function.Parameters,
		})
	}
	return result
}

func anthropicMessages(messages []llm.Message) (string, []map[string]any) {
	systemParts := make([]string, 0, 1)
	converted := make([]map[string]any, 0, len(messages))

	appendMessage := func(role string, blocks []map[string]any) {
		if len(blocks) == 0 {
			return
		}
		if len(converted) > 0 && converted[len(converted)-1]["role"] == role {
			existing := converted[len(converted)-1]["content"].([]map[string]any)
			converted[len(converted)-1]["content"] = append(existing, blocks...)
			return
		}
		converted = append(converted, map[string]any{
			"role":    role,
			"content": blocks,
		})
	}

	for _, message := range messages {
		switch message.Role {
		case "system":
			if strings.TrimSpace(message.Content) != "" {
				systemParts = append(systemParts, message.Content)
			}
		case "user":
			appendMessage("user", []map[string]any{{
				"type": "text",
				"text": message.Content,
			}})
		case "assistant":
			blocks := make([]map[string]any, 0, len(message.ToolCalls)+1)
			if strings.TrimSpace(message.Content) != "" {
				blocks = append(blocks, map[string]any{
					"type": "text",
					"text": message.Content,
				})
			}
			for _, call := range message.ToolCalls {
				blocks = append(blocks, map[string]any{
					"type":  "tool_use",
					"id":    call.ID,
					"name":  call.Function.Name,
					"input": parseJSONObject(call.Function.Arguments),
				})
			}
			appendMessage("assistant", blocks)
		case "tool":
			appendMessage("user", []map[string]any{{
				"type":        "tool_result",
				"tool_use_id": message.ToolCallID,
				"content":     message.Content,
			}})
		}
	}

	return strings.Join(systemParts, "\n\n"), converted
}

func parseJSONObject(raw string) any {
	if strings.TrimSpace(raw) == "" {
		return map[string]any{}
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return map[string]any{"raw": raw}
	}
	return value
}
