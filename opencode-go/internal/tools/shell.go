package tools

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ShellTool 实现执行shell命令的工具
type ShellTool struct {
	*BaseTool
}

// NewShellTool 创建shell工具
func NewShellTool() *ShellTool {
	schema := ParameterSchema(
		map[string]interface{}{
			"command": map[string]interface{}{
				"type":        "string",
				"description": "The shell command to execute",
			},
			"workdir": map[string]interface{}{
				"type":        "string",
				"description": "Working directory for the command (optional)",
			},
			"timeout": map[string]interface{}{
				"type":        "integer",
				"description": "Timeout in seconds (optional)",
				"minimum":     1,
			},
		},
		[]string{"command"},
	)

	return &ShellTool{
		BaseTool: NewBaseTool(
			"shell_execute",
			"Execute shell commands",
			schema,
		),
	}
}

// Execute 执行shell命令
func (t *ShellTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolResult, error) {
	// 解析参数
	commandVal, ok := args["command"]
	if !ok {
		return ErrorResult("command parameter is required"), nil
	}

	command, ok := commandVal.(string)
	if !ok || command == "" {
		return ErrorResult("command must be a non-empty string"), nil
	}

	workdirVal, _ := args["workdir"]
	workdir, _ := workdirVal.(string)
	if workdir == "" {
		workdir, _ = os.Getwd()
	}

	// 检查工作目录是否存在
	if _, err := os.Stat(workdir); os.IsNotExist(err) {
		return ErrorResult(fmt.Sprintf("working directory does not exist: %s", workdir)), nil
	}

	// 安全检查：限制危险命令（可选）
	// 这里可以添加命令白名单或黑名单

	// 执行命令
	var cmd *exec.Cmd

	// 根据操作系统选择不同的执行方式
	if strings.HasPrefix(strings.ToLower(os.Getenv("OS")), "windows") || os.PathSeparator == '\\' {
		// Windows 系统
		cmd = exec.Command("cmd", "/C", command)
	} else {
		// Unix-like 系统
		cmd = exec.Command("sh", "-c", command)
	}

	cmd.Dir = workdir

	// 执行命令并捕获输出
	output, err := cmd.CombinedOutput()
	result := string(output)

	if err != nil {
		// 命令执行失败
		return &ToolResult{
			Success: false,
			Output:  result,
			Error:   fmt.Sprintf("command failed: %v", err),
		}, nil
	}

	return SuccessResult(result), nil
}
