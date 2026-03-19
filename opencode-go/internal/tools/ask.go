package tools

import (
	"context"
	"fmt"
)

// AskTool 实现用户交互的工具
type AskTool struct {
	*BaseTool
}

// NewAskTool 创建ask工具
func NewAskTool() *AskTool {
	schema := ParameterSchema(
		map[string]interface{}{
			"type": map[string]interface{}{
				"type":        "string",
				"description": "Type of question: 'confirm' for yes/no, 'input' for text input",
				"enum":        []string{"confirm", "input"},
			},
			"question": map[string]interface{}{
				"type":        "string",
				"description": "The question to ask the user (for confirm type)",
			},
			"prompt": map[string]interface{}{
				"type":        "string",
				"description": "The prompt to show the user (for input type)",
			},
			"options": map[string]interface{}{
				"type":        "array",
				"description": "Optional list of options for the user to choose from",
				"items": map[string]interface{}{
					"type": "string",
				},
			},
		},
		[]string{"type"},
	)

	return &AskTool{
		BaseTool: NewBaseTool(
			"ask",
			"Ask the user for clarification or confirmation",
			schema,
		),
	}
}

// Execute 执行ask操作
func (t *AskTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	// 解析参数
	typeVal, ok := args["type"]
	if !ok {
		return ErrorResult("type parameter is required"), nil
	}

	questionType, ok := typeVal.(string)
	if !ok {
		return ErrorResult("type must be a string"), nil
	}

	switch questionType {
	case "confirm":
		questionVal, _ := args["question"]
		question, _ := questionVal.(string)
		if question == "" {
			question = "Continue?"
		}

		// 在实际实现中，这里应该阻塞等待用户输入
		// 但由于工具执行是同步的，我们需要返回一个特殊结果
		// 让上层处理用户交互
		return &ToolResult{
			Success: true,
			Output:  fmt.Sprintf("[ASK_CONFIRM] %s", question),
		}, nil

	case "input":
		promptVal, _ := args["prompt"]
		prompt, _ := promptVal.(string)
		if prompt == "" {
			prompt = "Please enter input:"
		}

		// 在实际实现中，这里应该阻塞等待用户输入
		return &ToolResult{
			Success: true,
			Output:  fmt.Sprintf("[ASK_INPUT] %s", prompt),
		}, nil

	default:
		return ErrorResult(fmt.Sprintf("unknown question type: %s", questionType)), nil
	}
}
