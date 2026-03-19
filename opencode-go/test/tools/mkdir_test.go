package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencode-go/internal/tools"
)

func TestMkdirTool_Execute_Success(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	dirPath := filepath.Join(tmpdir, "newdir")
	tool := tools.NewMkdirTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": dirPath,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Created directory:") {
		t.Errorf("Expected success message, got: %s", result.Output)
	}

	// 验证目录确实被创建
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		t.Error("Directory was not created")
	}
}

func TestMkdirTool_Execute_WithParents(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	dirPath := filepath.Join(tmpdir, "parent", "child", "grandchild")
	tool := tools.NewMkdirTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    dirPath,
		"parents": true,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}

	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		t.Error("Directory was not created")
	}
}

func TestMkdirTool_Execute_WithoutParents(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	dirPath := filepath.Join(tmpdir, "parent", "child")
	tool := tools.NewMkdirTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    dirPath,
		"parents": false,
	})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to missing parent directories")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestMkdirTool_Execute_MissingPath(t *testing.T) {
	tool := tools.NewMkdirTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to missing path")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestMkdirTool_Execute_EmptyPath(t *testing.T) {
	tool := tools.NewMkdirTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "",
	})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to empty path")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestMkdirTool_Execute_InvalidPathType(t *testing.T) {
	tool := tools.NewMkdirTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": 123,
	})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to invalid path type")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestMkdirTool_Execute_InvalidParentsType(t *testing.T) {
	tool := tools.NewMkdirTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    "/tmp/test",
		"parents": "yes",
	})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if !result.Success {
		t.Error("Expected success with default parents=true")
	}
}

func TestMkdirTool_NameAndDescription(t *testing.T) {
	tool := tools.NewMkdirTool()
	if tool.Name() != "mkdir" {
		t.Errorf("Expected name 'mkdir', got %s", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Expected non-empty description")
	}
}

func TestMkdirTool_Schema(t *testing.T) {
	tool := tools.NewMkdirTool()
	schema := tool.Schema()

	if schema == nil {
		t.Fatal("Schema should not be nil")
	}

	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Schema should have properties")
	}

	// 检查 path 参数
	pathProp, ok := props["path"].(map[string]interface{})
	if !ok {
		t.Fatal("path property should exist")
	}
	if pathProp["type"] != "string" {
		t.Errorf("path type should be 'string', got %v", pathProp["type"])
	}

	// 检查 parents 参数
	parentsProp, ok := props["parents"].(map[string]interface{})
	if !ok {
		t.Fatal("parents property should exist")
	}
	if parentsProp["type"] != "boolean" {
		t.Errorf("parents type should be 'boolean', got %v", parentsProp["type"])
	}
	if parentsProp["default"] != true {
		t.Errorf("parents default should be true, got %v", parentsProp["default"])
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

	if len(requiredStrs) != 1 || requiredStrs[0] != "path" {
		t.Errorf("Required should contain 'path', got %v", requiredStrs)
	}
}

func TestMkdirTool_Execute_RelativePath(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// 更改工作目录到临时目录
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)
	os.Chdir(tmpdir)

	tool := tools.NewMkdirTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "relative_dir",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}

	// 验证目录被创建
	if _, err := os.Stat(filepath.Join(tmpdir, "relative_dir")); os.IsNotExist(err) {
		t.Error("Relative directory was not created")
	}
}
