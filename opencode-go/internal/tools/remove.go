package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// RemoveTool 删除文件或目录的工具
type RemoveTool struct {
	*BaseTool
}

// NewRemoveTool 创建remove工具
func NewRemoveTool() *RemoveTool {
	schema := ParameterSchema(
		map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The path to remove",
			},
			"recursive": map[string]interface{}{
				"type":        "boolean",
				"description": "Remove directories recursively (optional)",
				"default":     false,
			},
			"force": map[string]interface{}{
				"type":        "boolean",
				"description": "Force removal without checking (optional)",
				"default":     false,
			},
		},
		[]string{"path"},
	)

	return &RemoveTool{
		BaseTool: NewBaseTool(
			"remove",
			"Remove a file or directory",
			schema,
		),
	}
}

// Execute 执行删除操作
func (t *RemoveTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	// 解析参数
	pathVal, ok := args["path"]
	if !ok {
		return ErrorResult("path parameter is required"), nil
	}

	path, ok := pathVal.(string)
	if !ok || path == "" {
		return ErrorResult("path must be a non-empty string"), nil
	}

	recursiveVal, _ := args["recursive"]
	recursive, _ := recursiveVal.(bool)

	forceVal, _ := args["force"]
	force, _ := forceVal.(bool)

	// 确保路径是绝对路径
	if !filepath.IsAbs(path) {
		wd, _ := os.Getwd()
		path = filepath.Join(wd, path)
	}

	// 检查路径是否存在
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SuccessResult(fmt.Sprintf("Already removed or does not exist: %s", path)), nil
		}
		return ErrorResult(fmt.Sprintf("Error checking path: %v", err)), nil
	}

	// 安全警告：如果是目录且未指定recursive
	if info.IsDir() && !recursive {
		// 检查目录是否为空
		entries, err := os.ReadDir(path)
		if err != nil {
			return ErrorResult(fmt.Sprintf("Failed to read directory: %v", err)), nil
		}

		if len(entries) > 0 && !force {
			return ErrorResult(fmt.Sprintf("Directory is not empty: %s. Use recursive=true to remove.", path)), nil
		}
	}

	// 执行删除
	if info.IsDir() {
		if recursive || force {
			err = os.RemoveAll(path)
		} else {
			err = os.Remove(path)
		}
	} else {
		err = os.Remove(path)
	}

	if err != nil {
		return ErrorResult(fmt.Sprintf("Failed to remove: %v", err)), nil
	}

	return SuccessResult(fmt.Sprintf("Removed: %s", path)), nil
}
