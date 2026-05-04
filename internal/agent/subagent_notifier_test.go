package agent

import (
	"strings"
	"testing"

	"github.com/1024XEngineer/bytemind/internal/llm"
	"github.com/1024XEngineer/bytemind/internal/session"
)

func TestDefaultSubAgentNotifierNilReceiver(t *testing.T) {
	var n *defaultSubAgentNotifier
	// Should not panic
	n.NotifyCompletion(SubAgentCompletionNotification{})
	if got := n.DrainPending(); got != nil {
		t.Fatalf("expected nil from nil receiver drain, got %v", got)
	}
}

func TestDefaultSubAgentNotifierDrainEmpty(t *testing.T) {
	n := &defaultSubAgentNotifier{}
	if got := n.DrainPending(); got != nil {
		t.Fatalf("expected nil for empty drain, got %v", got)
	}
}

func TestDefaultSubAgentNotifierNotifyAndDrain(t *testing.T) {
	n := &defaultSubAgentNotifier{}
	n.NotifyCompletion(SubAgentCompletionNotification{TaskID: "t1", Agent: "explorer"})
	n.NotifyCompletion(SubAgentCompletionNotification{TaskID: "t2", Agent: "writer"})

	pending := n.DrainPending()
	if len(pending) != 2 {
		t.Fatalf("expected 2 notifications, got %d", len(pending))
	}
	if pending[0].TaskID != "t1" || pending[1].TaskID != "t2" {
		t.Fatalf("unexpected notification order: %v", pending)
	}

	// Second drain should be empty
	if got := n.DrainPending(); got != nil {
		t.Fatalf("expected nil after drain, got %v", got)
	}
}

func TestBuildNotificationMessageCompleted(t *testing.T) {
	msg := buildNotificationMessage(SubAgentCompletionNotification{
		TaskID:       "task-1",
		Agent:        "explorer",
		InvocationID: "inv-1",
		Status:       "completed",
		Summary:      "scan done",
	})
	if !strings.Contains(msg, "<task_id>task-1</task_id>") {
		t.Fatalf("expected task_id in message, got %q", msg)
	}
	if !strings.Contains(msg, "<agent>explorer</agent>") {
		t.Fatalf("expected agent in message, got %q", msg)
	}
	if !strings.Contains(msg, "<invocation_id>inv-1</invocation_id>") {
		t.Fatalf("expected invocation_id in message, got %q", msg)
	}
	if !strings.Contains(msg, "<status>completed</status>") {
		t.Fatalf("expected status in message, got %q", msg)
	}
	if !strings.Contains(msg, "<summary>scan done</summary>") {
		t.Fatalf("expected summary in message, got %q", msg)
	}
	if !strings.HasPrefix(msg, "<subagent-notification>") {
		t.Fatalf("expected XML wrapper, got %q", msg)
	}
	if !strings.HasSuffix(strings.TrimSpace(msg), "</subagent-notification>") {
		t.Fatalf("expected closing XML wrapper, got %q", msg)
	}
}

func TestBuildNotificationMessageFailed(t *testing.T) {
	msg := buildNotificationMessage(SubAgentCompletionNotification{
		TaskID:       "task-2",
		Agent:        "writer",
		InvocationID: "inv-2",
		Status:       "failed",
		ErrorCode:    "subagent_execution_failed",
		ErrorMessage: "timeout exceeded",
	})
	if !strings.Contains(msg, "<error_code>subagent_execution_failed</error_code>") {
		t.Fatalf("expected error_code in message, got %q", msg)
	}
	if !strings.Contains(msg, "<error_message>timeout exceeded</error_message>") {
		t.Fatalf("expected error_message in message, got %q", msg)
	}
}

func TestBuildNotificationMessageEmptyFields(t *testing.T) {
	msg := buildNotificationMessage(SubAgentCompletionNotification{})
	// Should still have wrapper but no inner fields
	if !strings.Contains(msg, "<subagent-notification>") {
		t.Fatalf("expected wrapper, got %q", msg)
	}
	if strings.Contains(msg, "<task_id>") {
		t.Fatalf("expected no task_id for empty notification, got %q", msg)
	}
}

func TestDrainSubAgentNotificationsNilRunner(t *testing.T) {
	sess := session.New("/ws")
	// Should not panic
	drainSubAgentNotifications(nil, sess)
}

func TestDrainSubAgentNotificationsNilNotifier(t *testing.T) {
	runner := &Runner{}
	sess := session.New("/ws")
	// Should not panic
	drainSubAgentNotifications(runner, sess)
}

func TestDrainSubAgentNotificationsSessionMismatch(t *testing.T) {
	notifier := &defaultSubAgentNotifier{}
	runner := &Runner{subAgentNotifier: notifier}
	sess := session.New("/ws")
	otherSess := session.New("/ws")

	notifier.NotifyCompletion(SubAgentCompletionNotification{
		ParentSession: otherSess,
		TaskID:        "t1",
		Agent:         "explorer",
		Status:        "completed",
		Summary:       "done",
	})

	before := len(sess.Messages)
	drainSubAgentNotifications(runner, sess)
	if len(sess.Messages) != before {
		t.Fatalf("expected no messages added for mismatched session, got %d messages", len(sess.Messages))
	}
}

func TestDrainSubAgentNotificationsMatchingSession(t *testing.T) {
	notifier := &defaultSubAgentNotifier{}
	runner := &Runner{subAgentNotifier: notifier}
	sess := session.New("/ws")

	notifier.NotifyCompletion(SubAgentCompletionNotification{
		ParentSession: sess,
		TaskID:        "t1",
		Agent:         "explorer",
		InvocationID:  "inv-1",
		Status:        "completed",
		Summary:       "scan done",
	})

	drainSubAgentNotifications(runner, sess)
	if len(sess.Messages) != 1 {
		t.Fatalf("expected 1 message added, got %d", len(sess.Messages))
	}
	msg := sess.Messages[0]
	if msg.Role != llm.RoleUser {
		t.Fatalf("expected user role for notification message, got %q", msg.Role)
	}
	if !strings.Contains(msg.Content, "scan done") {
		t.Fatalf("expected summary in notification message, got %q", msg.Content)
	}
}
