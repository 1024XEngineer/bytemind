package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// MkdirTool 创建目录的工具
type MkdirTool struct {
	*BaseTool
}

// NewMkdirTool 创建mkdir工具
func NewMkdirTool() *MkdirTool {
	schema := ParameterSchema(
		map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The directory path to create",
			},
			"parents": map[string]interface{}{
				"type":        "boolean",
				"description": "Create parent directories if they don't exist (optional)",
				"default":     true,
			},
		},
		[]string{"path"},
	)

	return &MkdirTool{
		BaseTool: NewBaseTool(
			"mkdir",
			"Create a directory",
			schema,
		),
	}
}

// Execute 执行创建目录操作
func (t *MkdirTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	// 解析参数
	pathVal, ok := args["path"]
	if !ok {
		return ErrorResult("path parameter is required"), nil
	}

	path, ok := pathVal.(string)
	if !ok || path == "" {
		return ErrorResult("path must be a non-empty string"), nil
	}

	parentsVal, _ := args["parents"]
	parents, ok := parentsVal.(bool)
	if !ok {
		parents = true // 默认创建父目录
	}

	// 确保路径是绝对路径
	if !filepath.IsAbs(path) {
		wd, _ := os.Getwd()
		path = filepath.Join(wd, path)
	}

	var err error
	if parents {
		err = os.MkdirAll(path, 0755)
	} else {
		err = os.Mkdir(path, 0755)
	}

	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to create directory: %v", err)), nil
	}

	return SuccessResult(fmt.Sprintf("Created directory: %s", path)), nil
}
