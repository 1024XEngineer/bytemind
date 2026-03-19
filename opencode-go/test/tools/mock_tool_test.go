package tools_test

import (
	"context"
	"github.com/opencode-go/internal/tools"
)

// MockTool 用于测试工具接口
type MockTool struct {
	name        string
	description string
	schema      map[string]interface{}
	executeFn   func(ctx context.Context, args map[string]interface{}) (*tools.ToolResult, error)
}

func (m *MockTool) Name() string                   { return m.name }
func (m *MockTool) Description() string            { return m.description }
func (m *MockTool) Schema() map[string]interface{} { return m.schema }
func (m *MockTool) Execute(ctx context.Context, args map[string]interface{}) (*tools.ToolResult, error) {
	if m.executeFn != nil {
		return m.executeFn(ctx, args)
	}
	return tools.SuccessResult("default"), nil
}

// NewMockTool 创建一个简单的模拟工具
func NewMockTool(name string) *MockTool {
	return &MockTool{
		name:        name,
		description: "mock tool",
		schema:      map[string]interface{}{},
	}
}
