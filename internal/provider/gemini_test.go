package provider

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func TestGeminiCreateMessageUsesNativeHeaderAndPayload(t *testing.T) {
	var apiKeyHeader string
	var requestPath string
	var requestBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		apiKeyHeader = r.Header.Get("x-goog-api-key")
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]any{
						{"text": "done"},
						{"functionCall": map[string]any{"name": "list_files", "args": map[string]any{"path": "."}}},
					},
				},
			}},
			"usageMetadata": map[string]any{
				"promptTokenCount":     12,
				"candidatesTokenCount": 5,
				"totalTokenCount":      17,
			},
		})
	}))
	defer server.Close()

	client := NewGemini(Config{
		BaseURL: server.URL,
		APIKey:  "gem-key",
		Model:   "gemini-default",
	})

	msg, err := client.CreateMessage(context.Background(), llm.ChatRequest{
		Model: "gemini-2.5-flash",
		Messages: []llm.Message{
			{Role: llm.RoleSystem, Content: "follow rules"},
			{Role: llm.RoleUser, Content: "inspect repo"},
		},
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
	if requestPath != "/models/gemini-2.5-flash:generateContent" {
		t.Fatalf("unexpected path %q", requestPath)
	}
	if apiKeyHeader != "gem-key" {
		t.Fatalf("unexpected api key header %q", apiKeyHeader)
	}
	if msg.Content != "done" {
		t.Fatalf("unexpected content %#v", msg)
	}
	if len(msg.ToolCalls) != 1 || msg.ToolCalls[0].Function.Name != "list_files" || !strings.Contains(msg.ToolCalls[0].Function.Arguments, `"path"`) {
		t.Fatalf("unexpected tool calls %#v", msg.ToolCalls)
	}
	if msg.Usage == nil || msg.Usage.InputTokens != 12 || msg.Usage.OutputTokens != 5 || msg.Usage.TotalTokens != 17 {
		t.Fatalf("unexpected usage %#v", msg.Usage)
	}

	systemInstruction, _ := requestBody["systemInstruction"].(map[string]any)
	systemParts, _ := systemInstruction["parts"].([]any)
	if len(systemParts) != 1 || systemParts[0].(map[string]any)["text"] != "follow rules" {
		t.Fatalf("unexpected system instruction %#v", requestBody["systemInstruction"])
	}
	contents, _ := requestBody["contents"].([]any)
	if len(contents) != 1 || contents[0].(map[string]any)["role"] != "user" {
		t.Fatalf("unexpected contents %#v", requestBody["contents"])
	}
	tools, _ := requestBody["tools"].([]any)
	if len(tools) != 1 {
		t.Fatalf("expected gemini tool declarations, got %#v", requestBody["tools"])
	}
}

func TestGeminiCreateMessageConvertsImagesAndToolResults(t *testing.T) {
	var requestBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&requestBody); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]any{{"text": "ok"}},
				},
			}},
		})
	}))
	defer server.Close()

	client := NewGemini(Config{BaseURL: server.URL, APIKey: "gem-key", Model: "gemini-2.5-flash"})
	_, err := client.CreateMessage(context.Background(), llm.ChatRequest{
		Messages: []llm.Message{
			{
				Role: llm.RoleUser,
				Parts: []llm.Part{
					{Type: llm.PartText, Text: &llm.TextPart{Value: "describe"}},
					{Type: llm.PartImageRef, Image: &llm.ImagePartRef{AssetID: "img-1"}},
				},
			},
			{
				Role:  llm.RoleAssistant,
				Parts: []llm.Part{{Type: llm.PartToolUse, ToolUse: &llm.ToolUsePart{ID: "call-1", Name: "read_file", Arguments: `{"path":"README.md"}`}}},
			},
			llm.NewToolResultMessage("call-1", `{"content":"done"}`),
		},
		Assets: map[llm.AssetID]llm.ImageAsset{
			"img-1": {MediaType: "image/png", Data: []byte("image-bytes")},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	contents := requestBody["contents"].([]any)
	userParts := contents[0].(map[string]any)["parts"].([]any)
	inlineData := userParts[1].(map[string]any)["inline_data"].(map[string]any)
	if inlineData["mime_type"] != "image/png" || inlineData["data"] != base64.StdEncoding.EncodeToString([]byte("image-bytes")) {
		t.Fatalf("unexpected inline image payload %#v", inlineData)
	}
	assistantParts := contents[1].(map[string]any)["parts"].([]any)
	if assistantParts[0].(map[string]any)["functionCall"].(map[string]any)["name"] != "read_file" {
		t.Fatalf("unexpected function call %#v", assistantParts[0])
	}
	toolResultParts := contents[2].(map[string]any)["parts"].([]any)
	functionResponse := toolResultParts[0].(map[string]any)["functionResponse"].(map[string]any)
	if functionResponse["name"] != "read_file" {
		t.Fatalf("unexpected function response %#v", functionResponse)
	}
}

func TestGeminiStreamMessageInvokesDelta(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"candidates": []map[string]any{{
				"content": map[string]any{
					"parts": []map[string]any{{"text": "hello from gemini"}},
				},
			}},
		})
	}))
	defer server.Close()

	client := NewGemini(Config{BaseURL: server.URL, APIKey: "gem-key", Model: "gemini-2.5-flash"})
	var gotDelta string
	msg, err := client.StreamMessage(context.Background(), llm.ChatRequest{}, func(delta string) {
		gotDelta += delta
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotDelta != "hello from gemini" || msg.Content != gotDelta {
		t.Fatalf("unexpected stream result delta=%q msg=%#v", gotDelta, msg)
	}
}

func TestGeminiCreateMessageReturnsProviderError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"bad key"}`))
	}))
	defer server.Close()

	client := NewGemini(Config{BaseURL: server.URL, APIKey: "bad-key", Model: "gemini-2.5-flash"})
	_, err := client.CreateMessage(context.Background(), llm.ChatRequest{})
	if err == nil {
		t.Fatal("expected provider error")
	}
	var providerErr *llm.ProviderError
	if !strings.Contains(err.Error(), "bad key") || !errors.As(err, &providerErr) || providerErr.Provider != "gemini" {
		t.Fatalf("unexpected error %T %v", err, err)
	}
}
