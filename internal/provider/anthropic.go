package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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
	system, messages, err := anthropicMessages(req)
	if err != nil {
		return llm.Message{}, err
	}
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
		return llm.Message{}, llm.MapProviderError("anthropic", resp.StatusCode, string(respBody), nil)
	}

	var completion struct {
		Content []struct {
			Type     string          `json:"type"`
			Text     string          `json:"text"`
			ID       string          `json:"id"`
			Name     string          `json:"name"`
			Input    json.RawMessage `json:"input"`
			Thinking string          `json:"thinking"`
		} `json:"content"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return llm.Message{}, err
	}

	message := llm.Message{Role: llm.RoleAssistant}
	for _, block := range completion.Content {
		switch block.Type {
		case "text":
			message.AppendPart(llm.Part{Type: llm.PartText, Text: &llm.TextPart{Value: block.Text}})
		case "tool_use":
			arguments := "{}"
			if len(block.Input) > 0 {
				arguments = string(block.Input)
			}
			message.AppendPart(llm.Part{
				Type: llm.PartToolUse,
				ToolUse: &llm.ToolUsePart{
					ID:        block.ID,
					Name:      block.Name,
					Arguments: arguments,
				},
			})
		case "thinking":
			thinking := block.Thinking
			if strings.TrimSpace(thinking) == "" {
				thinking = block.Text
			}
			if strings.TrimSpace(thinking) != "" {
				message.AppendPart(llm.Part{Type: llm.PartThinking, Thinking: &llm.ThinkingPart{Value: thinking}})
			}
		}
	}
	message.Normalize()
	contextTokens := max(0, completion.Usage.CacheReadInputTokens) + max(0, completion.Usage.CacheCreationInputTokens)
	total := max(0, completion.Usage.InputTokens) + max(0, completion.Usage.OutputTokens) + contextTokens
	if total > 0 {
		message.Usage = &llm.Usage{
			InputTokens:   max(0, completion.Usage.InputTokens),
			OutputTokens:  max(0, completion.Usage.OutputTokens),
			ContextTokens: contextTokens,
			TotalTokens:   total,
		}
	}
	return message, nil
}

func (c *Anthropic) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	message, err := c.CreateMessage(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	if onDelta != nil && message.Text() != "" {
		onDelta(message.Text())
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

func anthropicMessages(req llm.ChatRequest) (string, []map[string]any, error) {
	systemParts := make([]string, 0, 1)
	converted := make([]map[string]any, 0, len(req.Messages))

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

	for _, message := range req.Messages {
		message.Normalize()

		if message.Role == "tool" {
			appendMessage("user", []map[string]any{{
				"type":        "tool_result",
				"tool_use_id": message.ToolCallID,
				"content":     message.Text(),
			}})
			continue
		}

		switch message.Role {
		case llm.RoleSystem:
			for _, part := range message.Parts {
				if part.Text != nil && strings.TrimSpace(part.Text.Value) != "" {
					systemParts = append(systemParts, part.Text.Value)
				}
			}
		case llm.RoleUser:
			blocks := make([]map[string]any, 0, len(message.Parts))
			for _, part := range message.Parts {
				switch part.Type {
				case llm.PartText:
					blocks = append(blocks, map[string]any{"type": "text", "text": part.Text.Value})
				case llm.PartImageRef:
					assetID := llm.AssetID("")
					if part.Image != nil {
						assetID = part.Image.AssetID
					}
					asset, ok := req.Assets[assetID]
					if !ok || len(asset.Data) == 0 {
						blocks = append(blocks, map[string]any{"type": "text", "text": missingImageAssetFallback(assetID)})
						continue
					}
					mediaType := strings.TrimSpace(asset.MediaType)
					if mediaType == "" {
						mediaType = "image/png"
					}
					blocks = append(blocks, map[string]any{
						"type": "image",
						"source": map[string]any{
							"type":       "base64",
							"media_type": mediaType,
							"data":       base64.StdEncoding.EncodeToString(asset.Data),
						},
					})
				case llm.PartToolResult:
					block := map[string]any{
						"type":        "tool_result",
						"tool_use_id": part.ToolResult.ToolUseID,
						"content":     part.ToolResult.Content,
					}
					if part.ToolResult.IsError {
						block["is_error"] = true
					}
					blocks = append(blocks, block)
				}
			}
			appendMessage("user", blocks)
		case llm.RoleAssistant:
			blocks := make([]map[string]any, 0, len(message.Parts))
			for _, part := range message.Parts {
				switch part.Type {
				case llm.PartText:
					blocks = append(blocks, map[string]any{"type": "text", "text": part.Text.Value})
				case llm.PartThinking:
					blocks = append(blocks, map[string]any{"type": "text", "text": part.Thinking.Value})
				case llm.PartToolUse:
					blocks = append(blocks, map[string]any{
						"type":  "tool_use",
						"id":    part.ToolUse.ID,
						"name":  part.ToolUse.Name,
						"input": parseJSONObject(part.ToolUse.Arguments),
					})
				}
			}
			appendMessage("assistant", blocks)
		}
	}

	return strings.Join(systemParts, "\n\n"), converted, nil
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
