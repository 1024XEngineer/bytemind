package report

import (
	"fmt"
	"strings"

	"forgecli/internal/session"
)

func Render(current *session.Session) string {
	var builder strings.Builder

	builder.WriteString("ForgeCLI MVP 结果总结\n")
	builder.WriteString("====================\n")
	builder.WriteString(fmt.Sprintf("任务: %s\n", current.Task))
	builder.WriteString(fmt.Sprintf("工作区: %s\n", current.RepoRoot))
	if current.TargetFile != "" {
		builder.WriteString(fmt.Sprintf("目标文件: %s\n", current.TargetFile))
	}
	if current.Plan.Summary != "" {
		builder.WriteString(fmt.Sprintf("计划: %s\n", current.Plan.Summary))
	}

	if current.FileWritten {
		builder.WriteString(fmt.Sprintf("写入结果: 已写入 %d 个文件\n", len(current.ChangedFiles)))
	} else {
		builder.WriteString("写入结果: 未写入文件\n")
	}

	if current.VerifyCommand != "" {
		builder.WriteString(fmt.Sprintf("验证命令: %s\n", current.VerifyCommand))
		if current.VerifyApproved {
			if current.VerifyResult != nil {
				builder.WriteString(fmt.Sprintf("验证退出码: %d\n", current.VerifyResult.ExitCode))
				if current.VerifyResult.TimedOut {
					builder.WriteString("验证状态: 命令超时\n")
				} else if current.VerifyResult.ExitCode == 0 {
					builder.WriteString("验证状态: 通过\n")
				} else {
					builder.WriteString("验证状态: 失败\n")
				}
			}
		} else {
			builder.WriteString("验证状态: 用户未批准执行\n")
		}
	} else {
		builder.WriteString("验证状态: 未提供验证命令\n")
	}

	if len(current.Notes) > 0 {
		builder.WriteString("说明:\n")
		for _, note := range current.Notes {
			builder.WriteString("- " + note + "\n")
		}
	}

	return builder.String()
}
