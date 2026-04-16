package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
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
	Offset    uint64
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
	TaskID    corepkg.TaskID
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

type TaskEventStore interface {
	AppendTaskEvent(ctx context.Context, event TaskEvent) error
	AppendTaskLog(ctx context.Context, taskID corepkg.TaskID, entry TaskLogEntry) error
}

type NopTaskEventStore struct{}

func (NopTaskEventStore) AppendTaskEvent(context.Context, TaskEvent) error {
	return nil
}

func (NopTaskEventStore) AppendTaskLog(context.Context, corepkg.TaskID, TaskLogEntry) error {
	return nil
}

type InMemoryTaskManagerOption func(*InMemoryTaskManager)

func WithTaskEventStore(store TaskEventStore) InMemoryTaskManagerOption {
	return func(m *InMemoryTaskManager) {
		m.eventStore = store
	}
}

func WithTerminalHistoryCap(limit int) InMemoryTaskManagerOption {
	return func(m *InMemoryTaskManager) {
		m.terminalHistoryCap = limit
	}
}

func WithTaskExecutor(executor TaskExecutorFunc) InMemoryTaskManagerOption {
	return func(m *InMemoryTaskManager) {
		m.executor = executor
	}
}

const (
	defaultReadIncrementLimit = 200
	streamBufferSize          = 32
	defaultTerminalHistoryCap = 256
)

// InMemoryTaskManager is a safe placeholder until runtime orchestration is wired.
type InMemoryTaskManager struct {
	mu                 sync.RWMutex
	tasks              map[corepkg.TaskID]Task
	waiters            map[corepkg.TaskID][]chan TaskResult
	runCancels         map[corepkg.TaskID]context.CancelFunc
	parentChildren     map[corepkg.TaskID]map[corepkg.TaskID]struct{}
	childParent        map[corepkg.TaskID]corepkg.TaskID
	idSeq              atomic.Uint64
	events             map[corepkg.TaskID][]TaskEvent
	logs               map[corepkg.TaskID][]TaskLogEntry
	streamSubscribers  map[corepkg.TaskID]map[uint64]chan TaskEvent
	nextSubscriberID   uint64
	terminalHistory    []corepkg.TaskID
	terminalTasks      map[corepkg.TaskID]struct{}
	terminalHistoryCap int
	executor           TaskExecutorFunc
	eventStore         TaskEventStore
}

func NewInMemoryTaskManager(opts ...InMemoryTaskManagerOption) *InMemoryTaskManager {
	manager := &InMemoryTaskManager{
		tasks:              make(map[corepkg.TaskID]Task),
		waiters:            make(map[corepkg.TaskID][]chan TaskResult),
		runCancels:         make(map[corepkg.TaskID]context.CancelFunc),
		parentChildren:     make(map[corepkg.TaskID]map[corepkg.TaskID]struct{}),
		childParent:        make(map[corepkg.TaskID]corepkg.TaskID),
		events:             make(map[corepkg.TaskID][]TaskEvent),
		logs:               make(map[corepkg.TaskID][]TaskLogEntry),
		streamSubscribers:  make(map[corepkg.TaskID]map[uint64]chan TaskEvent),
		terminalTasks:      make(map[corepkg.TaskID]struct{}),
		terminalHistoryCap: defaultTerminalHistoryCap,
		eventStore:         NopTaskEventStore{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(manager)
		}
	}
	if manager.eventStore == nil {
		manager.eventStore = NopTaskEventStore{}
	}
	return manager
}

func (m *InMemoryTaskManager) Submit(ctx context.Context, spec TaskSpec) (corepkg.TaskID, error) {
	if m == nil {
		return "", ErrTaskNotImplemented
	}
	id := newTaskID(time.Now().UTC(), m.idSeq.Add(1))
	now := time.Now().UTC()
	task := Task{
		ID:        id,
		Spec:      cloneTaskSpec(spec),
		Status:    corepkg.TaskPending,
		Attempt:   0,
		CreatedAt: now,
	}

	var (
		event       TaskEvent
		logEntry    TaskLogEntry
		subscribers []chan TaskEvent
	)
	m.mu.Lock()
	m.tasks[id] = task
	m.registerParentChildLocked(id, task.Spec.ParentTaskID)
	event, subscribers = m.appendTaskEventLocked(task, TaskEventStatus, nil, task.ErrorCode, now)
	logEntry = m.appendTaskLogLocked(task.ID, []byte(fmt.Sprintf("status=%s", task.Status)), now)
	m.mu.Unlock()
	m.publishTaskEvent(event, subscribers)
	m.persistTaskEvent(event)
	m.persistTaskLog(task.ID, logEntry)
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
		result      TaskResult
		waiters     []chan TaskResult
		event       TaskEvent
		logEntry    TaskLogEntry
		subscribers []chan TaskEvent
		hasTask     bool
		childIDs    []corepkg.TaskID
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
	event, subscribers = m.appendTaskEventLocked(task, TaskEventStatus, nil, task.ErrorCode, now)
	logEntry = m.appendTaskLogLocked(task.ID, []byte(fmt.Sprintf("status=%s", task.Status)), now)
	result = taskToResult(task)
	waiters = m.waiters[id]
	delete(m.waiters, id)
	hasTask = true
	m.mu.Unlock()

	m.publishTaskEvent(event, subscribers)
	m.persistTaskEvent(event)
	m.persistTaskLog(task.ID, logEntry)
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

	var (
		event       TaskEvent
		logEntry    TaskLogEntry
		subscribers []chan TaskEvent
	)

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
	event, subscribers = m.appendTaskEventLocked(task, TaskEventStatus, nil, task.ErrorCode, time.Now().UTC())
	logEntry = m.appendTaskLogLocked(task.ID, []byte(fmt.Sprintf("status=%s", task.Status)), time.Now().UTC())
	m.mu.Unlock()

	m.publishTaskEvent(event, subscribers)
	m.persistTaskEvent(event)
	m.persistTaskLog(task.ID, logEntry)
	m.enqueueTask(detachContext(ctx), id)

	return id, nil
}

func (m *InMemoryTaskManager) Stream(ctx context.Context, id corepkg.TaskID) (<-chan TaskEvent, error) {
	if m == nil {
		return nil, ErrTaskNotImplemented
	}
	if ctx == nil {
		ctx = context.Background()
	}

	out := make(chan TaskEvent, streamBufferSize)
	live := make(chan TaskEvent, streamBufferSize)

	var (
		history      []TaskEvent
		subscriberID uint64
		useLive      bool
	)

	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok {
		m.mu.Unlock()
		return nil, taskNotFoundError(id)
	}
	history = cloneTaskEvents(m.events[id])
	if !IsTerminalTaskStatus(task.Status) {
		m.nextSubscriberID++
		subscriberID = m.nextSubscriberID
		if m.streamSubscribers[id] == nil {
			m.streamSubscribers[id] = make(map[uint64]chan TaskEvent)
		}
		m.streamSubscribers[id][subscriberID] = live
		useLive = true
	}
	m.mu.Unlock()

	go func() {
		defer close(out)
		defer func() {
			if useLive {
				m.removeStreamSubscriber(id, subscriberID)
			}
		}()

		for _, event := range history {
			select {
			case out <- event:
			case <-ctx.Done():
				return
			}
		}

		if !useLive {
			return
		}

		for {
			select {
			case event := <-live:
				select {
				case out <- event:
				case <-ctx.Done():
					return
				}
				if IsTerminalTaskStatus(event.Status) {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return out, nil
}

func (m *InMemoryTaskManager) ReadIncrement(_ context.Context, id corepkg.TaskID, offset uint64, limit int) (items []TaskLogEntry, nextOffset uint64, hasMore bool, err error) {
	if m == nil {
		return nil, 0, false, ErrTaskNotImplemented
	}
	if limit <= 0 {
		limit = defaultReadIncrementLimit
	}

	m.mu.RLock()
	_, ok := m.tasks[id]
	if !ok {
		m.mu.RUnlock()
		return nil, 0, false, taskNotFoundError(id)
	}
	entries := m.logs[id]
	total := uint64(len(entries))
	start := offset
	if start > total {
		start = total
	}
	end := start + uint64(limit)
	if end > total {
		end = total
	}
	copied := make([]TaskLogEntry, 0, end-start)
	for _, entry := range entries[start:end] {
		copied = append(copied, cloneTaskLogEntry(entry))
	}
	m.mu.RUnlock()

	nextOffset = end
	hasMore = end < total
	return copied, nextOffset, hasMore, nil
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

func (m *InMemoryTaskManager) appendTaskEventLocked(task Task, eventType TaskEventType, payload []byte, errorCode string, at time.Time) (TaskEvent, []chan TaskEvent) {
	event := TaskEvent{
		Type:      eventType,
		TaskID:    task.ID,
		SessionID: task.Spec.SessionID,
		TraceID:   task.Spec.TraceID,
		Status:    task.Status,
		Attempt:   task.Attempt,
		Payload:   append([]byte(nil), payload...),
		Metadata:  cloneTaskMetadata(task.Spec.Metadata),
		ErrorCode: errorCode,
		Timestamp: at.UTC(),
	}
	current := m.events[task.ID]
	event.Offset = uint64(len(current))
	m.events[task.ID] = append(current, event)
	return cloneTaskEvent(event), m.snapshotStreamSubscribersLocked(task.ID)
}

func (m *InMemoryTaskManager) appendTaskLogLocked(taskID corepkg.TaskID, payload []byte, at time.Time) TaskLogEntry {
	entry := TaskLogEntry{
		TaskID:    taskID,
		Payload:   append([]byte(nil), payload...),
		Timestamp: at.UTC(),
	}
	current := m.logs[taskID]
	entry.Offset = uint64(len(current))
	m.logs[taskID] = append(current, entry)
	return cloneTaskLogEntry(entry)
}

func (m *InMemoryTaskManager) snapshotStreamSubscribersLocked(taskID corepkg.TaskID) []chan TaskEvent {
	subs := m.streamSubscribers[taskID]
	if len(subs) == 0 {
		return nil
	}
	copied := make([]chan TaskEvent, 0, len(subs))
	for _, ch := range subs {
		copied = append(copied, ch)
	}
	return copied
}

func (m *InMemoryTaskManager) publishTaskEvent(event TaskEvent, subscribers []chan TaskEvent) {
	for _, subscriber := range subscribers {
		subscriber <- cloneTaskEvent(event)
	}
}

func (m *InMemoryTaskManager) persistTaskEvent(event TaskEvent) {
	if m == nil || m.eventStore == nil {
		return
	}
	_ = m.eventStore.AppendTaskEvent(context.Background(), cloneTaskEvent(event))
}

func (m *InMemoryTaskManager) persistTaskLog(taskID corepkg.TaskID, entry TaskLogEntry) {
	if m == nil || m.eventStore == nil {
		return
	}
	_ = m.eventStore.AppendTaskLog(context.Background(), taskID, cloneTaskLogEntry(entry))
}

func (m *InMemoryTaskManager) enqueueTask(parentCtx context.Context, id corepkg.TaskID) {
	if m == nil || m.executor == nil {
		return
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	go m.runTaskAttempt(parentCtx, id)
}

func (m *InMemoryTaskManager) runTaskAttempt(parentCtx context.Context, id corepkg.TaskID) {
	if m == nil || m.executor == nil {
		return
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}

	now := time.Now().UTC()
	var (
		startEvent       TaskEvent
		startLog         TaskLogEntry
		startSubscribers []chan TaskEvent
	)

	m.mu.Lock()
	task, ok := m.tasks[id]
	if !ok {
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
	task.Output = nil
	m.tasks[id] = task
	startEvent, startSubscribers = m.appendTaskEventLocked(task, TaskEventStatus, nil, task.ErrorCode, now)
	startLog = m.appendTaskLogLocked(task.ID, []byte(fmt.Sprintf("status=%s", task.Status)), now)

	runCtx, runCancel := context.WithCancel(parentCtx)
	m.runCancels[id] = runCancel
	m.mu.Unlock()

	m.publishTaskEvent(startEvent, startSubscribers)
	m.persistTaskEvent(startEvent)
	m.persistTaskLog(task.ID, startLog)

	execCtx := runCtx
	timeoutCancel := func() {}
	if task.Spec.Timeout > 0 {
		execCtx, timeoutCancel = context.WithTimeout(runCtx, task.Spec.Timeout)
	}
	output, execErr := m.executor(execCtx, task)
	timeoutCancel()
	runCancel()

	status, errorCodeValue := mapExecutionResult(execErr)
	finished := time.Now().UTC()

	var (
		result      TaskResult
		waiters     []chan TaskResult
		event       TaskEvent
		logEntry    TaskLogEntry
		subscribers []chan TaskEvent
		hasTask     bool
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
	event, subscribers = m.appendTaskEventLocked(current, TaskEventStatus, nil, current.ErrorCode, finished)
	logEntry = m.appendTaskLogLocked(current.ID, []byte(fmt.Sprintf("status=%s", current.Status)), finished)
	result = taskToResult(current)
	waiters = m.waiters[id]
	delete(m.waiters, id)
	hasTask = true
	m.mu.Unlock()

	m.publishTaskEvent(event, subscribers)
	m.persistTaskEvent(event)
	m.persistTaskLog(current.ID, logEntry)

	if hasTask {
		for _, waiter := range waiters {
			select {
			case waiter <- result:
			default:
			}
			close(waiter)
		}
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

func (m *InMemoryTaskManager) removeStreamSubscriber(taskID corepkg.TaskID, subscriberID uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	subs := m.streamSubscribers[taskID]
	if len(subs) == 0 {
		return
	}
	delete(subs, subscriberID)
	if len(subs) == 0 {
		delete(m.streamSubscribers, taskID)
		return
	}
	m.streamSubscribers[taskID] = subs
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

func newTaskID(ts time.Time, seq uint64) corepkg.TaskID {
	return corepkg.TaskID(fmt.Sprintf("%s-%06d", ts.Format("20060102150405.000000000"), seq))
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

func detachContext(ctx context.Context) context.Context {
	return context.Background()
}

func cloneTaskEvents(events []TaskEvent) []TaskEvent {
	if len(events) == 0 {
		return nil
	}
	copied := make([]TaskEvent, 0, len(events))
	for _, event := range events {
		copied = append(copied, cloneTaskEvent(event))
	}
	return copied
}

func cloneTaskEvent(event TaskEvent) TaskEvent {
	cloned := event
	if len(event.Payload) > 0 {
		cloned.Payload = append([]byte(nil), event.Payload...)
	}
	cloned.Metadata = cloneTaskMetadata(event.Metadata)
	return cloned
}

func cloneTaskMetadata(metadata map[string]string) map[string]string {
	if len(metadata) == 0 {
		return nil
	}
	copied := make(map[string]string, len(metadata))
	for k, v := range metadata {
		copied[k] = v
	}
	return copied
}

func cloneTaskLogEntry(entry TaskLogEntry) TaskLogEntry {
	cloned := entry
	if len(entry.Payload) > 0 {
		cloned.Payload = append([]byte(nil), entry.Payload...)
	}
	return cloned
}
