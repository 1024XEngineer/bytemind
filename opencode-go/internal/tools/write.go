package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// WriteTool 实现写入文件内容的工具
type WriteTool struct {
	*BaseTool
}

// NewWriteTool 创建写入工具
func NewWriteTool() *WriteTool {
	schema := ParameterSchema(
		map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The absolute path to the file to write",
			},
			"content": map[string]interface{}{
				"type":        "string",
				"description": "The content to write to the file",
			},
			"append": map[string]interface{}{
				"type":        "boolean",
				"description": "Whether to append to the file instead of overwriting (optional)",
				"default":     false,
			},
		},
		[]string{"path", "content"},
	)

	return &WriteTool{
		BaseTool: NewBaseTool(
			"write",
			"Write content to a file (creates or overwrites)",
			schema,
		),
	}
}

// Execute 执行写入操作
func (t *WriteTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	// 解析参数
	pathVal, ok := args["path"]
	if !ok {
		return ErrorResult("path parameter is required"), nil
	}

	path, ok := pathVal.(string)
	if !ok || path == "" {
		return ErrorResult("path must be a non-empty string"), nil
	}

	contentVal, ok := args["content"]
	if !ok {
		return ErrorResult("content parameter is required"), nil
	}

	content, ok := contentVal.(string)
	if !ok {
		return ErrorResult("content must be a string"), nil
	}

	appendVal, _ := args["append"]
	append, _ := appendVal.(bool)

	// 确保路径是绝对路径
	if !filepath.IsAbs(path) {
		wd, _ := os.Getwd()
		path = filepath.Join(wd, path)
	}

	// 创建父目录（如果需要）
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return ErrorResult(fmt.Sprintf("failed to create directory: %v", err)), nil
		}
	}

	// 确定写入模式
	var flags int
	if append {
		flags = os.O_APPEND | os.O_CREATE | os.O_WRONLY
	} else {
		flags = os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	}

	// 写入文件
	file, err := os.OpenFile(path, flags, 0644)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to open file: %v", err)), nil
	}
	defer file.Close()

	if _, err := file.WriteString(content); err != nil {
		return ErrorResult(fmt.Sprintf("failed to write to file: %v", err)), nil
	}

	// 获取文件信息以返回详细信息
	info, err := os.Stat(path)
	if err != nil {
		return SuccessResult(fmt.Sprintf("Written to %s", path)), nil
	}

	return SuccessResult(fmt.Sprintf("Written %d bytes to %s (total size: %d bytes)", len(content), path, info.Size())), nil
}
