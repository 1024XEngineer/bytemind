package tools_test

import (
	"context"
	"os"
	"testing"

	"github.com/opencode-go/internal/tools"
)

func TestShellTool_Execute_Success(t *testing.T) {
	tool := tools.NewShellTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "echo test",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if result.Output == "" {
		t.Error("Expected output from echo command")
	}
}

func TestShellTool_Execute_WithWorkdir(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	tool := tools.NewShellTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "echo hello",
		"workdir": tmpdir,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
}

func TestShellTool_Execute_MissingCommand(t *testing.T) {
	tool := tools.NewShellTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to missing command")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestShellTool_Execute_EmptyCommand(t *testing.T) {
	tool := tools.NewShellTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "",
	})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to empty command")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestShellTool_Execute_InvalidCommandType(t *testing.T) {
	tool := tools.NewShellTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": 123,
	})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to invalid command type")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestShellTool_Execute_WorkdirNotFound(t *testing.T) {
	tool := tools.NewShellTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "echo test",
		"workdir": "/nonexistent/directory/123456",
	})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to nonexistent workdir")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestShellTool_NameAndDescription(t *testing.T) {
	tool := tools.NewShellTool()
	if tool.Name() != "shell_execute" {
		t.Errorf("Expected name 'shell_execute', got %s", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Expected non-empty description")
	}
}

func TestShellTool_Schema(t *testing.T) {
	tool := tools.NewShellTool()
	schema := tool.Schema()

	if schema == nil {
		t.Fatal("Schema should not be nil")
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Schema should have properties")
	}

	// 检查 command 参数
	cmdProp, ok := props["command"].(map[string]interface{})
	if !ok {
		t.Fatal("command property should exist")
	}
	if cmdProp["type"] != "string" {
		t.Errorf("command type should be 'string', got %v", cmdProp["type"])
	}

	// 检查 workdir 参数
	workdirProp, ok := props["workdir"].(map[string]interface{})
	if !ok {
		t.Fatal("workdir property should exist")
	}
	if workdirProp["type"] != "string" {
		t.Errorf("workdir type should be 'string', got %v", workdirProp["type"])
	}

	// 检查 timeout 参数
	timeoutProp, ok := props["timeout"].(map[string]interface{})
	if !ok {
		t.Fatal("timeout property should exist")
	}
	if timeoutProp["type"] != "integer" {
		t.Errorf("timeout type should be 'integer', got %v", timeoutProp["type"])
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

	if len(requiredStrs) != 1 || requiredStrs[0] != "command" {
		t.Errorf("Required should contain 'command', got %v", requiredStrs)
	}
}
