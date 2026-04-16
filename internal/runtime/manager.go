package runtime

import (
	"context"
	"fmt"
	"sync"
	"time"

	corepkg "bytemind/internal/core"
)

var ErrTaskNotImplemented = newRuntimeError(ErrorCodeNotImplemented, "runtime task manager is not wired", false, nil)

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
	SessionID corepkg.SessionID
	TraceID   corepkg.TraceID
	Status    corepkg.TaskStatus
	Attempt   int
	Payload   []byte
	Metadata  map[string]string
	ErrorCode string
	Timestamp time.Time
}

type TaskManager interface {
	Submit(ctx context.Context, spec TaskSpec) (corepkg.TaskID, error)
	Get(ctx context.Context, id corepkg.TaskID) (Task, error)
	Cancel(ctx context.Context, id corepkg.TaskID, reason string) error
	Retry(ctx context.Context, id corepkg.TaskID) (corepkg.TaskID, error)
	Stream(ctx context.Context, id corepkg.TaskID) (<-chan TaskEvent, error)
	Wait(ctx context.Context, id corepkg.TaskID) (TaskResult, error)
}

type Scheduler interface {
	Enqueue(ctx context.Context, taskID corepkg.TaskID) error
}

type TaskLogEntry struct {
	Offset    uint64
	Payload   []byte
	Timestamp time.Time
}

type LogReader interface {
	ReadIncrement(ctx context.Context, taskID corepkg.TaskID, offset uint64, limit int) (items []TaskLogEntry, nextOffset uint64, hasMore bool, err error)
}

type SubAgentCoordinator interface {
	Spawn(ctx context.Context, spec TaskSpec) (corepkg.TaskID, error)
	Wait(ctx context.Context, id corepkg.TaskID) (TaskResult, error)
}

type QuotaManager interface {
	Acquire(ctx context.Context, key string) error
	Release(key string)
}

// InMemoryTaskManager is a safe placeholder until runtime orchestration is wired.
type InMemoryTaskManager struct {
	mu      sync.RWMutex
	tasks   map[corepkg.TaskID]Task
	waiters map[corepkg.TaskID][]chan TaskResult
}

func NewInMemoryTaskManager() *InMemoryTaskManager {
	return &InMemoryTaskManager{
		tasks:   make(map[corepkg.TaskID]Task),
		waiters: make(map[corepkg.TaskID][]chan TaskResult),
	}
}

func (m *InMemoryTaskManager) Submit(_ context.Context, spec TaskSpec) (corepkg.TaskID, error) {
	if m == nil {
		return "", ErrTaskNotImplemented
	}
	id := newTaskID(time.Now().UTC())
	now := time.Now().UTC()
	task := Task{
		ID:        id,
		Spec:      spec,
		Status:    corepkg.TaskPending,
		Attempt:   0,
		CreatedAt: now,
	}
	m.mu.Lock()
	m.tasks[id] = task
	m.mu.Unlock()
	return id, nil
}

func (m *InMemoryTaskManager) Get(_ context.Context, id corepkg.TaskID) (Task, error) {
	if m == nil {
		return Task{}, ErrTaskNotImplemented
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	task, ok := m.tasks[id]
	if !ok {
		return Task{}, taskNotFoundError(id)
	}
	return task, nil
}

func (m *InMemoryTaskManager) Cancel(_ context.Context, id corepkg.TaskID, _ string) error {
	if m == nil {
		return ErrTaskNotImplemented
	}

	now := time.Now().UTC()
	var (
		result  TaskResult
		waiters []chan TaskResult
		hasTask bool
	)

	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return taskNotFoundError(id)
	}
	if task.Status == corepkg.TaskKilled {
		m.mu.Unlock()
		return nil
	}
	if err := ValidateTaskTransition(task.Status, corepkg.TaskKilled, TransitionOptions{}); err != nil {
		m.mu.Unlock()
		return err
	}

	task.Status = corepkg.TaskKilled
	task.ErrorCode = ErrorCodeTaskCancelled
	task.FinishedAt = &now
	m.tasks[id] = task
	result = taskToResult(task)
	waiters = m.waiters[id]
	delete(m.waiters, id)
	hasTask = true
	m.mu.Unlock()

	if hasTask {
		for _, waiter := range waiters {
			select {
			case waiter <- result:
			default:
			}
			close(waiter)
		}
	}

	return nil
}

func (m *InMemoryTaskManager) Retry(_ context.Context, id corepkg.TaskID) (corepkg.TaskID, error) {
	if m == nil {
		return "", ErrTaskNotImplemented
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	task, ok := m.tasks[id]
	if !ok {
		return "", taskNotFoundError(id)
	}
	if task.Status != corepkg.TaskFailed {
		return "", invalidTransitionError(task.Status, corepkg.TaskPending)
	}
	if task.Attempt >= task.Spec.MaxRetries {
		return "", newRuntimeError(ErrorCodeRetryExhausted, fmt.Sprintf("task %q exhausted retries", id), false, nil)
	}
	if err := ValidateTaskTransition(task.Status, corepkg.TaskPending, TransitionOptions{AllowRetryTransition: true}); err != nil {
		return "", err
	}

	task.Attempt++
	task.Status = corepkg.TaskPending
	task.ErrorCode = ""
	task.StartedAt = nil
	task.FinishedAt = nil
	m.tasks[id] = task

	return id, nil
}

func (m *InMemoryTaskManager) Stream(_ context.Context, _ corepkg.TaskID) (<-chan TaskEvent, error) {
	return nil, ErrTaskNotImplemented
}

func (m *InMemoryTaskManager) Wait(ctx context.Context, id corepkg.TaskID) (TaskResult, error) {
	if m == nil {
		return TaskResult{}, ErrTaskNotImplemented
	}
	if ctx == nil {
		ctx = context.Background()
	}

	waiter := make(chan TaskResult, 1)

	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return TaskResult{}, taskNotFoundError(id)
	}
	if IsTerminalTaskStatus(task.Status) {
		result := taskToResult(task)
		m.mu.Unlock()
		return result, nil
	}
	m.waiters[id] = append(m.waiters[id], waiter)
	m.mu.Unlock()

	select {
	case result := <-waiter:
		return result, nil
	case <-ctx.Done():
		m.removeWaiter(id, waiter)
		return TaskResult{}, ctx.Err()
	}
}

func (m *InMemoryTaskManager) removeWaiter(id corepkg.TaskID, waiter chan TaskResult) {
	m.mu.Lock()
	defer m.mu.Unlock()
	waiters := m.waiters[id]
	if len(waiters) == 0 {
		return
	}
	filtered := waiters[:0]
	for _, current := range waiters {
		if current == waiter {
			continue
		}
		filtered = append(filtered, current)
	}
	if len(filtered) == 0 {
		delete(m.waiters, id)
		return
	}
	m.waiters[id] = filtered
}

func newTaskID(ts time.Time) corepkg.TaskID {
	return corepkg.TaskID(ts.Format("20060102150405.000000000"))
}

func taskToResult(task Task) TaskResult {
	finishedAt := task.CreatedAt
	if task.FinishedAt != nil {
		finishedAt = *task.FinishedAt
	}
	return TaskResult{
		TaskID:     task.ID,
		Status:     task.Status,
		ErrorCode:  task.ErrorCode,
		FinishedAt: finishedAt,
	}
}
