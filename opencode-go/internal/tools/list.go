package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ListTool 列出目录内容的工具
type ListTool struct {
	*BaseTool
}

// NewListTool 创建list工具
func NewListTool() *ListTool {
	schema := ParameterSchema(
		map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "The directory path to list",
			},
			"recursive": map[string]interface{}{
				"type":        "boolean",
				"description": "List recursively (optional)",
				"default":     false,
			},
			"show_hidden": map[string]interface{}{
				"type":        "boolean",
				"description": "Show hidden files (starting with .) (optional)",
				"default":     false,
			},
		},
		[]string{},
	)

	return &ListTool{
		BaseTool: NewBaseTool(
			"list",
			"List files and directories",
			schema,
		),
	}
}

// Execute 执行列出目录操作
func (t *ListTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	// 解析参数
	pathVal, _ := args["path"]
	path, _ := pathVal.(string)
	if path == "" {
		path, _ = os.Getwd()
	}

	recursiveVal, _ := args["recursive"]
	recursive, _ := recursiveVal.(bool)

	showHiddenVal, _ := args["show_hidden"]
	showHidden, _ := showHiddenVal.(bool)

	// 确保路径是绝对路径
	if !filepath.IsAbs(path) {
		wd, _ := os.Getwd()
		path = filepath.Join(wd, path)
	}

	// 检查路径是否存在
	info, err := os.Stat(path)
	if err != nil {
		return ErrorResult(fmt.Sprintf("path does not exist: %s", path)), nil
	}

	if !info.IsDir() {
		return ErrorResult(fmt.Sprintf("path is not a directory: %s", path)), nil
	}

	var result strings.Builder

	if recursive {
		// 递归列出
		err = filepath.Walk(path, func(filePath string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // 跳过错误
			}

			relPath, _ := filepath.Rel(path, filePath)
			if relPath == "." {
				return nil // 跳过根目录
			}

			// 检查隐藏文件
			if !showHidden && strings.HasPrefix(info.Name(), ".") {
				if info.IsDir() {
					return filepath.SkipDir // 跳过隐藏目录
				}
				return nil // 跳过隐藏文件
			}

			marker := "[FILE]"
			if info.IsDir() {
				marker = "[DIR] "
			}

			result.WriteString(fmt.Sprintf("%s %s\n", marker, relPath))
			return nil
		})
	} else {
		// 非递归列出
		entries, err := os.ReadDir(path)
		if err != nil {
			return ErrorResult(fmt.Sprintf("failed to read directory: %v", err)), nil
		}

		for _, entry := range entries {
			// 检查隐藏文件
			if !showHidden && strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			marker := "[FILE]"
			if entry.IsDir() {
				marker = "[DIR] "
			}

			result.WriteString(fmt.Sprintf("%s %s\n", marker, entry.Name()))
		}
	}

	if err != nil {
		return ErrorResult(fmt.Sprintf("error while listing: %v", err)), nil
	}

	if result.Len() == 0 {
		return SuccessResult("(empty directory)"), nil
	}

	return SuccessResult(result.String()), nil
}
