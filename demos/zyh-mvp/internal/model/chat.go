package model

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"forgecli/internal/config"
)

type ChatClient interface {
	Complete(ctx context.Context, req ChatRequest) (ChatResponse, error)
}

type ChatRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
}

type ChatResponse struct {
	Message      Message
	FinishReason string
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	Name       string     `json:"name,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type ToolDefinition struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"`
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function FunctionCallData `json:"function"`
}

type FunctionCallData struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAICompatibleClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

type completionRequest struct {
	Model       string           `json:"model"`
	Messages    []Message        `json:"messages"`
	Tools       []ToolDefinition `json:"tools,omitempty"`
	ToolChoice  string           `json:"tool_choice,omitempty"`
	Temperature float64          `json:"temperature,omitempty"`
}

type completionResponse struct {
	Choices []struct {
		Message      Message `json:"message"`
		FinishReason string  `json:"finish_reason"`
	} `json:"choices"`
}

type errorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

func NewChatClient(cfg config.ModelConfig) (ChatClient, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Provider)) {
	case "", "openai", "openai_compatible", "openai-compatible":
		return NewOpenAICompatibleClient(cfg), nil
	default:
		return nil, fmt.Errorf("unsupported model provider: %s", cfg.Provider)
	}
}

func NewOpenAICompatibleClient(cfg config.ModelConfig) *OpenAICompatibleClient {
	timeout := time.Duration(cfg.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	return &OpenAICompatibleClient{
		baseURL:    normalizeChatCompletionsURL(cfg.BaseURL),
		apiKey:     resolveAPIKey(cfg),
		httpClient: &http.Client{Timeout: timeout},
	}
}

func (c *OpenAICompatibleClient) Complete(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	if strings.TrimSpace(req.Model) == "" {
		return ChatResponse{}, errors.New("model is required")
	}
	if len(req.Messages) == 0 {
		return ChatResponse{}, errors.New("at least one message is required")
	}

	body := completionRequest{Model: req.Model, Messages: req.Messages, Tools: req.Tools, Temperature: req.Temperature}
	if len(req.Tools) > 0 {
		body.ToolChoice = "auto"
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("marshal request: %w", err)
	}

	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		response, err := c.doComplete(ctx, payload)
		if err == nil {
			return response, nil
		}
		lastErr = err
		if !isRetryableModelError(err) || attempt == 1 {
			break
		}
		time.Sleep(350 * time.Millisecond)
	}
	return ChatResponse{}, lastErr
}

func (c *OpenAICompatibleClient) doComplete(ctx context.Context, payload []byte) (ChatResponse, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(payload))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("build request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("request model: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var apiErr errorResponse
		if err := json.NewDecoder(resp.Body).Decode(&apiErr); err == nil && strings.TrimSpace(apiErr.Error.Message) != "" {
			return ChatResponse{}, fmt.Errorf("model API error: %s", apiErr.Error.Message)
		}
		return ChatResponse{}, fmt.Errorf("model API returned status %s", resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read response: %w", err)
	}
	var parsed completionResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return ChatResponse{}, fmt.Errorf("decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return ChatResponse{}, errors.New("model returned no choices")
	}
	choice := parsed.Choices[0]
	return ChatResponse{Message: choice.Message, FinishReason: choice.FinishReason}, nil
}

func isRetryableModelError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "deadline exceeded") || strings.Contains(lower, "timeout")
}

func normalizeChatCompletionsURL(baseURL string) string {
	trimmed := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if trimmed == "" {
		trimmed = "https://api.openai.com/v1"
	}
	if strings.HasSuffix(trimmed, "/chat/completions") {
		return trimmed
	}
	return trimmed + "/chat/completions"
}

func resolveAPIKey(cfg config.ModelConfig) string {
	if strings.TrimSpace(cfg.APIKey) != "" {
		return cfg.APIKey
	}
	if strings.TrimSpace(cfg.APIKeyEnv) == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(cfg.APIKeyEnv))
}
