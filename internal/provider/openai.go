package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"bytemind/internal/llm"
)

type Config struct {
	Type             string
	BaseURL          string
	APIPath          string
	APIKey           string
	Model            string
	AuthHeader       string
	AuthScheme       string
	ExtraHeaders     map[string]string
	AnthropicVersion string
}

type OpenAICompatible struct {
	apiKey             string
	model              string
	authHeader         string
	authScheme         string
	extraHeaders       map[string]string
	endpointCandidates []string
	httpClient         *http.Client
}

func NewOpenAICompatible(cfg Config) *OpenAICompatible {
	authHeader := strings.TrimSpace(cfg.AuthHeader)
	if authHeader == "" {
		authHeader = "Authorization"
	}
	authScheme := strings.TrimSpace(cfg.AuthScheme)
	if authScheme == "" && strings.EqualFold(authHeader, "Authorization") {
		authScheme = "Bearer"
	}
	return &OpenAICompatible{
		apiKey:             cfg.APIKey,
		model:              cfg.Model,
		authHeader:         authHeader,
		authScheme:         authScheme,
		extraHeaders:       cloneHeaders(cfg.ExtraHeaders),
		endpointCandidates: resolveEndpointCandidates(cfg.BaseURL, cfg.APIPath, openAIDefaultPaths(cfg.BaseURL), []string{"/chat/completions", "/v1/chat/completions", "/responses"}),
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

func (c *OpenAICompatible) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
	if len(c.endpointCandidates) == 0 {
		return llm.Message{}, fmt.Errorf("provider base URL is required")
	}

	payloads := c.chatPayloadVariants(req, false)
	var lastErr error
	for _, endpoint := range c.endpointCandidates {
		for i, payload := range payloads {
			respBody, err := c.postJSON(ctx, endpoint, payload)
			if err == nil {
				return parseOpenAIMessageResponse(respBody)
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

func (c *OpenAICompatible) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	if len(c.endpointCandidates) == 0 {
		return llm.Message{}, fmt.Errorf("provider base URL is required")
	}

	payloads := c.chatPayloadVariants(req, true)
	var lastErr error
	for _, endpoint := range c.endpointCandidates {
		for i, payload := range payloads {
			message, err := c.streamMessageWithPayload(ctx, endpoint, payload, onDelta)
			if err == nil {
				return message, nil
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
		lastErr = fmt.Errorf("provider stream request failed without an explicit error")
	}
	return llm.Message{}, lastErr
}

func (c *OpenAICompatible) streamMessageWithPayload(ctx context.Context, endpoint string, payload map[string]any, onDelta func(string)) (llm.Message, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.Message{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return llm.Message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	applyAuthAndExtraHeaders(httpReq, c.authHeader, c.authScheme, c.apiKey, c.extraHeaders)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return llm.Message{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return llm.Message{}, &providerHTTPError{
			StatusCode: resp.StatusCode,
			Body:       strings.TrimSpace(string(respBody)),
		}
	}

	assembled := llm.Message{Role: "assistant"}
	toolCalls := map[int]*llm.ToolCall{}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}
		rawPayload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if rawPayload == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta   openAIMessageEnvelope `json:"delta"`
				Message openAIMessageEnvelope `json:"message"`
			} `json:"choices"`
			OutputText json.RawMessage `json:"output_text"`
		}
		if err := json.Unmarshal([]byte(rawPayload), &chunk); err != nil {
			return llm.Message{}, err
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Role != "" {
				assembled.Role = choice.Delta.Role
			}
			deltaText := parseMessageContent(choice.Delta.Content)
			if deltaText != "" {
				assembled.Content += deltaText
				if onDelta != nil {
					onDelta(deltaText)
				}
			}
			if isEmptyOpenAIMessage(choice.Delta) && !isEmptyOpenAIMessage(choice.Message) {
				assembled = mergeOpenAIMessageEnvelope(assembled, choice.Message, onDelta)
			}
			for _, callDelta := range choice.Delta.ToolCalls {
				call := ensureToolCall(toolCalls, callDelta.Index)
				mergeToolCallDelta(call, callDelta)
			}
		}

		responseText := parseMessageContent(chunk.OutputText)
		if responseText != "" {
			assembled.Content += responseText
			if onDelta != nil {
				onDelta(responseText)
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
		assembled.ToolCalls = make([]llm.ToolCall, 0, len(indexes))
		for _, index := range indexes {
			assembled.ToolCalls = append(assembled.ToolCalls, *toolCalls[index])
		}
	}
	if assembled.Role == "" {
		assembled.Role = "assistant"
	}
	return assembled, nil
}

func (c *OpenAICompatible) chatPayload(req llm.ChatRequest, stream bool) map[string]any {
	payload := map[string]any{
		"model":       choose(req.Model, c.model),
		"messages":    req.Messages,
		"temperature": req.Temperature,
	}
	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
		payload["tool_choice"] = "auto"
	}
	if stream {
		payload["stream"] = true
	}
	return payload
}

func (c *OpenAICompatible) chatPayloadVariants(req llm.ChatRequest, stream bool) []map[string]any {
	base := c.chatPayload(req, stream)
	variants := make([]map[string]any, 0, 4)
	seen := map[string]struct{}{}

	appendPayloadVariant(&variants, seen, base)

	if len(req.Tools) > 0 {
		withoutTools := clonePayload(base)
		delete(withoutTools, "tools")
		delete(withoutTools, "tool_choice")
		appendPayloadVariant(&variants, seen, withoutTools)
	}

	withoutTemperature := clonePayload(base)
	delete(withoutTemperature, "temperature")
	appendPayloadVariant(&variants, seen, withoutTemperature)

	if len(req.Tools) > 0 {
		compact := clonePayload(base)
		delete(compact, "tools")
		delete(compact, "tool_choice")
		delete(compact, "temperature")
		appendPayloadVariant(&variants, seen, compact)
	}

	return variants
}

func appendPayloadVariant(variants *[]map[string]any, seen map[string]struct{}, payload map[string]any) {
	data, err := json.Marshal(payload)
	if err != nil {
		*variants = append(*variants, payload)
		return
	}
	signature := string(data)
	if _, ok := seen[signature]; ok {
		return
	}
	seen[signature] = struct{}{}
	*variants = append(*variants, payload)
}

func clonePayload(payload map[string]any) map[string]any {
	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = value
	}
	return cloned
}

func (c *OpenAICompatible) postJSON(ctx context.Context, endpoint string, payload map[string]any) ([]byte, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
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

type openAIMessageEnvelope struct {
	Role      string                `json:"role"`
	Content   json.RawMessage       `json:"content"`
	ToolCalls []openAIToolCallChunk `json:"tool_calls"`
}

type openAIToolCallChunk struct {
	Index    int    `json:"index"`
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

func parseOpenAIMessageResponse(respBody []byte) (llm.Message, error) {
	var completion struct {
		Choices []struct {
			Message openAIMessageEnvelope `json:"message"`
			Delta   openAIMessageEnvelope `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return llm.Message{}, err
	}
	if len(completion.Choices) > 0 {
		selected := completion.Choices[0].Message
		if isEmptyOpenAIMessage(selected) {
			selected = completion.Choices[0].Delta
		}
		if !isEmptyOpenAIMessage(selected) {
			return messageFromOpenAIEnvelope(selected), nil
		}
	}

	var responseStyle struct {
		OutputText json.RawMessage `json:"output_text"`
		Output     []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(respBody, &responseStyle); err != nil {
		return llm.Message{}, err
	}

	text := parseMessageContent(responseStyle.OutputText)
	if text == "" {
		for _, block := range responseStyle.Output {
			for _, item := range block.Content {
				if item.Type == "output_text" || item.Type == "text" {
					text += item.Text
				}
			}
		}
	}
	if strings.TrimSpace(text) == "" {
		return llm.Message{}, fmt.Errorf("provider returned no choices")
	}
	return llm.Message{Role: "assistant", Content: text}, nil
}

func messageFromOpenAIEnvelope(envelope openAIMessageEnvelope) llm.Message {
	msg := llm.Message{
		Role:    envelope.Role,
		Content: parseMessageContent(envelope.Content),
	}
	if msg.Role == "" {
		msg.Role = "assistant"
	}
	if len(envelope.ToolCalls) > 0 {
		msg.ToolCalls = make([]llm.ToolCall, 0, len(envelope.ToolCalls))
		for _, call := range envelope.ToolCalls {
			arguments := parseToolArgumentsPiece(call.Function.Arguments)
			if strings.TrimSpace(arguments) == "" {
				arguments = "{}"
			}
			msg.ToolCalls = append(msg.ToolCalls, llm.ToolCall{
				ID:   call.ID,
				Type: choose(call.Type, "function"),
				Function: llm.ToolFunctionCall{
					Name:      call.Function.Name,
					Arguments: arguments,
				},
			})
		}
	}
	return msg
}

func parseMessageContent(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}

	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}

	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &blocks); err == nil {
		var b strings.Builder
		for _, block := range blocks {
			if block.Type == "text" || block.Type == "output_text" || block.Text != "" {
				b.WriteString(block.Text)
			}
		}
		return b.String()
	}

	var single struct {
		Text  string `json:"text"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(raw, &single); err == nil {
		return choose(single.Text, single.Value)
	}
	return ""
}

func parseToolArgumentsPiece(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var asText string
	if err := json.Unmarshal(raw, &asText); err == nil {
		return asText
	}
	return string(raw)
}

func mergeOpenAIMessageEnvelope(current llm.Message, envelope openAIMessageEnvelope, onDelta func(string)) llm.Message {
	if envelope.Role != "" {
		current.Role = envelope.Role
	}
	content := parseMessageContent(envelope.Content)
	if content != "" {
		current.Content += content
		if onDelta != nil {
			onDelta(content)
		}
	}
	for _, call := range envelope.ToolCalls {
		current.ToolCalls = append(current.ToolCalls, llm.ToolCall{
			ID:   call.ID,
			Type: choose(call.Type, "function"),
			Function: llm.ToolFunctionCall{
				Name:      call.Function.Name,
				Arguments: parseToolArgumentsPiece(call.Function.Arguments),
			},
		})
	}
	return current
}

func ensureToolCall(calls map[int]*llm.ToolCall, index int) *llm.ToolCall {
	call, ok := calls[index]
	if !ok {
		call = &llm.ToolCall{Type: "function"}
		calls[index] = call
	}
	return call
}

func mergeToolCallDelta(call *llm.ToolCall, delta openAIToolCallChunk) {
	if delta.ID != "" {
		call.ID = delta.ID
	}
	if delta.Type != "" {
		call.Type = delta.Type
	}
	if delta.Function.Name != "" {
		call.Function.Name += delta.Function.Name
	}
	if arguments := parseToolArgumentsPiece(delta.Function.Arguments); arguments != "" {
		call.Function.Arguments += arguments
	}
}

func isEmptyOpenAIMessage(message openAIMessageEnvelope) bool {
	return strings.TrimSpace(message.Role) == "" &&
		len(bytes.TrimSpace(message.Content)) == 0 &&
		len(message.ToolCalls) == 0
}

func openAIDefaultPaths(baseURL string) []string {
	path := strings.ToLower(strings.TrimSuffix(extractPath(baseURL), "/"))
	if strings.HasSuffix(path, "/v1") {
		return []string{"chat/completions"}
	}
	return []string{"chat/completions", "v1/chat/completions"}
}

func cloneHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return map[string]string{}
	}
	cloned := make(map[string]string, len(headers))
	for key, value := range headers {
		cloned[key] = value
	}
	return cloned
}

func choose(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}
