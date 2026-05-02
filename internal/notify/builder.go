package notify

import (
	"fmt"
	"strings"
)

const (
	titleApprovalRequired = "ByteMind 需要权限审批"
	titleRunCompleted     = "ByteMind 任务已完成"
	titleRunFailed        = "ByteMind 任务失败"
	titleRunCanceled      = "ByteMind 任务已取消"
)

func BuildApprovalRequiredMessage(command, reason string) Message {
	sanitizedReason := sanitizeNotificationText(reason)
	sanitizedCommand := sanitizeNotificationText(command)
	displayReason := truncate(sanitizedReason, 100)
	displayCommand := truncate(sanitizedCommand, 120)

	parts := make([]string, 0, 2)
	if displayReason != "" {
		parts = append(parts, "原因: "+displayReason)
	}
	if displayCommand != "" {
		parts = append(parts, "命令: "+displayCommand)
	}

	body := "有新的权限审批请求，请回到终端处理。"
	if len(parts) > 0 {
		body = strings.Join(parts, " | ")
	}
	body = truncate(body, 180)

	return Message{
		Event: EventApprovalRequired,
		Title: titleApprovalRequired,
		Body:  body,
		Key: fmt.Sprintf(
			"approval_required|cmd=%s|reason=%s",
			normalizeForKey(sanitizedCommand),
			normalizeForKey(sanitizedReason),
		),
	}
}

func BuildRunCompletedMessage(runID int) Message {
	return Message{
		Event: EventRunCompleted,
		Title: titleRunCompleted,
		Body:  "任务已完成，可回到终端查看结果。",
		Key:   fmt.Sprintf("run_completed|id=%d", runID),
	}
}

func BuildRunFailedMessage(runID int, detail string) Message {
	detail = truncate(sanitizeNotificationText(detail), 140)
	body := "任务执行失败，请回到终端查看详情。"
	if detail != "" {
		body = "任务执行失败: " + detail
	}
	return Message{
		Event: EventRunFailed,
		Title: titleRunFailed,
		Body:  truncate(body, 180),
		Key:   fmt.Sprintf("run_failed|id=%d", runID),
	}
}

func BuildRunCanceledMessage(runID int) Message {
	return Message{
		Event: EventRunCanceled,
		Title: titleRunCanceled,
		Body:  "任务已取消。",
		Key:   fmt.Sprintf("run_canceled|id=%d", runID),
	}
}

func normalizeMessage(msg Message) Message {
	msg.Title = truncate(sanitizeNotificationText(msg.Title), 80)
	msg.Body = truncate(sanitizeNotificationText(msg.Body), 180)
	if msg.Key == "" {
		msg.Key = fmt.Sprintf(
			"%s|%s|%s",
			normalizeForKey(string(msg.Event)),
			normalizeForKey(msg.Title),
			normalizeForKey(msg.Body),
		)
	}
	return msg
}
