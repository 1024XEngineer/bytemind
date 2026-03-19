package tools

import (
	"context"
	"os"
	"path/filepath"
)

// ToolContext 包含工具执行所需的上下文信息
type ToolContext struct {
	WorkDir string          // 当前工作目录
	Context context.Context // Go context
}

// NewToolContext 创建新的工具上下文
func NewToolContext(workDir string) *ToolContext {
	if workDir == "" {
		workDir, _ = os.Getwd()
	}
	return &ToolContext{
		WorkDir: workDir,
		Context: context.Background(),
	}
}

// WithContext 返回带有新 Go context 的上下文副本
func (tc *ToolContext) WithContext(ctx context.Context) *ToolContext {
	return &ToolContext{
		WorkDir: tc.WorkDir,
		Context: ctx,
	}
}

// ResolvePath 解析相对路径为绝对路径
func (tc *ToolContext) ResolvePath(path string) string {
	if path == "" {
		return tc.WorkDir
	}
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(tc.WorkDir, path)
}

// BaseTool 是工具的基础实现，嵌入到具体工具中
type BaseTool struct {
	name        string
	description string
	schema      map[string]interface{}
}

// NewBaseTool 创建基础工具
func NewBaseTool(name, description string, schema map[string]interface{}) *BaseTool {
	return &BaseTool{
		name:        name,
		description: description,
		schema:      schema,
	}
}

// Name 返回工具名称
func (bt *BaseTool) Name() string {
	return bt.name
}

// Description 返回工具描述
func (bt *BaseTool) Description() string {
	return bt.description
}

// Schema 返回工具参数定义
func (bt *BaseTool) Schema() map[string]interface{} {
	return bt.schema
}
