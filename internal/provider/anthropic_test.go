package provider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bytemind/internal/llm"
)

func TestNewAnthropicDefaultsVersion(t *testing.T) {
	client := NewAnthropic(Config{BaseURL: "https://example.com", APIKey: "test-key", Model: "claude"})
	if client.anthropicVersion != "2023-06-01" {
		t.Fatalf("expected default anthropic version, got %q", client.anthropicVersion)
	}
}

func TestAnthropicCreateMessageParsesTextAndToolUse(t *testing.T) {
	var versionHeader string
	var apiKeyHeader string
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		versionHeader = r.Header.Get("anthropic-version")
		apiKeyHeader = r.Header.Get("x-api-key")
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "Plan ready. "},
				{"type": "tool_use", "id": "tool-1", "name": "list_files", "input": map[string]any{"path": "."}},
			},
			"usage": map[string]any{
				"input_tokens":                42,
				"output_tokens":               11,
				"cache_read_input_tokens":     3,
				"cache_creation_input_tokens": 2,
			},
		})
	}))
	defer server.Close()

	client := NewAnthropic(Config{
		BaseURL:          server.URL,
		APIKey:           "test-key",
		Model:            "claude-default",
		AnthropicVersion: "2025-01-01",
	})

	msg, err := client.CreateMessage(context.Background(), llm.ChatRequest{
		Model: "request-model",
		Messages: []llm.Message{
			{Role: "system", Content: "follow rules"},
			{Role: "user", Content: "inspect repo"},
		},
		Tools: []llm.ToolDefinition{{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "list_files",
				Description: "list files",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
		Temperature: 0.3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if apiKeyHeader != "test-key" || versionHeader != "2025-01-01" {
		t.Fatalf("unexpected headers apiKey=%q version=%q", apiKeyHeader, versionHeader)
	}
	if got := requestBody["model"]; got != "request-model" {
		t.Fatalf("expected request model override, got %#v", got)
	}
	if got := requestBody["system"]; got != "follow rules" {
		t.Fatalf("expected system content, got %#v", got)
	}
	if msg.Content != "Plan ready. " {
		t.Fatalf("unexpected content: %#v", msg)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %#v", msg.ToolCalls)
	}
	if msg.ToolCalls[0].Function.Name != "list_files" || msg.ToolCalls[0].Function.Arguments != "{\"path\":\".\"}" {
		t.Fatalf("unexpected tool call: %#v", msg.ToolCalls[0])
	}
	if msg.Usage == nil || msg.Usage.InputTokens != 42 || msg.Usage.OutputTokens != 11 || msg.Usage.ContextTokens != 5 || msg.Usage.TotalTokens != 58 {
		t.Fatalf("unexpected usage parse result: %#v", msg.Usage)
	}
}

func TestAnthropicCreateMessageReturnsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer server.Close()

	client := NewAnthropic(Config{BaseURL: server.URL, APIKey: "test-key", Model: "claude"})
	_, err := client.CreateMessage(context.Background(), llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected provider error")
	}
	var providerErr *llm.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected provider error type, got %T", err)
	}
	if providerErr.Status != http.StatusBadRequest || providerErr.Code != llm.ErrorCodeUnknown {
		t.Fatalf("unexpected provider error: %#v", providerErr)
	}
}

func TestAnthropicStreamMessageInvokesDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": "hello from anthropic"},
			},
		})
	}))
	defer server.Close()

	client := NewAnthropic(Config{BaseURL: server.URL, APIKey: "test-key", Model: "claude"})
	var gotDelta string
	msg, err := client.StreamMessage(context.Background(), llm.ChatRequest{}, func(delta string) {
		gotDelta += delta
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotDelta != "hello from anthropic" || msg.Content != gotDelta {
		t.Fatalf("unexpected stream result message=%#v delta=%q", msg, gotDelta)
	}
}

func TestAnthropicMessagesConvertsConversation(t *testing.T) {
	system, converted, err := anthropicMessages(llm.ChatRequest{
		Messages: []llm.Message{
			{Role: "system", Content: "sys-1"},
			{Role: "system", Content: "sys-2"},
			{Role: "user", Content: "question"},
			{
				Role:    "assistant",
				Content: "thinking",
				ToolCalls: []llm.ToolCall{{
					ID:   "call-1",
					Type: "function",
					Function: llm.ToolFunctionCall{
						Name:      "list_files",
						Arguments: "{\"path\":\".\"}",
					},
				}},
			},
			{Role: "tool", ToolCallID: "call-1", Content: "{\"ok\":true}"},
		},
	})
	if err != nil {
		t.Fatalf("anthropicMessages: %v", err)
	}

	if system != "sys-1\n\nsys-2" {
		t.Fatalf("unexpected system prompt %q", system)
	}
	if len(converted) != 3 {
		t.Fatalf("expected 3 converted messages, got %#v", converted)
	}
	assistant := converted[1]
	blocks := assistant["content"].([]map[string]any)
	if assistant["role"] != "assistant" || len(blocks) != 2 {
		t.Fatalf("unexpected assistant conversion: %#v", assistant)
	}
	userToolResult := converted[2]["content"].([]map[string]any)
	if userToolResult[0]["type"] != "tool_result" {
		t.Fatalf("expected tool_result block, got %#v", userToolResult)
	}
}

func TestParseJSONObjectFallsBackToRawValue(t *testing.T) {
	got := parseJSONObject("{not-json}")
	value, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected fallback object, got %#v", got)
	}
	if value["raw"] != "{not-json}" {
		t.Fatalf("unexpected fallback payload %#v", value)
	}
}

func TestAnthropicMessagesDegradesMissingImageAsset(t *testing.T) {
	_, converted, err := anthropicMessages(llm.ChatRequest{
		Messages: []llm.Message{{
			Role: llm.RoleUser,
			Parts: []llm.Part{{
				Type:  llm.PartImageRef,
				Image: &llm.ImagePartRef{AssetID: "asset-1"},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("anthropicMessages: %v", err)
	}
	if len(converted) != 1 {
		t.Fatalf("expected single converted message, got %#v", converted)
	}
	blocks := converted[0]["content"].([]map[string]any)
	if len(blocks) != 1 || blocks[0]["type"] != "text" {
		t.Fatalf("expected missing image to degrade to text block, got %#v", converted[0])
	}
	text, _ := blocks[0]["text"].(string)
	if !strings.Contains(text, "unavailable asset asset-1") {
		t.Fatalf("expected fallback text to include asset id, got %#v", blocks[0]["text"])
	}
}

func TestAnthropicMessagesPreservesToolResultErrorFlag(t *testing.T) {
	_, converted, err := anthropicMessages(llm.ChatRequest{
		Messages: []llm.Message{{
			Role: llm.RoleUser,
			Parts: []llm.Part{{
				Type: llm.PartToolResult,
				ToolResult: &llm.ToolResultPart{
					ToolUseID: "call-1",
					Content:   "{\"ok\":false}",
					IsError:   true,
				},
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(converted) != 1 {
		t.Fatalf("unexpected converted messages: %#v", converted)
	}
	blocks := converted[0]["content"].([]map[string]any)
	if len(blocks) != 1 || blocks[0]["is_error"] != true {
		t.Fatalf("expected is_error propagated, got %#v", blocks)
	}
}
