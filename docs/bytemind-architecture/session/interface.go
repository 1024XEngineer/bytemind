package session

import (
	"context"
	"time"

	"bytemind/internal/core"
)

type SessionMode string

const (
	ModeDefault           SessionMode = "default"
	ModeAcceptEdits       SessionMode = "acceptEdits"
	ModeBypassPermissions SessionMode = "bypassPermissions"
	ModePlan              SessionMode = "plan"
)

type SessionStatus string

const (
	StatusActive  SessionStatus = "active"
	StatusClosing SessionStatus = "closing"
	StatusClosed  SessionStatus = "closed"
)

type ErrorCode string

const (
	ErrCodeSessionNotFound ErrorCode = "session_not_found"
	ErrCodeSessionClosed   ErrorCode = "session_closed"
	ErrCodeInvalidMode     ErrorCode = "invalid_mode"
	ErrCodeReplayFailed    ErrorCode = "replay_failed"
)

type Message struct {
	ID        string
	Role      core.Role
	Content   string
	CreatedAt time.Time
}

type TurnRecord struct {
	TurnID    string
	Input     Message
	Outputs   []Message
	StartedAt time.Time
	EndedAt   time.Time
}

type UsageStat struct {
	InputTokens   int64
	OutputTokens  int64
	TotalRequests int64
}

type SessionSnapshot struct {
	ID           core.SessionID
	Mode         SessionMode
	Status       SessionStatus
	Messages     []Message
	Usage        UsageStat
	ActiveTasks  []core.TaskID
	CreatedAt    time.Time
	LastActiveAt time.Time
}

type SessionEventType string

const (
	SessionEventCreated SessionEventType = "created"
	SessionEventMode    SessionEventType = "mode_changed"
	SessionEventTurn    SessionEventType = "turn_appended"
	SessionEventClosed  SessionEventType = "closed"
	SessionEventError   SessionEventType = "error"
)

type SessionEvent struct {
	Type      SessionEventType
	SessionID core.SessionID
	EventID   string
	Offset    int64
	Payload   []byte
	ErrorCode string
	Timestamp time.Time
}

type CreateRequest struct {
	Mode     SessionMode
	Metadata map[string]string
}

type Manager interface {
	Create(ctx context.Context, req CreateRequest) (SessionSnapshot, error)
	Get(ctx context.Context, id core.SessionID) (SessionSnapshot, error)
	SwitchMode(ctx context.Context, id core.SessionID, mode SessionMode) error
	AppendTurn(ctx context.Context, id core.SessionID, turn TurnRecord) error
	AttachTask(ctx context.Context, id core.SessionID, taskID core.TaskID) error
	DetachTask(ctx context.Context, id core.SessionID, taskID core.TaskID) error
	Close(ctx context.Context, id core.SessionID, reason string) error
}

type Reader interface {
	Snapshot(ctx context.Context, id core.SessionID) (SessionSnapshot, error)
	ReadEvents(ctx context.Context, id core.SessionID, fromOffset int64, limit int) ([]SessionEvent, int64, error)
	Replay(ctx context.Context, id core.SessionID, fromOffset int64) (<-chan SessionEvent, error)
}

