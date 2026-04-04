package llm

import "testing"

func TestValidatePartRequiresOnePayload(t *testing.T) {
	part := Part{Type: PartText}
	if err := ValidatePart(part); err == nil {
		t.Fatal("expected one-of validation error")
	}
}

func TestValidateMessageRejectsInvalidRolePartCombination(t *testing.T) {
	msg := Message{
		Role: RoleSystem,
		Parts: []Part{{
			Type:    PartToolUse,
			ToolUse: &ToolUsePart{ID: "call-1", Name: "x", Arguments: "{}"},
		}},
	}
	if err := ValidateMessage(msg); err == nil {
		t.Fatal("expected role-part validation error")
	}
}

func TestValidateMessageAllowsUserToolResult(t *testing.T) {
	msg := NewToolResultMessage("call-1", `{"ok":true}`)
	if err := ValidateMessage(msg); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidatePartRejectsPayloadMismatchAndMissingFields(t *testing.T) {
	if err := ValidatePart(Part{
		Type: PartImageRef,
		Text: &TextPart{Value: "oops"},
	}); err == nil {
		t.Fatal("expected payload mismatch error")
	}

	if err := ValidatePart(Part{
		Type:  PartToolUse,
		Image: &ImagePartRef{AssetID: "asset-1"},
	}); err == nil {
		t.Fatal("expected tool_use payload mismatch error")
	}

	if err := ValidatePart(Part{
		Type:       PartToolResult,
		ToolResult: &ToolResultPart{Content: "x"},
	}); err == nil {
		t.Fatal("expected missing tool_use_id error")
	}
}

func TestValidateMessageRejectsUnknownRole(t *testing.T) {
	err := ValidateMessage(Message{
		Role: "hacker",
		Parts: []Part{{
			Type: PartText,
			Text: &TextPart{Value: "x"},
		}},
	})
	if err == nil || !IsInvalidRole(err) {
		t.Fatalf("expected invalid role error, got %v", err)
	}
}

func TestValidateMessageRejectsAssistantImageRef(t *testing.T) {
	err := ValidateMessage(Message{
		Role: RoleAssistant,
		Parts: []Part{{
			Type:  PartImageRef,
			Image: &ImagePartRef{AssetID: "asset-1"},
		}},
	})
	if err == nil {
		t.Fatal("expected assistant image_ref to be rejected")
	}
}
