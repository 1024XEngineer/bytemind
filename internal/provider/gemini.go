package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

type Gemini struct {
	baseURL    string
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewGemini(cfg Config) *Gemini {
	return &Gemini{
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
		apiKey:  cfg.APIKey,
		model:   cfg.Model,
		httpClient: &http.Client{
			Timeout: 2 * time.Minute,
		},
	}
}

func (c *Gemini) CreateMessage(ctx context.Context, req llm.ChatRequest) (llm.Message, error) {
	payload, err := geminiPayload(req)
	if err != nil {
		return llm.Message{}, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return llm.Message{}, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.generateContentURL(choose(req.Model, c.model)), bytes.NewReader(body))
	if err != nil {
		return llm.Message{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-goog-api-key", c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return llm.Message{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return llm.Message{}, err
	}
	if resp.StatusCode >= 300 {
		return llm.Message{}, llm.MapProviderError("gemini", resp.StatusCode, string(respBody), nil)
	}
	return parseGeminiMessage(respBody)
}

func (c *Gemini) StreamMessage(ctx context.Context, req llm.ChatRequest, onDelta func(string)) (llm.Message, error) {
	message, err := c.CreateMessage(ctx, req)
	if err != nil {
		return llm.Message{}, err
	}
	if onDelta != nil && message.Text() != "" {
		onDelta(message.Text())
	}
	return message, nil
}

func (c *Gemini) generateContentURL(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		model = strings.TrimSpace(c.model)
	}
	if model == "" {
		model = "gemini-2.5-flash"
	}
	if strings.HasPrefix(model, "models/") || strings.HasPrefix(model, "tunedModels/") {
		return c.baseURL + "/" + model + ":generateContent"
	}
	return c.baseURL + "/models/" + model + ":generateContent"
}

func geminiPayload(req llm.ChatRequest) (map[string]any, error) {
	systemParts := make([]map[string]any, 0)
	contents := make([]map[string]any, 0, len(req.Messages))
	toolNamesByID := make(map[string]string)

	for _, message := range req.Messages {
		message.Normalize()
		if message.Role == llm.RoleSystem {
			systemParts = append(systemParts, geminiTextParts(message)...)
			continue
		}

		parts := geminiParts(message, req.Assets, toolNamesByID)
		if len(parts) == 0 {
			continue
		}
		role := "user"
		if message.Role == llm.RoleAssistant {
			role = "model"
		}
		contents = append(contents, map[string]any{
			"role":  role,
			"parts": parts,
		})
	}

	payload := map[string]any{
		"contents": contents,
		"generationConfig": map[string]any{
			"temperature": req.Temperature,
		},
	}
	if len(systemParts) > 0 {
		payload["systemInstruction"] = map[string]any{"parts": systemParts}
	}
	if tools := geminiTools(req.Tools); len(tools) > 0 {
		payload["tools"] = tools
	}
	return payload, nil
}

func geminiTextParts(message llm.Message) []map[string]any {
	parts := make([]map[string]any, 0, len(message.Parts))
	for _, part := range message.Parts {
		switch {
		case part.Text != nil && strings.TrimSpace(part.Text.Value) != "":
			parts = append(parts, map[string]any{"text": part.Text.Value})
		case part.Thinking != nil && strings.TrimSpace(part.Thinking.Value) != "":
			parts = append(parts, map[string]any{"text": part.Thinking.Value})
		}
	}
	return parts
}

func geminiParts(message llm.Message, assets map[llm.AssetID]llm.ImageAsset, toolNamesByID map[string]string) []map[string]any {
	parts := make([]map[string]any, 0, len(message.Parts))
	for _, part := range message.Parts {
		switch part.Type {
		case llm.PartText:
			if part.Text != nil && strings.TrimSpace(part.Text.Value) != "" {
				parts = append(parts, map[string]any{"text": part.Text.Value})
			}
		case llm.PartThinking:
			if part.Thinking != nil && strings.TrimSpace(part.Thinking.Value) != "" {
				parts = append(parts, map[string]any{"text": part.Thinking.Value})
			}
		case llm.PartImageRef:
			assetID := llm.AssetID("")
			if part.Image != nil {
				assetID = part.Image.AssetID
			}
			asset, ok := assets[assetID]
			if !ok || len(asset.Data) == 0 {
				parts = append(parts, map[string]any{"text": missingImageAssetFallback(assetID)})
				continue
			}
			mediaType := strings.TrimSpace(asset.MediaType)
			if mediaType == "" {
				mediaType = "image/png"
			}
			parts = append(parts, map[string]any{
				"inline_data": map[string]any{
					"mime_type": mediaType,
					"data":      base64.StdEncoding.EncodeToString(asset.Data),
				},
			})
		case llm.PartToolUse:
			if part.ToolUse == nil || strings.TrimSpace(part.ToolUse.Name) == "" {
				continue
			}
			if strings.TrimSpace(part.ToolUse.ID) != "" {
				toolNamesByID[part.ToolUse.ID] = part.ToolUse.Name
			}
			parts = append(parts, map[string]any{
				"functionCall": map[string]any{
					"name": part.ToolUse.Name,
					"args": parseJSONObject(part.ToolUse.Arguments),
				},
			})
		case llm.PartToolResult:
			if part.ToolResult == nil {
				continue
			}
			name := toolNamesByID[part.ToolResult.ToolUseID]
			if strings.TrimSpace(name) == "" {
				name = part.ToolResult.ToolUseID
			}
			if strings.TrimSpace(name) == "" {
				name = "tool_result"
			}
			parts = append(parts, map[string]any{
				"functionResponse": map[string]any{
					"name":     name,
					"response": geminiFunctionResponse(part.ToolResult.Content),
				},
			})
		}
	}
	return parts
}

func geminiFunctionResponse(content string) map[string]any {
	content = strings.TrimSpace(content)
	if content == "" {
		return map[string]any{"content": ""}
	}
	var value any
	if err := json.Unmarshal([]byte(content), &value); err == nil {
		if obj, ok := value.(map[string]any); ok {
			return obj
		}
		return map[string]any{"result": value}
	}
	return map[string]any{"content": content}
}

func geminiTools(tools []llm.ToolDefinition) []map[string]any {
	if len(tools) == 0 {
		return nil
	}
	declarations := make([]map[string]any, 0, len(tools))
	for _, tool := range tools {
		if strings.TrimSpace(tool.Function.Name) == "" {
			continue
		}
		declaration := map[string]any{
			"name":        tool.Function.Name,
			"description": tool.Function.Description,
		}
		if tool.Function.Parameters != nil {
			declaration["parameters"] = tool.Function.Parameters
		}
		declarations = append(declarations, declaration)
	}
	if len(declarations) == 0 {
		return nil
	}
	return []map[string]any{{"functionDeclarations": declarations}}
}

type geminiFunctionCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type geminiPart struct {
	Text              string              `json:"text"`
	FunctionCall      *geminiFunctionCall `json:"functionCall"`
	FunctionCallSnake *geminiFunctionCall `json:"function_call"`
}

func (p geminiPart) functionCall() *geminiFunctionCall {
	if p.FunctionCall != nil {
		return p.FunctionCall
	}
	return p.FunctionCallSnake
}

func parseGeminiMessage(respBody []byte) (llm.Message, error) {
	var completion struct {
		Candidates []struct {
			Content struct {
				Parts []geminiPart `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		UsageMetadata struct {
			PromptTokenCount     int `json:"promptTokenCount"`
			CandidatesTokenCount int `json:"candidatesTokenCount"`
			TotalTokenCount      int `json:"totalTokenCount"`
		} `json:"usageMetadata"`
	}
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return llm.Message{}, err
	}
	if len(completion.Candidates) == 0 {
		return llm.Message{}, fmt.Errorf("provider returned no candidates")
	}

	message := llm.Message{Role: llm.RoleAssistant}
	for idx, part := range completion.Candidates[0].Content.Parts {
		if part.Text != "" {
			message.AppendPart(llm.Part{Type: llm.PartText, Text: &llm.TextPart{Value: part.Text}})
		}
		if call := part.functionCall(); call != nil && strings.TrimSpace(call.Name) != "" {
			arguments := "{}"
			if len(call.Args) > 0 {
				arguments = string(call.Args)
			}
			message.AppendPart(llm.Part{
				Type: llm.PartToolUse,
				ToolUse: &llm.ToolUsePart{
					ID:        fmt.Sprintf("call-%d", idx),
					Name:      call.Name,
					Arguments: arguments,
				},
			})
		}
	}
	message.Normalize()

	inputTokens := max(0, completion.UsageMetadata.PromptTokenCount)
	outputTokens := max(0, completion.UsageMetadata.CandidatesTokenCount)
	totalTokens := max(0, completion.UsageMetadata.TotalTokenCount)
	if totalTokens == 0 {
		totalTokens = inputTokens + outputTokens
	}
	if totalTokens > 0 {
		message.Usage = &llm.Usage{
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			TotalTokens:  totalTokens,
		}
	}
	return message, nil
}
