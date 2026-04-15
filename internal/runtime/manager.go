package runtime

import (
	"context"
	"errors"
	"sync"
	"time"

	corepkg "bytemind/internal/core"
)

var ErrTaskNotImplemented = errors.New("runtime task manager is not wired")

type TaskSpec struct {
	SessionID        corepkg.SessionID
	TraceID          corepkg.TraceID
	Name             string
	Kind             string
	Input            []byte
	ParentTaskID     corepkg.TaskID
	Timeout          time.Duration
	MaxRetries       int
	Background       bool
	IsolatedWorktree bool
	Metadata         map[string]string
}

type Task struct {
	ID         corepkg.TaskID
	Spec       TaskSpec
	Status     corepkg.TaskStatus
	Attempt    int
	CreatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
	ErrorCode  string
}

type TaskResult struct {
	TaskID     corepkg.TaskID
	Status     corepkg.TaskStatus
	Output     []byte
	ErrorCode  string
	FinishedAt time.Time
}

type TaskEventType string

const (
	TaskEventStatus TaskEventType = "status"
	TaskEventLog    TaskEventType = "log"
	TaskEventResult TaskEventType = "result"
	TaskEventError  TaskEventType = "error"
)

type TaskEvent struct {
	Type      TaskEventType
	TaskID    corepkg.TaskID
	Status    corepkg.TaskStatus
	Payload   []byte
	ErrorCode string
	Timestamp time.Time
}

type TaskManager interface {
	Submit(ctx context.Context, spec TaskSpec) (corepkg.TaskID, error)
	Get(ctx context.Context, id corepkg.TaskID) (Task, error)
	Cancel(ctx context.Context, id corepkg.TaskID, reason string) error
	Retry(ctx context.Context, id corepkg.TaskID) (corepkg.TaskID, error)
	Stream(ctx context.Context, id corepkg.TaskID) (<-chan TaskEvent, error)
}

// InMemoryTaskManager is a safe placeholder until runtime orchestration is wired.
type InMemoryTaskManager struct {
	mu    sync.Mutex
	tasks map[corepkg.TaskID]Task
}

func NewInMemoryTaskManager() *InMemoryTaskManager {
	return &InMemoryTaskManager{tasks: make(map[corepkg.TaskID]Task)}
}

func (m *InMemoryTaskManager) Submit(_ context.Context, spec TaskSpec) (corepkg.TaskID, error) {
	if m == nil {
		return "", ErrTaskNotImplemented
	}
	id := corepkg.TaskID(time.Now().UTC().Format("20060102150405.000000000"))
	now := time.Now().UTC()
	m.mu.Lock()
	m.tasks[id] = Task{ID: id, Spec: spec, Status: corepkg.TaskPending, CreatedAt: now}
	m.mu.Unlock()
	return id, nil
}

func (m *InMemoryTaskManager) Get(_ context.Context, id corepkg.TaskID) (Task, error) {
	if m == nil {
		return Task{}, ErrTaskNotImplemented
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok {
		return Task{}, ErrTaskNotImplemented
	}
	return task, nil
}

func (m *InMemoryTaskManager) Cancel(_ context.Context, id corepkg.TaskID, _ string) error {
	if m == nil {
		return ErrTaskNotImplemented
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok {
		return ErrTaskNotImplemented
	}
	task.Status = corepkg.TaskKilled
	finished := time.Now().UTC()
	task.FinishedAt = &finished
	m.tasks[id] = task
	return nil
}

func (m *InMemoryTaskManager) Retry(_ context.Context, _ corepkg.TaskID) (corepkg.TaskID, error) {
	return "", ErrTaskNotImplemented
}

func (m *InMemoryTaskManager) Stream(_ context.Context, _ corepkg.TaskID) (<-chan TaskEvent, error) {
	return nil, ErrTaskNotImplemented
}
