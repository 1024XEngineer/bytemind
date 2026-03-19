package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencode-go/internal/tools"
)

func TestRemoveTool_Execute_FileSuccess(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	// 注意：我们不在这里 defer Remove，因为工具应该删除它

	tool := tools.NewRemoveTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpfile.Name(),
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Removed:") {
		t.Errorf("Expected 'Removed:' in output, got: %s", result.Output)
	}

	// 验证文件已被删除
	if _, err := os.Stat(tmpfile.Name()); !os.IsNotExist(err) {
		t.Error("File was not removed")
	}
}

func TestRemoveTool_Execute_DirectorySuccess(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	// 不 defer RemoveAll，因为工具应该删除它

	tool := tools.NewRemoveTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":      tmpdir,
		"recursive": true,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Removed:") {
		t.Errorf("Expected 'Removed:' in output, got: %s", result.Output)
	}

	if _, err := os.Stat(tmpdir); !os.IsNotExist(err) {
		t.Error("Directory was not removed")
	}
}

func TestRemoveTool_Execute_NonEmptyDirectoryWithoutRecursive(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// 在目录中创建一个文件
	filepath := filepath.Join(tmpdir, "file.txt")
	os.WriteFile(filepath, []byte("content"), 0644)

	tool := tools.NewRemoveTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpdir,
		// recursive 默认为 false
	})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to non-empty directory")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
	if !strings.Contains(result.Error, "not empty") {
		t.Errorf("Expected 'not empty' in error, got: %s", result.Error)
	}
}

func TestRemoveTool_Execute_NonEmptyDirectoryWithRecursive(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	// 不 defer RemoveAll，因为工具应该删除它

	// 创建嵌套结构
	subdir := filepath.Join(tmpdir, "subdir")
	os.MkdirAll(subdir, 0755)
	filepath := filepath.Join(subdir, "file.txt")
	os.WriteFile(filepath, []byte("content"), 0644)

	tool := tools.NewRemoveTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":      tmpdir,
		"recursive": true,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}

	if _, err := os.Stat(tmpdir); !os.IsNotExist(err) {
		t.Error("Directory was not removed")
	}
}

func TestRemoveTool_Execute_AlreadyRemoved(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	os.Remove(tmpfile.Name()) // 先删除

	tool := tools.NewRemoveTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpfile.Name(),
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success for already removed file, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Already removed") {
		t.Errorf("Expected 'Already removed' in output, got: %s", result.Output)
	}
}

func TestRemoveTool_Execute_MissingPath(t *testing.T) {
	tool := tools.NewRemoveTool()
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

func TestRemoveTool_Execute_EmptyPath(t *testing.T) {
	tool := tools.NewRemoveTool()
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

func TestRemoveTool_Execute_InvalidPathType(t *testing.T) {
	tool := tools.NewRemoveTool()
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

func TestRemoveTool_Execute_WithForce(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// 创建非空目录
	filepath := filepath.Join(tmpdir, "file.txt")
	os.WriteFile(filepath, []byte("content"), 0644)

	tool := tools.NewRemoveTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":  tmpdir,
		"force": true,
		// recursive 为 false，但 force 为 true 应该允许删除
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success with force=true, got error: %s", result.Error)
	}
}

func TestRemoveTool_NameAndDescription(t *testing.T) {
	tool := tools.NewRemoveTool()
	if tool.Name() != "remove" {
		t.Errorf("Expected name 'remove', got %s", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Expected non-empty description")
	}
}

func TestRemoveTool_Schema(t *testing.T) {
	tool := tools.NewRemoveTool()
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

	// 检查 recursive 参数
	recursiveProp, ok := props["recursive"].(map[string]interface{})
	if !ok {
		t.Fatal("recursive property should exist")
	}
	if recursiveProp["type"] != "boolean" {
		t.Errorf("recursive type should be 'boolean', got %v", recursiveProp["type"])
	}
	if recursiveProp["default"] != false {
		t.Errorf("recursive default should be false, got %v", recursiveProp["default"])
	}

	// 检查 force 参数
	forceProp, ok := props["force"].(map[string]interface{})
	if !ok {
		t.Fatal("force property should exist")
	}
	if forceProp["type"] != "boolean" {
		t.Errorf("force type should be 'boolean', got %v", forceProp["type"])
	}
	if forceProp["default"] != false {
		t.Errorf("force default should be false, got %v", forceProp["default"])
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
