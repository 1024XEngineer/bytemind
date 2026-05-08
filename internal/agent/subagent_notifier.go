package agent

import (
	"fmt"
	"strings"
	"sync"

	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
)

// SubAgentNotifier delivers async subagent completion results to a parent session.
// When an async task completes, the notifier queues a structured notification that
// the parent's turn loop can drain before each step.
type SubAgentNotifier interface {
	NotifyCompletion(notification SubAgentCompletionNotification)
	DrainPending() []SubAgentCompletionNotification
}

// SubAgentCompletionNotification carries the result of a completed async subagent task.
type SubAgentCompletionNotification struct {
	ParentSession   *session.Session
	TaskID          string
	Agent           string
	InvocationID    string
	Status          string // "completed" or "failed"
	Summary         string
	ErrorCode       string
	ErrorMessage    string
	WorktreePath    string
	WorktreeBranch  string
	WorktreeState   string
}

type defaultSubAgentNotifier struct {
	mu      sync.Mutex
	pending []SubAgentCompletionNotification
}

func (n *defaultSubAgentNotifier) NotifyCompletion(notification SubAgentCompletionNotification) {
	if n == nil {
		return
	}
	n.mu.Lock()
	n.pending = append(n.pending, notification)
	n.mu.Unlock()
}

func (n *defaultSubAgentNotifier) DrainPending() []SubAgentCompletionNotification {
	if n == nil {
		return nil
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if len(n.pending) == 0 {
		return nil
	}
	drained := n.pending
	n.pending = nil
	return drained
}

func buildNotificationMessage(n SubAgentCompletionNotification) string {
	var b strings.Builder
	b.WriteString("<subagent-notification>\n")
	if n.TaskID != "" {
		fmt.Fprintf(&b, "  <task_id>%s</task_id>\n", n.TaskID)
	}
	if n.Agent != "" {
		fmt.Fprintf(&b, "  <agent>%s</agent>\n", n.Agent)
	}
	if n.InvocationID != "" {
		fmt.Fprintf(&b, "  <invocation_id>%s</invocation_id>\n", n.InvocationID)
	}
	if n.Status != "" {
		fmt.Fprintf(&b, "  <status>%s</status>\n", n.Status)
	}
	if n.Summary != "" {
		fmt.Fprintf(&b, "  <summary>%s</summary>\n", n.Summary)
	}
	if n.ErrorCode != "" {
		fmt.Fprintf(&b, "  <error_code>%s</error_code>\n", n.ErrorCode)
	}
	if n.ErrorMessage != "" {
		fmt.Fprintf(&b, "  <error_message>%s</error_message>\n", n.ErrorMessage)
	}
	if n.WorktreePath != "" {
		fmt.Fprintf(&b, "  <worktree_path>%s</worktree_path>\n", n.WorktreePath)
	}
	if n.WorktreeBranch != "" {
		fmt.Fprintf(&b, "  <worktree_branch>%s</worktree_branch>\n", n.WorktreeBranch)
	}
	if n.WorktreeState != "" {
		fmt.Fprintf(&b, "  <worktree_state>%s</worktree_state>\n", n.WorktreeState)
	}
	b.WriteString("</subagent-notification>")
	return b.String()
}

func drainSubAgentNotifications(runner *Runner, sess *session.Session) {
	if runner == nil || runner.subAgentNotifier == nil {
		return
	}
	for _, notification := range runner.subAgentNotifier.DrainPending() {
		if notification.ParentSession == nil || notification.ParentSession != sess {
			continue
		}
		msg := buildNotificationMessage(notification)
		m := llm.NewUserTextMessage(msg)
		if m.Meta == nil {
			m.Meta = llm.MessageMeta{}
		}
		m.Meta["ephemeral"] = true
		sess.Messages = append(sess.Messages, m)
	}
}
