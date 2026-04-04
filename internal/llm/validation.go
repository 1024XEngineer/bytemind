package llm

import (
	"fmt"
	"strings"
)

var allowedPartTypesByRole = map[Role]map[PartType]struct{}{
	RoleSystem: {
		PartText: {},
	},
	RoleUser: {
		PartText:       {},
		PartImageRef:   {},
		PartToolResult: {},
	},
	RoleAssistant: {
		PartText:     {},
		PartToolUse:  {},
		PartThinking: {},
	},
}

func ValidatePart(part Part) error {
	if strings.TrimSpace(string(part.Type)) == "" {
		return fmt.Errorf("part type is required")
	}

	payloadCount := 0
	if part.Text != nil {
		payloadCount++
	}
	if part.Image != nil {
		payloadCount++
	}
	if part.ToolUse != nil {
		payloadCount++
	}
	if part.ToolResult != nil {
		payloadCount++
	}
	if part.Thinking != nil {
		payloadCount++
	}
	if payloadCount != 1 {
		return fmt.Errorf("part %q must contain exactly one payload", part.Type)
	}

	switch part.Type {
	case PartText:
		if part.Text == nil {
			return fmt.Errorf("part %q payload mismatch", part.Type)
		}
		if strings.TrimSpace(part.Text.Value) == "" {
			return fmt.Errorf("text value is required")
		}
	case PartImageRef:
		if part.Image == nil {
			return fmt.Errorf("part %q payload mismatch", part.Type)
		}
		if strings.TrimSpace(string(part.Image.AssetID)) == "" {
			return fmt.Errorf("image asset_id is required")
		}
	case PartToolUse:
		if part.ToolUse == nil {
			return fmt.Errorf("part %q payload mismatch", part.Type)
		}
		if strings.TrimSpace(part.ToolUse.ID) == "" || strings.TrimSpace(part.ToolUse.Name) == "" {
			return fmt.Errorf("tool_use id and name are required")
		}
	case PartToolResult:
		if part.ToolResult == nil {
			return fmt.Errorf("part %q payload mismatch", part.Type)
		}
		if strings.TrimSpace(part.ToolResult.ToolUseID) == "" {
			return fmt.Errorf("tool_result tool_use_id is required")
		}
	case PartThinking:
		if part.Thinking == nil {
			return fmt.Errorf("part %q payload mismatch", part.Type)
		}
		if strings.TrimSpace(part.Thinking.Value) == "" {
			return fmt.Errorf("thinking value is required")
		}
	default:
		return fmt.Errorf("unsupported part type %q", part.Type)
	}

	return nil
}

func ValidateMessage(message Message) error {
	message.Normalize()

	if _, ok := allowedPartTypesByRole[message.Role]; !ok {
		return fmt.Errorf("%w: %q", errInvalidRole, message.Role)
	}
	if len(message.Parts) == 0 {
		return fmt.Errorf("message content must not be empty")
	}

	allowed := allowedPartTypesByRole[message.Role]
	for idx, part := range message.Parts {
		if err := ValidatePart(part); err != nil {
			return fmt.Errorf("message part[%d]: %w", idx, err)
		}
		if _, ok := allowed[part.Type]; !ok {
			return fmt.Errorf("role %q does not allow part type %q", message.Role, part.Type)
		}
	}

	return nil
}
