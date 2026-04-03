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
	payload := c.chatPayload(req, false)
	respBody, err := c.postJSON(ctx, c.baseURL+"/chat/completions", payload)
	if err != nil {
		return llm.Message{}, err
	}

	var completion struct {
		Choices []struct {
			Message llm.Message `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return llm.Message{}, err
	}
	if len(completion.Choices) == 0 {
		return llm.Message{}, fmt.Errorf("provider returned no choices")
	}
	return completion.Choices[0].Message, nil
}

func (c *OpenAICompatible) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	payload := c.chatPayload(req, true)
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
		return llm.Message{}, fmt.Errorf("provider error %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
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
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			break
		}

		var chunk struct {
			Choices []struct {
				Delta struct {
					Role      string `json:"role"`
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return llm.Message{}, err
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Role != "" {
				assembled.Role = choice.Delta.Role
			}
			if choice.Delta.Content != "" {
				assembled.Content += choice.Delta.Content
				if onDelta != nil {
					onDelta(choice.Delta.Content)
				}
			}
			for _, callDelta := range choice.Delta.ToolCalls {
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
				if callDelta.Function.Name != "" {
					call.Function.Name += callDelta.Function.Name
				}
				if callDelta.Function.Arguments != "" {
					call.Function.Arguments += callDelta.Function.Arguments
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
		assembled.ToolCalls = make([]llm.ToolCall, 0, len(indexes))
		for _, index := range indexes {
			assembled.ToolCalls = append(assembled.ToolCalls, *toolCalls[index])
		}
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
		return nil, fmt.Errorf("provider error %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}
	return respBody, nil
}

func choose(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}
