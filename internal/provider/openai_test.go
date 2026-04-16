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

func TestOpenAICompatibleCreateMessageUsesCustomGatewayConfig(t *testing.T) {
	var path string
	var authHeader string
	var extraHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		authHeader = r.Header.Get("X-API-Key")
		extraHeader = r.Header.Get("X-Gateway")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "done",
				},
			}},
		})
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{
		BaseURL:      server.URL,
		APIPath:      "/v42/chat",
		APIKey:       "test-key",
		AuthHeader:   "X-API-Key",
		AuthScheme:   "",
		ExtraHeaders: map[string]string{"X-Gateway": "enabled"},
		Model:        "fallback-model",
	})

	if _, err := client.CreateMessage(context.Background(), llm.ChatRequest{}); err != nil {
		t.Fatal(err)
	}
	if path != "/v42/chat" {
		t.Fatalf("unexpected path %q", path)
	}
	if authHeader != "test-key" {
		t.Fatalf("unexpected auth header %q", authHeader)
	}
	if extraHeader != "enabled" {
		t.Fatalf("unexpected extra header %q", extraHeader)
	}
}

func TestOpenAICompatibleCreateMessageReturnsFirstChoice(t *testing.T) {
	var authHeader string
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		authHeader = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "done",
				},
			}},
			"usage": map[string]any{
				"prompt_tokens":     12,
				"completion_tokens": 5,
				"total_tokens":      17,
			},
		})
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Model:   "fallback-model",
	})

	msg, err := client.CreateMessage(context.Background(), llm.ChatRequest{
		Model: "request-model",
		Messages: []llm.Message{{
			Role:    "user",
			Content: "hello",
		}},
		Temperature: 0.2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Role != "assistant" || msg.Content != "done" {
		t.Fatalf("unexpected message: %#v", msg)
	}
	if msg.Usage == nil || msg.Usage.TotalTokens != 17 || msg.Usage.InputTokens != 12 || msg.Usage.OutputTokens != 5 {
		t.Fatalf("expected usage payload in message, got %#v", msg.Usage)
	}
	if authHeader != "Bearer test-key" {
		t.Fatalf("unexpected authorization header %q", authHeader)
	}
	if got := requestBody["model"]; got != "request-model" {
		t.Fatalf("expected request model override, got %#v", got)
	}
}

func TestOpenAICompatibleCreateMessageRejectsEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{}})
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{BaseURL: server.URL, APIKey: "test-key", Model: "fallback-model"})
	_, err := client.CreateMessage(context.Background(), llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected empty choices error")
	}
	if !strings.Contains(err.Error(), "provider returned no choices") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAICompatibleCreateMessageReturnsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{BaseURL: server.URL, APIKey: "test-key", Model: "fallback-model"})
	_, err := client.CreateMessage(context.Background(), llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected provider error")
	}
	var providerErr *llm.ProviderError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected provider error type, got %T", err)
	}
	if providerErr.Code != llm.ErrorCodeRateLimited || providerErr.Status != http.StatusTooManyRequests {
		t.Fatalf("unexpected provider error: %#v", providerErr)
	}
}

func TestOpenAICompatibleStreamMessageAssemblesContentAndToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"choices":[{"delta":{"role":"assistant","content":"Hello "}}]}`,
			`data: {"choices":[{"delta":{"content":"world","tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"list_","arguments":"{\"path\":\"src"}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"files","arguments":"\"}"}}]}}]}`,
			`data: {"choices":[],"usage":{"prompt_tokens":20,"completion_tokens":8,"total_tokens":28}}`,
			`data: [DONE]`,
			"",
		}, "\n")))
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{BaseURL: server.URL, APIKey: "test-key", Model: "fallback-model"})
	deltas := make([]string, 0, 2)
	msg, err := client.StreamMessage(context.Background(), llm.ChatRequest{}, func(delta string) {
		deltas = append(deltas, delta)
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Role != "assistant" {
		t.Fatalf("expected assistant role, got %#v", msg)
	}
	if msg.Content != "Hello world" {
		t.Fatalf("expected assembled content, got %q", msg.Content)
	}
	if strings.Join(deltas, "") != "Hello world" {
		t.Fatalf("expected delta callback content, got %#v", deltas)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %#v", msg.ToolCalls)
	}
	call := msg.ToolCalls[0]
	if call.ID != "call-1" || call.Type != "function" {
		t.Fatalf("unexpected tool call envelope: %#v", call)
	}
	if call.Function.Name != "list_files" {
		t.Fatalf("expected tool name concatenation, got %#v", call.Function)
	}
	if call.Function.Arguments != "{\"path\":\"src\"}" {
		t.Fatalf("expected tool arguments concatenation, got %q", call.Function.Arguments)
	}
	if msg.Usage == nil || msg.Usage.TotalTokens != 28 || msg.Usage.InputTokens != 20 || msg.Usage.OutputTokens != 8 {
		t.Fatalf("expected stream usage parse result, got %#v", msg.Usage)
	}
}

func TestOpenAICompatibleStreamMessageRejectsInvalidChunk(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {not-json}\n\n"))
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{BaseURL: server.URL, APIKey: "test-key", Model: "fallback-model"})
	_, err := client.StreamMessage(context.Background(), llm.ChatRequest{}, nil)
	if err == nil {
		t.Fatal("expected invalid chunk error")
	}
}

func TestOpenAICompatibleCreateMessageDoesNotExposeReasoningOnlyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":              "assistant",
					"content":           nil,
					"reasoning_content": "final from reasoning",
				},
			}},
		})
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{BaseURL: server.URL, APIKey: "test-key", Model: "fallback-model"})
	msg, err := client.CreateMessage(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "" {
		t.Fatalf("expected reasoning-only response to stay empty, got %#v", msg)
	}
}

func TestOpenAICompatibleCreateMessageParsesLegacyFunctionCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "",
					"function_call": map[string]any{
						"name":      "list_files",
						"arguments": "{\"path\":\".\"}",
					},
				},
			}},
		})
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{BaseURL: server.URL, APIKey: "test-key", Model: "fallback-model"})
	msg, err := client.CreateMessage(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected one parsed legacy function call, got %#v", msg.ToolCalls)
	}
	if msg.ToolCalls[0].Function.Name != "list_files" || msg.ToolCalls[0].Function.Arguments != "{\"path\":\".\"}" {
		t.Fatalf("unexpected legacy tool call parse result: %#v", msg.ToolCalls[0])
	}
}

func TestOpenAICompatibleStreamMessageDoesNotExposeReasoningOnlyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"choices":[{"delta":{"role":"assistant","reasoning_content":"hello"}}]}`,
			`data: {"choices":[{"delta":{"reasoning_content":" world"}}]}`,
			`data: [DONE]`,
			"",
		}, "\n")))
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{BaseURL: server.URL, APIKey: "test-key", Model: "fallback-model"})
	msg, err := client.StreamMessage(context.Background(), llm.ChatRequest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "" {
		t.Fatalf("expected reasoning-only stream to stay empty, got %#v", msg)
	}
}

func TestOpenAICompatibleStreamMessageAssemblesLegacyFunctionCallAcrossChunks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"choices":[{"delta":{"role":"assistant","function_call":{"name":"run_shell"}}}]}`,
			`data: {"choices":[{"delta":{"function_call":{"arguments":"{\"cmd\":[\"bash\",\"-lc\",\"ls -R\"]}"}}}]}`,
			`data: [DONE]`,
			"",
		}, "\n")))
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{BaseURL: server.URL, APIKey: "test-key", Model: "fallback-model"})
	msg, err := client.StreamMessage(context.Background(), llm.ChatRequest{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected one legacy tool call, got %#v", msg.ToolCalls)
	}
	call := msg.ToolCalls[0]
	if call.Function.Name != "run_shell" {
		t.Fatalf("expected legacy tool call name run_shell, got %#v", call)
	}
	if call.Function.Arguments != "{\"cmd\":[\"bash\",\"-lc\",\"ls -R\"]}" {
		t.Fatalf("unexpected legacy tool call arguments: %#v", call)
	}
}

func TestOpenAICompatibleCreateMessageParsesToolCallObjectArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role": "assistant",
					"tool_calls": []map[string]any{{
						"id":   "call-1",
						"type": "function",
						"function": map[string]any{
							"name":      "list_files",
							"arguments": map[string]any{"path": "."},
						},
					}},
				},
			}},
		})
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{BaseURL: server.URL, APIKey: "test-key", Model: "fallback-model"})
	msg, err := client.CreateMessage(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.ToolCalls) != 1 {
		t.Fatalf("expected one tool call, got %#v", msg.ToolCalls)
	}
	if msg.ToolCalls[0].Function.Arguments != "{\"path\":\".\"}" {
		t.Fatalf("expected object arguments to serialize as json, got %#v", msg.ToolCalls[0])
	}
}

func TestOpenAICompatibleChatPayloadUsesFallbackModelAndTools(t *testing.T) {
	client := NewOpenAICompatible(Config{BaseURL: "https://example.com", APIKey: "test-key", Model: "fallback-model"})
	payload, err := client.chatPayload(llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
		Tools: []llm.ToolDefinition{{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name: "list_files",
			},
		}},
		Temperature: 0,
	}, true)
	if err != nil {
		t.Fatalf("chat payload: %v", err)
	}

	if got := payload["model"]; got != "fallback-model" {
		t.Fatalf("expected fallback model, got %#v", got)
	}
	if got := payload["stream"]; got != true {
		t.Fatalf("expected stream=true, got %#v", got)
	}
	if got := payload["tool_choice"]; got != "auto" {
		t.Fatalf("expected tool_choice auto, got %#v", got)
	}
	if got := payload["temperature"]; got != float64(0) {
		t.Fatalf("expected temperature zero to be preserved, got %#v", got)
	}
}

func TestOpenAIMessagesMapsThinkingAndToolResultParts(t *testing.T) {
	messages, err := openAIMessages(llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role: llm.RoleAssistant,
				Parts: []llm.Part{
					{Type: llm.PartThinking, Thinking: &llm.ThinkingPart{Value: "reasoning"}},
					{Type: llm.PartToolUse, ToolUse: &llm.ToolUsePart{ID: "call-1", Name: "list_files", Arguments: `{"path":"."}`}},
				},
			},
			llm.NewToolResultMessage("call-1", `{"ok":true}`),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected assistant and tool_result messages, got %#v", messages)
	}

	assistant := messages[0]
	if assistant["content"] != "reasoning" {
		t.Fatalf("expected thinking mapped as string content, got %#v", assistant)
	}
	toolCalls, _ := assistant["tool_calls"].([]map[string]any)
	if len(toolCalls) != 1 || toolCalls[0]["id"] != "call-1" {
		t.Fatalf("expected tool call mapping, got %#v", assistant)
	}
	if messages[1]["role"] != "tool" || messages[1]["tool_call_id"] != "call-1" {
		t.Fatalf("expected tool_result mapping, got %#v", messages[1])
	}
}

func TestOpenAIMessagesDegradesMissingImageAsset(t *testing.T) {
	messages, err := openAIMessages(llm.ChatRequest{
		Messages: []llm.Message{{
			Role: llm.RoleUser,
			Parts: []llm.Part{{
				Type:  llm.PartImageRef,
				Image: &llm.ImagePartRef{AssetID: "asset-1"},
			}},
		}},
	})
	if err != nil {
		t.Fatalf("openAIMessages: %v", err)
	}
	if len(messages) != 1 {
		t.Fatalf("expected a single converted message, got %#v", messages)
	}
	if messages[0]["content"] != "unavailable asset asset-1" {
		t.Fatalf("expected missing image to degrade to string fallback, got %#v", messages[0])
	}
}

func TestParseOpenAIMessageHandlesStructuredContentAndOutputText(t *testing.T) {
	msg := parseOpenAIMessage([]byte(`{"role":"assistant","content":[{"type":"text","text":"hello "},{"type":"text","text":"world"}]}`))
	if msg.Content != "hello world" {
		t.Fatalf("expected structured content extraction, got %#v", msg)
	}
	msg = parseOpenAIMessage([]byte(`{"role":"assistant","output_text":[{"type":"text","text":"fallback text"}]}`))
	if msg.Content != "fallback text" {
		t.Fatalf("expected output_text fallback, got %#v", msg)
	}
}

func TestParseOpenAIDeltaParsesStructuredContentAndReasoning(t *testing.T) {
	delta, err := parseOpenAIDelta([]byte(`{
  "role":"assistant",
  "content":[{"type":"text","text":"hello "},{"type":"text","text":"world"}],
  "reasoning_content":"thinking",
  "tool_calls":[{"index":1,"id":"call-1","type":"function","function":{"name":"list_","arguments":"{\"path\":\"src\"}"}}]
}`))
	if err != nil {
		t.Fatal(err)
	}
	if delta.Role != llm.RoleAssistant || delta.Reasoning != "thinking" || delta.Content != "hello world" {
		t.Fatalf("unexpected parsed delta: %#v", delta)
	}
	if len(delta.ToolCalls) != 1 || delta.ToolCalls[0].FunctionName != "list_" {
		t.Fatalf("unexpected tool call delta parse: %#v", delta.ToolCalls)
	}
}

func TestParseOpenAIUsageRestoresDetailedTokenAccounting(t *testing.T) {
	usage := parseOpenAIUsage([]byte(`{"prompt_tokens":10,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":3,"audio_tokens":2},"completion_tokens_details":{"audio_tokens":1}}`))
	if usage == nil || usage.InputTokens != 12 || usage.OutputTokens != 6 || usage.ContextTokens != 3 || usage.TotalTokens != 21 {
		t.Fatalf("unexpected usage %#v", usage)
	}
}

func TestParseOpenAIDeltaParsesToolCallsAndReasoning(t *testing.T) {
	delta, err := parseOpenAIDelta([]byte(`{
  "role":"assistant",
  "reasoning_content":"thinking",
  "tool_calls":[{"index":1,"id":"call-1","type":"function","function":{"name":"list_","arguments":"{\"path\":\"src\"}"}}]
}`))

	if err != nil {
		t.Fatal(err)
	}
	if delta.Role != llm.RoleAssistant || delta.Reasoning != "thinking" {
		t.Fatalf("unexpected parsed delta: %#v", delta)
	}
	if len(delta.ToolCalls) != 1 || delta.ToolCalls[0].FunctionName != "list_" {
		t.Fatalf("unexpected tool call delta parse: %#v", delta.ToolCalls)
	}
}
