package runtime

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	Output     []byte
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

type TaskExecutorFunc func(ctx context.Context, task Task) ([]byte, error)

const TaskExecutionTokenMetadataKey = "runtime_execution_token"

type TaskExecutionRegistry interface {
	RegisterExecution(token string, executor TaskExecutorFunc)
	UnregisterExecution(token string)
}

type InMemoryTaskManagerOption func(*InMemoryTaskManager)

func WithTaskExecutor(executor TaskExecutorFunc) InMemoryTaskManagerOption {
	return func(m *InMemoryTaskManager) {
		m.executor = executor
	}
}

func (m *InMemoryTaskManager) RegisterExecution(token string, executor TaskExecutorFunc) {
	if m == nil {
		return
	}
	token = normalizeExecutionToken(token)
	if token == "" || executor == nil {
		return
	}
	m.mu.Lock()
	m.executions[token] = executor
	m.mu.Unlock()
}

func (m *InMemoryTaskManager) UnregisterExecution(token string) {
	if m == nil {
		return
	}
	token = normalizeExecutionToken(token)
	if token == "" {
		return
	}
	m.mu.Lock()
	delete(m.executions, token)
	m.mu.Unlock()
}

// InMemoryTaskManager is a safe placeholder until runtime orchestration is wired.
type InMemoryTaskManager struct {
	mu             sync.RWMutex
	tasks          map[corepkg.TaskID]Task
	waiters        map[corepkg.TaskID][]chan TaskResult
	runCancels     map[corepkg.TaskID]context.CancelFunc
	parentChildren map[corepkg.TaskID]map[corepkg.TaskID]struct{}
	childParent    map[corepkg.TaskID]corepkg.TaskID
	executions     map[string]TaskExecutorFunc
	executor       TaskExecutorFunc
}

func NewInMemoryTaskManager(opts ...InMemoryTaskManagerOption) *InMemoryTaskManager {
	manager := &InMemoryTaskManager{
		tasks:          make(map[corepkg.TaskID]Task),
		waiters:        make(map[corepkg.TaskID][]chan TaskResult),
		runCancels:     make(map[corepkg.TaskID]context.CancelFunc),
		parentChildren: make(map[corepkg.TaskID]map[corepkg.TaskID]struct{}),
		childParent:    make(map[corepkg.TaskID]corepkg.TaskID),
		executions:     make(map[string]TaskExecutorFunc),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(manager)
		}
	}
	return manager
}

func (m *InMemoryTaskManager) Submit(ctx context.Context, spec TaskSpec) (corepkg.TaskID, error) {
	if m == nil {
		return "", ErrTaskNotImplemented
	}
	id := newTaskID(time.Now().UTC())
	now := time.Now().UTC()
	task := Task{
		ID:        id,
		Spec:      cloneTaskSpec(spec),
		Status:    corepkg.TaskPending,
		Attempt:   0,
		CreatedAt: now,
	}
	m.mu.Lock()
	m.tasks[id] = task
	m.registerParentChildLocked(id, task.Spec.ParentTaskID)
	m.mu.Unlock()

	m.enqueueTask(detachContext(ctx), id)

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

func (m *InMemoryTaskManager) Cancel(ctx context.Context, id corepkg.TaskID, _ string) error {
	if m == nil {
		return ErrTaskNotImplemented
	}
	if ctx == nil {
		ctx = context.Background()
	}

	now := time.Now().UTC()
	var (
		result   TaskResult
		waiters  []chan TaskResult
		hasTask  bool
		childIDs []corepkg.TaskID
	)

	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return taskNotFoundError(id)
	}
	childIDs = m.snapshotChildIDsLocked(id)
	if task.Status == corepkg.TaskRunning {
		cancel := m.runCancels[id]
		m.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if err := m.cancelChildTasks(ctx, childIDs, "parent_cancelled"); err != nil {
			return err
		}
		_, waitErr := m.Wait(ctx, id)
		if waitErr != nil {
			return waitErr
		}
		return nil
	}
	if task.Status == corepkg.TaskKilled {
		m.mu.Unlock()
		if err := m.cancelChildTasks(ctx, childIDs, "parent_cancelled"); err != nil {
			return err
		}
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
	delete(m.runCancels, id)
	m.detachParentChildLinkLocked(id)
	result = taskToResult(task)
	waiters = m.waiters[id]
	delete(m.waiters, id)
	hasTask = true
	m.mu.Unlock()

	if err := m.cancelChildTasks(ctx, childIDs, "parent_cancelled"); err != nil {
		return err
	}

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

func (m *InMemoryTaskManager) Retry(ctx context.Context, id corepkg.TaskID) (corepkg.TaskID, error) {
	if m == nil {
		return "", ErrTaskNotImplemented
	}
	if ctx == nil {
		ctx = context.Background()
	}

	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return "", taskNotFoundError(id)
	}
	if task.Status != corepkg.TaskFailed {
		m.mu.Unlock()
		return "", invalidTransitionError(task.Status, corepkg.TaskPending)
	}
	if task.Attempt >= task.Spec.MaxRetries {
		m.mu.Unlock()
		return "", newRuntimeError(ErrorCodeRetryExhausted, fmt.Sprintf("task %q exhausted retries", id), false, nil)
	}
	if err := ValidateTaskTransition(task.Status, corepkg.TaskPending, TransitionOptions{AllowRetryTransition: true}); err != nil {
		m.mu.Unlock()
		return "", err
	}

	task.Attempt++
	task.Status = corepkg.TaskPending
	task.ErrorCode = ""
	task.StartedAt = nil
	task.FinishedAt = nil
	m.tasks[id] = task
	m.mu.Unlock()

	m.enqueueTask(detachContext(ctx), id)

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

func (m *InMemoryTaskManager) enqueueTask(parentCtx context.Context, id corepkg.TaskID) {
	if m == nil {
		return
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	go m.runTaskAttempt(parentCtx, id)
}

func (m *InMemoryTaskManager) runTaskAttempt(parentCtx context.Context, id corepkg.TaskID) {
	if m == nil {
		return
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	now := time.Now().UTC()

	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return
	}
	executor := m.resolveExecutionLocked(task)
	if executor == nil {
		m.mu.Unlock()
		return
	}
	if task.Status != corepkg.TaskPending {
		m.mu.Unlock()
		return
	}
	if err := ValidateTaskTransition(task.Status, corepkg.TaskRunning, TransitionOptions{}); err != nil {
		m.mu.Unlock()
		return
	}
	task.Status = corepkg.TaskRunning
	task.ErrorCode = ""
	task.StartedAt = &now
	task.FinishedAt = nil
	m.tasks[id] = task

	runCtx, runCancel := context.WithCancel(parentCtx)
	m.runCancels[id] = runCancel
	m.mu.Unlock()

	execCtx := runCtx
	timeoutCancel := func() {}
	if task.Spec.Timeout > 0 {
		execCtx, timeoutCancel = context.WithTimeout(runCtx, task.Spec.Timeout)
	}
	output, execErr := executor(execCtx, task)
	timeoutCancel()
	runCancel()

	status, errorCodeValue := mapExecutionResult(execErr)
	finished := time.Now().UTC()

	var (
		result  TaskResult
		waiters []chan TaskResult
	)

	m.mu.Lock()
	current, ok := m.tasks[id]
	if !ok {
		delete(m.runCancels, id)
		m.mu.Unlock()
		return
	}
	if current.Status != corepkg.TaskRunning {
		delete(m.runCancels, id)
		m.mu.Unlock()
		return
	}
	if err := ValidateTaskTransition(current.Status, status, TransitionOptions{}); err != nil {
		delete(m.runCancels, id)
		m.mu.Unlock()
		return
	}

	current.Status = status
	current.ErrorCode = errorCodeValue
	current.Output = append([]byte(nil), output...)
	current.FinishedAt = &finished
	m.tasks[id] = current
	delete(m.runCancels, id)
	m.detachParentChildLinkLocked(id)

	result = taskToResult(current)
	waiters = m.waiters[id]
	delete(m.waiters, id)
	m.mu.Unlock()

	for _, waiter := range waiters {
		select {
		case waiter <- result:
		default:
		}
		close(waiter)
	}
}

func mapExecutionResult(execErr error) (corepkg.TaskStatus, string) {
	if execErr == nil {
		return corepkg.TaskCompleted, ""
	}
	if errors.Is(execErr, context.DeadlineExceeded) {
		return corepkg.TaskFailed, ErrorCodeTaskTimeout
	}
	if errors.Is(execErr, context.Canceled) {
		return corepkg.TaskKilled, ErrorCodeTaskCancelled
	}
	if code := errorCode(execErr); code != "" {
		return corepkg.TaskFailed, code
	}
	return corepkg.TaskFailed, ErrorCodeTaskExecutionFailed
}

func (m *InMemoryTaskManager) registerParentChildLocked(childID, parentID corepkg.TaskID) {
	if parentID == "" {
		return
	}
	if m.parentChildren[parentID] == nil {
		m.parentChildren[parentID] = make(map[corepkg.TaskID]struct{})
	}
	m.parentChildren[parentID][childID] = struct{}{}
	m.childParent[childID] = parentID
}

func (m *InMemoryTaskManager) detachParentChildLinkLocked(taskID corepkg.TaskID) {
	parentID, hasParent := m.childParent[taskID]
	if hasParent {
		delete(m.childParent, taskID)
		children := m.parentChildren[parentID]
		delete(children, taskID)
		if len(children) == 0 {
			delete(m.parentChildren, parentID)
		}
	}
	if children, ok := m.parentChildren[taskID]; ok {
		delete(m.parentChildren, taskID)
		for childID := range children {
			delete(m.childParent, childID)
		}
	}
}

func (m *InMemoryTaskManager) snapshotChildIDsLocked(parentID corepkg.TaskID) []corepkg.TaskID {
	children := m.parentChildren[parentID]
	if len(children) == 0 {
		return nil
	}
	ids := make([]corepkg.TaskID, 0, len(children))
	for childID := range children {
		ids = append(ids, childID)
	}
	return ids
}

func (m *InMemoryTaskManager) cancelChildTasks(ctx context.Context, childIDs []corepkg.TaskID, reason string) error {
	for _, childID := range childIDs {
		err := m.Cancel(ctx, childID, reason)
		if err == nil {
			continue
		}
		code := errorCode(err)
		if code == ErrorCodeTaskNotFound || code == ErrorCodeInvalidTransition {
			continue
		}
		return err
	}
	return nil
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
		Output:     append([]byte(nil), task.Output...),
		ErrorCode:  task.ErrorCode,
		FinishedAt: finishedAt,
	}
}

func cloneTaskSpec(spec TaskSpec) TaskSpec {
	cloned := spec
	if len(spec.Input) > 0 {
		cloned.Input = append([]byte(nil), spec.Input...)
	}
	if len(spec.Metadata) > 0 {
		cloned.Metadata = make(map[string]string, len(spec.Metadata))
		for k, v := range spec.Metadata {
			cloned.Metadata[k] = v
		}
	}
	return cloned
}

func (m *InMemoryTaskManager) resolveExecutionLocked(task Task) TaskExecutorFunc {
	if m == nil {
		return nil
	}
	if len(task.Spec.Metadata) > 0 {
		token := normalizeExecutionToken(task.Spec.Metadata[TaskExecutionTokenMetadataKey])
		if token != "" {
			if executor := m.executions[token]; executor != nil {
				return executor
			}
		}
	}
	return m.executor
}

func normalizeExecutionToken(token string) string {
	return strings.TrimSpace(token)
}

func detachContext(ctx context.Context) context.Context {
	// Keep execution lifecycle independent from request-scoped cancellation.
	// Task timeout and explicit Cancel(...) remain the runtime controls.
	return context.Background()
}
