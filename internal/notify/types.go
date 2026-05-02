package notify

import "context"

type EventType string

const (
	EventApprovalRequired EventType = "approval_required"
	EventRunCompleted     EventType = "run_completed"
	EventRunFailed        EventType = "run_failed"
	EventRunCanceled      EventType = "run_canceled"
)

type Message struct {
	Event EventType
	Title string
	Body  string
	Key   string
}

type Notifier interface {
	Notify(msg Message)
	Close(ctx context.Context) error
}

type DesktopConfig struct {
	Enabled         bool
	CooldownSeconds int
	QueueSize       int
	SendTimeoutMs   int
}
