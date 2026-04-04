package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"bytemind/internal/llm"
)

const legacyToolCallIndex = -1

type Config struct {
	Type             string
	BaseURL          string
	APIKey           string
	Model            string
	AnthropicVersion string
}

type OpenAICompatible struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewOpenAICompatible(cfg Config) *OpenAICompatible {
	return &OpenAICompatible{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

func (c *OpenAICompatible) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
	payload, err := c.chatPayload(req, false)
	if err != nil {
		return llm.Message{}, err
	}
	respBody, err := c.postJSON(ctx, c.baseURL+"/chat/completions", payload)
	if err != nil {
		return llm.Message{}, err
	}

	var completion struct {
		Choices []struct {
			Message json.RawMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return llm.Message{}, err
	}
	if len(completion.Choices) == 0 {
		return llm.Message{}, fmt.Errorf("provider returned no choices")
	}
	return parseOpenAIMessage(completion.Choices[0].Message), nil
}

func (c *OpenAICompatible) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	payload, err := c.chatPayload(req, true)
	if err != nil {
		return llm.Message{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.Message{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return llm.Message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return llm.Message{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return llm.Message{}, llm.MapProviderError("openai", resp.StatusCode, string(respBody), nil)
	}

	assembled := llm.Message{Role: llm.RoleAssistant}
	toolCalls := map[int]*llm.ToolCall{}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta json.RawMessage `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return llm.Message{}, err
		}

		for _, choice := range chunk.Choices {
			delta, err := parseOpenAIDelta(choice.Delta)
			if err != nil {
				return llm.Message{}, err
			}
			if delta.Role != "" {
				assembled.Role = delta.Role
			}
			if delta.Content != "" {
				assembled.Content += delta.Content
				if onDelta != nil {
					onDelta(delta.Content)
				}
			}
			for _, callDelta := range delta.ToolCalls {
				call, ok := toolCalls[callDelta.Index]
				if !ok {
					call = &llm.ToolCall{Type: "function"}
					toolCalls[callDelta.Index] = call
				}
				if callDelta.ID != "" {
					call.ID = callDelta.ID
				}
				if callDelta.Type != "" {
					call.Type = callDelta.Type
				}
				if callDelta.FunctionName != "" {
					call.Function.Name += callDelta.FunctionName
				}
				if callDelta.FunctionArguments != "" {
					call.Function.Arguments += callDelta.FunctionArguments
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return llm.Message{}, err
	}

	if len(toolCalls) > 0 {
		indexes := make([]int, 0, len(toolCalls))
		for index := range toolCalls {
			indexes = append(indexes, index)
		}
		sort.Ints(indexes)
		filtered := make([]llm.ToolCall, 0, len(indexes))
		for _, index := range indexes {
			call := *toolCalls[index]
			if strings.TrimSpace(call.Function.Name) == "" {
				continue
			}
			if call.Type == "" {
				call.Type = "function"
			}
			if call.ID == "" {
				if index == legacyToolCallIndex {
					call.ID = "call-legacy"
				} else {
					call.ID = fmt.Sprintf("call-%d", index)
				}
			}
			filtered = append(filtered, call)
		}
		if len(filtered) > 0 {
			assembled.ToolCalls = filtered
		}
	}

	assembled.Normalize()
	return assembled, nil
}

type streamDelta struct {
	Role      llm.Role
	Content   string
	Reasoning string
	ToolCalls []streamToolCallDelta
}

type streamToolCallDelta struct {
	Index             int
	ID                string
	Type              string
	FunctionName      string
	FunctionArguments string
}

func parseOpenAIDelta(raw json.RawMessage) (streamDelta, error) {
	delta := streamDelta{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return delta, nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return streamDelta{}, err
	}

	if roleRaw, ok := obj["role"]; ok {
		var role string
		if err := json.Unmarshal(roleRaw, &role); err == nil {
			delta.Role = llm.Role(strings.TrimSpace(role))
		}
	}
	if contentRaw, ok := obj["content"]; ok {
		delta.Content = extractTextFromRaw(contentRaw, false)
	}
	if delta.Content == "" {
		if outputRaw, ok := obj["output_text"]; ok {
			delta.Content = extractTextFromRaw(outputRaw, false)
		}
	}
	if reasoningRaw, ok := obj["reasoning_content"]; ok {
		delta.Reasoning = extractTextFromRaw(reasoningRaw, true)
	}
	if delta.Reasoning == "" {
		if reasoningRaw, ok := obj["reasoning"]; ok {
			delta.Reasoning = extractTextFromRaw(reasoningRaw, true)
		}
	}

	if toolCallsRaw, ok := obj["tool_calls"]; ok {
		delta.ToolCalls = append(delta.ToolCalls, parseStreamToolCalls(toolCallsRaw)...)
	}
	if functionCallRaw, ok := obj["function_call"]; ok {
		legacy := parseLegacyFunctionCall(functionCallRaw)
		if legacy.FunctionName != "" || legacy.FunctionArguments != "" {
			legacy.Index = legacyToolCallIndex
			delta.ToolCalls = append(delta.ToolCalls, legacy)
		}
	}

	return delta, nil
}

func parseOpenAIMessage(raw json.RawMessage) llm.Message {
	msg := llm.Message{Role: llm.RoleAssistant}
	if len(bytes.TrimSpace(raw)) == 0 {
		return msg
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return msg
	}
	if roleRaw, ok := obj["role"]; ok {
		var role string
		if err := json.Unmarshal(roleRaw, &role); err == nil && strings.TrimSpace(role) != "" {
			msg.Role = llm.Role(role)
		}
	}
	if contentRaw, ok := obj["content"]; ok {
		msg.Content = extractTextFromRaw(contentRaw, false)
	}
	if msg.Content == "" {
		if outputRaw, ok := obj["output_text"]; ok {
			msg.Content = extractTextFromRaw(outputRaw, false)
		}
	}

	if toolCallsRaw, ok := obj["tool_calls"]; ok {
		msg.ToolCalls = parseToolCalls(toolCallsRaw)
	}
	if len(msg.ToolCalls) == 0 {
		if functionCallRaw, ok := obj["function_call"]; ok {
			legacy := parseLegacyFunctionCall(functionCallRaw)
			if legacy.FunctionName != "" {
				msg.ToolCalls = []llm.ToolCall{{
					ID:   "call-legacy",
					Type: "function",
					Function: llm.ToolFunctionCall{
						Name:      legacy.FunctionName,
						Arguments: legacy.FunctionArguments,
					},
				}}
			}
		}
	}

	msg.Normalize()
	return msg
}

func parseToolCalls(raw json.RawMessage) []llm.ToolCall {
	var calls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(calls))
	for i, call := range calls {
		name := strings.TrimSpace(call.Function.Name)
		if name == "" {
			continue
		}
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = fmt.Sprintf("call-%d", i)
		}
		callType := strings.TrimSpace(call.Type)
		if callType == "" {
			callType = "function"
		}
		out = append(out, llm.ToolCall{
			ID:   id,
			Type: callType,
			Function: llm.ToolFunctionCall{
				Name:      call.Function.Name,
				Arguments: argumentString(call.Function.Arguments),
			},
		})
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func parseStreamToolCalls(raw json.RawMessage) []streamToolCallDelta {
	var calls []struct {
		Index    int    `json:"index"`
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil
	}
	out := make([]streamToolCallDelta, 0, len(calls))
	for _, call := range calls {
		out = append(out, streamToolCallDelta{
			Index:             call.Index,
			ID:                call.ID,
			Type:              call.Type,
			FunctionName:      call.Function.Name,
			FunctionArguments: argumentString(call.Function.Arguments),
		})
	}
	return out
}

func parseLegacyFunctionCall(raw json.RawMessage) streamToolCallDelta {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return streamToolCallDelta{}
	}
	args := argumentString(call.Arguments)
	if strings.TrimSpace(call.Name) == "" && strings.TrimSpace(args) == "" {
		return streamToolCallDelta{}
	}
	return streamToolCallDelta{
		Type:              "function",
		FunctionName:      call.Name,
		FunctionArguments: args,
	}
}

func argumentString(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	return strings.TrimSpace(string(raw))
}

func extractTextFromRaw(raw json.RawMessage, includeReasoning bool) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return extractTextFromAny(value, includeReasoning)
}

func extractTextFromAny(value any, includeReasoning bool) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var b strings.Builder
		for _, item := range typed {
			b.WriteString(extractTextFromAny(item, includeReasoning))
		}
		return b.String()
	case map[string]any:
		var b strings.Builder
		for _, key := range []string{"text", "output_text", "content", "value"} {
			if nested, ok := typed[key]; ok {
				b.WriteString(extractTextFromAny(nested, includeReasoning))
			}
		}
		if includeReasoning {
			for _, key := range []string{"reasoning_content", "reasoning", "summary"} {
				if nested, ok := typed[key]; ok {
					b.WriteString(extractTextFromAny(nested, includeReasoning))
				}
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
		for _, key := range []string{"message", "delta", "part", "parts"} {
			if nested, ok := typed[key]; ok {
				b.WriteString(extractTextFromAny(nested, includeReasoning))
			}
		}
		return b.String()
	default:
		return ""
	}
}

func (c *OpenAICompatible) chatPayload(req llm.ChatRequest, stream bool) (map[string]any, error) {
	messages, err := openAIMessages(req)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"model":       choose(req.Model, c.model),
		"messages":    messages,
		"temperature": req.Temperature,
	}
	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
		payload["tool_choice"] = "auto"
	}
	if stream {
		payload["stream"] = true
	}
	return payload, nil
}

func openAIMessages(req llm.ChatRequest) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(req.Messages))

	for _, message := range req.Messages {
		message.Normalize()
		content := make([]map[string]any, 0, len(message.Parts))
		toolCalls := make([]map[string]any, 0, len(message.Parts))
		toolResults := make([]map[string]any, 0, len(message.Parts))

		for _, part := range message.Parts {
			switch part.Type {
			case llm.PartText:
				content = append(content, map[string]any{"type": "text", "text": part.Text.Value})
			case llm.PartThinking:
				content = append(content, map[string]any{"type": "text", "text": part.Thinking.Value})
			case llm.PartImageRef:
				asset, ok := req.Assets[part.Image.AssetID]
				if !ok {
					return nil, llm.WrapError("openai", llm.ErrorCodeAssetNotFound, fmt.Errorf("asset %q not found", part.Image.AssetID))
				}
				if len(asset.Data) == 0 {
					return nil, llm.WrapError("openai", llm.ErrorCodeAssetNotFound, fmt.Errorf("asset %q has empty payload", part.Image.AssetID))
				}
				mediaType := strings.TrimSpace(asset.MediaType)
				if mediaType == "" {
					mediaType = "image/png"
				}
				content = append(content, map[string]any{
					"type": "image_url",
					"image_url": map[string]any{
						"url": "data:" + mediaType + ";base64," + base64.StdEncoding.EncodeToString(asset.Data),
					},
				})
			case llm.PartToolUse:
				toolCalls = append(toolCalls, map[string]any{
					"id":   part.ToolUse.ID,
					"type": "function",
					"function": map[string]any{
						"name":      part.ToolUse.Name,
						"arguments": part.ToolUse.Arguments,
					},
				})
			case llm.PartToolResult:
				toolResults = append(toolResults, map[string]any{
					"role":         "tool",
					"tool_call_id": part.ToolResult.ToolUseID,
					"content":      part.ToolResult.Content,
				})
			}
		}

		if message.Role == "tool" {
			toolID := message.ToolCallID
			if toolID == "" && len(message.Parts) > 0 {
				for _, part := range message.Parts {
					if part.ToolResult != nil {
						toolID = part.ToolResult.ToolUseID
						break
					}
				}
			}
			result = append(result, map[string]any{
				"role":         "tool",
				"tool_call_id": toolID,
				"content":      message.Text(),
			})
			continue
		}

		if len(content) > 0 || len(toolCalls) > 0 {
			wire := map[string]any{"role": string(message.Role)}
			if len(content) > 0 {
				wire["content"] = content
			}
			if len(toolCalls) > 0 {
				wire["tool_calls"] = toolCalls
			}
			result = append(result, wire)
		}
		if len(toolResults) > 0 {
			result = append(result, toolResults...)
		}
	}

	return result, nil
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
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

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

func choose(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}
