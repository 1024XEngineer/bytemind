package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func missingImageAssetFallback(assetID llm.AssetID) string {
	if strings.TrimSpace(string(assetID)) == "" {
		return "unavailable image asset"
	}
	return fmt.Sprintf("unavailable asset %s", assetID)
}

func (c *OpenAICompatible) postJSON(ctx context.Context, url string, payload map[string]any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		value := c.apiKey
		if c.authScheme != "" {
			value = c.authScheme + " " + c.apiKey
		}
		httpReq.Header.Set(c.authHeader, value)
	}
	for key, value := range c.extraHeaders {
		httpReq.Header.Set(key, value)
	}
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
		return nil, llm.MapProviderError("openai", resp.StatusCode, string(respBody), nil)
	}
	return respBody, nil
}

func (c *OpenAICompatible) chatPayload(req llm.ChatRequest, stream bool) (map[string]any, error) {
	messages, err := openAIMessages(req)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"model":    choose(req.Model, c.model),
		"messages": messages,
		"stream":   stream,
	}
	if req.Temperature >= 0 {
		payload["temperature"] = req.Temperature
	}
	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
		payload["tool_choice"] = "auto"
	}
	return payload, nil
}

func openAIMessages(req llm.ChatRequest) ([]map[string]any, error) {
	converted := make([]map[string]any, 0, len(req.Messages))
	for _, message := range req.Messages {
		message.Normalize()
		switch message.Role {
		case llm.RoleSystem, llm.RoleUser, llm.RoleAssistant:
			entry := map[string]any{"role": string(message.Role)}
			content := make([]map[string]any, 0)
			for _, part := range message.Parts {
				switch part.Type {
				case llm.PartText:
					content = append(content, map[string]any{"type": "text", "text": part.Text.Value})
				case llm.PartThinking:
					content = append(content, map[string]any{"type": "text", "text": part.Thinking.Value})
				case llm.PartImageRef:
					assetID := llm.AssetID("")
					if part.Image != nil {
						assetID = part.Image.AssetID
					}
					asset, ok := req.Assets[assetID]
					if !ok || len(asset.Data) == 0 {
						content = append(content, map[string]any{"type": "text", "text": missingImageAssetFallback(assetID)})
						continue
					}
					mediaType := strings.TrimSpace(asset.MediaType)
					if mediaType == "" {
						mediaType = "image/png"
					}
					content = append(content, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(asset.Data)}})
				case llm.PartToolUse:
					entry["tool_calls"] = append(asToolCalls(entry["tool_calls"]), map[string]any{"id": part.ToolUse.ID, "type": "function", "function": map[string]any{"name": part.ToolUse.Name, "arguments": part.ToolUse.Arguments}})
				case llm.PartToolResult:
					converted = append(converted, map[string]any{"role": "tool", "tool_call_id": part.ToolResult.ToolUseID, "content": part.ToolResult.Content})
				}
			}
			hasToolCalls := len(asToolCalls(entry["tool_calls"])) > 0
			if len(content) == 1 && content[0]["type"] == "text" {
				entry["content"] = content[0]["text"]
			} else if len(content) > 0 {
				entry["content"] = content
			}
			if _, hasContent := entry["content"]; hasContent || hasToolCalls {
				converted = append(converted, entry)
			}
		case "tool":
			converted = append(converted, map[string]any{"role": "tool", "tool_call_id": message.ToolCallID, "content": message.Text()})
		}
	}
	return converted, nil
}

func asToolCalls(value any) []map[string]any {
	if value == nil {
		return []map[string]any{}
	}
	calls, _ := value.([]map[string]any)
	return calls
}
