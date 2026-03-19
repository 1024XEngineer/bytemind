package tools_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/opencode-go/internal/tools"
)

func TestListTool_Execute_Success(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// 创建一些测试文件和目录
	file1 := filepath.Join(tmpdir, "file1.txt")
	file2 := filepath.Join(tmpdir, "file2.txt")
	subdir := filepath.Join(tmpdir, "subdir")

	os.WriteFile(file1, []byte("content1"), 0644)
	os.WriteFile(file2, []byte("content2"), 0644)
	os.Mkdir(subdir, 0755)

	tool := tools.NewListTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpdir,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "file1.txt") || !strings.Contains(result.Output, "file2.txt") {
		t.Errorf("Expected output to contain file names, got: %s", result.Output)
	}
	if !strings.Contains(result.Output, "subdir") {
		t.Errorf("Expected output to contain subdirectory, got: %s", result.Output)
	}
}

func TestListTool_Execute_DefaultPath(t *testing.T) {
	// 获取当前工作目录（仅检查错误）
	if _, err := os.Getwd(); err != nil {
		t.Fatal(err)
	}

	tool := tools.NewListTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	// 至少应该列出当前目录
	if result.Output == "" {
		t.Error("Expected non-empty output")
	}
}

func TestListTool_Execute_Recursive(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// 创建嵌套结构
	subdir := filepath.Join(tmpdir, "a", "b")
	os.MkdirAll(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "file.txt"), []byte("content"), 0644)

	tool := tools.NewListTool()
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
	// 应该包含嵌套路径
	if !strings.Contains(result.Output, "a/b/file.txt") {
		t.Errorf("Expected recursive listing, got: %s", result.Output)
	}
}

func TestListTool_Execute_HiddenFiles(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// 创建普通文件和隐藏文件
	os.WriteFile(filepath.Join(tmpdir, "normal.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(tmpdir, ".hidden.txt"), []byte("content"), 0644)

	tool := tools.NewListTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":        tmpdir,
		"show_hidden": true,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	// 应该包含隐藏文件
	if !strings.Contains(result.Output, ".hidden.txt") {
		t.Errorf("Expected hidden file in output, got: %s", result.Output)
	}
}

func TestListTool_Execute_WithoutHiddenFiles(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	// 创建普通文件和隐藏文件
	os.WriteFile(filepath.Join(tmpdir, "normal.txt"), []byte("content"), 0644)
	os.WriteFile(filepath.Join(tmpdir, ".hidden.txt"), []byte("content"), 0644)

	tool := tools.NewListTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpdir,
		// show_hidden 默认为 false
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	// 不应该包含隐藏文件
	if strings.Contains(result.Output, ".hidden.txt") {
		t.Errorf("Expected hidden file to be excluded, got: %s", result.Output)
	}
}

func TestListTool_Execute_PathNotFound(t *testing.T) {
	tool := tools.NewListTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "/nonexistent/path/123456",
	})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to nonexistent path")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestListTool_Execute_NotADirectory(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	tool := tools.NewListTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpfile.Name(),
	})

	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Success {
		t.Error("Expected failure due to path not being a directory")
	}
	if result.Error == "" {
		t.Error("Expected error message")
	}
}

func TestListTool_Execute_EmptyDirectory(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	tool := tools.NewListTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpdir,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "(empty directory)") {
		t.Errorf("Expected empty directory message, got: %s", result.Output)
	}
}

func TestListTool_NameAndDescription(t *testing.T) {
	tool := tools.NewListTool()
	if tool.Name() != "list" {
		t.Errorf("Expected name 'list', got %s", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Expected non-empty description")
	}
}

func TestListTool_Schema(t *testing.T) {
	tool := tools.NewListTool()
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

	// 检查 show_hidden 参数
	showHiddenProp, ok := props["show_hidden"].(map[string]interface{})
	if !ok {
		t.Fatal("show_hidden property should exist")
	}
	if showHiddenProp["type"] != "boolean" {
		t.Errorf("show_hidden type should be 'boolean', got %v", showHiddenProp["type"])
	}
	if showHiddenProp["default"] != false {
		t.Errorf("show_hidden default should be false, got %v", showHiddenProp["default"])
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

	// list 工具没有 required 参数
	if len(requiredStrs) != 0 {
		t.Errorf("Required should be empty, got %v", requiredStrs)
	}
}
