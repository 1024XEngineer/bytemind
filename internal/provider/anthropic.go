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
	apiKey             string
	model              string
	anthropicVersion   string
	authHeader         string
	authScheme         string
	extraHeaders       map[string]string
	endpointCandidates []string
	httpClient         *http.Client
}

func NewAnthropic(cfg Config) *Anthropic {
	version := strings.TrimSpace(cfg.AnthropicVersion)
	if version == "" {
		version = "2023-06-01"
	}
	authHeader := strings.TrimSpace(cfg.AuthHeader)
	if authHeader == "" {
		authHeader = "x-api-key"
	}
	return &Anthropic{
		apiKey:             cfg.APIKey,
		model:              cfg.Model,
		anthropicVersion:   version,
		authHeader:         authHeader,
		authScheme:         strings.TrimSpace(cfg.AuthScheme),
		extraHeaders:       cloneHeaders(cfg.ExtraHeaders),
		endpointCandidates: resolveEndpointCandidates(cfg.BaseURL, cfg.APIPath, anthropicDefaultPaths(cfg.BaseURL), []string{"/v1/messages", "/messages"}),
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

func (c *Anthropic) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
	if len(c.endpointCandidates) == 0 {
		return llm.Message{}, fmt.Errorf("provider base URL is required")
	}

	system, messages := anthropicMessages(req.Messages)
	payloads := c.messagePayloadVariants(req, system, messages)

	var lastErr error
	for _, endpoint := range c.endpointCandidates {
		for i, payload := range payloads {
			respBody, err := c.postJSON(ctx, endpoint, payload)
			if err == nil {
				return parseAnthropicMessage(respBody)
			}
			lastErr = err
			if isEndpointNotFoundError(err) {
				break
			}
			if i+1 < len(payloads) && isCompatibilityPayloadError(err) {
				continue
			}
			return llm.Message{}, err
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("provider request failed without an explicit error")
	}
	return llm.Message{}, lastErr
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

func (c *Anthropic) messagePayload(req llm.ChatRequest, system string, messages []map[string]any) map[string]any {
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
	return payload
}

func (c *Anthropic) messagePayloadVariants(req llm.ChatRequest, system string, messages []map[string]any) []map[string]any {
	base := c.messagePayload(req, system, messages)
	variants := make([]map[string]any, 0, 4)
	seen := map[string]struct{}{}

	appendPayloadVariant(&variants, seen, base)

	if len(req.Tools) > 0 {
		withoutTools := clonePayload(base)
		delete(withoutTools, "tools")
		appendPayloadVariant(&variants, seen, withoutTools)
	}

	withoutTemperature := clonePayload(base)
	delete(withoutTemperature, "temperature")
	appendPayloadVariant(&variants, seen, withoutTemperature)

	if len(req.Tools) > 0 {
		compact := clonePayload(base)
		delete(compact, "tools")
		delete(compact, "temperature")
		appendPayloadVariant(&variants, seen, compact)
	}
	return variants
}

func (c *Anthropic) postJSON(ctx context.Context, endpoint string, payload map[string]any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("anthropic-version", c.anthropicVersion)
	applyAuthAndExtraHeaders(httpReq, c.authHeader, c.authScheme, c.apiKey, c.extraHeaders)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, &providerHTTPError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(respBody)),
		}
	}
	return respBody, nil
}

func parseAnthropicMessage(respBody []byte) (llm.Message, error) {
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

func anthropicDefaultPaths(baseURL string) []string {
	path := strings.ToLower(strings.TrimSuffix(extractPath(baseURL), "/"))
	if strings.HasSuffix(path, "/v1") {
		return []string{"messages"}
	}
	return []string{"v1/messages", "messages"}
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
