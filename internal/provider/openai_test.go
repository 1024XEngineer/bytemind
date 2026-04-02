package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
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

func TestOpenAICompatibleCreateMessageFallsBackToV1Endpoint(t *testing.T) {
	requestPaths := make([]string, 0, 2)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPaths = append(requestPaths, r.URL.Path)
		if r.URL.Path == "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "fallback path works",
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
	if msg.Content != "fallback path works" {
		t.Fatalf("unexpected message content: %#v", msg)
	}
	if len(requestPaths) != 2 || requestPaths[0] != "/chat/completions" || requestPaths[1] != "/v1/chat/completions" {
		t.Fatalf("unexpected request paths: %#v", requestPaths)
	}
}

func TestOpenAICompatibleCreateMessageUsesCustomAuthHeaderAndPath(t *testing.T) {
	var gotHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/custom/complete" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		gotHeader = r.Header.Get("api-key")
		if auth := r.Header.Get("Authorization"); auth != "" {
			t.Fatalf("expected no default Authorization header, got %q", auth)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "ok",
				},
			}},
		})
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{
		BaseURL:    server.URL,
		APIPath:    "custom/complete",
		APIKey:     "test-key",
		AuthHeader: "api-key",
		AuthScheme: "",
		Model:      "fallback-model",
	})
	msg, err := client.CreateMessage(context.Background(), llm.ChatRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "ok" {
		t.Fatalf("unexpected message content: %#v", msg)
	}
	if gotHeader != "test-key" {
		t.Fatalf("unexpected api-key header %q", gotHeader)
	}
}

func TestOpenAICompatibleCreateMessageRetriesWithoutUnsupportedFields(t *testing.T) {
	bodies := make([]map[string]any, 0, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		bodies = append(bodies, body)

		if _, hasTemperature := body["temperature"]; hasTemperature {
			http.Error(w, `{"error":"unknown field temperature"}`, http.StatusBadRequest)
			return
		}

		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role":    "assistant",
					"content": "retry worked",
				},
			}},
		})
	}))
	defer server.Close()

	client := NewOpenAICompatible(Config{BaseURL: server.URL, APIKey: "test-key", Model: "fallback-model"})
	msg, err := client.CreateMessage(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{{Role: "user", Content: "hello"}},
		Tools: []llm.ToolDefinition{{
			Type: "function",
			Function: llm.FunctionDefinition{
				Name:        "list_files",
				Description: "list files",
				Parameters:  map[string]any{"type": "object"},
			},
		}},
		Temperature: 0.2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if msg.Content != "retry worked" {
		t.Fatalf("unexpected message content: %#v", msg)
	}
	if len(bodies) < 2 {
		t.Fatalf("expected retries, got %d requests", len(bodies))
	}
	if _, hasTemperature := bodies[0]["temperature"]; !hasTemperature {
		t.Fatalf("expected initial request to include temperature, got %#v", bodies[0])
	}
	last := bodies[len(bodies)-1]
	if _, hasTemperature := last["temperature"]; hasTemperature {
		t.Fatalf("expected retried request to remove temperature, got %#v", last)
	}
	if _, hasTools := last["tools"]; !hasTools {
		t.Fatalf("expected retried request to preserve tools when possible, got %#v", last)
	}
}

func TestOpenAICompatibleCreateMessageParsesArrayContentAndObjectArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"role": "assistant",
					"content": []map[string]any{
						{"type": "text", "text": "Hello "},
						{"type": "text", "text": "world"},
					},
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
	if msg.Content != "Hello world" {
		t.Fatalf("unexpected content %q", msg.Content)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Arguments != "{\"path\":\".\"}" {
		t.Fatalf("unexpected tool calls %#v", msg.ToolCalls)
	}
}

func TestOpenAIDefaultPathsByBaseURL(t *testing.T) {
	if got := openAIDefaultPaths("https://api.openai.com/v1"); !slices.Equal(got, []string{"chat/completions"}) {
		t.Fatalf("unexpected paths for /v1 base: %#v", got)
	}
	if got := openAIDefaultPaths("https://example.com"); !slices.Equal(got, []string{"chat/completions", "v1/chat/completions"}) {
		t.Fatalf("unexpected paths for host base: %#v", got)
	}
}
