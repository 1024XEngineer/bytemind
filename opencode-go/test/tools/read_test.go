package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencode-go/internal/tools"
)

func TestReadTool_Execute_Success(t *testing.T) {
	// 创建临时文件
	tmpfile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	content := "Hello, World!\nSecond line\nThird line"
	_, err = tmpfile.WriteString(content)
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	// 执行读取
	tool := tools.NewReadTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpfile.Name(),
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if result.Output != content {
		t.Fatalf("Content mismatch. Expected:\n%s\nGot:\n%s", content, result.Output)
	}
}

func TestReadTool_Execute_FileNotFound(t *testing.T) {
	tool := tools.NewReadTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "/nonexistent/path/to/file.txt",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Success {
		t.Fatal("Expected failure for non-existent file")
	}
	if result.Error == "" {
		t.Fatal("Error message should not be empty")
	}
}

func TestReadTool_Execute_MissingPath(t *testing.T) {
	tool := tools.NewReadTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Success {
		t.Fatal("Expected failure for missing path")
	}
	if result.Error != "path parameter is required" {
		t.Fatalf("Expected error 'path parameter is required', got %s", result.Error)
	}
}

func TestReadTool_Execute_EmptyPath(t *testing.T) {
	tool := tools.NewReadTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Success {
		t.Fatal("Expected failure for empty path")
	}
	if result.Error != "path must be a non-empty string" {
		t.Fatalf("Expected error 'path must be a non-empty string', got %s", result.Error)
	}
}

func TestReadTool_Execute_InvalidPathType(t *testing.T) {
	tool := tools.NewReadTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": 123, // 数字而不是字符串
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Success {
		t.Fatal("Expected failure for invalid path type")
	}
	if result.Error != "path must be a non-empty string" {
		t.Fatalf("Expected error 'path must be a non-empty string', got %s", result.Error)
	}
}

func TestReadTool_Execute_RelativePath(t *testing.T) {
	// 创建临时目录和文件
	tmpdir, err := os.MkdirTemp("", "testdir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	filename := filepath.Join(tmpdir, "test.txt")
	content := "Relative path test"
	err = os.WriteFile(filename, []byte(content), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// 切换到临时目录
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldwd)
	os.Chdir(tmpdir)

	// 使用相对路径
	tool := tools.NewReadTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "test.txt",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if result.Output != content {
		t.Fatalf("Content mismatch. Expected:\n%s\nGot:\n%s", content, result.Output)
	}
}

func TestReadTool_NameAndDescription(t *testing.T) {
	tool := tools.NewReadTool()
	if tool.Name() != "read" {
		t.Errorf("Expected name 'read', got %s", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
}

func TestReadTool_Schema(t *testing.T) {
	tool := tools.NewReadTool()
	schema := tool.Schema()

	if schema == nil {
		t.Fatal("Schema should not be nil")
	}

	// 检查必需的字段
	props, ok := schema["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("Schema should have properties")
	}

	// 检查 path 参数定义
	pathProp, ok := props["path"].(map[string]interface{})
	if !ok {
		t.Fatal("path property should exist")
	}
	if pathProp["type"] != "string" {
		t.Errorf("path type should be 'string', got %v", pathProp["type"])
	}

	// 检查 required 字段
	required := schema["required"]
	if required == nil {
		t.Fatal("Schema should have required field")
	}

	// 处理不同类型的 required 字段
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
