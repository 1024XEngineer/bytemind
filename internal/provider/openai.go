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

const legacyToolCallIndex = -1

type Config struct {
	Type             string
	BaseURL          string
	APIPath          string
	APIKey           string
	AuthHeader       string
	AuthScheme       string
	ExtraHeaders     map[string]string
	Model            string
	AnthropicVersion string
}

type OpenAICompatible struct {
	baseURL      string
	apiPath      string
	apiKey       string
	authHeader   string
	authScheme   string
	extraHeaders map[string]string
	model        string
	httpClient   *http.Client
}

func NewOpenAICompatible(cfg Config) *OpenAICompatible {
	apiPath := strings.TrimSpace(cfg.APIPath)
	if apiPath == "" {
		apiPath = "/chat/completions"
	}
	if !strings.HasPrefix(apiPath, "/") {
		apiPath = "/" + apiPath
	}
	authHeader := strings.TrimSpace(cfg.AuthHeader)
	if authHeader == "" {
		authHeader = "Authorization"
	}
	authScheme := strings.TrimSpace(cfg.AuthScheme)
	if authScheme == "" && authHeader == "Authorization" {
		authScheme = "Bearer"
	}
	extraHeaders := make(map[string]string, len(cfg.ExtraHeaders))
	for key, value := range cfg.ExtraHeaders {
		extraHeaders[key] = value
	}
	return &OpenAICompatible{
		baseURL:      strings.TrimRight(cfg.BaseURL, "/"),
		apiPath:      apiPath,
		apiKey:       cfg.APIKey,
		authHeader:   authHeader,
		authScheme:   authScheme,
		extraHeaders: extraHeaders,
		model:        cfg.Model,
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
	respBody, err := c.postJSON(ctx, c.baseURL+c.apiPath, payload)
	if err != nil {
		return llm.Message{}, err
	}

	var completion struct {
		Choices []struct {
			Message json.RawMessage `json:"message"`
		} `json:"choices"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return llm.Message{}, err
	}
	if len(completion.Choices) == 0 {
		return llm.Message{}, fmt.Errorf("provider returned no choices")
	}
	msg := parseOpenAIMessage(completion.Choices[0].Message)
	msg.Usage = parseOpenAIUsage(completion.Usage)
	return msg, nil
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

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+c.apiPath, bytes.NewReader(body))
	if err != nil {
		return llm.Message{}, err
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
		return llm.Message{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return llm.Message{}, llm.MapProviderError("openai", resp.StatusCode, string(respBody), nil)
	}

	assembled := llm.Message{Role: llm.RoleAssistant}
	toolCalls := map[int]*llm.ToolCall{}

	reader := bufio.NewReader(resp.Body)
	for {
		rawLine, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return llm.Message{}, err
		}
		line := strings.TrimSpace(rawLine)
		if err == io.EOF && line == "" {
			break
		}
		if line == "" || !strings.HasPrefix(line, "data:") {
			if err == io.EOF {
				break
			}
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
			Usage json.RawMessage `json:"usage"`
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
		if usage := parseOpenAIUsage(chunk.Usage); usage != nil {
			assembled.Usage = usage
		}
		if err == io.EOF {
			break
		}
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

func choose(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}
