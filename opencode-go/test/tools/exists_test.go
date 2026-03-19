package tools_test

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/opencode-go/internal/tools"
)

func TestExistsTool_Execute_FileExists(t *testing.T) {
	tmpfile, err := os.CreateTemp("", "test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()
	defer os.Remove(tmpfile.Name())

	tool := tools.NewExistsTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpfile.Name(),
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Exists (file") {
		t.Errorf("Expected 'Exists (file' in output, got: %s", result.Output)
	}
}

func TestExistsTool_Execute_DirectoryExists(t *testing.T) {
	tmpdir, err := os.MkdirTemp("", "test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpdir)

	tool := tools.NewExistsTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": tmpdir,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Exists (directory") {
		t.Errorf("Expected 'Exists (directory' in output, got: %s", result.Output)
	}
}

func TestExistsTool_Execute_NotExists(t *testing.T) {
	tool := tools.NewExistsTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "/nonexistent/path/123456",
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success for non-existent path, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Does not exist:") {
		t.Errorf("Expected 'Does not exist:' in output, got: %s", result.Output)
	}
}

func TestExistsTool_Execute_MissingPath(t *testing.T) {
	tool := tools.NewExistsTool()
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

func TestExistsTool_Execute_EmptyPath(t *testing.T) {
	tool := tools.NewExistsTool()
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

func TestExistsTool_Execute_InvalidPathType(t *testing.T) {
	tool := tools.NewExistsTool()
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

func TestExistsTool_Execute_RelativePath(t *testing.T) {
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

	// 在当前目录创建文件
	filename := "testfile.txt"
	os.WriteFile(filename, []byte("content"), 0644)
	defer os.Remove(filename)

	tool := tools.NewExistsTool()
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": filename,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if !result.Success {
		t.Fatalf("Expected success, got error: %s", result.Error)
	}
	if !strings.Contains(result.Output, "Exists (file") {
		t.Errorf("Expected 'Exists (file' in output, got: %s", result.Output)
	}
}

func TestExistsTool_NameAndDescription(t *testing.T) {
	tool := tools.NewExistsTool()
	if tool.Name() != "exists" {
		t.Errorf("Expected name 'exists', got %s", tool.Name())
	}
	if tool.Description() == "" {
		t.Error("Expected non-empty description")
	}
}

func TestExistsTool_Schema(t *testing.T) {
	tool := tools.NewExistsTool()
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
