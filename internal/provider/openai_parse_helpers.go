package provider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/1024XEngineer/bytemind/internal/llm"
)

func parseOpenAIUsage(raw json.RawMessage) *llm.Usage {
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil
	}
	var usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
		PromptDetails    struct {
			CachedTokens int `json:"cached_tokens"`
			AudioTokens  int `json:"audio_tokens"`
		} `json:"prompt_tokens_details"`
		CompletionDetails struct {
			AudioTokens int `json:"audio_tokens"`
		} `json:"completion_tokens_details"`
	}
	if err := json.Unmarshal(raw, &usage); err != nil {
		return nil
	}
	input := max(0, usage.PromptTokens) + max(0, usage.PromptDetails.AudioTokens)
	output := max(0, usage.CompletionTokens) + max(0, usage.CompletionDetails.AudioTokens)
	context := max(0, usage.PromptDetails.CachedTokens)
	total := usage.TotalTokens
	if total <= 0 {
		total = input + output + context
	}
	if input == 0 && output == 0 && context == 0 && total == 0 {
		return nil
	}
	return &llm.Usage{InputTokens: input, OutputTokens: output, ContextTokens: context, TotalTokens: max(0, total)}
}

type streamDelta struct {
	Role      llm.Role
	Content   string
	Reasoning string
	ToolCalls []streamToolCallDelta
}

type streamToolCallDelta struct {
	Index             int
	ID                string
	Type              string
	FunctionName      string
	FunctionArguments string
}

func parseOpenAIDelta(raw json.RawMessage) (streamDelta, error) {
	delta := streamDelta{}
	if len(bytes.TrimSpace(raw)) == 0 {
		return delta, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return streamDelta{}, err
	}
	if roleRaw, ok := obj["role"]; ok {
		var role string
		_ = json.Unmarshal(roleRaw, &role)
		delta.Role = llm.Role(strings.TrimSpace(role))
	}
	if contentRaw, ok := obj["content"]; ok {
		delta.Content = extractTextFromRaw(contentRaw, false)
	}
	if delta.Content == "" {
		if outputRaw, ok := obj["output_text"]; ok {
			delta.Content = extractTextFromRaw(outputRaw, false)
		}
	}
	if reasoningRaw, ok := obj["reasoning_content"]; ok {
		_ = json.Unmarshal(reasoningRaw, &delta.Reasoning)
	}
	if toolCallsRaw, ok := obj["tool_calls"]; ok {
		delta.ToolCalls = append(delta.ToolCalls, parseStreamToolCalls(toolCallsRaw)...)
	}
	if functionCallRaw, ok := obj["function_call"]; ok {
		legacy := parseLegacyFunctionCall(functionCallRaw)
		if legacy.FunctionName != "" || legacy.FunctionArguments != "" {
			legacy.Index = legacyToolCallIndex
			delta.ToolCalls = append(delta.ToolCalls, legacy)
		}
	}
	return delta, nil
}

func parseOpenAIMessage(raw json.RawMessage) llm.Message {
	msg := llm.Message{Role: llm.RoleAssistant}
	if len(bytes.TrimSpace(raw)) == 0 {
		return msg
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return msg
	}
	if roleRaw, ok := obj["role"]; ok {
		var role string
		_ = json.Unmarshal(roleRaw, &role)
		if strings.TrimSpace(role) != "" {
			msg.Role = llm.Role(role)
		}
	}
	if contentRaw, ok := obj["content"]; ok {
		msg.Content = extractTextFromRaw(contentRaw, false)
	}
	if msg.Content == "" {
		if outputRaw, ok := obj["output_text"]; ok {
			msg.Content = extractTextFromRaw(outputRaw, false)
		}
	}
	if toolCallsRaw, ok := obj["tool_calls"]; ok {
		msg.ToolCalls = parseToolCalls(toolCallsRaw)
	}
	if len(msg.ToolCalls) == 0 {
		if functionCallRaw, ok := obj["function_call"]; ok {
			legacy := parseLegacyFunctionCall(functionCallRaw)
			if legacy.FunctionName != "" {
				msg.ToolCalls = []llm.ToolCall{{ID: "call-legacy", Type: "function", Function: llm.ToolFunctionCall{Name: legacy.FunctionName, Arguments: legacy.FunctionArguments}}}
			}
		}
	}
	msg.Normalize()
	return msg
}

func parseToolCalls(raw json.RawMessage) []llm.ToolCall {
	var calls []struct {
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil
	}
	out := make([]llm.ToolCall, 0, len(calls))
	for i, call := range calls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = fmt.Sprintf("call-%d", i)
		}
		typ := strings.TrimSpace(call.Type)
		if typ == "" {
			typ = "function"
		}
		out = append(out, llm.ToolCall{ID: id, Type: typ, Function: llm.ToolFunctionCall{Name: call.Function.Name, Arguments: argumentString(call.Function.Arguments)}})
	}
	return out
}

func parseStreamToolCalls(raw json.RawMessage) []streamToolCallDelta {
	var calls []struct {
		Index    int    `json:"index"`
		ID       string `json:"id"`
		Type     string `json:"type"`
		Function struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal(raw, &calls); err != nil {
		return nil
	}
	out := make([]streamToolCallDelta, 0, len(calls))
	for _, call := range calls {
		out = append(out, streamToolCallDelta{Index: call.Index, ID: call.ID, Type: call.Type, FunctionName: call.Function.Name, FunctionArguments: argumentString(call.Function.Arguments)})
	}
	return out
}

func parseLegacyFunctionCall(raw json.RawMessage) streamToolCallDelta {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return streamToolCallDelta{}
	}
	return streamToolCallDelta{Type: "function", FunctionName: call.Name, FunctionArguments: argumentString(call.Arguments)}
}

func argumentString(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	trimmed := strings.TrimSpace(string(raw))
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		var compact bytes.Buffer
		if err := json.Compact(&compact, raw); err == nil {
			return compact.String()
		}
	}
	return trimmed
}

func extractTextFromRaw(raw json.RawMessage, includeReasoning bool) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return ""
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return text
	}
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return ""
	}
	return extractTextFromAny(value, includeReasoning)
}

func extractTextFromAny(value any, includeReasoning bool) string {
	switch typed := value.(type) {
	case string:
		return typed
	case []any:
		var b strings.Builder
		for _, item := range typed {
			b.WriteString(extractTextFromAny(item, includeReasoning))
		}
		return b.String()
	case map[string]any:
		var b strings.Builder
		for _, key := range []string{"text", "output_text", "content", "value"} {
			if nested, ok := typed[key]; ok {
				b.WriteString(extractTextFromAny(nested, includeReasoning))
			}
		}
		if includeReasoning {
			for _, key := range []string{"reasoning_content", "reasoning", "summary"} {
				if nested, ok := typed[key]; ok {
					b.WriteString(extractTextFromAny(nested, includeReasoning))
				}
			}
		}
		if b.Len() > 0 {
			return b.String()
		}
		for _, key := range []string{"message", "delta", "part", "parts"} {
			if nested, ok := typed[key]; ok {
				b.WriteString(extractTextFromAny(nested, includeReasoning))
			}
		}
		return b.String()
	default:
		return ""
	}
}
