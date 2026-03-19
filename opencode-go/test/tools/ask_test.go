package tools_test

import (
	"context"
	"strings"
	"testing"

	"github.com/opencode-go/internal/tools"
)

func TestAskTool_Execute_Confirm(t *testing.T) {
	tool := tools.NewAskTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"type":     "confirm",
		"question": "Are you sure?",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.HasPrefix(result.Output, "[ASK_CONFIRM]") {
		t.Errorf("Expected output to start with [ASK_CONFIRM], got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Are you sure?") {
		t.Errorf("Expected output to contain question, got: %s", result.Output)
	}
}

func TestAskTool_Execute_Confirm_DefaultQuestion(t *testing.T) {
	tool := tools.NewAskTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"type": "confirm",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.HasPrefix(result.Output, "[ASK_CONFIRM]") {
		t.Errorf("Expected output to start with [ASK_CONFIRM], got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Continue?") {
		t.Errorf("Expected default question 'Continue?', got: %s", result.Output)
	}
}

func TestAskTool_Execute_Input(t *testing.T) {
	tool := tools.NewAskTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"type":   "input",
		"prompt": "Enter your name:",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.HasPrefix(result.Output, "[ASK_INPUT]") {
		t.Errorf("Expected output to start with [ASK_INPUT], got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Enter your name:") {
		t.Errorf("Expected output to contain prompt, got: %s", result.Output)
	}
}

func TestAskTool_Execute_Input_DefaultPrompt(t *testing.T) {
	tool := tools.NewAskTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"type": "input",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.HasPrefix(result.Output, "[ASK_INPUT]") {
		t.Errorf("Expected output to start with [ASK_INPUT], got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Please enter input:") {
		t.Errorf("Expected default prompt 'Please enter input:', got: %s", result.Output)
	}
}

func TestAskTool_Execute_InvalidType(t *testing.T) {
	tool := tools.NewAskTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"type": "invalid",
	})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to invalid type")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestAskTool_Execute_MissingType(t *testing.T) {
	tool := tools.NewAskTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to missing type")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestAskTool_Execute_InvalidTypeType(t *testing.T) {
	tool := tools.NewAskTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"type": 123,
	})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to invalid type type")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestAskTool_NameAndDescription(t *testing.T) {
	tool := tools.NewAskTool()
	if tool.Name() != "ask" {
		t.Errorf("Expected name 'ask', got %s", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Expected non-empty description")
	}
}

func TestAskTool_Schema(t *testing.T) {
	tool := tools.NewAskTool()
	schema := tool.Schema()

	if schema == nil {
		t.Fatal("Schema should not be nil")
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Schema should have properties")
	}

	// 检查 type 参数
	typeProp, ok := props["type"].(map[string]interface{})
	if !ok {
		t.Fatal("type property should exist")
	}
	if typeProp["type"] != "string" {
		t.Errorf("type should be 'string', got %v", typeProp["type"])
	}
	enum, ok := typeProp["enum"].([]interface{})
	if !ok {
		t.Fatal("type should have enum")
	}
	if len(enum) != 2 || enum[0] != "confirm" || enum[1] != "input" {
		t.Errorf("type enum should be ['confirm', 'input'], got %v", enum)
	}

	// 检查 question 参数
	questionProp, ok := props["question"].(map[string]interface{})
	if !ok {
		t.Fatal("question property should exist")
	}
	if questionProp["type"] != "string" {
		t.Errorf("question type should be 'string', got %v", questionProp["type"])
	}

	// 检查 prompt 参数
	promptProp, ok := props["prompt"].(map[string]interface{})
	if !ok {
		t.Fatal("prompt property should exist")
	}
	if promptProp["type"] != "string" {
		t.Errorf("prompt type should be 'string', got %v", promptProp["type"])
	}

	// 检查 options 参数
	optionsProp, ok := props["options"].(map[string]interface{})
	if !ok {
		t.Fatal("options property should exist")
	}
	if optionsProp["type"] != "array" {
		t.Errorf("options type should be 'array', got %v", optionsProp["type"])
	}

	// 检查 required 字段
	required := schema["required"]
	if required == nil {
		t.Fatal("Schema should have required field")
	}

	var requiredStrs []string
	switch v := required.(type) {
	case []interface{}:
		for _, item := range v {
			if str, ok := item.(string); ok {
				requiredStrs = append(requiredStrs, str)
			}
		}
	case []string:
		requiredStrs = v
	default:
		t.Fatalf("required field has unexpected type: %T", v)
	}

	if len(requiredStrs) != 1 || requiredStrs[0] != "type" {
		t.Errorf("Required should contain 'type', got %v", requiredStrs)
	}
}
