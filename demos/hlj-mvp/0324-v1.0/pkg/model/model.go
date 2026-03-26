package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

type Config struct {
	APIKey  string `json:"api_key"`
	Model   string `json:"model"`
	APIBase string `json:"api_base"`
}

type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

type Client struct {
	config Config
	client *http.Client
}

func NewClient(apiKey, model, apiBase string) *Client {
	return &Client{
		config: Config{
			APIKey:  apiKey,
			Model:   model,
			APIBase: apiBase,
		},
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

type ChatResponse struct {
	Content    string
	ToolCalls  []ToolCall
	StopReason string
}

func (c *Client) Chat(messages []Message, tools []Tool) (*ChatResponse, error) {
	reqBody := map[string]interface{}{
		"model":      c.config.Model,
		"messages":   messages,
		"max_tokens": 2000,
	}

	if len(tools) > 0 {
		reqBody["tools"] = tools
	}

	jsonData, _ := json.Marshal(reqBody)
	req, err := http.NewRequest("POST", c.config.APIBase+"/chat/completions", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode == 401 {
		return nil, fmt.Errorf("authentication failed: %s", string(body))
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("api error (%d): %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	json.Unmarshal(body, &result)

	response := &ChatResponse{}

	if choices, ok := result["choices"].([]interface{}); ok && len(choices) > 0 {
		choice := choices[0].(map[string]interface{})

		if reason, ok := choice["finish_reason"].(string); ok {
			response.StopReason = reason
		}

		if msg, ok := choice["message"].(map[string]interface{}); ok {
			if content, ok := msg["content"].(string); ok {
				response.Content = content
			}

			// 解析 tool_calls
			if toolCalls, ok := msg["tool_calls"].([]interface{}); ok {
				for _, tcRaw := range toolCalls {
					if tcMap, ok := tcRaw.(map[string]interface{}); ok {
						var tc ToolCall
						tc.Type = "function" // 默认为 function
						if id, ok := tcMap["id"].(string); ok {
							tc.ID = id
						}
						if fn, ok := tcMap["function"].(map[string]interface{}); ok {
							if name, ok := fn["name"].(string); ok {
								tc.Function.Name = name
							}
							if args, ok := fn["arguments"].(string); ok {
								tc.Function.Arguments = args
							}
						}
						response.ToolCalls = append(response.ToolCalls, tc)
					}
				}
			}
		}
	}

	return response, nil
}

func GetConfigPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".forgecli", "config.json")
}

func LoadConfig() (Config, error) {
	path := GetConfigPath()
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	var cfg Config
	err = json.Unmarshal(data, &cfg)
	return cfg, err
}

func SaveConfig(cfg Config) error {
	path := GetConfigPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, _ := json.MarshalIndent(cfg, "", "  ")
	return os.WriteFile(path, data, 0600)
}

func (c *Client) IsConfigured() bool {
	return c.config.APIKey != ""
}
