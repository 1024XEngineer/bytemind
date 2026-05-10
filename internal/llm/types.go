package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
)

type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

type PartType string

const (
	PartText       PartType = "text"
	PartImageRef   PartType = "image_ref"
	PartToolUse    PartType = "tool_use"
	PartToolResult PartType = "tool_result"
	PartThinking   PartType = "thinking"
)

type AssetID string

type MessageMeta map[string]any

type Message struct {
	ID        string      `json:"id,omitempty"`
	Role      Role        `json:"role"`
	Parts     []Part      `json:"content,omitempty"`
	CreatedAt string      `json:"created_at,omitempty"`
	Meta      MessageMeta `json:"meta,omitempty"`
	Usage     *Usage      `json:"usage,omitempty"`

	// Legacy compatibility fields used by existing runner/tui/provider paths.
	Content    string     `json:"-"`
	Name       string     `json:"-"`
	ToolCallID string     `json:"-"`
	ToolCalls  []ToolCall `json:"-"`
}

// one-of: exactly one payload should be populated.
type Part struct {
	Type       PartType        `json:"type"`
	Text       *TextPart       `json:"text,omitempty"`
	Image      *ImagePartRef   `json:"image,omitempty"`
	ToolUse    *ToolUsePart    `json:"tool_use,omitempty"`
	ToolResult *ToolResultPart `json:"tool_result,omitempty"`
	Thinking   *ThinkingPart   `json:"thinking,omitempty"`
}

type TextPart struct {
	Value string `json:"value"`
}

type ImagePartRef struct {
	AssetID AssetID `json:"asset_id"`
}

type ToolUsePart struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type ToolResultPart struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
	IsError   bool   `json:"is_error,omitempty"`
}

type ThinkingPart struct {
	Value string `json:"value"`
}

func NewTextMessage(role Role, text string) Message {
	m := Message{Role: role}
	m.AppendText(text)
	return m
}

func NewUserTextMessage(text string) Message {
	return NewTextMessage(RoleUser, text)
}

func NewAssistantTextMessage(text string) Message {
	return NewTextMessage(RoleAssistant, text)
}

func NewToolResultMessage(toolUseID, content string) Message {
	m := Message{Role: RoleUser}
	m.AppendPart(Part{
		Type: PartToolResult,
		ToolResult: &ToolResultPart{
			ToolUseID: toolUseID,
			Content:   content,
		},
	})
	return m
}

func (m *Message) AppendText(value string) {
	if m == nil {
		return
	}
	m.AppendPart(Part{Type: PartText, Text: &TextPart{Value: value}})
}

func (m *Message) AppendPart(part Part) {
	if m == nil {
		return
	}
	m.Parts = append(m.Parts, part)
	m.hydrateLegacyFromParts()
}

func (m Message) Text() string {
	if strings.TrimSpace(m.Content) != "" {
		return m.Content
	}
	if len(m.Parts) == 0 {
		return ""
	}
	parts := make([]string, 0, len(m.Parts))
	for _, part := range m.Parts {
		switch {
		case part.Text != nil:
			parts = append(parts, part.Text.Value)
		case part.Thinking != nil:
			parts = append(parts, part.Thinking.Value)
		case part.ToolResult != nil:
			parts = append(parts, part.ToolResult.Content)
		}
	}
	return strings.Join(parts, "")
}

func (m *Message) Normalize() {
	if m == nil {
		return
	}
	if len(m.Parts) == 0 {
		m.Parts = legacyParts(*m)
	}
	m.hydrateLegacyFromParts()
}

func (m *Message) hydrateLegacyFromParts() {
	if m == nil {
		return
	}
	if len(m.Parts) == 0 {
		return
	}

	textParts := make([]string, 0, len(m.Parts))
	toolCalls := make([]ToolCall, 0, len(m.Parts))
	toolResultText := ""
	toolResultID := ""

	for _, part := range m.Parts {
		switch {
		case part.Text != nil:
			textParts = append(textParts, part.Text.Value)
		case part.Thinking != nil:
			textParts = append(textParts, part.Thinking.Value)
		case part.ToolUse != nil:
			toolCalls = append(toolCalls, ToolCall{
				ID:   part.ToolUse.ID,
				Type: "function",
				Function: ToolFunctionCall{
					Name:      part.ToolUse.Name,
					Arguments: part.ToolUse.Arguments,
				},
			})
		case part.ToolResult != nil:
			toolResultID = part.ToolResult.ToolUseID
			toolResultText += part.ToolResult.Content
		}
	}

	if len(textParts) > 0 {
		m.Content = strings.Join(textParts, "")
	} else if toolResultText != "" {
		m.Content = toolResultText
	}
	if len(toolCalls) > 0 {
		m.ToolCalls = toolCalls
	}
	if toolResultID != "" {
		m.ToolCallID = toolResultID
	}
}

func legacyParts(m Message) []Part {
	parts := make([]Part, 0, 2+len(m.ToolCalls))
	if m.Content != "" {
		parts = append(parts, Part{Type: PartText, Text: &TextPart{Value: m.Content}})
	}
	for _, call := range m.ToolCalls {
		parts = append(parts, Part{
			Type: PartToolUse,
			ToolUse: &ToolUsePart{
				ID:        call.ID,
				Name:      call.Function.Name,
				Arguments: call.Function.Arguments,
			},
		})
	}
	if m.ToolCallID != "" {
		parts = append(parts, Part{
			Type: PartToolResult,
			ToolResult: &ToolResultPart{
				ToolUseID: m.ToolCallID,
				Content:   m.Content,
			},
		})
	}
	return parts
}

func (m Message) MarshalJSON() ([]byte, error) {
	parts := m.Parts
	if len(parts) == 0 {
		parts = legacyParts(m)
	}
	wire := struct {
		ID        string      `json:"id,omitempty"`
		Role      Role        `json:"role"`
		Content   []Part      `json:"content,omitempty"`
		CreatedAt string      `json:"created_at,omitempty"`
		Meta      MessageMeta `json:"meta,omitempty"`
		Usage     *Usage      `json:"usage,omitempty"`
	}{
		ID:        m.ID,
		Role:      m.Role,
		Content:   parts,
		CreatedAt: m.CreatedAt,
		Meta:      m.Meta,
		Usage:     m.Usage,
	}
	return json.Marshal(wire)
}

func (m *Message) UnmarshalJSON(data []byte) error {
	type wire struct {
		ID        string          `json:"id"`
		Role      Role            `json:"role"`
		Content   json.RawMessage `json:"content"`
		CreatedAt string          `json:"created_at"`
		Meta      MessageMeta     `json:"meta"`
		Usage     *Usage          `json:"usage"`

		ToolCallID string     `json:"tool_call_id"`
		ToolCalls  []ToolCall `json:"tool_calls"`
	}
	var raw wire
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	m.ID = raw.ID
	m.Role = raw.Role
	m.CreatedAt = raw.CreatedAt
	m.Meta = raw.Meta
	m.Usage = raw.Usage
	m.ToolCalls = raw.ToolCalls
	m.ToolCallID = raw.ToolCallID
	m.Content = ""
	m.Parts = nil

	if len(bytes.TrimSpace(raw.Content)) > 0 {
		content := strings.TrimSpace(string(raw.Content))
		switch {
		case strings.HasPrefix(content, "\""):
			var text string
			if err := json.Unmarshal(raw.Content, &text); err != nil {
				return err
			}
			m.Content = text
			m.Parts = []Part{{Type: PartText, Text: &TextPart{Value: text}}}
		case strings.HasPrefix(content, "["):
			if err := json.Unmarshal(raw.Content, &m.Parts); err != nil {
				return err
			}
		default:
			return errors.New("invalid message content format")
		}
	}

	if len(m.Parts) == 0 {
		m.Parts = legacyParts(*m)
	}
	m.hydrateLegacyFromParts()
	return nil
}

type ToolCall struct {
	ID       string           `json:"id"`
	Type     string           `json:"type"`
	Function ToolFunctionCall `json:"function"`
}

type ToolFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type Usage struct {
	InputTokens   int `json:"input_tokens,omitempty"`
	OutputTokens  int `json:"output_tokens,omitempty"`
	ContextTokens int `json:"context_tokens,omitempty"`
	TotalTokens   int `json:"total_tokens,omitempty"`
}

type ToolDefinition struct {
	Type     string             `json:"type"`
	Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type ImageAsset struct {
	MediaType string
	Data      []byte
}

type ChatRequest struct {
	Model       string
	Messages    []Message
	Tools       []ToolDefinition
	Assets      map[AssetID]ImageAsset
	Temperature float64
}

type Client interface {
	CreateMessage(ctx context.Context, req ChatRequest) (Message, error)
	StreamMessage(ctx context.Context, req ChatRequest, onDelta func(string)) (Message, error)
}
