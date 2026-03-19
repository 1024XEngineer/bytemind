package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ExistsTool 检查文件或目录是否存在的工具
type ExistsTool struct {
	*BaseTool
}

// NewExistsTool 创建exists工具
func NewExistsTool() *ExistsTool {
	schema := ParameterSchema(
		map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The path to check",
			},
		},
		[]string{"path"},
	)

	return &ExistsTool{
		BaseTool: NewBaseTool(
			"exists",
			"Check if a file or directory exists",
			schema,
		),
	}
}

// Execute 执行检查存在性操作
func (t *ExistsTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
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
		wd, _ := os.Getwd()
		path = filepath.Join(wd, path)
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SuccessResult(fmt.Sprintf("Does not exist: %s", path)), nil
		}
		return ErrorResult(fmt.Sprintf("Error checking path: %v", err)), nil
	}

	var typeStr string
	if info.IsDir() {
		typeStr = "directory"
	} else {
		typeStr = "file"
	}

	return SuccessResult(fmt.Sprintf("Exists (%s, size: %d bytes, modified: %v): %s",
		typeStr, info.Size(), info.ModTime().Format("2006-01-02 15:04:05"), path)), nil
}
