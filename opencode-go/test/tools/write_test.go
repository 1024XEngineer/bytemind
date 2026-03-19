package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencode-go/internal/tools"
)

func TestWriteTool_Execute_Success(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	content := "Hello, World!"
	tool := tools.NewWriteTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    tmpfile.Name(),
		"content": content,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}

	// 验证文件内容
	data, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("File content mismatch. Expected:\n%s\nGot:\n%s", content, string(data))
	}
}

func TestWriteTool_Execute_Append(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	// 写入初始内容
	initial := "Initial content\n"
	err = os.WriteFile(tmpfile.Name(), []byte(initial), 0644)
	if err != nil {
		t.Fatal(err)
	}

	// 追加内容
	appendContent := "Appended content"
	tool := tools.NewWriteTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    tmpfile.Name(),
		"content": appendContent,
		"append":  true,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}

	// 验证文件内容
	data, err := os.ReadFile(tmpfile.Name())
	if err != nil {
		t.Fatal(err)
	}
	expected := initial + appendContent
	if string(data) != expected {
		t.Fatalf("File content mismatch. Expected:\n%s\nGot:\n%s", expected, string(data))
	}
}

func TestWriteTool_Execute_CreateDirectories(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "testdir-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// 尝试写入深层目录中的文件
	deepPath := filepath.Join(tmpdir, "a", "b", "c", "file.txt")
	content := "Deep directory test"

	tool := tools.NewWriteTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    deepPath,
		"content": content,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}

	// 验证文件已创建
	if _, err := os.Stat(deepPath); os.IsNotExist(err) {
		t.Fatal("File was not created")
	}

	// 验证内容
	data, err := os.ReadFile(deepPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("Content mismatch")
	}
}

func TestWriteTool_Execute_MissingPath(t *testing.T) {
	tool := tools.NewWriteTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"content": "test",
	})

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

func TestWriteTool_Execute_MissingContent(t *testing.T) {
	tool := tools.NewWriteTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "/tmp/test.txt",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Success {
		t.Fatal("Expected failure for missing content")
	}
	if result.Error != "content parameter is required" {
		t.Fatalf("Expected error 'content parameter is required', got %s", result.Error)
	}
}

func TestWriteTool_Execute_InvalidPathType(t *testing.T) {
	tool := tools.NewWriteTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    123,
		"content": "test",
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

func TestWriteTool_Execute_InvalidContentType(t *testing.T) {
	tool := tools.NewWriteTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    "/tmp/test.txt",
		"content": 123, // 数字而不是字符串
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if result.Success {
		t.Fatal("Expected failure for invalid content type")
	}
	if result.Error != "content must be a string" {
		t.Fatalf("Expected error 'content must be a string', got %s", result.Error)
	}
}

func TestWriteTool_NameAndDescription(t *testing.T) {
	tool := tools.NewWriteTool()
	if tool.Name() != "write" {
		t.Errorf("Expected name 'write', got %s", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Description should not be empty")
	}
}

func TestWriteTool_Schema(t *testing.T) {
	tool := tools.NewWriteTool()
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

	// 检查 content 参数
	contentProp, ok := props["content"].(map[string]interface{})
	if !ok {
		t.Fatal("content property should exist")
	}
	if contentProp["type"] != "string" {
		t.Errorf("content type should be 'string', got %v", contentProp["type"])
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

	// 应该包含 path 和 content
	if len(requiredStrs) != 2 {
		t.Errorf("Required should have 2 elements, got %v", requiredStrs)
	}

	hasPath := false
	hasContent := false
	for _, req := range requiredStrs {
		if req == "path" {
			hasPath = true
		}
		if req == "content" {
			hasContent = true
		}
	}
	if !hasPath || !hasContent {
		t.Errorf("Required should contain 'path' and 'content', got %v", requiredStrs)
	}
}
