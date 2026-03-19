package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ReadTool 实现读取文件内容的工具
type ReadTool struct {
	*BaseTool
}

// NewReadTool 创建读取工具
func NewReadTool() *ReadTool {
	schema := ParameterSchema(
		map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The absolute path to the file to read",
			},
			"offset": map[string]interface{}{
				"type":        "integer",
				"description": "The line number to start reading from (1-indexed, optional)",
				"minimum":     1,
			},
			"limit": map[string]interface{}{
				"type":        "integer",
				"description": "The maximum number of lines to read (optional)",
				"minimum":     1,
			},
		},
		[]string{"path"},
	)

	return &ReadTool{
		BaseTool: NewBaseTool(
			"read",
			"Read a file from the local filesystem",
			schema,
		),
	}
}

// Execute 执行读取操作
func (t *ReadTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	// 解析参数
	pathVal, ok := args["path"]
	if !ok {
		return ErrorResult("path parameter is required"), nil
	}

	path, ok := pathVal.(string)
	if !ok || path == "" {
		return ErrorResult("path must be a non-empty string"), nil
	}

	// 确保路径是绝对路径
	if !filepath.IsAbs(path) {
		// 如果有工具上下文，可以解析相对路径
		// 暂时直接使用当前工作目录
		wd, _ := os.Getwd()
		path = filepath.Join(wd, path)
	}

	// 检查文件是否存在
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return ErrorResult(fmt.Sprintf("file does not exist: %s", path)), nil
	}

	// 读取文件内容
	content, err := os.ReadFile(path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to read file: %v", err)), nil
	}

	// TODO: 实现 offset 和 limit 支持
	// 这需要将内容按行分割，然后截取指定范围

	return SuccessResult(string(content)), nil
}
