package llm

import (
	"encoding/json"
	"testing"
)

func TestMessageMarshalUnmarshalLegacyAndPartsCompatibility(t *testing.T) {
	legacy := Message{
		Role:    RoleAssistant,
		Content: "hello",
		ToolCalls: []ToolCall{{
			ID:   "call-1",
			Type: "function",
			Function: ToolFunctionCall{
				Name:      "list_files",
				Arguments: `{"path":"."}`,
			},
		}},
	}
	data, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	content, ok := decoded["content"].([]any)
	if !ok || len(content) != 2 {
		t.Fatalf("expected content parts in wire json, got %#v", decoded["content"])
	}

	var roundtrip Message
	if err := json.Unmarshal(data, &roundtrip); err != nil {
		t.Fatal(err)
	}
	if roundtrip.Text() != "hello" || len(roundtrip.ToolCalls) != 1 {
		t.Fatalf("unexpected roundtrip message: %#v", roundtrip)
	}
}

func TestMessageUnmarshalSupportsStringContentAndRejectsInvalidFormat(t *testing.T) {
	var fromString Message
	if err := json.Unmarshal([]byte(`{"role":"user","content":"hi"}`), &fromString); err != nil {
		t.Fatal(err)
	}
	if fromString.Text() != "hi" || len(fromString.Parts) != 1 || fromString.Parts[0].Type != PartText {
		t.Fatalf("unexpected decoded string content: %#v", fromString)
	}

	var invalid Message
	if err := json.Unmarshal([]byte(`{"role":"user","content":{}}`), &invalid); err == nil {
		t.Fatal("expected invalid content format error")
	}
}

func TestMessageTextAndNormalizeForToolResult(t *testing.T) {
	msg := NewToolResultMessage("call-1", `{"ok":true}`)
	if msg.Text() != `{"ok":true}` {
		t.Fatalf("unexpected tool text: %q", msg.Text())
	}
	if msg.ToolCallID != "call-1" {
		t.Fatalf("expected tool call id hydrated, got %#v", msg)
	}
}
