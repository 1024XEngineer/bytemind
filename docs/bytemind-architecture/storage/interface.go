package storage

import "context"

// SessionStore persists and replays session events.
type SessionStore interface {
	AppendEvent(ctx context.Context, sessionID string, evt Event) error
	Replay(ctx context.Context, sessionID string, fromOffset int64) ([]Event, error)
}

// TaskStore persists task logs.
type TaskStore interface {
	AppendLog(ctx context.Context, taskID string, chunk []byte) error
}

// Event is a storage event record.
type Event struct {
	ID   string
	Type string
	Data []byte
}
