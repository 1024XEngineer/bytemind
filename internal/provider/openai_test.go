package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"bytemind/internal/llm"
)

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
	if !strings.Contains(err.Error(), "provider error 429") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestOpenAICompatibleStreamMessageAssemblesContentAndToolCalls(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(strings.Join([]string{
			`data: {"choices":[{"delta":{"role":"assistant","content":"Hello "}}]}`,
			`data: {"choices":[{"delta":{"content":"world","tool_calls":[{"index":0,"id":"call-1","type":"function","function":{"name":"list_","arguments":"{\"path\":\"src"}}]}}]}`,
			`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"name":"files","arguments":"\"}"}}]}}]}`,
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
	payload := client.chatPayload(llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
		Tools: []llm.ToolDefinition{{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name: "list_files",
			},
		}},
		Temperature: 0.4,
	}, true)

	if got := payload["model"]; got != "fallback-model" {
		t.Fatalf("expected fallback model, got %#v", got)
	}
	if got := payload["stream"]; got != true {
		t.Fatalf("expected stream=true, got %#v", got)
	}
	if got := payload["tool_choice"]; got != "auto" {
		t.Fatalf("expected tool_choice auto, got %#v", got)
	}
}

