package tools

import (
	"context"
	"encoding/json"
	"fmt"
)

// ToolResult 是工具执行的结果
type ToolResult struct {
	Success bool   `json:"success"`
	Output  string `json:"output,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Tool 接口定义了所有工具必须实现的方法
type Tool interface {
	Name() string                   // 工具名称 (如 "read", "write", "shell_execute")
	Description() string            // 工具描述
	Schema() map[string]interface{} // JSON Schema 参数定义
	Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error)
}

// ToolDefinition 用于 Function Calling 的工具定义
type ToolDefinition struct {
	Type     string             `json:"type,omitempty"`
	Function FunctionDefinition `json:"function"`
}

type FunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ParameterSchema 用于生成 JSON Schema
func ParameterSchema(props map[string]interface{}, required []string) map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": props,
		"required":   required,
	}
}

// NewResult 创建工具执行结果
func NewResult(success bool, output string, err string) *ToolResult {
	return &ToolResult{
		Success: success,
		Output:  output,
		Error:   err,
	}
}

// SuccessResult 创建成功的工具结果
func SuccessResult(output string) *ToolResult {
	return NewResult(true, output, "")
}

// ErrorResult 创建失败的工具结果
func ErrorResult(err string) *ToolResult {
	return NewResult(false, "", err)
}

// FormatResult 格式化工具结果用于返回给 LLM
func FormatResult(result *ToolResult) string {
	if result.Success {
		return fmt.Sprintf("Success: %s", result.Output)
	}
	return fmt.Sprintf("Error: %s", result.Error)
}

// ParseArgs 解析 JSON 字符串为 map[string]interface{}
func ParseArgs(jsonStr string) (map[string]interface{}, error) {
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &args); err != nil {
		return nil, fmt.Errorf("failed to parse arguments: %v", err)
	}
	return args, nil
}
